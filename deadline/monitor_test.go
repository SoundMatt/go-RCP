//fusa:test REQ-DL-001
//fusa:test REQ-DL-002
//fusa:test REQ-DL-003
//fusa:test REQ-DL-004
//fusa:test REQ-DL-005
//fusa:test REQ-DL-006

package deadline_test

import (
	"errors"
	"testing"
	"time"

	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/deadline"
	"github.com/SoundMatt/go-RCP/server"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func testStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x04, 0, 0, 0, 0, 1}, 1)
}

// TestMonitor_NeverObservedIsDead checks a stream Monitor has never heard
// from at all reports LivenessDead (REQ-DL-001).
func TestMonitor_NeverObservedIsDead(t *testing.T) {
	m, err := deadline.NewMonitor(deadline.Config{Deadline: time.Hour})
	if err != nil {
		t.Fatalf("NewMonitor: %v", err)
	}
	stream := testStream()
	if got := m.State(stream); got != deadline.LivenessDead {
		t.Errorf("State(never observed) = %v, want Dead", got)
	}
	if m.Alive(stream) {
		t.Errorf("Alive(never observed) = true, want false")
	}
}

// TestMonitor_TriggerAliveThenDecays checks ObserveTrigger reports
// LivenessAlive immediately, decaying to LivenessIdle once Deadline elapses
// with only a heartbeat observed in between, and to LivenessDead once
// Deadline elapses with nothing observed at all (REQ-DL-002).
func TestMonitor_TriggerAliveThenDecays(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	m, err := deadline.NewMonitorWithClock(deadline.Config{Deadline: 100 * time.Millisecond}, clock.now)
	if err != nil {
		t.Fatalf("NewMonitorWithClock: %v", err)
	}
	stream := testStream()

	m.ObserveTrigger(stream)
	if got := m.State(stream); got != deadline.LivenessAlive {
		t.Fatalf("State(just triggered) = %v, want Alive", got)
	}

	// Half the deadline passes with only a heartbeat: still within Deadline
	// of the trigger, so still Alive.
	clock.advance(50 * time.Millisecond)
	m.ObserveHeartbeat(stream)
	if got := m.State(stream); got != deadline.LivenessAlive {
		t.Errorf("State(heartbeat within trigger deadline) = %v, want still Alive", got)
	}

	// Past the trigger's own deadline, but the heartbeat above keeps lastAny
	// recent: Idle, not Dead.
	clock.advance(60 * time.Millisecond)
	if got := m.State(stream); got != deadline.LivenessIdle {
		t.Errorf("State(trigger stale, heartbeat recent) = %v, want Idle", got)
	}

	// Nothing further observed past Deadline since that heartbeat: Dead.
	clock.advance(200 * time.Millisecond)
	if got := m.State(stream); got != deadline.LivenessDead {
		t.Errorf("State(nothing recent at all) = %v, want Dead", got)
	}
}

// TestMonitor_HeartbeatAloneNeverAlive checks ObserveHeartbeat alone (no
// trigger ever observed) keeps a stream at LivenessIdle, never
// LivenessAlive (REQ-DL-003).
func TestMonitor_HeartbeatAloneNeverAlive(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	m, err := deadline.NewMonitorWithClock(deadline.Config{Deadline: 100 * time.Millisecond}, clock.now)
	if err != nil {
		t.Fatalf("NewMonitorWithClock: %v", err)
	}
	stream := testStream()

	m.ObserveHeartbeat(stream)
	if got := m.State(stream); got != deadline.LivenessIdle {
		t.Errorf("State(heartbeat only) = %v, want Idle", got)
	}
	if !m.Alive(stream) {
		t.Errorf("Alive(heartbeat only) = false, want true (Idle still counts as not-Dead)")
	}

	clock.advance(50 * time.Millisecond)
	m.ObserveHeartbeat(stream)
	if got := m.State(stream); got != deadline.LivenessIdle {
		t.Errorf("State(repeated heartbeats, no trigger) = %v, want still Idle", got)
	}
}

// TestMonitor_ConfigValidation checks Config.Validate (and by extension
// NewMonitor/NewMonitorWithClock) rejects a non-positive Deadline
// (REQ-DL-004).
func TestMonitor_ConfigValidation(t *testing.T) {
	cases := []struct {
		name string
		cfg  deadline.Config
		want error
	}{
		{"zero", deadline.Config{Deadline: 0}, deadline.ErrInvalidDeadline},
		{"negative", deadline.Config{Deadline: -time.Second}, deadline.ErrInvalidDeadline},
		{"positive", deadline.Config{Deadline: time.Second}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.cfg.Validate()
			if !errors.Is(err, c.want) {
				t.Errorf("Validate() = %v, want %v", err, c.want)
			}
			_, newErr := deadline.NewMonitor(c.cfg)
			if !errors.Is(newErr, c.want) {
				t.Errorf("NewMonitor() err = %v, want %v", newErr, c.want)
			}
		})
	}
}

// TestDeadlineForQueue checks DeadlineForQueue derives a Deadline from a
// server.QueueConfig's HeartbeatIntervalMillis and a missed-heartbeat
// multiplier, and rejects a zero HeartbeatIntervalMillis or an
// out-of-range multiplier (REQ-DL-005).
func TestDeadlineForQueue(t *testing.T) {
	got, err := deadline.DeadlineForQueue(server.QueueConfig{HeartbeatIntervalMillis: 50}, 3)
	if err != nil {
		t.Fatalf("DeadlineForQueue: %v", err)
	}
	if want := 150 * time.Millisecond; got != want {
		t.Errorf("DeadlineForQueue = %v, want %v", got, want)
	}

	if _, err := deadline.DeadlineForQueue(server.QueueConfig{HeartbeatIntervalMillis: 0}, 3); !errors.Is(err, deadline.ErrNoHeartbeatConfigured) {
		t.Errorf("DeadlineForQueue(no heartbeat) err = %v, want ErrNoHeartbeatConfigured", err)
	}
	if _, err := deadline.DeadlineForQueue(server.QueueConfig{HeartbeatIntervalMillis: 50}, 0); !errors.Is(err, deadline.ErrInvalidMissedHeartbeats) {
		t.Errorf("DeadlineForQueue(missedHeartbeats=0) err = %v, want ErrInvalidMissedHeartbeats", err)
	}
}

// TestMonitor_Forget checks Forget discards a stream's recorded state, after
// which State reports LivenessDead as if it had never been observed
// (REQ-DL-006).
func TestMonitor_Forget(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	m, err := deadline.NewMonitorWithClock(deadline.Config{Deadline: time.Hour}, clock.now)
	if err != nil {
		t.Fatalf("NewMonitorWithClock: %v", err)
	}
	stream := testStream()

	m.ObserveTrigger(stream)
	if got := m.State(stream); got != deadline.LivenessAlive {
		t.Fatalf("State(after Observe) = %v, want Alive", got)
	}

	m.Forget(stream)
	if got := m.State(stream); got != deadline.LivenessDead {
		t.Errorf("State(after Forget) = %v, want Dead", got)
	}
}
