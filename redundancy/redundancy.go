// Package redundancy provides a hot-standby *udp.Controller pair for
// ASIL-B fault tolerance, for the OPEN Alliance TC18 Remote Control
// Protocol (RCP), as described by the "OPEN Alliance TC18 Remote Control
// Protocol Specification v0.5.1_RC". A primary controller handles all
// Requests; if the primary fails (returns a non-nil error), the standby
// automatically takes over and becomes the new primary. The failover is
// transparent to the caller: a single Request/Read/Write surface is
// presented, and callers see at most one error (the primary failure)
// before the standby takes over. Subsequent Requests go to the (formerly)
// standby.
//
// This is ROADMAP.md Milestone 55 (v0.68.0)'s ADAPT-flagged rebuild: per
// Phase 17's disposition table, "hot-standby failover between two
// controllers is a pattern independent of what 'controller' means
// underneath," so the failover algorithm and its atomic primary/standby
// swap are unchanged; only the wrapped type changes, from the retired
// rcp.Controller to *udp.Controller, following the same "wrap the concrete
// transport type directly, since the caller/inner interface contract is a
// Phase 18 root-module concern" precedent Milestone 54's loan package
// already established for wrapping *udp.Controller.
//
// ASIL-B rationale: ISO 26262 Part 9 requires hardware/software redundancy
// for ASIL-B functions that cannot be made safe-state on single failure. A
// hot standby provides single-fault tolerance without requiring a
// safe-state transition.
package redundancy

//fusa:req REQ-RD-001
//fusa:req REQ-RD-002
//fusa:req REQ-RD-003
//fusa:req REQ-RD-004
//fusa:req REQ-RD-005
//fusa:req REQ-RD-006
//fusa:req REQ-RD-007
//fusa:req REQ-RD-008

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/udp"
)

// FailoverPolicy decides whether a given error from the active controller
// should trigger a failover to the standby.
// Return true to failover, false to return the error as-is.
// A nil FailoverPolicy triggers on any non-nil error.
type FailoverPolicy func(err error) bool

// Controller is a hot-standby pair. At any point one member is "active";
// on a qualifying error the other becomes active.
type Controller struct {
	mu        sync.Mutex
	primary   *udp.Controller
	standby   *udp.Controller
	policy    FailoverPolicy
	failovers atomic.Int32
	closed    atomic.Bool
}

// NewController creates a redundant pair.
// primary is tried first; standby becomes active on failover.
// policy may be nil (fail over on any error).
func NewController(primary, standby *udp.Controller, policy FailoverPolicy) *Controller {
	return &Controller{primary: primary, standby: standby, policy: policy}
}

// Request dispatches to the active (primary) controller. On a qualifying
// error the standby becomes active and the error is returned to the
// caller. The next Request will use the new active controller.
func (c *Controller) Request(ctx context.Context, addr avtp.ByteBusID, control acf.ControlFlags, body []byte) (acf.Message, error) {
	if c.closed.Load() {
		return acf.Message{}, fmt.Errorf("rcp/redundancy: stream %s: %w", c.StreamID(), udp.ErrClosed)
	}
	c.mu.Lock()
	active := c.primary
	c.mu.Unlock()

	resp, err := active.Request(ctx, addr, control, body)
	if err == nil {
		return resp, nil
	}
	if c.policy != nil && !c.policy(err) {
		return acf.Message{}, err
	}

	// Failover: swap primary and standby under the lock.
	c.mu.Lock()
	if c.primary == active { // only swap once per failure
		c.primary, c.standby = c.standby, c.primary
		c.failovers.Add(1)
	}
	c.mu.Unlock()

	return acf.Message{}, err
}

// Read is Request with acf.FlagRead set and no body.
func (c *Controller) Read(ctx context.Context, addr avtp.ByteBusID) (acf.Message, error) {
	return c.Request(ctx, addr, acf.FlagRead, nil)
}

// Write is Request with acf.FlagWrite set and the given body.
func (c *Controller) Write(ctx context.Context, addr avtp.ByteBusID, body []byte) (acf.Message, error) {
	return c.Request(ctx, addr, acf.FlagWrite, body)
}

// Failovers returns the number of times the active controller has been
// swapped.
func (c *Controller) Failovers() int { return int(c.failovers.Load()) }

// Active returns the currently active controller (primarily for testing).
func (c *Controller) Active() *udp.Controller {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.primary
}

// StreamID returns the currently active controller's own avtp.StreamID
// identity.
func (c *Controller) StreamID() avtp.StreamID {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.primary.StreamID()
}

// Close closes both the primary and standby controllers. Safe to call
// multiple times.
func (c *Controller) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	c.mu.Lock()
	p, s := c.primary, c.standby
	c.mu.Unlock()

	var errs []error
	if err := p.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := s.Close(); err != nil {
		errs = append(errs, err)
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("rcp/redundancy: close: %v", errs)
}
