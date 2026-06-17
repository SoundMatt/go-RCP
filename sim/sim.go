// Package sim provides a timing-realistic zone controller simulator for
// Software-in-the-Loop (SiL) and Hardware-in-the-Loop (HIL) testing without
// physical ECUs.
//
// A sim.Controller implements the full rcp.Controller interface and adds
// Fault/Recover controls for deterministic scenario testing. Configurable
// latency (constant or jitter model), periodic Status publishing, and
// watchdog-miss detection enable validation of the safety mechanisms
// introduced in v0.11.0–v0.16.0.
package sim

//fusa:req REQ-SIM-001
//fusa:req REQ-SIM-002
//fusa:req REQ-SIM-003
//fusa:req REQ-SIM-004
//fusa:req REQ-SIM-005
//fusa:req REQ-SIM-006
//fusa:req REQ-SIM-007
//fusa:req REQ-SIM-008
//fusa:req REQ-SIM-009

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
)

// LatencyModel selects how response latency is simulated.
type LatencyModel uint8

const (
	LatencyConstant LatencyModel = iota // fixed BaseLatency on every Send
	LatencyJitter                        // uniform jitter in [0, Jitter] added to BaseLatency
)

// Config holds simulator parameters.
type Config struct {
	Zone            rcp.Zone
	BaseLatency     time.Duration // minimum simulated response latency
	Jitter          time.Duration // upper bound of random extra latency (LatencyJitter only)
	StatusInterval  time.Duration // period at which Status is published to subscribers (0 = disabled)
	WatchdogTimeout time.Duration // if CmdWatchdog not received within this, WatchdogMissed() returns true (0 = disabled)
	LatencyModel    LatencyModel
}

// DefaultConfig returns ASIL-B recommended simulator values for the given zone.
func DefaultConfig(zone rcp.Zone) Config {
	return Config{
		Zone:            zone,
		BaseLatency:     2 * time.Millisecond,
		Jitter:          1 * time.Millisecond,
		StatusInterval:  10 * time.Millisecond,
		WatchdogTimeout: 50 * time.Millisecond,
		LatencyModel:    LatencyJitter,
	}
}

type subscriber struct {
	ch     chan *rcp.Status
	mu     sync.Mutex
	closed bool
}

// close closes the subscriber channel exactly once. Idempotent.
func (s *subscriber) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		close(s.ch)
	}
}

// trySend delivers st without blocking. It is a no-op if the subscriber has
// been closed or its buffer is full. Holding mu makes the send mutually
// exclusive with close, so it can never send on a closed channel.
func (s *subscriber) trySend(st *rcp.Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	select {
	case s.ch <- st:
	default:
	}
}

// Controller is a timing-realistic zone controller simulator.
type Controller struct {
	cfg     Config
	mu      sync.Mutex
	fault   error
	subs    []*subscriber
	done    chan struct{}
	closed  atomic.Bool
	wdMiss  atomic.Bool
	wdReset chan struct{} // non-blocking signal to watchdogLoop to reset timer
	rng     *rand.Rand
}

// NewController creates a new simulator with the given Config.
// If Config.StatusInterval > 0 a background goroutine publishes Status updates.
// If Config.WatchdogTimeout > 0 a background goroutine detects watchdog misses.
func NewController(cfg Config) *Controller {
	c := &Controller{
		cfg:     cfg,
		done:    make(chan struct{}),
		wdReset: make(chan struct{}, 1),
		rng:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	if cfg.StatusInterval > 0 {
		go c.statusLoop()
	}
	if cfg.WatchdogTimeout > 0 {
		go c.watchdogLoop()
	}
	return c
}

func (c *Controller) computeLatency() time.Duration {
	if c.cfg.LatencyModel == LatencyJitter && c.cfg.Jitter > 0 {
		c.mu.Lock()
		jitter := time.Duration(c.rng.Int63n(int64(c.cfg.Jitter) + 1))
		c.mu.Unlock()
		return c.cfg.BaseLatency + jitter
	}
	return c.cfg.BaseLatency
}

// Send simulates command processing with configured latency.
// Returns StatusError if a fault is active; StatusOK otherwise.
// Returns rcp.ErrTimeout if ctx expires during latency, rcp.ErrClosed after Close.
func (c *Controller) Send(ctx context.Context, cmd *rcp.Command) (*rcp.Response, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("rcp/sim: zone %s: %w", c.cfg.Zone, rcp.ErrClosed)
	}
	if lat := c.computeLatency(); lat > 0 {
		select {
		case <-time.After(lat):
		case <-ctx.Done():
			return nil, fmt.Errorf("rcp/sim: zone %s: %w", c.cfg.Zone, rcp.ErrTimeout)
		case <-c.done:
			return nil, fmt.Errorf("rcp/sim: zone %s: %w", c.cfg.Zone, rcp.ErrClosed)
		}
	}
	if cmd.Type == rcp.CmdWatchdog {
		c.wdMiss.Store(false)
		select {
		case c.wdReset <- struct{}{}:
		default:
		}
	}
	c.mu.Lock()
	fault := c.fault
	c.mu.Unlock()
	status := rcp.StatusOK
	if fault != nil {
		status = rcp.StatusError
	}
	return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: status}, nil
}

// Zone returns the simulator's configured zone.
func (c *Controller) Zone() rcp.Zone { return c.cfg.Zone }

// Subscribe returns a channel that receives periodic Status events.
// The channel is closed when ctx is cancelled or Close() is called.
func (c *Controller) Subscribe(ctx context.Context) (<-chan *rcp.Status, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("rcp/sim: zone %s: %w", c.cfg.Zone, rcp.ErrClosed)
	}
	sub := &subscriber{ch: make(chan *rcp.Status, 8)}
	c.mu.Lock()
	c.subs = append(c.subs, sub)
	c.mu.Unlock()
	go func() {
		select {
		case <-ctx.Done():
		case <-c.done:
		}
		sub.close()
	}()
	return sub.ch, nil
}

// Fault injects a fault: subsequent Send calls return StatusError (not StatusOK).
// Pass errors.New("reason") for the fault cause. This is the primary entry-point
// for v0.18.0 fault injection.
func (c *Controller) Fault(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fault = err
}

// Recover clears any active fault and resets the watchdog-missed flag.
func (c *Controller) Recover() {
	c.mu.Lock()
	c.fault = nil
	c.mu.Unlock()
	c.wdMiss.Store(false)
}

// WatchdogMissed returns true if the watchdog deadline has elapsed since the
// last CmdWatchdog was received (or since construction if none ever received).
func (c *Controller) WatchdogMissed() bool { return c.wdMiss.Load() }

// Close shuts down background goroutines and closes all subscriber channels.
// Safe to call multiple times.
func (c *Controller) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	close(c.done)
	c.mu.Lock()
	subs := append([]*subscriber{}, c.subs...)
	c.mu.Unlock()
	for _, s := range subs {
		s.close()
	}
	return nil
}

func (c *Controller) publish() {
	c.mu.Lock()
	subs := append([]*subscriber{}, c.subs...)
	c.mu.Unlock()
	for _, s := range subs {
		s.trySend(&rcp.Status{Zone: c.cfg.Zone})
	}
}

func (c *Controller) statusLoop() {
	ticker := time.NewTicker(c.cfg.StatusInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			c.publish()
		}
	}
}

func (c *Controller) watchdogLoop() {
	timer := time.NewTimer(c.cfg.WatchdogTimeout)
	defer timer.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-c.wdReset:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(c.cfg.WatchdogTimeout)
		case <-timer.C:
			c.wdMiss.Store(true)
			timer.Reset(c.cfg.WatchdogTimeout)
		}
	}
}
