// Package mock provides an in-process RCP controller and registry for unit tests.
//
// All operations execute synchronously in memory — no network, no goroutines
// unless Subscribe is called. The mock is safe for concurrent use.
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
//fusa:req REQ-REG-001
//fusa:req REQ-REG-002
//fusa:req REQ-REG-003
//fusa:req REQ-REG-004
//fusa:req REQ-REG-005
//fusa:req REQ-REG-006
//fusa:req REQ-REG-007

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

	if c.handler != nil {
		return c.handler(cmd), nil
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
	st := &rcp.Status{
		Zone:    c.zone,
		Seq:     seq,
		Healthy: !c.closed.Load(),
		Payload: payload,
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
type Registry struct {
	mu      sync.RWMutex
	ctrls   map[rcp.Zone]*Controller
	closed  bool
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
