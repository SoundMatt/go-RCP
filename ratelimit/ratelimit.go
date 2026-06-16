// Package ratelimit provides per-zone token-bucket admission control for
// command flooding protection (SG-009, H-009).
//
// A Controller wraps any rcp.Controller and enforces a sustained rate limit
// and burst capacity. PriorityCritical commands bypass the bucket by default so
// safety-critical traffic (watchdog kicks, emergency actuations) is never
// throttled. All other commands consume one token; when the bucket is exhausted
// Send returns rcp.ErrBusy immediately without blocking.
package ratelimit

//fusa:req REQ-RL-001
//fusa:req REQ-RL-002
//fusa:req REQ-RL-003
//fusa:req REQ-RL-004
//fusa:req REQ-RL-005
//fusa:req REQ-RL-006
//fusa:req REQ-RL-007
//fusa:req REQ-RL-008

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
)

// Config holds token-bucket parameters for a Controller.
type Config struct {
	Rate           float64 // sustained token refill rate in tokens per second
	Burst          int     // maximum token accumulation (bucket capacity)
	ExemptCritical bool    // if true, PriorityCritical commands bypass the bucket
}

// DefaultConfig returns ASIL-B recommended values: 100 cmd/s sustained,
// burst of 20, with PriorityCritical exempt.
func DefaultConfig() Config {
	return Config{
		Rate:           100,
		Burst:          20,
		ExemptCritical: true,
	}
}

// Controller wraps any rcp.Controller and applies token-bucket rate limiting.
// The bucket starts full. Send returns rcp.ErrBusy immediately when exhausted.
type Controller struct {
	inner  rcp.Controller
	cfg    Config
	now    func() time.Time // injectable for testing; defaults to time.Now

	mu     sync.Mutex
	tokens float64
	last   time.Time

	closed atomic.Bool
}

// NewController wraps inner with the supplied Config.
func NewController(inner rcp.Controller, cfg Config) *Controller {
	return NewControllerWithClock(inner, cfg, time.Now)
}

// NewControllerWithClock is like NewController but accepts a custom clock
// function, used in tests to avoid real-time sleeps.
func NewControllerWithClock(inner rcp.Controller, cfg Config, now func() time.Time) *Controller {
	t := now()
	return &Controller{
		inner:  inner,
		cfg:    cfg,
		now:    now,
		tokens: float64(cfg.Burst),
		last:   t,
	}
}

// Send dispatches cmd through the rate limiter. Returns rcp.ErrBusy immediately
// if the token bucket is exhausted. Returns rcp.ErrClosed if the Controller is
// closed. PriorityCritical commands bypass the bucket when ExemptCritical is true.
func (c *Controller) Send(ctx context.Context, cmd *rcp.Command) (*rcp.Response, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("rcp/ratelimit: zone %s: %w", c.inner.Zone(), rcp.ErrClosed)
	}
	exempt := c.cfg.ExemptCritical && cmd.Priority == rcp.PriorityCritical
	if !exempt {
		if !c.take() {
			return nil, fmt.Errorf("rcp/ratelimit: zone %s: %w", c.inner.Zone(), rcp.ErrBusy)
		}
	}
	return c.inner.Send(ctx, cmd)
}

// take atomically refills the bucket and consumes one token.
// Returns true if a token was available, false if the bucket was exhausted.
func (c *Controller) take() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	elapsed := now.Sub(c.last).Seconds()
	c.last = now
	c.tokens += elapsed * c.cfg.Rate
	if c.tokens > float64(c.cfg.Burst) {
		c.tokens = float64(c.cfg.Burst)
	}
	if c.tokens < 1 {
		return false
	}
	c.tokens--
	return true
}

// Zone delegates to the inner controller.
func (c *Controller) Zone() rcp.Zone { return c.inner.Zone() }

// Subscribe delegates to the inner controller.
func (c *Controller) Subscribe(ctx context.Context) (<-chan *rcp.Status, error) {
	return c.inner.Subscribe(ctx)
}

// Close stops the controller and closes the inner controller.
// Safe to call multiple times.
func (c *Controller) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	return c.inner.Close()
}
