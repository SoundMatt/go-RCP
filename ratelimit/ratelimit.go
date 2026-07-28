// Package ratelimit provides per-endpoint token-bucket admission control for
// request flooding protection (SG-009, H-009), for the OPEN Alliance TC18
// Remote Control Protocol (RCP), as described by the "OPEN Alliance TC18
// Remote Control Protocol Specification v0.5.1_RC".
//
// This is ROADMAP.md Milestone 55 (v0.68.0)'s ADAPT-flagged rebuild: per
// Phase 17's disposition table, "token-bucket admission control is an
// algorithm independent of what's being rate-limited," so the algorithm
// itself is unchanged. Two things do change:
//
//   - Re-keying: the retired package held one bucket per Controller, an
//     implicit stand-in for "one zone." A *udp.Controller addresses many
//     endpoints (avtp.ByteBusID values) on one stream, so this package now
//     tracks one bucket per endpoint, so a flood aimed at one endpoint
//     cannot starve requests to an unrelated endpoint on the same
//     Controller.
//   - Exemption: the retired ExemptCritical check compared cmd.Priority
//     against a client-assigned rcp.PriorityCritical enum value that no
//     longer exists. request.Kind.Priority() (ROADMAP.md Milestone 49)
//     already fixes a cross-type execution-priority ordering by request
//     *kind*, ranking the three cancellation Kinds first — the closest
//     surviving analogue to "safety-critical traffic must never be
//     throttled," since a cancellation exists specifically to retire other
//     pending work and is otherwise immune to the watchdog-driven purge
//     (request.Kind's KindCancelAll/KindCancelTransaction/
//     KindCancelSequencer doc comments). ExemptCancellation replaces
//     ExemptCritical on that basis.
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
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/request"
	"github.com/SoundMatt/go-RCP/udp"
)

// ErrBusy is returned when a request is rejected because its endpoint's
// token bucket is exhausted.
var ErrBusy = errors.New("rcp/ratelimit: token bucket exhausted")

// Config holds token-bucket parameters for a Controller. The same Config
// applies independently to every endpoint's bucket.
type Config struct {
	Rate               float64 // sustained token refill rate in tokens per second
	Burst              int     // maximum token accumulation (bucket capacity)
	ExemptCancellation bool    // if true, cancellation-kind requests bypass the bucket
}

// DefaultConfig returns ASIL-B recommended values: 100 req/s sustained per
// endpoint, burst of 20, with cancellation requests exempt.
func DefaultConfig() Config {
	return Config{
		Rate:               100,
		Burst:              20,
		ExemptCancellation: true,
	}
}

// bucket is one endpoint's token-bucket state.
type bucket struct {
	tokens float64
	last   time.Time
}

// Controller wraps a *udp.Controller and applies token-bucket rate limiting,
// tracked independently per target endpoint. Each bucket starts full.
// Request returns ErrBusy immediately when the addressed endpoint's bucket
// is exhausted.
type Controller struct {
	inner *udp.Controller
	cfg   Config
	now   func() time.Time // injectable for testing; defaults to time.Now

	mu      sync.Mutex
	buckets map[avtp.ByteBusID]*bucket

	closed atomic.Bool
}

// NewController wraps inner with the supplied Config.
func NewController(inner *udp.Controller, cfg Config) *Controller {
	return NewControllerWithClock(inner, cfg, time.Now)
}

// NewControllerWithClock is like NewController but accepts a custom clock
// function, used in tests to avoid real-time sleeps.
func NewControllerWithClock(inner *udp.Controller, cfg Config, now func() time.Time) *Controller {
	return &Controller{
		inner:   inner,
		cfg:     cfg,
		now:     now,
		buckets: make(map[avtp.ByteBusID]*bucket),
	}
}

// StreamID returns the inner controller's own avtp.StreamID identity.
func (c *Controller) StreamID() avtp.StreamID { return c.inner.StreamID() }

// Request dispatches to addr through the rate limiter for that endpoint.
// Returns ErrBusy immediately if addr's token bucket is exhausted. Returns
// udp.ErrClosed if the Controller is closed. When Config.ExemptCancellation
// is true, a cancellation-kind request (kind.IsCancellation()) bypasses the
// bucket entirely.
func (c *Controller) Request(ctx context.Context, addr avtp.ByteBusID, control acf.ControlFlags, body []byte, kind request.Kind) (acf.Message, error) {
	if c.closed.Load() {
		return acf.Message{}, fmt.Errorf("rcp/ratelimit: stream %s: %w", c.StreamID(), udp.ErrClosed)
	}
	exempt := c.cfg.ExemptCancellation && kind.IsCancellation()
	if !exempt {
		if !c.take(addr) {
			return acf.Message{}, fmt.Errorf("rcp/ratelimit: stream %s endpoint %d: %w", c.StreamID(), addr, ErrBusy)
		}
	}
	return c.inner.Request(ctx, addr, control, body)
}

// Read is Request with acf.FlagRead set, no body, and request.KindPlain.
func (c *Controller) Read(ctx context.Context, addr avtp.ByteBusID) (acf.Message, error) {
	return c.Request(ctx, addr, acf.FlagRead, nil, request.KindPlain)
}

// Write is Request with acf.FlagWrite set and request.KindPlain.
func (c *Controller) Write(ctx context.Context, addr avtp.ByteBusID, body []byte) (acf.Message, error) {
	return c.Request(ctx, addr, acf.FlagWrite, body, request.KindPlain)
}

// take atomically refills addr's bucket and consumes one token.
// Returns true if a token was available, false if the bucket was exhausted.
func (c *Controller) take(addr avtp.ByteBusID) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, ok := c.buckets[addr]
	if !ok {
		b = &bucket{tokens: float64(c.cfg.Burst), last: c.now()}
		c.buckets[addr] = b
	}
	now := c.now()
	elapsed := now.Sub(b.last).Seconds()
	b.last = now
	b.tokens += elapsed * c.cfg.Rate
	if b.tokens > float64(c.cfg.Burst) {
		b.tokens = float64(c.cfg.Burst)
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Close closes the inner controller. Safe to call multiple times.
func (c *Controller) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	return c.inner.Close()
}
