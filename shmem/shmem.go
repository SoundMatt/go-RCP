// Package shmem provides a zero-copy intra-host transport for the OPEN
// Alliance TC18 Remote Control Protocol (RCP), as described by the "OPEN
// Alliance TC18 Remote Control Protocol Specification v0.5.1_RC", using
// shared in-process memory in place of a real socket.
//
// This is ROADMAP.md Milestone 54 (v0.67.0)'s ADAPT-flagged rebuild: per
// Phase 17's disposition table, "zero-copy intra-host IPC is a
// transport-layer optimization independent of frame shape," so this
// package keeps the Bus pattern and only retargets what travels across it
// — acf.Message request/response pairs instead of *rcp.Command/
// *rcp.Response — and, since Message dispatch by avtp.ByteBusID is exactly
// what udp.Router already does and this package's whole point is that
// frame shape shouldn't matter to it, shmem reuses *udp.Router directly
// rather than re-implementing EP0/endpoint-Handler routing a second time.
//
// Within a single process (or two goroutines sharing the same address
// space) shmem avoids the serialization overhead of a real UDP/TLS socket
// by passing acf.Message values through buffered channels, copying a
// request's body bytes exactly once onto the bus (see copyBody) so a
// caller's later mutation of its own buffer is never visible on the other
// side. The retired shmem package additionally pooled that copy's backing
// array via sync.Pool (poolAlloc); this rebuild does not carry that over —
// poolAlloc's own implementation never actually reused a popped buffer's
// backing array for the new copy (see the git history this replaces), so
// it bought no real benefit, and pooling a buffer that outlives the copying
// call (as Router.Route's Handler may still be holding req.Body when this
// function would otherwise recycle it) is only safe with the loan
// package's own explicit-release tracking, not silently inside a plain
// copy helper.
package shmem

//fusa:req REQ-SHMEM-001
//fusa:req REQ-SHMEM-002
//fusa:req REQ-SHMEM-003
//fusa:req REQ-SHMEM-004
//fusa:req REQ-SHMEM-005
//fusa:req REQ-SHMEM-006

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/udp"
)

// copyBody returns a fresh copy of src, or nil for an empty src.
func copyBody(src []byte) []byte {
	if len(src) == 0 {
		return nil
	}
	buf := make([]byte, len(src))
	copy(buf, src)
	return buf
}

// pendingOp is an in-flight Request waiting for its response.
type pendingOp struct {
	hdr  avtp.Header
	req  acf.Message
	resp chan acf.Message
}

// Bus is the shared in-memory transport channel between a Controller and an
// Endpoint.
type Bus struct {
	reqCh   chan pendingOp
	closed  atomic.Bool
	closeCh chan struct{}
}

func newBus() *Bus {
	return &Bus{
		reqCh:   make(chan pendingOp, 64),
		closeCh: make(chan struct{}),
	}
}

// Endpoint is the server side of the shmem bus: it drains Bus.reqCh and
// answers each request through a *udp.Router, the same dispatch logic
// (EP0/registered-Handler routing, avtp.Header.Disposition drop rule) the
// networked udp.Server applies.
type Endpoint struct {
	bus      *Bus
	router   *udp.Router
	streamID avtp.StreamID
	done     chan struct{}
}

// newEndpoint creates an Endpoint attached to bus, answering through
// router and presenting streamID as its own AVTPDU identity, and starts its
// serve goroutine.
func newEndpoint(bus *Bus, router *udp.Router, streamID avtp.StreamID) *Endpoint {
	e := &Endpoint{bus: bus, router: router, streamID: streamID, done: make(chan struct{})}
	go e.serve()
	return e
}

func (e *Endpoint) serve() {
	defer close(e.done)
	for {
		select {
		case op, ok := <-e.bus.reqCh:
			if !ok {
				return
			}
			resp, shouldReply := e.router.Route(op.hdr, op.req)
			if shouldReply {
				select {
				case op.resp <- resp:
				default:
				}
			}
		case <-e.bus.closeCh:
			return
		}
	}
}

// Controller is the client side of the shmem bus: it presents streamID as
// its own identity and correlates requests to responses by
// acf.Message.TransactionNum, mirroring udp.Controller's own API shape so
// callers can swap between the two transports with minimal change.
type Controller struct {
	streamID avtp.StreamID
	bus      *Bus
	endpoint *Endpoint
	nextTxn  atomic.Uint32
	closed   atomic.Bool
}

func newController(streamID avtp.StreamID, bus *Bus, endpoint *Endpoint) *Controller {
	return &Controller{streamID: streamID, bus: bus, endpoint: endpoint}
}

// StreamID returns this Controller's own avtp.StreamID identity.
func (c *Controller) StreamID() avtp.StreamID { return c.streamID }

// Request sends one plain (KindShort) request to addr and blocks for the
// matching response or ctx's expiry, whichever comes first.
func (c *Controller) Request(ctx context.Context, addr avtp.ByteBusID, control acf.ControlFlags, body []byte) (acf.Message, error) {
	if c.closed.Load() || c.bus.closed.Load() {
		return acf.Message{}, fmt.Errorf("rcp/shmem: stream %s: %w", c.streamID, udp.ErrClosed)
	}
	select {
	case <-ctx.Done():
		return acf.Message{}, fmt.Errorf("rcp/shmem: stream %s: %w", c.streamID, udp.ErrTimeout)
	default:
	}

	txn := avtp.TransactionNum(uint16(c.nextTxn.Add(1)))
	req := acf.Message{
		Kind:           acf.KindShort,
		ByteBusID:      addr,
		TransactionNum: txn,
		Control:        control,
		Body:           copyBody(body),
	}
	hdr := avtp.Header{StreamIDValid: true, StreamID: c.streamID}

	respCh := make(chan acf.Message, 1)
	op := pendingOp{hdr: hdr, req: req, resp: respCh}

	select {
	case <-ctx.Done():
		return acf.Message{}, fmt.Errorf("rcp/shmem: stream %s: %w", c.streamID, udp.ErrTimeout)
	case c.bus.reqCh <- op:
	case <-c.bus.closeCh:
		return acf.Message{}, fmt.Errorf("rcp/shmem: stream %s: %w", c.streamID, udp.ErrClosed)
	}

	select {
	case <-ctx.Done():
		return acf.Message{}, fmt.Errorf("rcp/shmem: stream %s: %w", c.streamID, udp.ErrTimeout)
	case resp, ok := <-respCh:
		if !ok {
			return acf.Message{}, fmt.Errorf("rcp/shmem: stream %s: %w", c.streamID, udp.ErrClosed)
		}
		return resp, nil
	case <-c.bus.closeCh:
		return acf.Message{}, fmt.Errorf("rcp/shmem: stream %s: %w", c.streamID, udp.ErrClosed)
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

// Close marks the Controller closed. It does not close the underlying Bus.
func (c *Controller) Close() error {
	c.closed.Store(true)
	return nil
}

// Registry is a caller-keyed collection of shmem buses, mirroring
// udp.Registry's own re-keying rationale (see udp/registry.go).
type Registry struct {
	mu        sync.RWMutex
	buses     map[string]*Bus
	endpoints map[string]*Endpoint
	ctrls     map[string]*Controller
	closed    bool
}

// NewRegistry returns an empty shmem Registry.
func NewRegistry() *Registry {
	return &Registry{
		buses:     make(map[string]*Bus),
		endpoints: make(map[string]*Endpoint),
		ctrls:     make(map[string]*Controller),
	}
}

// Open creates a Bus + Endpoint + Controller under key, with the Endpoint
// answering through router and presenting serverStream as its identity, and
// the Controller presenting clientStream as its own. Returns (Endpoint,
// Controller) for test-side/caller-side access. Returns ErrAlreadyExists if
// key is already registered.
func (r *Registry) Open(key string, router *udp.Router, serverStream, clientStream avtp.StreamID) (*Endpoint, *Controller, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, nil, fmt.Errorf("rcp/shmem: registry: %w", udp.ErrClosed)
	}
	if _, ok := r.ctrls[key]; ok {
		return nil, nil, fmt.Errorf("rcp/shmem: registry key %s: %w", key, udp.ErrAlreadyExists)
	}
	bus := newBus()
	ep := newEndpoint(bus, router, serverStream)
	ctrl := newController(clientStream, bus, ep)
	r.buses[key] = bus
	r.endpoints[key] = ep
	r.ctrls[key] = ctrl
	return ep, ctrl, nil
}

// Lookup returns the Controller registered under key.
func (r *Registry) Lookup(key string) (*Controller, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return nil, fmt.Errorf("rcp/shmem: registry: %w", udp.ErrClosed)
	}
	ctrl, ok := r.ctrls[key]
	if !ok {
		return nil, fmt.Errorf("rcp/shmem: registry key %s: %w", key, udp.ErrNotFound)
	}
	return ctrl, nil
}

// Deregister closes and removes the Bus/Endpoint/Controller registered
// under key.
func (r *Registry) Deregister(key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ctrl, ok := r.ctrls[key]
	if !ok {
		return fmt.Errorf("rcp/shmem: registry key %s: %w", key, udp.ErrNotFound)
	}
	delete(r.ctrls, key)
	_ = ctrl.Close()
	if bus, ok := r.buses[key]; ok {
		bus.closed.Store(true)
		close(bus.closeCh)
		delete(r.buses, key)
	}
	if ep, ok := r.endpoints[key]; ok {
		<-ep.done
		delete(r.endpoints, key)
	}
	return nil
}

// Close closes every registered Bus/Endpoint/Controller.
func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	for key, bus := range r.buses {
		bus.closed.Store(true)
		close(bus.closeCh)
		delete(r.buses, key)
	}
	for key, ep := range r.endpoints {
		<-ep.done
		delete(r.endpoints, key)
	}
	for key := range r.ctrls {
		delete(r.ctrls, key)
	}
	return nil
}
