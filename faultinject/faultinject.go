// Package faultinject provides structured fault injection for validating the
// safety mechanisms introduced in v0.11.0–v0.16.0.
//
// A faultinject.Controller wraps any rcp.Controller and intercepts Send calls
// according to an ordered list of Rules. Rules may drop responses, add latency,
// return errors, or return timeouts without touching the inner controller.
// Count-based rules auto-expire after a configured number of applications.
//
// Composable with sim.Controller (v0.17.0) for full SiL/HIL regression scenarios.
package faultinject

//fusa:req REQ-FI-001
//fusa:req REQ-FI-002
//fusa:req REQ-FI-003
//fusa:req REQ-FI-004
//fusa:req REQ-FI-005
//fusa:req REQ-FI-006
//fusa:req REQ-FI-007
//fusa:req REQ-FI-008

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
)

// FaultType describes the failure mode to inject.
type FaultType uint8

const (
	// FaultDrop returns an error from Send without forwarding to the inner controller.
	FaultDrop FaultType = iota + 1
	// FaultSlow sleeps Rule.Latency before forwarding the command to the inner controller.
	FaultSlow
	// FaultError returns a Response with StatusError without forwarding to the inner controller.
	FaultError
	// FaultTimeout returns rcp.ErrTimeout without forwarding to the inner controller.
	FaultTimeout
)

// Rule describes a single fault injection rule.
// Count controls how many times the rule fires: -1 means indefinitely, >0 means
// exactly that many times then the rule is automatically cleared.
type Rule struct {
	Type    FaultType
	Latency time.Duration // used by FaultSlow
	Count   int           // -1 = forever; > 0 = fires Count times then auto-removed
	fired   int
}

// Controller wraps any rcp.Controller and intercepts Send calls.
// Rules are applied in order; the first matching (unexpired) rule wins.
type Controller struct {
	inner  rcp.Controller
	mu     sync.Mutex
	rules  []*Rule
	closed atomic.Bool
}

// NewController wraps inner with fault injection support.
// Call AddRule to install faults before sending commands.
func NewController(inner rcp.Controller) *Controller {
	return &Controller{inner: inner}
}

// AddRule appends a fault rule. Rules are evaluated in insertion order; the
// first active rule wins. Thread-safe.
func (c *Controller) AddRule(r Rule) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rules = append(c.rules, &r)
}

// ClearRules removes all active fault rules. Subsequent Sends go straight to
// the inner controller. Thread-safe.
func (c *Controller) ClearRules() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rules = nil
}

// Send applies the first active Rule to cmd and either handles it locally
// (for FaultDrop, FaultError, FaultTimeout) or forwards after a delay
// (FaultSlow). With no active rules Send delegates directly to inner.
func (c *Controller) Send(ctx context.Context, cmd *rcp.Command) (*rcp.Response, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("rcp/faultinject: zone %s: %w", c.inner.Zone(), rcp.ErrClosed)
	}
	rule := c.pickRule()
	if rule == nil {
		return c.inner.Send(ctx, cmd)
	}
	switch rule.Type {
	case FaultDrop:
		return nil, fmt.Errorf("rcp/faultinject: zone %s: injected drop fault", c.inner.Zone())
	case FaultSlow:
		if rule.Latency > 0 {
			select {
			case <-time.After(rule.Latency):
			case <-ctx.Done():
				return nil, fmt.Errorf("rcp/faultinject: zone %s: %w", c.inner.Zone(), rcp.ErrTimeout)
			}
		}
		return c.inner.Send(ctx, cmd)
	case FaultError:
		return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusError}, nil
	case FaultTimeout:
		return nil, fmt.Errorf("rcp/faultinject: zone %s: %w", c.inner.Zone(), rcp.ErrTimeout)
	}
	return c.inner.Send(ctx, cmd)
}

// pickRule returns the first active rule and updates its fire count.
// Returns nil if no rules are active.
func (c *Controller) pickRule() *Rule {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, r := range c.rules {
		if r.Count > 0 && r.fired >= r.Count {
			continue // exhausted
		}
		r.fired++
		if r.Count > 0 && r.fired >= r.Count {
			// auto-remove exhausted rule
			c.rules = append(c.rules[:i], c.rules[i+1:]...)
		}
		return r
	}
	return nil
}

// Zone delegates to the inner controller.
func (c *Controller) Zone() rcp.Zone { return c.inner.Zone() }

// Subscribe delegates to the inner controller.
func (c *Controller) Subscribe(ctx context.Context) (<-chan *rcp.Status, error) {
	return c.inner.Subscribe(ctx)
}

// Close closes the inner controller. Safe to call multiple times.
func (c *Controller) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	return c.inner.Close()
}
