// Package redundancy provides a hot-standby Controller pair for ASIL-B fault
// tolerance. A primary controller handles all Sends; if the primary fails
// (returns a non-nil error), the standby automatically takes over and becomes
// the new primary. Status events are subscribed from whichever controller is
// currently active.
//
// The failover is transparent to the caller: a single rcp.Controller interface
// is presented, and callers see at most one error (the primary failure) before
// the standby takes over. Subsequent Sends go to the (formerly) standby.
//
// ASIL-B rationale: ISO 26262 Part 9 requires hardware/software redundancy for
// ASIL-B functions that cannot be made safe-state on single failure. A hot
// standby provides single-fault tolerance without requiring a safe-state
// transition.
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

	rcp "github.com/SoundMatt/go-RCP"
)

// FailoverPolicy decides whether a given error from the active controller
// should trigger a failover to the standby.
// Return true to failover, false to return the error as-is.
// A nil FailoverPolicy triggers on any non-nil error.
type FailoverPolicy func(err error) bool

// Controller is a hot-standby pair.  At any point one member is "active";
// on a qualifying error the other becomes active.
type Controller struct {
	mu       sync.Mutex
	primary  rcp.Controller
	standby  rcp.Controller
	policy   FailoverPolicy
	failovers atomic.Int32
	closed   atomic.Bool
}

// NewController creates a redundant pair.
// primary is tried first; standby becomes active on failover.
// policy may be nil (fail over on any error).
func NewController(primary, standby rcp.Controller, policy FailoverPolicy) *Controller {
	return &Controller{primary: primary, standby: standby, policy: policy}
}

// Send dispatches to the active (primary) controller. On a qualifying error the
// standby becomes active and the error is returned to the caller. The next Send
// will use the new active controller.
func (c *Controller) Send(ctx context.Context, cmd *rcp.Command) (*rcp.Response, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("rcp/redundancy: zone %s: %w", c.zone(), rcp.ErrClosed)
	}
	c.mu.Lock()
	active := c.primary
	c.mu.Unlock()

	resp, err := active.Send(ctx, cmd)
	if err == nil {
		return resp, nil
	}
	if c.policy != nil && !c.policy(err) {
		return nil, err
	}

	// Failover: swap primary and standby under the lock.
	c.mu.Lock()
	if c.primary == active { // only swap once per failure
		c.primary, c.standby = c.standby, c.primary
		c.failovers.Add(1)
	}
	c.mu.Unlock()

	return nil, err
}

// Failovers returns the number of times the active controller has been swapped.
func (c *Controller) Failovers() int { return int(c.failovers.Load()) }

// Active returns the currently active controller (primarily for testing).
func (c *Controller) Active() rcp.Controller {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.primary
}

// Zone returns the zone of the primary controller (both controllers must manage the same zone).
func (c *Controller) Zone() rcp.Zone { return c.zone() }

func (c *Controller) zone() rcp.Zone {
	c.mu.Lock()
	z := c.primary.Zone()
	c.mu.Unlock()
	return z
}

// Subscribe returns a status channel from the currently active controller.
func (c *Controller) Subscribe(ctx context.Context) (<-chan *rcp.Status, error) {
	c.mu.Lock()
	active := c.primary
	c.mu.Unlock()
	return active.Subscribe(ctx)
}

// Close closes both the primary and standby controllers. Safe to call multiple times.
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
