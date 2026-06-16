package tlstransport

import (
	"crypto/tls"
	"fmt"
	"sync"

	rcp "github.com/SoundMatt/go-RCP"
)

// Registry is an rcp.Registry backed by static TLS-dialled zone addresses.
type Registry struct {
	mu     sync.RWMutex
	ctrls  map[rcp.Zone]*Controller
	closed bool
}

// NewRegistry returns an empty TLS Registry.
func NewRegistry() *Registry {
	return &Registry{ctrls: make(map[rcp.Zone]*Controller)}
}

// Dial dials a TLS Controller for zone at serverAddr and registers it.
// Returns ErrAlreadyExists if zone is already registered.
func (r *Registry) Dial(zone rcp.Zone, serverAddr string, tlsCfg *tls.Config) error {
	ctrl, err := NewController(zone, serverAddr, tlsCfg)
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
		return fmt.Errorf("rcp/tls: registry: %w", rcp.ErrClosed)
	}
	if _, ok := r.ctrls[ctrl.Zone()]; ok {
		return fmt.Errorf("rcp/tls: registry zone %s: %w", ctrl.Zone(), rcp.ErrAlreadyExists)
	}
	tlsCtrl, ok := ctrl.(*Controller)
	if !ok {
		return fmt.Errorf("rcp/tls: registry: only *tlstransport.Controller may be registered")
	}
	r.ctrls[ctrl.Zone()] = tlsCtrl
	return nil
}

// Deregister implements rcp.Registry.
func (r *Registry) Deregister(zone rcp.Zone) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ctrl, ok := r.ctrls[zone]
	if !ok {
		return fmt.Errorf("rcp/tls: registry zone %s: %w", zone, rcp.ErrNotFound)
	}
	delete(r.ctrls, zone)
	return ctrl.Close()
}

// Lookup implements rcp.Registry.
func (r *Registry) Lookup(zone rcp.Zone) (rcp.Controller, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return nil, fmt.Errorf("rcp/tls: registry: %w", rcp.ErrClosed)
	}
	ctrl, ok := r.ctrls[zone]
	if !ok {
		return nil, fmt.Errorf("rcp/tls: registry zone %s: %w", zone, rcp.ErrNotFound)
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
