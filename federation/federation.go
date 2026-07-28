// Package federation coordinates multiple HPCs that each own a disjoint
// subset of RC Servers, for the OPEN Alliance TC18 Remote Control Protocol
// (RCP), as described by the "OPEN Alliance TC18 Remote Control Protocol
// Specification v0.5.1_RC". A Registry maps a caller-chosen server identity
// to the HPC (*udp.Controller) that owns it; cross-HPC requests are
// forwarded transparently.
//
// Each HPC calls Register to claim ownership of its servers. A Lookup
// returns the owning controller for a server regardless of which HPC is
// calling; the caller sees a single unified server namespace.
//
// Ownership is exclusive: registering a server that is already owned
// returns ErrAlreadyOwned. An HPC may release its servers by calling
// Release.
//
// This is an in-process coordination layer suitable for testing and
// single-binary deployments. For cross-process federation, each HPC would
// hold a remote controller (e.g. a gRPC bridge) that presents the same
// Request/Read/Write surface.
//
// This is ROADMAP.md Milestone 55 (v0.68.0)'s ADAPT-flagged rebuild: per
// Phase 17's disposition table, "ownership/leasing coordination across
// multiple HPCs is reusable; re-key ownership by server/endpoint instead of
// Zone." The retired rcp.Zone key was a fixed, small enum; this milestone's
// addressing model has no equivalent closed set (an HPC's target is a
// *udp.Controller dialed at runtime, possibly after discovery), so Registry
// re-keys by a caller-chosen string label instead — the exact "a caller is
// free to use a server's discovered avtp.StreamID.String(), its dialed
// address, or any other identity scheme that suits it" pattern
// udp.Registry's own doc comment already establishes for this same
// re-keying problem (see udp/registry.go).
package federation

//fusa:req REQ-FED-001
//fusa:req REQ-FED-002
//fusa:req REQ-FED-003
//fusa:req REQ-FED-004
//fusa:req REQ-FED-005
//fusa:req REQ-FED-006
//fusa:req REQ-FED-007
//fusa:req REQ-FED-008

import (
	"errors"
	"fmt"
	"sync"

	"github.com/SoundMatt/go-RCP/udp"
)

// ErrAlreadyOwned is returned when a server key is already registered to
// another HPC.
var ErrAlreadyOwned = errors.New("rcp/federation: server already owned by another HPC")

// ErrNotOwned is returned when a server key has no registered owner.
var ErrNotOwned = errors.New("rcp/federation: server has no registered owner")

// Registry is a thread-safe map of server key -> owning controller.
// Multiple HPCs share a single Registry instance.
type Registry struct {
	mu     sync.RWMutex
	owners map[string]*udp.Controller
}

// NewRegistry creates an empty federation Registry.
func NewRegistry() *Registry {
	return &Registry{owners: make(map[string]*udp.Controller)}
}

// Register claims ownership of key for ctrl.
// Returns ErrAlreadyOwned if another controller already owns the key.
func (r *Registry) Register(key string, ctrl *udp.Controller) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.owners[key]; exists {
		return fmt.Errorf("rcp/federation: server %s: %w", key, ErrAlreadyOwned)
	}
	r.owners[key] = ctrl
	return nil
}

// Release removes ownership of key. Returns ErrNotOwned if key is not
// registered.
func (r *Registry) Release(key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.owners[key]; !exists {
		return fmt.Errorf("rcp/federation: server %s: %w", key, ErrNotOwned)
	}
	delete(r.owners, key)
	return nil
}

// Lookup returns the controller that owns key.
// Returns ErrNotOwned if no HPC has registered key.
func (r *Registry) Lookup(key string) (*udp.Controller, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ctrl, ok := r.owners[key]
	if !ok {
		return nil, fmt.Errorf("rcp/federation: server %s: %w", key, ErrNotOwned)
	}
	return ctrl, nil
}

// Keys returns all currently registered server keys in an unspecified
// order.
func (r *Registry) Keys() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	keys := make([]string, 0, len(r.owners))
	for k := range r.owners {
		keys = append(keys, k)
	}
	return keys
}

// Owner returns the controller that owns key, or nil if unowned.
func (r *Registry) Owner(key string) *udp.Controller {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.owners[key]
}

// TransferOwnership atomically transfers key from one HPC's controller to
// another's. Returns ErrNotOwned if from does not currently own key.
func (r *Registry) TransferOwnership(key string, from, to *udp.Controller) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, exists := r.owners[key]
	if !exists || current != from {
		return fmt.Errorf("rcp/federation: server %s: %w", key, ErrNotOwned)
	}
	r.owners[key] = to
	return nil
}
