//fusa:test REQ-DL-001
//fusa:test REQ-DL-002
//fusa:test REQ-DL-003
//fusa:test REQ-DL-004
//fusa:test REQ-DL-005
//fusa:test REQ-DL-006
//fusa:test REQ-DL-007
//fusa:test REQ-DL-008

package deadline_test

import (
	"context"
	"errors"
	"testing"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/deadline"
	"github.com/SoundMatt/go-RCP/mock"
)

// fastConfig returns a deadline config with 10 ms deadline for fast tests.
func fastConfig() deadline.Config {
	return deadline.Config{Deadline: 10 * time.Millisecond}
}

// waitForLiveness drains events until Alive matches want or deadline exceeded.
func waitForLiveness(t *testing.T, events <-chan deadline.LivenessEvent, want bool, d time.Duration) {
	t.Helper()
	timer := time.NewTimer(d)
	defer timer.Stop()
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatalf("event channel closed before Alive=%v received", want)
			}
			if ev.Alive == want {
				return
			}
		case <-timer.C:
			t.Fatalf("timed out after %v waiting for Alive=%v", d, want)
		}
	}
}

// errSubscribeCtrl always returns an error from Subscribe.
type errSubscribeCtrl struct{ zone rcp.Zone }

func (e *errSubscribeCtrl) Zone() rcp.Zone { return e.zone }
func (e *errSubscribeCtrl) Send(_ context.Context, cmd *rcp.Command) (*rcp.Response, error) {
	return &rcp.Response{CommandID: cmd.ID, Zone: e.zone, Status: rcp.StatusOK}, nil
}
func (e *errSubscribeCtrl) Subscribe(_ context.Context) (<-chan *rcp.Status, error) {
	return nil, errors.New("subscribe unavailable")
}
func (e *errSubscribeCtrl) Close() error { return nil }

// TestMonitor_InitiallyNotAlive verifies zones start as not-alive before any Status (REQ-DL-001).
func TestMonitor_InitiallyNotAlive(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	mon := deadline.NewMonitor(fastConfig(), []rcp.Controller{ctrl})
	t.Cleanup(func() { _ = mon.Close() })

	if mon.Alive(rcp.ZoneFrontLeft) {
		t.Error("zone should not be alive before first status")
	}
}

// TestMonitor_Alive_AfterStatus verifies zone becomes alive after first Status (REQ-DL-001).
func TestMonitor_Alive_AfterStatus(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	mon := deadline.NewMonitor(fastConfig(), []rcp.Controller{ctrl})
	t.Cleanup(func() { _ = mon.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	events, err := mon.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	ctrl.Publish(nil)
	waitForLiveness(t, events, true, 200*time.Millisecond)

	if !mon.Alive(rcp.ZoneFrontLeft) {
		t.Error("Alive() should return true after status received")
	}
}

// TestMonitor_Dead_AfterDeadline verifies alive→dead transition when Status stops (REQ-DL-002).
func TestMonitor_Dead_AfterDeadline(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	cfg := deadline.Config{Deadline: 15 * time.Millisecond}
	mon := deadline.NewMonitor(cfg, []rcp.Controller{ctrl})
	t.Cleanup(func() { _ = mon.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	events, err := mon.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// Publish once to make zone alive.
	ctrl.Publish(nil)
	waitForLiveness(t, events, true, 200*time.Millisecond)

	// Stop publishing — deadline fires → dead.
	waitForLiveness(t, events, false, 300*time.Millisecond)

	if mon.Alive(rcp.ZoneFrontLeft) {
		t.Error("Alive() should be false after deadline exceeded")
	}
}

// TestMonitor_Recovery verifies dead→alive recovery when Status resumes (REQ-DL-003).
func TestMonitor_Recovery(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneRearLeft, nil)
	cfg := deadline.Config{Deadline: 15 * time.Millisecond}
	mon := deadline.NewMonitor(cfg, []rcp.Controller{ctrl})
	t.Cleanup(func() { _ = mon.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	events, err := mon.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// Alive → Dead cycle.
	ctrl.Publish(nil)
	waitForLiveness(t, events, true, 200*time.Millisecond)
	waitForLiveness(t, events, false, 300*time.Millisecond)

	// Recover: publish again.
	ctrl.Publish(nil)
	waitForLiveness(t, events, true, 200*time.Millisecond)
}

// TestMonitor_Config_Deadline verifies configurable deadline is honoured (REQ-DL-004).
func TestMonitor_Config_Deadline(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneRearRight, nil)
	// Use a very short deadline and verify it fires faster than the default.
	cfg := deadline.Config{Deadline: 8 * time.Millisecond}
	mon := deadline.NewMonitor(cfg, []rcp.Controller{ctrl})
	t.Cleanup(func() { _ = mon.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	events, err := mon.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	ctrl.Publish(nil)
	waitForLiveness(t, events, true, 200*time.Millisecond)

	// With 8 ms deadline, dead event must arrive well within 200 ms.
	start := time.Now()
	waitForLiveness(t, events, false, 200*time.Millisecond)
	if elapsed := time.Since(start); elapsed > 150*time.Millisecond {
		t.Errorf("dead event took %v, expected < 150ms for 8ms deadline", elapsed)
	}
}

// TestDefaultConfig verifies DefaultConfig returns 50 ms (REQ-DL-005).
func TestDefaultConfig(t *testing.T) {
	cfg := deadline.DefaultConfig()
	if cfg.Deadline != 50*time.Millisecond {
		t.Errorf("Deadline = %v, want 50ms", cfg.Deadline)
	}
}

// TestMonitor_UnknownZone_NotAlive verifies Alive() returns false for unregistered zone (REQ-DL-006).
func TestMonitor_UnknownZone_NotAlive(t *testing.T) {
	mon := deadline.NewMonitor(fastConfig(), nil)
	t.Cleanup(func() { _ = mon.Close() })

	if mon.Alive(rcp.ZoneCentral) {
		t.Error("Alive(unregistered) should return false")
	}
}

// TestMonitor_SubscribeError verifies zone stays not-alive when Subscribe fails (REQ-DL-001).
func TestMonitor_SubscribeError(t *testing.T) {
	ctrl := &errSubscribeCtrl{zone: rcp.ZoneFrontRight}
	mon := deadline.NewMonitor(fastConfig(), []rcp.Controller{ctrl})
	t.Cleanup(func() { _ = mon.Close() })

	// NewMonitor blocks until watch goroutine has attempted Subscribe.
	// The error path sets the zone to not-alive and returns immediately.
	if mon.Alive(rcp.ZoneFrontRight) {
		t.Error("zone whose Subscribe failed should not be alive")
	}
}

// TestMonitor_Subscribe_ClosedOnMonitorClose verifies subscriber channel closes on Monitor.Close() (REQ-DL-007).
func TestMonitor_Subscribe_ClosedOnMonitorClose(t *testing.T) {
	mon := deadline.NewMonitor(fastConfig(), nil)
	ctx := context.Background()
	events, err := mon.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	_ = mon.Close()

	timer := time.NewTimer(300 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case _, ok := <-events:
			if !ok {
				return
			}
		case <-timer.C:
			t.Fatal("subscriber channel not closed after Monitor.Close()")
		}
	}
}

// TestMonitor_Subscribe_ClosedOnContextCancel verifies subscriber channel closes on ctx cancel (REQ-DL-007).
func TestMonitor_Subscribe_ClosedOnContextCancel(t *testing.T) {
	mon := deadline.NewMonitor(fastConfig(), nil)
	t.Cleanup(func() { _ = mon.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	events, err := mon.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	cancel()

	timer := time.NewTimer(300 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case _, ok := <-events:
			if !ok {
				return
			}
		case <-timer.C:
			t.Fatal("subscriber channel not closed after context cancel")
		}
	}
}

// TestMonitor_Subscribe_AfterClose returns error when Monitor is closed (REQ-DL-007).
func TestMonitor_Subscribe_AfterClose(t *testing.T) {
	mon := deadline.NewMonitor(fastConfig(), nil)
	_ = mon.Close()

	_, err := mon.Subscribe(context.Background())
	if err == nil {
		t.Error("Subscribe after Close should return error")
	}
}

// TestMonitor_Close_Idempotent verifies Close() is safe to call multiple times (REQ-DL-008).
func TestMonitor_Close_Idempotent(t *testing.T) {
	mon := deadline.NewMonitor(fastConfig(), nil)
	if err := mon.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := mon.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}
