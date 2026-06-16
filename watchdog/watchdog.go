// Package watchdog implements the ASIL-B watchdog & heartbeat mechanism for go-RCP.
//
// A Keeper periodically sends CmdWatchdog to each registered zone controller.
// Consecutive failures transition a zone through a health state machine:
// Healthy → Degraded → Faulted.
//
// This directly addresses SG-001 (command delivery failure detection),
// SG-003 (CmdWatchdog delivery guaranteed), and SG-007 (safe state on cessation).
package watchdog

//fusa:req REQ-WDG-001
//fusa:req REQ-WDG-002
//fusa:req REQ-WDG-003
//fusa:req REQ-WDG-004
//fusa:req REQ-WDG-005
//fusa:req REQ-WDG-006
//fusa:req REQ-WDG-007
//fusa:req REQ-WDG-008

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
)

// HealthState is the liveness state of a zone controller.
type HealthState uint8

const (
	// HealthStateHealthy means all recent watchdog kicks succeeded.
	HealthStateHealthy HealthState = 0
	// HealthStateDegraded means some consecutive kicks failed; zone may recover.
	HealthStateDegraded HealthState = 1
	// HealthStateFaulted means enough consecutive kicks failed to trigger a fault.
	HealthStateFaulted HealthState = 2
)

func (h HealthState) String() string {
	switch h {
	case HealthStateHealthy:
		return "healthy"
	case HealthStateDegraded:
		return "degraded"
	case HealthStateFaulted:
		return "faulted"
	default:
		return "unknown"
	}
}

// HealthEvent is emitted when a zone's health state changes.
type HealthEvent struct {
	Zone  rcp.Zone
	State HealthState
	Err   error // last watchdog error, nil when recovering
}

// Config configures the Keeper's timing and thresholds.
type Config struct {
	// Interval is the watchdog kick period. Defaults to 10 ms (100 Hz).
	Interval time.Duration
	// Timeout is the per-kick response deadline. Defaults to 5 ms.
	Timeout time.Duration
	// DegradeAfter is the number of consecutive kick failures before
	// transitioning from Healthy to Degraded. Default: 3.
	DegradeAfter int
	// FaultAfter is the number of consecutive kick failures before
	// transitioning from Degraded to Faulted. Default: 5.
	FaultAfter int
}

// DefaultConfig returns the recommended ASIL-B watchdog configuration.
// Interval=10ms, Timeout=5ms, DegradeAfter=3, FaultAfter=5.
func DefaultConfig() Config {
	return Config{
		Interval:     10 * time.Millisecond,
		Timeout:      5 * time.Millisecond,
		DegradeAfter: 3,
		FaultAfter:   5,
	}
}

// zoneState tracks per-zone watchdog state.
type zoneState struct {
	health  HealthState
	misses  int
	cmdID   atomic.Uint32
}

type subscriber struct {
	ch   chan HealthEvent
	once sync.Once
}

func (s *subscriber) close() { s.once.Do(func() { close(s.ch) }) }

// Keeper runs periodic watchdog kicks across a set of zone controllers.
type Keeper struct {
	cfg    Config
	ctrls  map[rcp.Zone]rcp.Controller
	states map[rcp.Zone]*zoneState
	done   chan struct{}
	closed atomic.Bool

	mu   sync.Mutex
	subs []*subscriber
}

// NewKeeper creates and starts a Keeper for the given controllers.
func NewKeeper(cfg Config, ctrls []rcp.Controller) *Keeper {
	k := &Keeper{
		cfg:    cfg,
		ctrls:  make(map[rcp.Zone]rcp.Controller, len(ctrls)),
		states: make(map[rcp.Zone]*zoneState, len(ctrls)),
		done:   make(chan struct{}),
	}
	for _, c := range ctrls {
		k.ctrls[c.Zone()] = c
		k.states[c.Zone()] = &zoneState{health: HealthStateHealthy}
	}
	go k.run()
	return k
}

// Health returns the current health state for zone.
// Returns HealthStateFaulted if zone is not registered.
func (k *Keeper) Health(zone rcp.Zone) HealthState {
	k.mu.Lock()
	defer k.mu.Unlock()
	st, ok := k.states[zone]
	if !ok {
		return HealthStateFaulted
	}
	return st.health
}

// Subscribe returns a channel of HealthEvents until ctx is cancelled.
// The channel is buffered with capacity 16.
func (k *Keeper) Subscribe(ctx context.Context) (<-chan HealthEvent, error) {
	if k.closed.Load() {
		return nil, fmt.Errorf("rcp/watchdog: keeper closed")
	}
	sub := &subscriber{ch: make(chan HealthEvent, 16)}
	k.mu.Lock()
	k.subs = append(k.subs, sub)
	k.mu.Unlock()

	go func() {
		select {
		case <-ctx.Done():
		case <-k.done:
		}
		k.removeSub(sub)
		sub.close()
	}()
	return sub.ch, nil
}

func (k *Keeper) removeSub(sub *subscriber) {
	k.mu.Lock()
	defer k.mu.Unlock()
	for i, s := range k.subs {
		if s == sub {
			k.subs = append(k.subs[:i], k.subs[i+1:]...)
			return
		}
	}
}

// Close stops all watchdog goroutines and closes subscriber channels.
func (k *Keeper) Close() error {
	if !k.closed.CompareAndSwap(false, true) {
		return nil
	}
	close(k.done)
	return nil
}

func (k *Keeper) run() {
	ticker := time.NewTicker(k.cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-k.done:
			return
		case <-ticker.C:
			k.kickAll()
		}
	}
}

func (k *Keeper) kickAll() {
	for zone, ctrl := range k.ctrls {
		go k.kick(zone, ctrl)
	}
}

func (k *Keeper) kick(zone rcp.Zone, ctrl rcp.Controller) {
	st := k.states[zone]
	id := st.cmdID.Add(1)

	ctx, cancel := context.WithTimeout(context.Background(), k.cfg.Timeout)
	defer cancel()

	_, err := ctrl.Send(ctx, &rcp.Command{
		ID:       id,
		Zone:     zone,
		Type:     rcp.CmdWatchdog,
		Priority: rcp.PriorityHigh,
	})

	k.mu.Lock()
	defer k.mu.Unlock()

	var newHealth HealthState
	if err == nil {
		// Kick succeeded — reset misses, recover to Healthy.
		st.misses = 0
		newHealth = HealthStateHealthy
	} else {
		st.misses++
		switch {
		case st.misses >= k.cfg.FaultAfter:
			newHealth = HealthStateFaulted
		case st.misses >= k.cfg.DegradeAfter:
			newHealth = HealthStateDegraded
		default:
			newHealth = st.health // stay in current state
		}
	}

	if newHealth != st.health {
		st.health = newHealth
		event := HealthEvent{Zone: zone, State: newHealth, Err: err}
		for _, sub := range k.subs {
			select {
			case sub.ch <- event:
			default:
			}
		}
	}
}
