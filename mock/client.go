package mock

//fusa:req REQ-MCL-001
//fusa:req REQ-MCL-002
//fusa:req REQ-MCL-003
//fusa:req REQ-MCL-004
//fusa:req REQ-MCL-005
//fusa:req REQ-MCL-006
//fusa:req REQ-MCL-007
//fusa:req REQ-MCL-008

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/regmap"
	"github.com/SoundMatt/go-RCP/udp"
)

// Client is an in-process fake of *udp.Controller (ROADMAP.md Milestone
// 54): it presents the identical Request/Read/Write/Discover/Close/
// StreamID surface, but calls straight into a *udp.Router's own Route
// method rather than encoding an AVTPDU and writing it to a socket —
// udp.Router.Route is already transport-agnostic (a plain
// (avtp.Header, acf.Message) -> (acf.Message, bool) function), so faking
// the wire in between is this package's whole "close to a from-scratch
// rewrite" per Phase 17's disposition table: nothing about Router,
// EP0Handler, or server.Server needed reimplementing here, only the
// client-side correlation Controller's own UDP socket normally provides.
// Because Route is synchronous, Client needs no pending-request map, no
// background read goroutine, and no channel — the entire round trip
// happens inline within Request. All exported methods are safe for
// concurrent use.
type Client struct {
	streamID avtp.StreamID
	router   *udp.Router
	nextTxn  atomic.Uint32
	seq      atomic.Uint32
	closed   atomic.Bool
}

// NewClient returns a Client presenting streamID as its own identity and
// addressing router in-process.
func NewClient(streamID avtp.StreamID, router *udp.Router) *Client {
	return &Client{streamID: streamID, router: router}
}

// StreamID returns this Client's own avtp.StreamID identity.
func (c *Client) StreamID() avtp.StreamID { return c.streamID }

// Request submits one plain (KindShort) request to addr with the given
// control flags and body, and returns the router's response — the
// in-process equivalent of *udp.Controller.Request's blocking round trip,
// minus anything ctx-cancellation could actually interrupt (there is no
// network I/O in flight to cancel; ctx is still honored before Route is
// called, matching *udp.Controller.Request's own pre-flight check).
func (c *Client) Request(ctx context.Context, addr avtp.ByteBusID, control acf.ControlFlags, body []byte) (acf.Message, error) {
	if c.closed.Load() {
		return acf.Message{}, fmt.Errorf("rcp/mock: client %s: %w", c.streamID, ErrClosed)
	}
	select {
	case <-ctx.Done():
		return acf.Message{}, fmt.Errorf("rcp/mock: client %s: %w", c.streamID, ctx.Err())
	default:
	}

	txn := avtp.TransactionNum(uint16(c.nextTxn.Add(1)))
	req := acf.Message{
		Kind:           acf.KindShort,
		ByteBusID:      addr,
		TransactionNum: txn,
		Control:        control,
		Body:           body,
	}
	hdr := avtp.Header{
		StreamIDValid: true,
		SequenceNum:   uint8(c.seq.Add(1)),
		StreamID:      c.streamID,
	}
	resp, ok := c.router.Route(hdr, req)
	if !ok {
		return acf.Message{}, fmt.Errorf("rcp/mock: client %s: %w", c.streamID, ErrDropped)
	}
	return resp, nil
}

// Read is Request with acf.FlagRead set and no body.
func (c *Client) Read(ctx context.Context, addr avtp.ByteBusID) (acf.Message, error) {
	return c.Request(ctx, addr, acf.FlagRead, nil)
}

// Write is Request with acf.FlagWrite set and the given body.
func (c *Client) Write(ctx context.Context, addr avtp.ByteBusID, body []byte) (acf.Message, error) {
	return c.Request(ctx, addr, acf.FlagWrite, body)
}

// Discover issues a discovery read against regmap.EP0, mirroring
// *udp.Controller.Discover.
func (c *Client) Discover(ctx context.Context) ([]byte, error) {
	resp, err := c.Read(ctx, regmap.EP0)
	if err != nil {
		return nil, err
	}
	if resp.Control.Has(acf.FlagError) {
		return nil, fmt.Errorf("rcp/mock: client %s: discover: %s", c.streamID, resp.Body)
	}
	return resp.Body, nil
}

// Close marks the Client closed; subsequent Request calls report
// ErrClosed. Safe to call multiple times.
func (c *Client) Close() error {
	c.closed.Store(true)
	return nil
}
