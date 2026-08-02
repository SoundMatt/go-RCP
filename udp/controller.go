package udp

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/regmap"
)

// Controller is the client side of this package's AVTPDU/ACF-over-UDP/IP
// transport: it addresses one destination RC Server by UDP address and
// presents streamID as its own avtp.StreamID identity on every AVTPDU it
// sends. Every outbound request is framed as an untimed (NTSCF) header —
// see doc.go's "Explicit non-goals" for why this milestone's Controller
// never originates a timestamped (TSCF) request — and correlated to its
// response by acf.Message.TransactionNum.
type Controller struct {
	streamID avtp.StreamID
	conn     *net.UDPConn
	nextTxn  atomic.Uint32
	seq      atomic.Uint32
	encapSeq atomic.Uint32
	closed   atomic.Bool
	readDone chan struct{}

	mu      sync.Mutex
	pending map[avtp.TransactionNum]chan acf.Message
}

// NewController dials serverAddr and returns a Controller presenting
// streamID as its own identity. If serverAddr.Port is 0, it defaults to
// AnnexJControlPort (see defaultAnnexJPort) — a caller that wants a
// specific port states it explicitly in serverAddr, exactly as before.
func NewController(streamID avtp.StreamID, serverAddr *net.UDPAddr) (*Controller, error) {
	conn, err := net.DialUDP("udp", nil, defaultAnnexJPort(serverAddr))
	if err != nil {
		return nil, fmt.Errorf("rcp/udp: dial stream %s: %w", streamID, err)
	}
	c := &Controller{
		streamID: streamID,
		conn:     conn,
		readDone: make(chan struct{}),
		pending:  make(map[avtp.TransactionNum]chan acf.Message),
	}
	go c.readLoop()
	return c, nil
}

// StreamID returns this Controller's own avtp.StreamID identity.
func (c *Controller) StreamID() avtp.StreamID { return c.streamID }

// RawConn returns the underlying syscall.RawConn so callers can set socket
// options (e.g. SO_PRIORITY for TSN) via Control.
func (c *Controller) RawConn() (interface{ Control(func(uintptr)) error }, error) {
	return c.conn.SyscallConn()
}

// Request sends one plain (KindShort) request to addr with the given
// control flags and body, and blocks for the matching response (by
// acf.Message.TransactionNum) or ctx's expiry, whichever comes first.
//
// Request always constructs Kind: acf.KindShort — it does not yet have a
// way to send a request/-package conditional/cancel/chained/timed envelope
// (request.EncodeCompound, EncodeCancelAll, and friends), which the wire
// format requires routing as Kind: acf.KindLong with MTV false instead (see
// request/doc.go's "Wire layer" section). The acf.FlagExtended-based
// mechanism this comment used to describe was removed in acf v2.0 (see
// acf/doc.go) and was never replaced with a working Controller-level path;
// see the request package's own doc.go for the current, tracked scope of
// this gap.
func (c *Controller) Request(ctx context.Context, addr avtp.ByteBusID, control acf.ControlFlags, body []byte) (acf.Message, error) {
	if c.closed.Load() {
		return acf.Message{}, fmt.Errorf("rcp/udp: stream %s: %w", c.streamID, ErrClosed)
	}
	select {
	case <-ctx.Done():
		return acf.Message{}, fmt.Errorf("rcp/udp: stream %s: %w", c.streamID, ErrTimeout)
	default:
	}

	// TransactionNum is a 16-bit wire field; nextTxn wraps modulo 65536 the
	// same way the retired wire/udp package's uint32 Command.ID counter
	// wrapped modulo 2^32 — a collision requires more in-flight requests
	// than the field width allows, the same inherited assumption, just at a
	// narrower width forced by the wire format this milestone now targets.
	txn := avtp.TransactionNum(uint16(c.nextTxn.Add(1)))

	req := acf.Message{
		Kind:           acf.KindShort,
		ByteBusID:      addr,
		TransactionNum: txn,
		Control:        control,
		Body:           body,
	}
	frame, err := c.encode(req)
	if err != nil {
		return acf.Message{}, fmt.Errorf("rcp/udp: stream %s: encode: %w", c.streamID, err)
	}
	// Annex J UDP/IP framing: a 4-byte encapsulation sequence number ahead
	// of the AVTPDU itself — see annexj.go's provenance note. This field
	// does not exist on the l2 (raw Ethernet) transport.
	payload := prependEncapSeq(c.encapSeq.Add(1), frame)

	ch := make(chan acf.Message, 1)
	c.mu.Lock()
	c.pending[txn] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, txn)
		c.mu.Unlock()
	}()

	if _, err := c.conn.Write(payload); err != nil {
		return acf.Message{}, fmt.Errorf("rcp/udp: stream %s: write: %w", c.streamID, err)
	}

	select {
	case <-ctx.Done():
		return acf.Message{}, fmt.Errorf("rcp/udp: stream %s: %w", c.streamID, ErrTimeout)
	case resp, ok := <-ch:
		if !ok {
			return acf.Message{}, fmt.Errorf("rcp/udp: stream %s: %w", c.streamID, ErrClosed)
		}
		return resp, nil
	case <-c.readDone:
		return acf.Message{}, fmt.Errorf("rcp/udp: stream %s: %w", c.streamID, ErrClosed)
	}
}

// Read is Request with acf.FlagRead set and no body.
func (c *Controller) Read(ctx context.Context, addr avtp.ByteBusID) (acf.Message, error) {
	return c.Request(ctx, addr, acf.FlagRead, nil)
}

// Write is Request with acf.FlagWrite set and the given body.
func (c *Controller) Write(ctx context.Context, addr avtp.ByteBusID, body []byte) (acf.Message, error) {
	return c.Request(ctx, addr, acf.FlagWrite, body)
}

// Discover issues a Milestone 46 discovery read against regmap.EP0 and
// returns the responder's encoded register map (regmap.DecodeRegisterMap
// decodes it further). Unlike Read, this does not require an established
// grant — the responding server's own EP0Handler answers any untimed EP0
// read this way regardless of the caller's access state (see udp/ep0.go).
func (c *Controller) Discover(ctx context.Context) ([]byte, error) {
	resp, err := c.Read(ctx, regmap.EP0)
	if err != nil {
		return nil, err
	}
	if resp.Control.Has(acf.FlagError) {
		return nil, fmt.Errorf("rcp/udp: stream %s: discover: %s", c.streamID, resp.Body)
	}
	return resp.Body, nil
}

// Close shuts down the Controller's socket and unblocks every in-flight
// Request with ErrClosed.
func (c *Controller) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	err := c.conn.Close()
	<-c.readDone // waits for readLoop to exit, which closes readDone

	c.mu.Lock()
	for txn, ch := range c.pending {
		close(ch)
		delete(c.pending, txn)
	}
	c.mu.Unlock()

	return err
}

// encode wraps req in an untimed (NTSCF) avtp.Header presenting this
// Controller's own StreamID, incrementing the per-stream sequence number,
// and serializes the whole AVTPDU via acf.EncodeFrame.
func (c *Controller) encode(req acf.Message) ([]byte, error) {
	hdr := avtp.Header{
		Timed:         false,
		StreamIDValid: true,
		SequenceNum:   uint8(c.seq.Add(1)),
		StreamID:      c.streamID,
	}
	return acf.EncodeFrame(hdr, req)
}

func (c *Controller) readLoop() {
	defer close(c.readDone)
	buf := make([]byte, MaxFrameLen)
	for {
		n, err := c.conn.Read(buf)
		if err != nil {
			return
		}
		// Strip Annex J's leading encapsulation sequence number before
		// handing the remaining bytes to acf.DecodeFrame — see annexj.go.
		_, rest, err := stripEncapSeq(buf[:n])
		if err != nil {
			continue
		}
		frame, err := acf.DecodeFrame(rest)
		if err != nil {
			continue
		}
		// A response frame may itself carry more than one message (see
		// acf.DecodeFrame/TC18 §12.9.1.1) — e.g. a server answering
		// multiple requests this Controller batched into one outbound
		// frame. Dispatch each by its own TransactionNum rather than
		// assuming there is exactly one.
		for _, msg := range frame.Messages {
			c.mu.Lock()
			ch, ok := c.pending[msg.TransactionNum]
			c.mu.Unlock()
			if ok {
				select {
				case ch <- msg:
				default:
				}
			}
		}
	}
}
