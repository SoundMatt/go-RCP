// Package zonegroup provides atomic multi-zone command broadcast for automotive
// zonal architecture. A Group holds a typed set of zone controllers and dispatches
// a Command to all members concurrently, collecting all responses.
//
// Broadcast succeeds only if every member returns StatusOK; partial failures are
// reported per-zone in BroadcastResult so the caller can identify which zones
// are degraded. The atomic broadcast contract means either all zones receive the
// command within the same context deadline, or the operation is reported failed.
//
// Composable with prioqueue, ratelimit, authz, and faultinject controllers.
package zonegroup

//fusa:req REQ-ZG-001
//fusa:req REQ-ZG-002
//fusa:req REQ-ZG-003
//fusa:req REQ-ZG-004
//fusa:req REQ-ZG-005
//fusa:req REQ-ZG-006
//fusa:req REQ-ZG-007
//fusa:req REQ-ZG-008

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	rcp "github.com/SoundMatt/go-RCP"
)

// ZoneResult holds the outcome of a single-zone Send within a Broadcast.
type ZoneResult struct {
	Zone rcp.Zone
	Resp *rcp.Response
	Err  error
}

// BroadcastResult is the aggregate outcome of a Group.Broadcast call.
type BroadcastResult struct {
	Results []ZoneResult
}

// OK returns true if all zones responded with StatusOK and no errors.
func (r BroadcastResult) OK() bool {
	for _, zr := range r.Results {
		if zr.Err != nil || zr.Resp == nil || zr.Resp.Status != rcp.StatusOK {
			return false
		}
	}
	return true
}

// Errors returns per-zone errors for any failed zones.
func (r BroadcastResult) Errors() []error {
	var errs []error
	for _, zr := range r.Results {
		if zr.Err != nil {
			errs = append(errs, zr.Err)
		} else if zr.Resp != nil && zr.Resp.Status != rcp.StatusOK {
			errs = append(errs, fmt.Errorf("rcp/zonegroup: zone %s status %v", zr.Zone, zr.Resp.Status))
		}
	}
	return errs
}

// Group holds a fixed set of zone controllers and broadcasts commands to all
// of them concurrently.
type Group struct {
	members []rcp.Controller
	closed  atomic.Bool
}

// NewGroup creates a Group from the supplied controllers. The slice must be
// non-empty; all members must be non-nil.
func NewGroup(members []rcp.Controller) (*Group, error) {
	if len(members) == 0 {
		return nil, fmt.Errorf("rcp/zonegroup: group must have at least one member")
	}
	for i, m := range members {
		if m == nil {
			return nil, fmt.Errorf("rcp/zonegroup: member %d is nil", i)
		}
	}
	cp := make([]rcp.Controller, len(members))
	copy(cp, members)
	return &Group{members: cp}, nil
}

// Broadcast sends cmd to every member concurrently and waits for all responses.
// The same Command is sent to every zone; cmd.Zone is overridden per member.
// Returns ErrClosed if the Group has been closed.
func (g *Group) Broadcast(ctx context.Context, cmd *rcp.Command) (BroadcastResult, error) {
	if g.closed.Load() {
		return BroadcastResult{}, fmt.Errorf("rcp/zonegroup: %w", rcp.ErrClosed)
	}

	results := make([]ZoneResult, len(g.members))
	var wg sync.WaitGroup
	wg.Add(len(g.members))

	for i, m := range g.members {
		go func() {
			defer wg.Done()
			c := *cmd // copy so Zone override doesn't race
			c.Zone = m.Zone()
			resp, err := m.Send(ctx, &c)
			results[i] = ZoneResult{Zone: m.Zone(), Resp: resp, Err: err}
		}()
	}
	wg.Wait()
	return BroadcastResult{Results: results}, nil
}

// Zones returns the zone identifiers for all members.
func (g *Group) Zones() []rcp.Zone {
	zones := make([]rcp.Zone, len(g.members))
	for i, m := range g.members {
		zones[i] = m.Zone()
	}
	return zones
}

// Len returns the number of members.
func (g *Group) Len() int { return len(g.members) }

// Close closes all member controllers. Each controller is closed regardless of
// errors in previous members; all errors are combined. Safe to call multiple times.
func (g *Group) Close() error {
	if !g.closed.CompareAndSwap(false, true) {
		return nil
	}
	var errs []error
	for _, m := range g.members {
		if err := m.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("rcp/zonegroup: close errors: %v", errs)
}
