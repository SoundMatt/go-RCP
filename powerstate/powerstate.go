// Package powerstate manages zone controller power state transitions.
//
// A Manager sends CmdSleep / CmdWake to zone controllers and tracks the
// resulting power state (Active, Sleeping, BusOff). When a command fails
// the zone transitions to BusOff and the Manager automatically retries CmdWake
// at the configured RecoveryInterval until the zone responds, then transitions
// back to Active.
package powerstate

//fusa:req REQ-PWR-001
//fusa:req REQ-PWR-002
//fusa:req REQ-PWR-003
//fusa:req REQ-PWR-004
//fusa:req REQ-PWR-005
//fusa:req REQ-PWR-006
//fusa:req REQ-PWR-007
//fusa:req REQ-PWR-008

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
)

// PowerState is the power state of a zone controller.
type PowerState uint8

const (
	// PowerStateActive means the zone controller is running and reachable.
	PowerStateActive PowerState = 0
	// PowerStateSleeping means the zone controller has acknowledged CmdSleep.
	PowerStateSleeping PowerState = 1
	// PowerStateBusOff means the last command to the zone failed; recovery is pending.
	PowerStateBusOff PowerState = 2
)

func (p PowerState) String() string {
	switch p {
	case PowerStateActive:
		return "active"
	case PowerStateSleeping:
		return "sleeping"
	case PowerStateBusOff:
		return "bus-off"
	default:
		return "unknown"
	}
}

// PowerEvent is emitted when a zone's power state changes.
type PowerEvent struct {
	Zone  rcp.Zone
	State PowerState
	Err   error // non-nil on transitions caused by command failure
}

// Config configures Manager behaviour.
type Config struct {
	// RecoveryInterval is how often the Manager retries CmdWake for bus-off zones.
	// Default: 100 ms.
	RecoveryInterval time.Duration
	// RecoveryTimeout is the per-attempt command timeout during recovery.
	// Default: 50 ms.
	RecoveryTimeout time.Duration
}

// DefaultConfig returns the recommended ASIL-B power state configuration.
func DefaultConfig() Config {
	return Config{
		RecoveryInterval: 100 * time.Millisecond,
		RecoveryTimeout:  50 * time.Millisecond,
	}
}

type subscriber struct {
	ch   chan PowerEvent
	once sync.Once
}

func (s *subscriber) close() { s.once.Do(func() { close(s.ch) }) }

// Manager sends CmdSleep / CmdWake and tracks zone power states.
type Manager struct {
	cfg    Config
	ctrls  map[rcp.Zone]rcp.Controller
	done   chan struct{}
	closed atomic.Bool

	mu     sync.Mutex
	states map[rcp.Zone]PowerState
	subs   []*subscriber
}

// NewManager creates and starts a Manager for the given controllers.
// All zones start in PowerStateActive.
func NewManager(cfg Config, ctrls []rcp.Controller) *Manager {
	m := &Manager{
		cfg:    cfg,
		ctrls:  make(map[rcp.Zone]rcp.Controller, len(ctrls)),
		states: make(map[rcp.Zone]PowerState, len(ctrls)),
		done:   make(chan struct{}),
	}
	for _, c := range ctrls {
		m.ctrls[c.Zone()] = c
		m.states[c.Zone()] = PowerStateActive
	}
	go m.recoverLoop()
	return m
}

// State returns the current power state for zone.
// Returns PowerStateBusOff for unregistered zones.
func (m *Manager) State(zone rcp.Zone) PowerState {
	m.mu.Lock()
	defer m.mu.Unlock()
	st, ok := m.states[zone]
	if !ok {
		return PowerStateBusOff
	}
	return st
}

// Sleep sends CmdSleep to zone and transitions it from Active to Sleeping.
// Returns an error if the zone is not registered, already sleeping, or the
// command fails (in which case the zone transitions to BusOff).
func (m *Manager) Sleep(ctx context.Context, zone rcp.Zone) error {
	ctrl, err := m.ctrl(zone)
	if err != nil {
		return err
	}
	m.mu.Lock()
	if m.states[zone] != PowerStateActive {
		st := m.states[zone]
		m.mu.Unlock()
		return fmt.Errorf("rcp/powerstate: zone %s is %s, not active", zone, st)
	}
	m.mu.Unlock()

	_, sendErr := ctrl.Send(ctx, &rcp.Command{
		Zone:     zone,
		Type:     rcp.CmdSleep,
		Priority: rcp.PriorityHigh,
	})
	if sendErr != nil {
		m.transition(zone, PowerStateBusOff, sendErr)
		return fmt.Errorf("rcp/powerstate: Sleep zone %s: %w", zone, sendErr)
	}
	m.transition(zone, PowerStateSleeping, nil)
	return nil
}

// Wake sends CmdWake to zone and transitions it to Active.
// Works from both Sleeping and BusOff states.
// Returns an error if the zone is not registered, already active, or the
// command fails (in which case the zone transitions to BusOff).
func (m *Manager) Wake(ctx context.Context, zone rcp.Zone) error {
	ctrl, err := m.ctrl(zone)
	if err != nil {
		return err
	}
	m.mu.Lock()
	if m.states[zone] == PowerStateActive {
		m.mu.Unlock()
		return fmt.Errorf("rcp/powerstate: zone %s is already active", zone)
	}
	m.mu.Unlock()

	_, sendErr := ctrl.Send(ctx, &rcp.Command{
		Zone:     zone,
		Type:     rcp.CmdWake,
		Priority: rcp.PriorityHigh,
	})
	if sendErr != nil {
		m.transition(zone, PowerStateBusOff, sendErr)
		return fmt.Errorf("rcp/powerstate: Wake zone %s: %w", zone, sendErr)
	}
	m.transition(zone, PowerStateActive, nil)
	return nil
}

// Subscribe returns a channel of PowerEvents until ctx is cancelled.
// Returns an error if the Manager is already closed.
func (m *Manager) Subscribe(ctx context.Context) (<-chan PowerEvent, error) {
	if m.closed.Load() {
		return nil, fmt.Errorf("rcp/powerstate: manager closed")
	}
	sub := &subscriber{ch: make(chan PowerEvent, 16)}
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

// Close stops the recovery loop and closes all subscriber channels.
// Safe to call multiple times.
func (m *Manager) Close() error {
	if !m.closed.CompareAndSwap(false, true) {
		return nil
	}
	close(m.done)
	return nil
}

func (m *Manager) ctrl(zone rcp.Zone) (rcp.Controller, error) {
	m.mu.Lock()
	ctrl, ok := m.ctrls[zone]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("rcp/powerstate: zone %s not registered", zone)
	}
	return ctrl, nil
}

func (m *Manager) transition(zone rcp.Zone, newState PowerState, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.states[zone] == newState {
		return
	}
	m.states[zone] = newState
	ev := PowerEvent{Zone: zone, State: newState, Err: err}
	for _, sub := range m.subs {
		select {
		case sub.ch <- ev:
		default:
		}
	}
}

func (m *Manager) removeSub(sub *subscriber) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, s := range m.subs {
		if s == sub {
			m.subs = append(m.subs[:i], m.subs[i+1:]...)
			return
		}
	}
}

// recoverLoop periodically sends CmdWake to all BusOff zones.
func (m *Manager) recoverLoop() {
	ticker := time.NewTicker(m.cfg.RecoveryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.done:
			return
		case <-ticker.C:
			m.attemptRecovery()
		}
	}
}

func (m *Manager) attemptRecovery() {
	m.mu.Lock()
	var busOff []rcp.Zone
	for z, st := range m.states {
		if st == PowerStateBusOff {
			busOff = append(busOff, z)
		}
	}
	m.mu.Unlock()

	for _, zone := range busOff {
		ctrl, err := m.ctrl(zone)
		if err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), m.cfg.RecoveryTimeout)
		_, sendErr := ctrl.Send(ctx, &rcp.Command{
			Zone:     zone,
			Type:     rcp.CmdWake,
			Priority: rcp.PriorityHigh,
		})
		cancel()
		if sendErr == nil {
			m.transition(zone, PowerStateActive, nil)
		}
	}
}
