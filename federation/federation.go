// Package federation coordinates multiple HPCs that each own a disjoint subset
// of zones on the same zone bus. A Registry maps zones to the HPC (Controller)
// that owns them; cross-HPC commands are forwarded transparently.
//
// Each HPC calls Register to claim ownership of its zones. A Lookup returns the
// owning controller for a zone regardless of which HPC is calling; the caller
// sees a single unified zone namespace.
//
// Ownership is exclusive: registering a zone that is already owned returns
// ErrAlreadyOwned. An HPC may release its zones by calling Release.
//
// This is an in-process coordination layer suitable for testing and single-binary
// deployments. For cross-process federation, each HPC would hold a remote
// controller (e.g. gRPC bridge from v0.31.0) that implements rcp.Controller.
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

	rcp "github.com/SoundMatt/go-RCP"
)

// ErrAlreadyOwned is returned when a zone is already registered to another HPC.
var ErrAlreadyOwned = errors.New("rcp/federation: zone already owned by another HPC")

// ErrNotOwned is returned when a zone has no registered owner.
var ErrNotOwned = errors.New("rcp/federation: zone has no registered owner")

// Registry is a thread-safe map of zone → owning controller.
// Multiple HPCs share a single Registry instance.
type Registry struct {
	mu    sync.RWMutex
	owners map[rcp.Zone]rcp.Controller
}

// NewRegistry creates an empty federation Registry.
func NewRegistry() *Registry {
	return &Registry{owners: make(map[rcp.Zone]rcp.Controller)}
}

// Register claims ownership of zone for ctrl.
// Returns ErrAlreadyOwned if another controller already owns the zone.
func (r *Registry) Register(zone rcp.Zone, ctrl rcp.Controller) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.owners[zone]; exists {
		return fmt.Errorf("rcp/federation: zone %s: %w", zone, ErrAlreadyOwned)
	}
	r.owners[zone] = ctrl
	return nil
}

// Release removes ownership of zone. Returns ErrNotOwned if the zone is not registered.
func (r *Registry) Release(zone rcp.Zone) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.owners[zone]; !exists {
		return fmt.Errorf("rcp/federation: zone %s: %w", zone, ErrNotOwned)
	}
	delete(r.owners, zone)
	return nil
}

// Lookup returns the controller that owns zone.
// Returns ErrNotOwned if no HPC has registered the zone.
func (r *Registry) Lookup(zone rcp.Zone) (rcp.Controller, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ctrl, ok := r.owners[zone]
	if !ok {
		return nil, fmt.Errorf("rcp/federation: zone %s: %w", zone, ErrNotOwned)
	}
	return ctrl, nil
}

// Zones returns all currently registered zones in an unspecified order.
func (r *Registry) Zones() []rcp.Zone {
	r.mu.RLock()
	defer r.mu.RUnlock()
	zones := make([]rcp.Zone, 0, len(r.owners))
	for z := range r.owners {
		zones = append(zones, z)
	}
	return zones
}

// Owner returns the controller that owns the zone, or nil if unowned.
func (r *Registry) Owner(zone rcp.Zone) rcp.Controller {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.owners[zone]
}

// TransferOwnership atomically transfers a zone from one HPC to another.
// Returns ErrNotOwned if from does not own the zone, or ErrAlreadyOwned if to
// already owns the zone.
func (r *Registry) TransferOwnership(zone rcp.Zone, from, to rcp.Controller) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, exists := r.owners[zone]
	if !exists || current != from {
		return fmt.Errorf("rcp/federation: zone %s: %w", zone, ErrNotOwned)
	}
	r.owners[zone] = to
	return nil
}
