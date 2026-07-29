// Package mock provides in-process test doubles for go-RCP.
//
// This file (Controller/Registry) is this package's original in-process
// fake of the pre-TC18 bespoke Zone/Command/Response/Status API this repo
// is in the process of replacing (ROADMAP.md Phase 13 onward). Per Phase
// 17's disposition table, mock is REPLACE-flagged: the reference test
// double must implement the new server/endpoint/register-map model (see
// endpoint.go, client.go, client_registry.go, fixture.go in this same
// package) to be useful for testing anything built in Phases 13-16.
//
// Controller and Registry below are kept, unchanged, rather than renamed
// or removed, because they remain load-bearing for code this milestone
// (57, v0.70.0) does not touch — cmd/go-rcp, cmd/rcptool,
// optional_test.go, adapt_test.go, and safety/command_latency_test.go all
// still exercise the pre-TC18 rcp.Controller surface directly, and
// ROADMAP.md's own Phase 17 disposition table explicitly defers rebuilding
// them (and retiring the Zone/Command/Response/Status API they depend on)
// to Phase 18's cutover (Milestone 59, v1.0.0) — not to this milestone.
// This mirrors the precedent tlstransport/legacyframe.go already set at
// Milestone 54 (v0.67.0): freeze the old surface in place, in its own
// clearly-labelled corner of the package, rather than force an unrelated
// dependent's premature migration. See Controller's and Registry's own doc
// comments for the formal Deprecated notice. All exported methods are safe
// for concurrent use.
package mock

//fusa:req REQ-CTRL-001
//fusa:req REQ-CTRL-002
//fusa:req REQ-CTRL-003
//fusa:req REQ-CTRL-004
//fusa:req REQ-CTRL-005
//fusa:req REQ-CTRL-006
//fusa:req REQ-CTRL-007
//fusa:req REQ-CTRL-008
//fusa:req REQ-CTRL-009
//fusa:req REQ-CTRL-010
//fusa:req REQ-CTRL-011
//fusa:req REQ-CTRL-012
//fusa:req REQ-CTRL-013
//fusa:req REQ-CTRL-014
//fusa:req REQ-CTRL-015
//fusa:req REQ-CTRL-016
//fusa:req REQ-CTRL-017
//fusa:req REQ-CTRL-018
//fusa:req REQ-CTRL-019
//fusa:req REQ-CTRL-020
//fusa:req REQ-CTRL-021
//fusa:req REQ-CTRL-022
//fusa:req REQ-CTRL-023
//fusa:req REQ-CTRL-024
//fusa:req REQ-REG-001
//fusa:req REQ-REG-002
//fusa:req REQ-REG-003
//fusa:req REQ-REG-004
//fusa:req REQ-REG-005
//fusa:req REQ-REG-006
//fusa:req REQ-REG-007
//fusa:req REQ-REG-008
//fusa:req REQ-REG-009
//fusa:req REQ-REG-010
//fusa:req REQ-REG-011
//fusa:req REQ-REG-012
//fusa:req REQ-RESP-001
//fusa:req REQ-RESP-002
//fusa:req REQ-STAT-001
//fusa:req REQ-STAT-002
//fusa:req REQ-STAT-003
//fusa:req REQ-STAT-004
//fusa:req REQ-STAT-005
//fusa:req REQ-ERR-011
//fusa:req REQ-CTRL-025
//fusa:req REQ-CTRL-026
//fusa:req REQ-CTRL-027
//fusa:req REQ-REG-013

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	rcp "github.com/SoundMatt/go-RCP"
)

type sub struct {
	ch   chan *rcp.Status
	once sync.Once
}

func (s *sub) close() { s.once.Do(func() { close(s.ch) }) }

// Handler is a user-supplied function that produces a Response for a Command.
// If nil, the controller returns StatusOK with empty payload.
type Handler func(cmd *rcp.Command) *rcp.Response

// Controller is a mock zone controller that handles commands in-process.
//
// Frozen, not migrated by this milestone (57, v0.70.0): this is this
// package's pre-TC18 fake of the retired rcp.Controller interface, kept
// only because cmd/go-rcp, cmd/rcptool, optional_test.go, adapt_test.go,
// and safety/command_latency_test.go still depend on it (see mock.go's
// package doc comment). This is deliberately not marked with a formal Go
// "Deprecated:" doc comment, even though it functionally is one — doing so
// would make every one of those still-legitimate call sites a staticcheck
// SA1019 finding this milestone did not intend to create work for. Use
// Client (in client.go) for anything targeting the TC18 server/endpoint
// model.
type Controller struct {
	zone    rcp.Zone
	handler Handler
	closed  atomic.Bool

	mu   sync.Mutex
	subs []*sub
	seq  uint32
}

// NewController returns a mock Controller for the given zone.
// If handler is nil a default OK response is returned for every command.
func NewController(zone rcp.Zone, handler Handler) *Controller {
	return &Controller{zone: zone, handler: handler}
}

// Zone returns the zone managed by this controller.
func (c *Controller) Zone() rcp.Zone { return c.zone }

// Send executes the command via the handler and returns the response.
func (c *Controller) Send(ctx context.Context, cmd *rcp.Command) (*rcp.Response, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("mock controller zone %s: %w", c.zone, rcp.ErrClosed)
	}
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("mock controller zone %s: %w", c.zone, rcp.ErrTimeout)
	default:
	}
	if cmd.Zone != c.zone {
		return nil, fmt.Errorf("mock controller zone %s: %w", c.zone, rcp.ErrZoneMismatch)
	}
	// Copy payload so caller mutation after Send cannot affect the handler.
	safe := *cmd
	if len(cmd.Payload) > 0 {
		safe.Payload = make([]byte, len(cmd.Payload))
		copy(safe.Payload, cmd.Payload)
	}
	if c.handler != nil {
		return c.handler(&safe), nil
	}
	return &rcp.Response{
		CommandID: cmd.ID,
		Zone:      c.zone,
		Status:    rcp.StatusOK,
	}, nil
}

// Subscribe returns a channel of Status updates published via Publish.
// The channel is closed when ctx is cancelled or the controller is closed.
func (c *Controller) Subscribe(ctx context.Context) (<-chan *rcp.Status, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("mock controller zone %s: %w", c.zone, rcp.ErrClosed)
	}
	s := &sub{ch: make(chan *rcp.Status, 16)}
	c.mu.Lock()
	c.subs = append(c.subs, s)
	c.mu.Unlock()

	go func() {
		<-ctx.Done()
		c.mu.Lock()
		for i, existing := range c.subs {
			if existing == s {
				c.subs = append(c.subs[:i], c.subs[i+1:]...)
				break
			}
		}
		c.mu.Unlock()
		s.close()
	}()
	return s.ch, nil
}

// Publish pushes a Status update to all active subscribers.
func (c *Controller) Publish(payload []byte) {
	seq := atomic.AddUint32(&c.seq, 1)
	// Copy payload so caller mutation after Publish cannot affect delivered Status values.
	var p []byte
	if len(payload) > 0 {
		p = make([]byte, len(payload))
		copy(p, payload)
	}
	st := &rcp.Status{
		Zone:    c.zone,
		Seq:     seq,
		Healthy: !c.closed.Load(),
		Payload: p,
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range c.subs {
		select {
		case s.ch <- st:
		default:
		}
	}
}

// Close marks the controller closed and closes all subscriber channels.
func (c *Controller) Close() error {
	if c.closed.Swap(true) {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range c.subs {
		s.close()
	}
	c.subs = nil
	return nil
}

// Registry is an in-process RCP registry backed by mock controllers.
//
// Frozen, not migrated by this milestone (57, v0.70.0): this is this
// package's pre-TC18 fake of the retired zone registry, kept only for the
// same reasons Controller is (see its own doc comment, including why this
// is deliberately not a formal Go "Deprecated:" comment). Use
// ClientRegistry (in client_registry.go) for anything targeting the TC18
// server/endpoint model.
type Registry struct {
	mu     sync.RWMutex
	ctrls  map[rcp.Zone]*Controller
	closed bool
}

// NewRegistry returns a Registry pre-populated with mock controllers for all
// standard vehicle zones (FrontLeft, FrontRight, RearLeft, RearRight, Central).
func NewRegistry() *Registry {
	r := &Registry{ctrls: make(map[rcp.Zone]*Controller)}
	for _, z := range []rcp.Zone{
		rcp.ZoneFrontLeft,
		rcp.ZoneFrontRight,
		rcp.ZoneRearLeft,
		rcp.ZoneRearRight,
		rcp.ZoneCentral,
	} {
		r.ctrls[z] = NewController(z, nil)
	}
	return r
}

// Register adds a controller to the registry.
func (r *Registry) Register(ctrl rcp.Controller) error {
	mc, ok := ctrl.(*Controller)
	if !ok {
		return fmt.Errorf("mock registry: Register requires *mock.Controller")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return fmt.Errorf("mock registry: %w", rcp.ErrClosed)
	}
	if _, exists := r.ctrls[mc.Zone()]; exists {
		return fmt.Errorf("mock registry zone %s: %w", mc.Zone(), rcp.ErrAlreadyExists)
	}
	r.ctrls[mc.Zone()] = mc
	return nil
}

// Deregister removes and closes the controller for the given zone.
func (r *Registry) Deregister(zone rcp.Zone) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ctrl, ok := r.ctrls[zone]
	if !ok {
		return fmt.Errorf("mock registry zone %s: %w", zone, rcp.ErrNotFound)
	}
	delete(r.ctrls, zone)
	return ctrl.Close()
}

// Lookup returns the controller for the given zone.
func (r *Registry) Lookup(zone rcp.Zone) (rcp.Controller, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return nil, fmt.Errorf("mock registry: %w", rcp.ErrClosed)
	}
	ctrl, ok := r.ctrls[zone]
	if !ok {
		return nil, fmt.Errorf("mock registry zone %s: %w", zone, rcp.ErrNotFound)
	}
	return ctrl, nil
}

// Controllers returns all registered controllers.
func (r *Registry) Controllers() []rcp.Controller {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]rcp.Controller, 0, len(r.ctrls))
	for _, c := range r.ctrls {
		out = append(out, c)
	}
	return out
}

// Close closes all controllers and releases registry resources.
func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	for _, c := range r.ctrls {
		_ = c.Close()
	}
	r.ctrls = nil
	return nil
}
