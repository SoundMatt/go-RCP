//fusa:req REQ-CAN-001
//fusa:req REQ-CAN-002
//fusa:req REQ-CAN-003
//fusa:req REQ-CAN-004
//fusa:req REQ-CAN-005
//fusa:req REQ-CAN-006
//fusa:req REQ-CAN-007
//fusa:req REQ-CAN-008

// Package canbr provides a CAN bus bridge for go-RCP.
//
// CAN (Controller Area Network) is the dominant low-bandwidth serial bus in
// automotive systems. This package maps rcp.Commands to standard CAN frames
// (11-bit IDs, up to 8 data bytes) and vice versa.
//
// Bus is a pure-Go in-process CAN bus that lets multiple Servers and
// Controllers communicate without real CAN hardware. For production use,
// replace Bus with a socketcan-backed implementation (Linux: PF_CAN).
//
// Frame layout (9 bytes):
//
//	[0:4]  CANID  (uint32 big-endian; bits 0-10 = 11-bit standard ID)
//	[4]    DLC    (data length code, 0–8)
//	[5:13] Data   (8 bytes, zero-padded)
package canbr

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
)

const (
	frameSize = 13 // 4 (CANID) + 1 (DLC) + 8 (data)
	// responseFlag is OR'd into the CAN ID to mark response frames.
	responseFlag uint32 = 0x400
)

// ErrMalformedFrame is returned for frames shorter than frameSize.
var ErrMalformedFrame = errors.New("rcp/canbr: malformed CAN frame")

// Frame is a standard CAN frame.
type Frame struct {
	ID   uint32 // 11-bit standard ID (bits 0-10)
	DLC  uint8  // data length code (0–8)
	Data [8]byte
}

// Encode serialises f to wire bytes.
func (f Frame) Encode() []byte {
	out := make([]byte, frameSize)
	binary.BigEndian.PutUint32(out[0:], f.ID)
	out[4] = f.DLC
	copy(out[5:], f.Data[:])
	return out
}

// Decode parses wire bytes into a Frame.
func Decode(b []byte) (Frame, error) {
	if len(b) < frameSize {
		return Frame{}, ErrMalformedFrame
	}
	var f Frame
	f.ID = binary.BigEndian.Uint32(b[0:])
	f.DLC = b[4]
	copy(f.Data[:], b[5:])
	return f, nil
}

// ─── Bus ─────────────────────────────────────────────────────────────────────

// Bus is an in-process CAN bus that delivers frames to all subscribed receivers.
// It models the broadcast nature of a real CAN bus without OS sockets.
type Bus struct {
	mu   sync.RWMutex
	subs map[chan Frame]struct{}
}

// NewBus returns an empty Bus.
func NewBus() *Bus { return &Bus{subs: make(map[chan Frame]struct{})} }

// Send broadcasts f to all current subscribers.
func (b *Bus) Send(f Frame) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subs {
		select {
		case ch <- f:
		default:
		}
	}
}

// Subscribe returns a channel that receives all frames sent on the bus.
// The channel is buffered (depth 32). Call Unsubscribe when done.
func (b *Bus) Subscribe() chan Frame {
	ch := make(chan Frame, 32)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

// Unsubscribe removes ch from the bus and closes it.
func (b *Bus) Unsubscribe(ch chan Frame) {
	b.mu.Lock()
	delete(b.subs, ch)
	b.mu.Unlock()
	close(ch)
}

// ─── Server ───────────────────────────────────────────────────────────────────

// Server subscribes to a Bus, dispatches incoming RCP command frames to an
// rcp.Controller, and publishes response frames back onto the bus.
type Server struct {
	ctrl rcp.Controller
	bus  *Bus
	ch   chan Frame
	done chan struct{}
}

// NewServer starts a Server listening on bus, dispatching to ctrl.
func NewServer(ctrl rcp.Controller, bus *Bus) *Server {
	s := &Server{
		ctrl: ctrl,
		bus:  bus,
		ch:   bus.Subscribe(),
		done: make(chan struct{}),
	}
	go s.run()
	return s
}

// Close shuts the server down.
func (s *Server) Close() {
	s.bus.Unsubscribe(s.ch)
	<-s.done
}

func (s *Server) run() {
	defer close(s.done)
	for f := range s.ch {
		if f.ID&responseFlag != 0 {
			continue // ignore response frames
		}
		go s.dispatch(f)
	}
}

func (s *Server) dispatch(f Frame) {
	dlc := int(f.DLC)
	if dlc > 8 {
		dlc = 8
	}
	cmd := &rcp.Command{
		Zone:     s.ctrl.Zone(),
		Type:     rcp.CommandType(f.ID & 0x7FF),
		Priority: rcp.PriorityNormal,
		Payload:  f.Data[:dlc],
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := s.ctrl.Send(ctx, cmd)
	respID := f.ID | responseFlag
	respFrame := Frame{ID: respID}
	if err == nil && resp != nil && len(resp.Payload) > 0 {
		n := copy(respFrame.Data[:], resp.Payload)
		respFrame.DLC = uint8(n)
	}
	s.bus.Send(respFrame)
}

// ─── Controller ───────────────────────────────────────────────────────────────

// Controller implements rcp.Controller over a CAN bus.
// Commands are sent as CAN frames; responses are matched by frame ID.
type Controller struct {
	zone   rcp.Zone
	bus    *Bus
	ch     chan Frame
	closed atomic.Bool
	done   chan struct{}

	mu      sync.Mutex
	pending map[uint32]chan Frame
}

// NewController returns an rcp.Controller for zone that communicates on bus.
func NewController(zone rcp.Zone, bus *Bus) *Controller {
	c := &Controller{
		zone:    zone,
		bus:     bus,
		ch:      bus.Subscribe(),
		done:    make(chan struct{}),
		pending: make(map[uint32]chan Frame),
	}
	go c.readLoop()
	return c
}

// Zone implements rcp.Controller.
func (c *Controller) Zone() rcp.Zone { return c.zone }

// Send implements rcp.Controller — encodes cmd as a CAN frame and waits for response.
func (c *Controller) Send(ctx context.Context, cmd *rcp.Command) (*rcp.Response, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("rcp/canbr: zone %s: %w", c.zone, rcp.ErrClosed)
	}
	if cmd.Zone != c.zone {
		return nil, fmt.Errorf("rcp/canbr: zone %s: %w", c.zone, rcp.ErrZoneMismatch)
	}

	canID := uint32(cmd.Type) & 0x7FF
	respID := canID | responseFlag

	ch := make(chan Frame, 1)
	c.mu.Lock()
	c.pending[respID] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, respID)
		c.mu.Unlock()
	}()

	var f Frame
	f.ID = canID
	if len(cmd.Payload) > 0 {
		n := copy(f.Data[:], cmd.Payload)
		f.DLC = uint8(n)
	}
	c.bus.Send(f)

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("rcp/canbr: zone %s: %w", c.zone, rcp.ErrTimeout)
	case <-c.done:
		return nil, fmt.Errorf("rcp/canbr: zone %s: %w", c.zone, rcp.ErrClosed)
	case resp := <-ch:
		dlc := int(resp.DLC)
		if dlc > 8 {
			dlc = 8
		}
		payload := make([]byte, dlc)
		copy(payload, resp.Data[:dlc])
		return &rcp.Response{Zone: c.zone, Status: rcp.StatusOK, Payload: payload}, nil
	}
}

// Subscribe implements rcp.Controller — CAN broadcast events are not modelled
// as status subscriptions; this returns an empty channel that closes on Close.
func (c *Controller) Subscribe(_ context.Context) (<-chan *rcp.Status, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("rcp/canbr: zone %s: %w", c.zone, rcp.ErrClosed)
	}
	ch := make(chan *rcp.Status)
	go func() { <-c.done; close(ch) }()
	return ch, nil
}

// Close implements rcp.Controller — idempotent.
func (c *Controller) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	c.bus.Unsubscribe(c.ch)
	return nil
}

func (c *Controller) readLoop() {
	defer close(c.done)
	for f := range c.ch {
		if f.ID&responseFlag == 0 {
			continue // ignore request frames
		}
		c.mu.Lock()
		ch, ok := c.pending[f.ID]
		c.mu.Unlock()
		if ok {
			select {
			case ch <- f:
			default:
			}
		}
	}
}
