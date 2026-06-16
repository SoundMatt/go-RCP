// Package deadline monitors the liveness of zone controller Status streams.
//
// A Monitor subscribes to each registered zone controller and resets a per-zone
// deadline timer on every incoming Status frame. If no Status arrives within the
// deadline, the zone transitions from Alive to Dead and a LivenessEvent is
// emitted. Recovery to Alive is reported as soon as the next Status arrives.
//
// NewMonitor blocks until all watch goroutines have called Subscribe, so callers
// can safely Publish immediately after construction.
package deadline

//fusa:req REQ-DL-001
//fusa:req REQ-DL-002
//fusa:req REQ-DL-003
//fusa:req REQ-DL-004
//fusa:req REQ-DL-005
//fusa:req REQ-DL-006
//fusa:req REQ-DL-007
//fusa:req REQ-DL-008

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
)

// LivenessEvent is emitted when a zone's liveness state changes.
type LivenessEvent struct {
	Zone  rcp.Zone
	Alive bool
	Err   error // non-nil when the subscription itself failed
}

// Config configures Monitor behaviour.
type Config struct {
	// Deadline is the maximum time between consecutive Status frames before
	// a zone is declared dead. Default: 50 ms (20 Hz status cadence).
	Deadline time.Duration
}

// DefaultConfig returns the recommended ASIL-B deadline configuration (50 ms).
func DefaultConfig() Config {
	return Config{Deadline: 50 * time.Millisecond}
}

type subscriber struct {
	ch   chan LivenessEvent
	once sync.Once
}

func (s *subscriber) close() { s.once.Do(func() { close(s.ch) }) }

// Monitor watches Status streams from multiple zone controllers and reports
// liveness changes when Status frames stop arriving within the configured deadline.
type Monitor struct {
	cfg    Config
	cancel context.CancelFunc
	done   chan struct{}
	closed atomic.Bool

	mu     sync.Mutex
	states map[rcp.Zone]bool // true = alive
	subs   []*subscriber
}

// NewMonitor creates and starts a Monitor for the given controllers.
// It blocks until every watch goroutine has called Subscribe (or failed), so
// callers can safely Publish status immediately after NewMonitor returns.
func NewMonitor(cfg Config, ctrls []rcp.Controller) *Monitor {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Monitor{
		cfg:    cfg,
		cancel: cancel,
		done:   make(chan struct{}),
		states: make(map[rcp.Zone]bool, len(ctrls)),
	}
	var wg sync.WaitGroup
	wg.Add(len(ctrls))
	for _, c := range ctrls {
		m.states[c.Zone()] = false
		go m.watch(ctx, c, &wg)
	}
	wg.Wait() // block until all Subscribe calls have returned
	return m
}

// Alive reports whether zone has received a Status frame within the deadline.
// Returns false for unregistered zones.
func (m *Monitor) Alive(zone rcp.Zone) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.states[zone]
}

// Subscribe returns a channel of LivenessEvents until ctx is cancelled.
// The channel is buffered with capacity 16. Returns an error if Monitor is closed.
func (m *Monitor) Subscribe(ctx context.Context) (<-chan LivenessEvent, error) {
	if m.closed.Load() {
		return nil, fmt.Errorf("rcp/deadline: monitor closed")
	}
	sub := &subscriber{ch: make(chan LivenessEvent, 16)}
	m.mu.Lock()
	m.subs = append(m.subs, sub)
	m.mu.Unlock()

	go func() {
		select {
		case <-ctx.Done():
		case <-m.done:
		}
		m.removeSub(sub)
		sub.close()
	}()
	return sub.ch, nil
}

func (m *Monitor) removeSub(sub *subscriber) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, s := range m.subs {
		if s == sub {
			m.subs = append(m.subs[:i], m.subs[i+1:]...)
			return
		}
	}
}

// Close stops all monitoring goroutines and closes subscriber channels.
// Safe to call multiple times.
func (m *Monitor) Close() error {
	if !m.closed.CompareAndSwap(false, true) {
		return nil
	}
	m.cancel()
	close(m.done)
	return nil
}

func (m *Monitor) emit(ev LivenessEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.states[ev.Zone] = ev.Alive
	for _, sub := range m.subs {
		select {
		case sub.ch <- ev:
		default:
		}
	}
}

func (m *Monitor) watch(ctx context.Context, ctrl rcp.Controller, ready *sync.WaitGroup) {
	zone := ctrl.Zone()
	ch, err := ctrl.Subscribe(ctx)
	ready.Done() // always signal, even on error — unblocks NewMonitor

	if err != nil {
		// Subscribe failed: zone stays not-alive.
		m.emit(LivenessEvent{Zone: zone, Alive: false, Err: err})
		return
	}

	alive := false
	timer := time.NewTimer(m.cfg.Deadline)
	defer timer.Stop()

	for {
		select {
		case _, ok := <-ch:
			if !ok {
				// Controller closed its status channel.
				if alive {
					m.emit(LivenessEvent{Zone: zone, Alive: false})
				}
				return
			}
			// Status received — reset deadline timer.
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(m.cfg.Deadline)
			if !alive {
				alive = true
				m.emit(LivenessEvent{Zone: zone, Alive: true})
			}
		case <-timer.C:
			// Deadline exceeded — zone is dead.
			if alive {
				alive = false
				m.emit(LivenessEvent{Zone: zone, Alive: false})
			}
			// Keep the timer running to detect recovery.
			timer.Reset(m.cfg.Deadline)
		case <-ctx.Done():
			return
		}
	}
}
