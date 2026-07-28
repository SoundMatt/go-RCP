package udp

import (
	"fmt"
	"net"
	"sync"

	"github.com/SoundMatt/go-RCP/avtp"
)

// Registry is a caller-keyed collection of dialed Controllers. The retired
// udp package keyed its Registry by rcp.Zone — a fixed, small enum this
// milestone's model has no equivalent of (a client's addressing target is
// now a UDP address it may only learn at runtime via discovery, not a
// value out of a closed set declared up front). Registry re-keys by a
// caller-chosen label string instead, the same "re-key ownership by
// server/endpoint identity instead of Zone" pattern Phase 17's disposition
// table applies to federation/zonegroup (ROADMAP.md Milestone 55) — a
// caller is free to use a server's discovered avtp.StreamID.String(), its
// dialed address, or any other identity scheme that suits it.
type Registry struct {
	mu     sync.RWMutex
	ctrls  map[string]*Controller
	closed bool
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{ctrls: make(map[string]*Controller)}
}

// Dial resolves serverAddr, dials a Controller presenting streamID, and
// registers it under key. It returns ErrAlreadyExists if key is already
// registered.
func (r *Registry) Dial(key string, streamID avtp.StreamID, serverAddr string) (*Controller, error) {
	addr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		return nil, fmt.Errorf("rcp/udp: registry dial %s: %w", key, err)
	}
	ctrl, err := NewController(streamID, addr)
	if err != nil {
		return nil, err
	}
	if err := r.Register(key, ctrl); err != nil {
		_ = ctrl.Close()
		return nil, err
	}
	return ctrl, nil
}

// Register adds ctrl under key. It returns ErrAlreadyExists if key is
// already registered.
func (r *Registry) Register(key string, ctrl *Controller) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return fmt.Errorf("rcp/udp: registry: %w", ErrClosed)
	}
	if _, ok := r.ctrls[key]; ok {
		return fmt.Errorf("rcp/udp: registry key %s: %w", key, ErrAlreadyExists)
	}
	r.ctrls[key] = ctrl
	return nil
}

// Deregister closes and removes the Controller registered under key.
func (r *Registry) Deregister(key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ctrl, ok := r.ctrls[key]
	if !ok {
		return fmt.Errorf("rcp/udp: registry key %s: %w", key, ErrNotFound)
	}
	delete(r.ctrls, key)
	return ctrl.Close()
}

// Lookup returns the Controller registered under key.
func (r *Registry) Lookup(key string) (*Controller, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return nil, fmt.Errorf("rcp/udp: registry: %w", ErrClosed)
	}
	ctrl, ok := r.ctrls[key]
	if !ok {
		return nil, fmt.Errorf("rcp/udp: registry key %s: %w", key, ErrNotFound)
	}
	return ctrl, nil
}

// Controllers returns every currently registered Controller.
func (r *Registry) Controllers() []*Controller {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Controller, 0, len(r.ctrls))
	for _, c := range r.ctrls {
		out = append(out, c)
	}
	return out
}

// Close closes every registered Controller and marks the Registry closed to
// further Dial/Register calls.
func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	var last error
	for key, ctrl := range r.ctrls {
		if err := ctrl.Close(); err != nil {
			last = err
		}
		delete(r.ctrls, key)
	}
	return last
}
