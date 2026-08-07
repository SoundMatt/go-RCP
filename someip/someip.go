// Package someip provides a SOME/IP service-method bridge for go-RCP, for
// the OPEN Alliance TC18 Remote Control Protocol (RCP), as described by the
// "OPEN Alliance TC18 Remote Control Protocol Specification v0.5.1_RC".
//
// This is ROADMAP.md Milestone 56 (v0.69.0)'s ADAPT-flagged rebuild: per
// Phase 17's disposition table, SOME/IP service-method bridging is
// orthogonal to TC18 RCP and stays genuinely necessary, just re-pointed at
// endpoint requests/responses. A SOME/IP method ID now addresses an
// avtp.ByteBusID directly (its low byte — see addressFor), and a SOME/IP
// REQUEST's own Read/Write intent (the retired package had none to carry:
// rcp.CommandType was itself the whole operation) is inferred from whether
// its payload is empty: an empty-payload REQUEST reads the addressed
// endpoint, a non-empty one writes it. This is this package's own free
// design choice for a protocol the specification does not itself define an
// RCP mapping for, not a verified transcription of any SOME/IP or TC18 byte
// layout (see doc.go's spec-fidelity notes elsewhere in this repo for the
// same posture).
//
// Server listens for incoming SOME/IP REQUEST datagrams and forwards each
// to an upstream *udp.Controller as one plain request, replying with a
// SOME/IP RESPONSE. Controller is the reciprocal client-side stub: it
// presents the same Request/Read/Write/Close surface a *udp.Controller
// does (Milestone 54's own "Controller-equivalent interface" precedent —
// see grpcbridge/restbridge, this milestone's other cloud-facing bridges)
// but reaches a remote someip.Server over SOME/IP datagrams instead of
// dialing an RC Server directly.
//
// SOME/IP header layout (16 bytes) is unchanged from the retired package:
//
//	[0:2]  Service ID  (uint16 big-endian)
//	[2:4]  Method  ID  (uint16 big-endian)
//	[4:8]  Length      (uint32 big-endian, counts from ClientID to end)
//	[8:10] Client  ID  (uint16 big-endian)
//	[10:12]Session ID  (uint16 big-endian)
//	[12]   Proto   Ver (0x01)
//	[13]   Iface   Ver (0x01)
//	[14]   Msg Type    (0x00 REQUEST, 0x80 RESPONSE)
//	[15]   Return Code (0x00 OK, 0x01 NOT_OK)
package someip

//fusa:req REQ-SIPC-001
//fusa:req REQ-SIPC-002
//fusa:req REQ-SIPC-003
//fusa:req REQ-SIPC-004
//fusa:req REQ-SIPC-005
//fusa:req REQ-SIPC-006
//fusa:req REQ-SIPC-007
//fusa:req REQ-SIPC-008

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/udp"
)

const (
	headerLen = 16

	msgTypeRequest  uint8 = 0x00
	msgTypeResponse uint8 = 0x80

	retCodeOK    uint8 = 0x00
	retCodeNotOK uint8 = 0x01

	// DefaultServiceID is the SOME/IP service ID used for the RCP bridge.
	DefaultServiceID uint16 = 0x0E00

	// DefaultRequestTimeout bounds how long Server waits for the upstream
	// controller's response to a forwarded SOME/IP REQUEST.
	DefaultRequestTimeout = 5 * time.Second
)

// ErrMalformedFrame is returned when a SOME/IP frame is too short or invalid.
var ErrMalformedFrame = errors.New("rcp/someip: malformed SOME/IP frame")

// ErrClosed is returned by Controller methods once Close has been called.
var ErrClosed = errors.New("rcp/someip: closed")

// Header is a decoded SOME/IP message header.
type Header struct {
	ServiceID  uint16
	MethodID   uint16
	Length     uint32
	ClientID   uint16
	SessionID  uint16
	ProtoVer   uint8
	IfaceVer   uint8
	MsgType    uint8
	ReturnCode uint8
}

// addressFor derives the avtp.ByteBusID a SOME/IP MethodID addresses: its
// low byte, ByteBusID's own full representable width (see avtp/address.go).
func addressFor(methodID uint16) avtp.ByteBusID {
	return avtp.ByteBusID(methodID & 0xFF) //nolint:gosec // deliberate truncation, see addressFor's doc comment
}

// controlFor infers a SOME/IP REQUEST's Read/Write intent from its payload:
// empty means read, non-empty means write (see the package doc comment).
func controlFor(payload []byte) acf.ControlFlags {
	if len(payload) == 0 {
		return acf.FlagRead
	}
	return acf.FlagWrite
}

// encodeFrame serialises hdr and payload into a SOME/IP datagram.
func encodeFrame(hdr Header, payload []byte) []byte {
	out := make([]byte, headerLen+len(payload))
	binary.BigEndian.PutUint16(out[0:], hdr.ServiceID)
	binary.BigEndian.PutUint16(out[2:], hdr.MethodID)
	// Length field covers ClientID..end of payload (headerLen - 8 + len(payload))
	binary.BigEndian.PutUint32(out[4:], uint32(8+len(payload))) //nolint:gosec // payload is bounded by a UDP datagram's own size
	binary.BigEndian.PutUint16(out[8:], hdr.ClientID)
	binary.BigEndian.PutUint16(out[10:], hdr.SessionID)
	out[12] = hdr.ProtoVer
	out[13] = hdr.IfaceVer
	out[14] = hdr.MsgType
	out[15] = hdr.ReturnCode
	copy(out[headerLen:], payload)
	return out
}

// decodeFrame splits a SOME/IP datagram into header and payload.
func decodeFrame(b []byte) (Header, []byte, error) {
	if len(b) < headerLen {
		return Header{}, nil, ErrMalformedFrame
	}
	hdr := Header{
		ServiceID:  binary.BigEndian.Uint16(b[0:]),
		MethodID:   binary.BigEndian.Uint16(b[2:]),
		Length:     binary.BigEndian.Uint32(b[4:]),
		ClientID:   binary.BigEndian.Uint16(b[8:]),
		SessionID:  binary.BigEndian.Uint16(b[10:]),
		ProtoVer:   b[12],
		IfaceVer:   b[13],
		MsgType:    b[14],
		ReturnCode: b[15],
	}
	return hdr, b[headerLen:], nil
}

// ─── Server ───────────────────────────────────────────────────────────────────

// Server listens for SOME/IP REQUEST datagrams and forwards each to an
// upstream *udp.Controller as one plain request, replying with a SOME/IP
// RESPONSE datagram.
type Server struct {
	upstream  *udp.Controller
	serviceID uint16
	conn      *net.UDPConn
	timeout   time.Duration
	done      chan struct{}
}

// NewServer creates a Server forwarding to upstream, listening on addr.
// serviceID is the SOME/IP service identifier to accept; use DefaultServiceID.
func NewServer(upstream *udp.Controller, addr *net.UDPAddr, serviceID uint16) (*Server, error) {
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("rcp/someip: listen %s: %w", addr, err)
	}
	s := &Server{
		upstream:  upstream,
		serviceID: serviceID,
		conn:      conn,
		timeout:   DefaultRequestTimeout,
		done:      make(chan struct{}),
	}
	go s.readLoop()
	return s, nil
}

// Addr returns the local UDP address the server is listening on.
func (s *Server) Addr() *net.UDPAddr { return s.conn.LocalAddr().(*net.UDPAddr) } //nolint:errcheck

// Close shuts down the server.
func (s *Server) Close() error {
	err := s.conn.Close()
	<-s.done
	return err
}

func (s *Server) readLoop() {
	defer close(s.done)
	buf := make([]byte, 65535)
	for {
		n, remote, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		frame := make([]byte, n)
		copy(frame, buf[:n])
		go s.handle(frame, remote)
	}
}

func (s *Server) handle(frame []byte, remote *net.UDPAddr) {
	hdr, payload, err := decodeFrame(frame)
	if err != nil || hdr.MsgType != msgTypeRequest || hdr.ServiceID != s.serviceID {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()

	resp, err := s.upstream.Request(ctx, addressFor(hdr.MethodID), controlFor(payload), payload)
	retCode := retCodeOK
	var respPayload []byte
	if err != nil || resp.Control.Has(acf.FlagError) {
		retCode = retCodeNotOK
	} else {
		respPayload = resp.Body
	}

	respHdr := Header{
		ServiceID:  hdr.ServiceID,
		MethodID:   hdr.MethodID,
		ClientID:   hdr.ClientID,
		SessionID:  hdr.SessionID,
		ProtoVer:   0x01,
		IfaceVer:   0x01,
		MsgType:    msgTypeResponse,
		ReturnCode: retCode,
	}
	_, _ = s.conn.WriteToUDP(encodeFrame(respHdr, respPayload), remote)
}

// ─── Controller ───────────────────────────────────────────────────────────────

// Controller reaches a remote someip.Server over SOME/IP datagrams,
// presenting the same Request/Read/Write surface a *udp.Controller does.
type Controller struct {
	serviceID uint16
	conn      *net.UDPConn
	nextSess  atomic.Uint32
	closed    atomic.Bool
	readDone  chan struct{}

	mu      sync.Mutex
	pending map[uint16]chan pendingResult
}

// pendingResult pairs a response Header with its payload, delivered together
// on the same channel entry.
type pendingResult struct {
	hdr     Header
	payload []byte
}

// NewController dials a SOME/IP Server at serverAddr. serviceID must match
// the server's service ID.
func NewController(serverAddr *net.UDPAddr, serviceID uint16) (*Controller, error) {
	conn, err := net.DialUDP("udp", nil, serverAddr)
	if err != nil {
		return nil, fmt.Errorf("rcp/someip: dial %s: %w", serverAddr, err)
	}
	c := &Controller{
		serviceID: serviceID,
		conn:      conn,
		readDone:  make(chan struct{}),
		pending:   make(map[uint16]chan pendingResult),
	}
	go c.readLoop()
	return c, nil
}

// Request sends one SOME/IP REQUEST addressed to addr (its low byte becomes
// the SOME/IP MethodID) and blocks for the matching RESPONSE or ctx's
// expiry, whichever comes first.
func (c *Controller) Request(ctx context.Context, addr avtp.ByteBusID, control acf.ControlFlags, body []byte) (acf.Message, error) {
	if c.closed.Load() {
		return acf.Message{}, fmt.Errorf("rcp/someip: %w", ErrClosed)
	}

	sessID := uint16(c.nextSess.Add(1))
	ch := make(chan pendingResult, 1)
	c.mu.Lock()
	c.pending[sessID] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, sessID)
		c.mu.Unlock()
	}()

	reqHdr := Header{
		ServiceID: c.serviceID,
		MethodID:  uint16(addr),
		ClientID:  0x0001,
		SessionID: sessID,
		ProtoVer:  0x01,
		IfaceVer:  0x01,
		MsgType:   msgTypeRequest,
	}
	if _, err := c.conn.Write(encodeFrame(reqHdr, body)); err != nil {
		return acf.Message{}, fmt.Errorf("rcp/someip: Request write: %w", err)
	}

	select {
	case <-ctx.Done():
		return acf.Message{}, fmt.Errorf("rcp/someip: %w", ctx.Err())
	case result, ok := <-ch:
		if !ok {
			return acf.Message{}, fmt.Errorf("rcp/someip: %w", ErrClosed)
		}
		respControl := acf.FlagResponse | (control & (acf.FlagRead | acf.FlagWrite))
		if result.hdr.ReturnCode != retCodeOK {
			respControl |= acf.FlagError
		}
		return acf.Message{ByteBusID: addr, Control: respControl, Body: result.payload}, nil
	case <-c.readDone:
		return acf.Message{}, fmt.Errorf("rcp/someip: %w", ErrClosed)
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

// Close implements idempotent shutdown.
func (c *Controller) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	return c.conn.Close()
}

func (c *Controller) readLoop() {
	defer close(c.readDone)
	buf := make([]byte, 65535)
	for {
		n, err := c.conn.Read(buf)
		if err != nil {
			return
		}
		hdr, payload, err := decodeFrame(buf[:n])
		if err != nil || hdr.MsgType != msgTypeResponse {
			continue
		}
		c.mu.Lock()
		ch, ok := c.pending[hdr.SessionID]
		c.mu.Unlock()
		if ok {
			result := pendingResult{hdr: hdr, payload: append([]byte(nil), payload...)}
			select {
			case ch <- result:
			default:
			}
		}
	}
}
