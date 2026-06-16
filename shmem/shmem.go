// Package shmem provides a zero-copy intra-host command transport using
// shared in-process memory with sync.Pool buffer reuse.
//
// Within a single process (or two goroutines sharing the same address space)
// shmem avoids the serialisation overhead of UDP/TLS by passing *rcp.Command
// and *rcp.Response pointers through buffered channels, copying payload bytes
// exactly once into a pooled buffer on the Send path.
package shmem

//fusa:req REQ-SHMEM-001
//fusa:req REQ-SHMEM-002
//fusa:req REQ-SHMEM-003
//fusa:req REQ-SHMEM-004
//fusa:req REQ-SHMEM-005
//fusa:req REQ-SHMEM-006
//fusa:req REQ-SHMEM-007
//fusa:req REQ-SHMEM-008

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	rcp "github.com/SoundMatt/go-RCP"
)

// bufPool is the shared buffer pool used to minimise allocations on the hot path.
var bufPool = sync.Pool{New: func() any { b := make([]byte, 0, 256); return &b }}

func poolAlloc(n int) []byte {
	if n == 0 {
		return nil
	}
	bp, _ := bufPool.Get().(*[]byte)
	var old []byte
	if bp != nil {
		old = *bp
	}
	buf := make([]byte, n)
	if bp != nil && cap(old) >= n {
		bufPool.Put(bp)
	}
	return buf
}

// pendingOp is an in-flight Send waiting for its response.
type pendingOp struct {
	cmd  *rcp.Command
	resp chan *rcp.Response
}

// subscriptionEntry is a subscriber registered with the ZoneServer.
type subscriptionEntry struct {
	ch   chan *rcp.Status
	once sync.Once
}

func (s *subscriptionEntry) close() { s.once.Do(func() { close(s.ch) }) }

// Bus is the shared in-memory transport channel between a Controller and a ZoneServer.
type Bus struct {
	zone    rcp.Zone
	cmdCh   chan pendingOp   // Controller → ZoneServer
	statCh  chan *rcp.Status // ZoneServer → all subscribers (broadcast via fan-out goroutine)
	closed  atomic.Bool
	closeCh chan struct{}
}

func newBus(zone rcp.Zone) *Bus {
	return &Bus{
		zone:    zone,
		cmdCh:   make(chan pendingOp, 64),
		statCh:  make(chan *rcp.Status, 64),
		closeCh: make(chan struct{}),
	}
}

// ZoneServer is the server side of the shmem bus, analogous to a zone controller process.
type ZoneServer struct {
	bus     *Bus
	healthy atomic.Bool
	seq     atomic.Uint32

	mu      sync.Mutex
	handler func(*rcp.Command) *rcp.Response
	subs    []*subscriptionEntry
	done    chan struct{}
}

// newZoneServer creates a ZoneServer attached to the given Bus and starts its serve goroutine.
func newZoneServer(bus *Bus) *ZoneServer {
	s := &ZoneServer{bus: bus, done: make(chan struct{})}
	s.healthy.Store(true)
	go s.serve()
	return s
}

// SetHandler installs a command handler. nil returns StatusOK.
func (s *ZoneServer) SetHandler(h func(*rcp.Command) *rcp.Response) {
	s.mu.Lock()
	s.handler = h
	s.mu.Unlock()
}

// SetHealthy controls the Healthy flag in published Status frames.
func (s *ZoneServer) SetHealthy(v bool) { s.healthy.Store(v) }

// Publish broadcasts a Status to all current subscribers.
func (s *ZoneServer) Publish(payload []byte) {
	seq := s.seq.Add(1)
	var p []byte
	if len(payload) > 0 {
		p = poolAlloc(len(payload))
		copy(p, payload)
	}
	st := &rcp.Status{Zone: s.bus.zone, Seq: seq, Healthy: s.healthy.Load(), Payload: p}

	s.mu.Lock()
	subs := make([]*subscriptionEntry, len(s.subs))
	copy(subs, s.subs)
	s.mu.Unlock()

	for _, sub := range subs {
		select {
		case sub.ch <- st:
		default:
		}
	}
}

// subscribe registers a subscriber channel and returns it.
func (s *ZoneServer) subscribe(ctx context.Context) <-chan *rcp.Status {
	sub := &subscriptionEntry{ch: make(chan *rcp.Status, 16)}
	s.mu.Lock()
	s.subs = append(s.subs, sub)
	s.mu.Unlock()

	go func() {
		select {
		case <-ctx.Done():
		case <-s.bus.closeCh:
		}
		s.removeSub(sub)
		sub.close()
	}()
	return sub.ch
}

func (s *ZoneServer) removeSub(sub *subscriptionEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, e := range s.subs {
		if e == sub {
			s.subs = append(s.subs[:i], s.subs[i+1:]...)
			return
		}
	}
}

func (s *ZoneServer) serve() {
	defer close(s.done)
	for {
		select {
		case op, ok := <-s.bus.cmdCh:
			if !ok {
				return
			}
			s.mu.Lock()
			h := s.handler
			s.mu.Unlock()
			var resp *rcp.Response
			if h != nil {
				resp = h(op.cmd)
			} else {
				resp = &rcp.Response{CommandID: op.cmd.ID, Zone: s.bus.zone, Status: rcp.StatusOK}
			}
			select {
			case op.resp <- resp:
			default:
			}
		case <-s.bus.closeCh:
			return
		}
	}
}

// Controller is an rcp.Controller backed by a shared in-process Bus.
type Controller struct {
	bus    *Bus
	server *ZoneServer
	nextID atomic.Uint32
	closed atomic.Bool
}

func newController(bus *Bus, server *ZoneServer) *Controller {
	return &Controller{bus: bus, server: server}
}

// Zone implements rcp.Controller.
func (c *Controller) Zone() rcp.Zone { return c.bus.zone }

// Send implements rcp.Controller.
func (c *Controller) Send(ctx context.Context, cmd *rcp.Command) (*rcp.Response, error) {
	if c.closed.Load() || c.bus.closed.Load() {
		return nil, fmt.Errorf("rcp/shmem: zone %s: %w", c.bus.zone, rcp.ErrClosed)
	}
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("rcp/shmem: zone %s: %w", c.bus.zone, rcp.ErrTimeout)
	default:
	}
	if cmd.Zone != c.bus.zone {
		return nil, fmt.Errorf("rcp/shmem: zone %s: %w", c.bus.zone, rcp.ErrZoneMismatch)
	}

	id := c.nextID.Add(1)
	safe := rcp.Command{
		ID:       id,
		Zone:     cmd.Zone,
		Type:     cmd.Type,
		Priority: cmd.Priority,
	}
	if len(cmd.Payload) > 0 {
		safe.Payload = poolAlloc(len(cmd.Payload))
		copy(safe.Payload, cmd.Payload)
	}

	respCh := make(chan *rcp.Response, 1)
	op := pendingOp{cmd: &safe, resp: respCh}

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("rcp/shmem: zone %s: %w", c.bus.zone, rcp.ErrTimeout)
	case c.bus.cmdCh <- op:
	case <-c.bus.closeCh:
		return nil, fmt.Errorf("rcp/shmem: zone %s: %w", c.bus.zone, rcp.ErrClosed)
	}

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("rcp/shmem: zone %s: %w", c.bus.zone, rcp.ErrTimeout)
	case resp, ok := <-respCh:
		if !ok {
			return nil, fmt.Errorf("rcp/shmem: zone %s: %w", c.bus.zone, rcp.ErrClosed)
		}
		return resp, nil
	case <-c.bus.closeCh:
		return nil, fmt.Errorf("rcp/shmem: zone %s: %w", c.bus.zone, rcp.ErrClosed)
	}
}

// Subscribe implements rcp.Controller.
func (c *Controller) Subscribe(ctx context.Context) (<-chan *rcp.Status, error) {
	if c.closed.Load() || c.bus.closed.Load() {
		return nil, fmt.Errorf("rcp/shmem: zone %s: %w", c.bus.zone, rcp.ErrClosed)
	}
	return c.server.subscribe(ctx), nil
}

// Close implements rcp.Controller.
func (c *Controller) Close() error {
	c.closed.Store(true)
	return nil
}

// Registry is an rcp.Registry backed by shmem buses.
type Registry struct {
	mu      sync.RWMutex
	buses   map[rcp.Zone]*Bus
	servers map[rcp.Zone]*ZoneServer
	ctrls   map[rcp.Zone]*Controller
	closed  bool
}

// NewRegistry returns an empty shmem Registry.
func NewRegistry() *Registry {
	return &Registry{
		buses:   make(map[rcp.Zone]*Bus),
		servers: make(map[rcp.Zone]*ZoneServer),
		ctrls:   make(map[rcp.Zone]*Controller),
	}
}

// Open creates a Bus + ZoneServer + Controller for zone and registers it.
// Returns (ZoneServer, Controller) for test-side access to handler and publish.
// Returns ErrAlreadyExists if zone is already registered.
func (r *Registry) Open(zone rcp.Zone) (*ZoneServer, *Controller, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, nil, fmt.Errorf("rcp/shmem: registry: %w", rcp.ErrClosed)
	}
	if _, ok := r.ctrls[zone]; ok {
		return nil, nil, fmt.Errorf("rcp/shmem: registry zone %s: %w", zone, rcp.ErrAlreadyExists)
	}
	bus := newBus(zone)
	srv := newZoneServer(bus)
	ctrl := newController(bus, srv)
	r.buses[zone] = bus
	r.servers[zone] = srv
	r.ctrls[zone] = ctrl
	return srv, ctrl, nil
}

// Register implements rcp.Registry (accepts only *shmem.Controller).
func (r *Registry) Register(ctrl rcp.Controller) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return fmt.Errorf("rcp/shmem: registry: %w", rcp.ErrClosed)
	}
	if _, ok := r.ctrls[ctrl.Zone()]; ok {
		return fmt.Errorf("rcp/shmem: registry zone %s: %w", ctrl.Zone(), rcp.ErrAlreadyExists)
	}
	sc, ok := ctrl.(*Controller)
	if !ok {
		return fmt.Errorf("rcp/shmem: registry: only *shmem.Controller may be registered")
	}
	r.ctrls[ctrl.Zone()] = sc
	return nil
}

// Deregister implements rcp.Registry.
func (r *Registry) Deregister(zone rcp.Zone) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ctrl, ok := r.ctrls[zone]
	if !ok {
		return fmt.Errorf("rcp/shmem: registry zone %s: %w", zone, rcp.ErrNotFound)
	}
	delete(r.ctrls, zone)
	_ = ctrl.Close()
	if bus, ok := r.buses[zone]; ok {
		bus.closed.Store(true)
		close(bus.closeCh)
		delete(r.buses, zone)
	}
	if srv, ok := r.servers[zone]; ok {
		<-srv.done
		delete(r.servers, zone)
	}
	return nil
}

// Lookup implements rcp.Registry.
func (r *Registry) Lookup(zone rcp.Zone) (rcp.Controller, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return nil, fmt.Errorf("rcp/shmem: registry: %w", rcp.ErrClosed)
	}
	ctrl, ok := r.ctrls[zone]
	if !ok {
		return nil, fmt.Errorf("rcp/shmem: registry zone %s: %w", zone, rcp.ErrNotFound)
	}
	return ctrl, nil
}

// Controllers implements rcp.Registry.
func (r *Registry) Controllers() []rcp.Controller {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]rcp.Controller, 0, len(r.ctrls))
	for _, c := range r.ctrls {
		out = append(out, c)
	}
	return out
}

// Close implements rcp.Registry.
func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	for zone, bus := range r.buses {
		bus.closed.Store(true)
		close(bus.closeCh)
		delete(r.buses, zone)
	}
	for zone, srv := range r.servers {
		<-srv.done
		delete(r.servers, zone)
	}
	for zone := range r.ctrls {
		delete(r.ctrls, zone)
	}
	return nil
}
