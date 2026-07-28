// Package zonegroup provides atomic multi-target request broadcast for the
// OPEN Alliance TC18 Remote Control Protocol (RCP), as described by the
// "OPEN Alliance TC18 Remote Control Protocol Specification v0.5.1_RC". A
// Group holds a set of *udp.Controller members and dispatches the same
// request (a single avtp.ByteBusID address, control flags, and body) to
// every member concurrently, collecting all responses.
//
// Broadcast succeeds only if every member returns a response without
// FlagError set and no transport error; partial failures are reported
// per-member in BroadcastResult so the caller can identify which servers
// are degraded. The atomic broadcast contract means either every member is
// reached within the same context deadline, or the operation is reported
// failed.
//
// This is ROADMAP.md Milestone 55 (v0.68.0)'s ADAPT-flagged rebuild: per
// Phase 17's disposition table, "atomic multi-target broadcast-and-collect
// is reusable; re-target it at endpoint groups." A "member" here is a whole
// *udp.Controller — one destination RC Server, mirroring the retired
// package's own "one member per zone" shape — and Broadcast addresses one
// avtp.ByteBusID across every member, in place of the retired rcp.Command's
// per-member Zone override. Note the specification itself already lets one
// AVTPDU carry several independently-addressed requests, so this package's
// role narrows to client-side ergonomics on top of that mechanism, not the
// only way to reach several endpoints at once.
//
// Composable with authz, ratelimit, and proxy wrappers on either a member
// controller or the Group's own callers.
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

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/udp"
)

// MemberResult holds the outcome of a single-member Request within a
// Broadcast.
type MemberResult struct {
	Stream avtp.StreamID
	Resp   acf.Message
	Err    error
}

// BroadcastResult is the aggregate outcome of a Group.Broadcast call.
type BroadcastResult struct {
	Results []MemberResult
}

// OK returns true if every member responded without a transport error and
// without acf.FlagError set.
func (r BroadcastResult) OK() bool {
	for _, mr := range r.Results {
		if mr.Err != nil || mr.Resp.Control.Has(acf.FlagError) {
			return false
		}
	}
	return true
}

// Errors returns per-member errors for any failed members.
func (r BroadcastResult) Errors() []error {
	var errs []error
	for _, mr := range r.Results {
		if mr.Err != nil {
			errs = append(errs, mr.Err)
		} else if mr.Resp.Control.Has(acf.FlagError) {
			errs = append(errs, fmt.Errorf("rcp/zonegroup: stream %s: %s", mr.Stream, mr.Resp.Body))
		}
	}
	return errs
}

// Group holds a fixed set of member controllers and broadcasts requests to
// all of them concurrently.
type Group struct {
	members []*udp.Controller
	closed  atomic.Bool
}

// NewGroup creates a Group from the supplied controllers. The slice must be
// non-empty; all members must be non-nil.
func NewGroup(members []*udp.Controller) (*Group, error) {
	if len(members) == 0 {
		return nil, fmt.Errorf("rcp/zonegroup: group must have at least one member")
	}
	for i, m := range members {
		if m == nil {
			return nil, fmt.Errorf("rcp/zonegroup: member %d is nil", i)
		}
	}
	cp := make([]*udp.Controller, len(members))
	copy(cp, members)
	return &Group{members: cp}, nil
}

// Broadcast sends the same request (addr, control, body) to every member
// concurrently and waits for all responses. Returns udp.ErrClosed if the
// Group has been closed.
func (g *Group) Broadcast(ctx context.Context, addr avtp.ByteBusID, control acf.ControlFlags, body []byte) (BroadcastResult, error) {
	if g.closed.Load() {
		return BroadcastResult{}, fmt.Errorf("rcp/zonegroup: %w", udp.ErrClosed)
	}

	results := make([]MemberResult, len(g.members))
	var wg sync.WaitGroup
	wg.Add(len(g.members))

	for i, m := range g.members {
		go func(i int, m *udp.Controller) {
			defer wg.Done()
			resp, err := m.Request(ctx, addr, control, body)
			results[i] = MemberResult{Stream: m.StreamID(), Resp: resp, Err: err}
		}(i, m)
	}
	wg.Wait()
	return BroadcastResult{Results: results}, nil
}

// Read is Broadcast with acf.FlagRead set and no body.
func (g *Group) Read(ctx context.Context, addr avtp.ByteBusID) (BroadcastResult, error) {
	return g.Broadcast(ctx, addr, acf.FlagRead, nil)
}

// Write is Broadcast with acf.FlagWrite set and the given body.
func (g *Group) Write(ctx context.Context, addr avtp.ByteBusID, body []byte) (BroadcastResult, error) {
	return g.Broadcast(ctx, addr, acf.FlagWrite, body)
}

// StreamIDs returns the avtp.StreamID identity of each member in insertion
// order.
func (g *Group) StreamIDs() []avtp.StreamID {
	ids := make([]avtp.StreamID, len(g.members))
	for i, m := range g.members {
		ids[i] = m.StreamID()
	}
	return ids
}

// Len returns the number of members.
func (g *Group) Len() int { return len(g.members) }

// Close closes all member controllers. Each controller is closed
// regardless of errors in previous members; all errors are combined. Safe
// to call multiple times.
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
