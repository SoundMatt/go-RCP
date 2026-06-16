package udp

import (
	"fmt"
	"net"
	"sync"

	rcp "github.com/SoundMatt/go-RCP"
)

// Registry is an rcp.Registry backed by static unicast UDP zone addresses.
type Registry struct {
	mu     sync.RWMutex
	ctrls  map[rcp.Zone]*Controller
	closed bool
}

// NewRegistry returns an empty UDP Registry.
func NewRegistry() *Registry {
	return &Registry{ctrls: make(map[rcp.Zone]*Controller)}
}

// Dial resolves serverAddr, dials a UDP Controller for zone, and registers it.
// Returns ErrAlreadyExists if the zone is already registered.
func (r *Registry) Dial(zone rcp.Zone, serverAddr string) error {
	addr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		return fmt.Errorf("rcp/udp: registry dial zone %s: %w", zone, err)
	}
	ctrl, err := NewController(zone, addr)
	if err != nil {
		return err
	}
	if err := r.Register(ctrl); err != nil {
		_ = ctrl.Close()
		return err
	}
	return nil
}

// Register implements rcp.Registry.
func (r *Registry) Register(ctrl rcp.Controller) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return fmt.Errorf("rcp/udp: registry: %w", rcp.ErrClosed)
	}
	if _, ok := r.ctrls[ctrl.Zone()]; ok {
		return fmt.Errorf("rcp/udp: registry zone %s: %w", ctrl.Zone(), rcp.ErrAlreadyExists)
	}
	udpCtrl, ok := ctrl.(*Controller)
	if !ok {
		return fmt.Errorf("rcp/udp: registry: only *udp.Controller may be registered")
	}
	r.ctrls[ctrl.Zone()] = udpCtrl
	return nil
}

// Deregister implements rcp.Registry.
func (r *Registry) Deregister(zone rcp.Zone) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ctrl, ok := r.ctrls[zone]
	if !ok {
		return fmt.Errorf("rcp/udp: registry zone %s: %w", zone, rcp.ErrNotFound)
	}
	delete(r.ctrls, zone)
	return ctrl.Close()
}

// Lookup implements rcp.Registry.
func (r *Registry) Lookup(zone rcp.Zone) (rcp.Controller, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return nil, fmt.Errorf("rcp/udp: registry: %w", rcp.ErrClosed)
	}
	ctrl, ok := r.ctrls[zone]
	if !ok {
		return nil, fmt.Errorf("rcp/udp: registry zone %s: %w", zone, rcp.ErrNotFound)
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
	var last error
	for zone, ctrl := range r.ctrls {
		if err := ctrl.Close(); err != nil {
			last = err
		}
		delete(r.ctrls, zone)
	}
	return last
}
