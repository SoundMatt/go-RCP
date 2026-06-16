//fusa:test REQ-WDG-001
//fusa:test REQ-WDG-002
//fusa:test REQ-WDG-003
//fusa:test REQ-WDG-004
//fusa:test REQ-WDG-005
//fusa:test REQ-WDG-006
//fusa:test REQ-WDG-007
//fusa:test REQ-WDG-008

package watchdog_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/mock"
	"github.com/SoundMatt/go-RCP/watchdog"
)

// testConfig returns a fast watchdog config for tests: 2 ms interval, 1 ms timeout.
func testConfig() watchdog.Config {
	return watchdog.Config{
		Interval:     2 * time.Millisecond,
		Timeout:      1 * time.Millisecond,
		DegradeAfter: 3,
		FaultAfter:   5,
	}
}

// failCtrl always returns an error from Send, simulating a dead zone.
type failCtrl struct{ zone rcp.Zone }

func (f *failCtrl) Zone() rcp.Zone { return f.zone }
func (f *failCtrl) Send(_ context.Context, _ *rcp.Command) (*rcp.Response, error) {
	return nil, errors.New("simulated zone failure")
}
func (f *failCtrl) Subscribe(_ context.Context) (<-chan *rcp.Status, error) {
	return make(chan *rcp.Status, 1), nil
}
func (f *failCtrl) Close() error { return nil }

// toggleCtrl switches between success and failure.
type toggleCtrl struct {
	zone   rcp.Zone
	failing atomic.Bool
}

func (t *toggleCtrl) Zone() rcp.Zone { return t.zone }
func (t *toggleCtrl) Send(_ context.Context, cmd *rcp.Command) (*rcp.Response, error) {
	if t.failing.Load() {
		return nil, errors.New("simulated failure")
	}
	return &rcp.Response{CommandID: cmd.ID, Zone: t.zone, Status: rcp.StatusOK}, nil
}
func (t *toggleCtrl) Subscribe(_ context.Context) (<-chan *rcp.Status, error) {
	return make(chan *rcp.Status, 1), nil
}
func (t *toggleCtrl) Close() error { return nil }

// waitForState drains events until the target state is received or deadline exceeded.
func waitForState(t *testing.T, events <-chan watchdog.HealthEvent, want watchdog.HealthState, deadline time.Duration) {
	t.Helper()
	timer := time.NewTimer(deadline)
	defer timer.Stop()
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatalf("event channel closed before state %v reached", want)
			}
			if ev.State == want {
				return
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for state %v after %v", want, deadline)
		}
	}
}

// TestKeeper_HealthyByDefault verifies a new Keeper reports Healthy for all zones (REQ-WDG-001).
func TestKeeper_HealthyByDefault(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	k := watchdog.NewKeeper(testConfig(), []rcp.Controller{ctrl})
	t.Cleanup(func() { _ = k.Close() })

	if got := k.Health(rcp.ZoneFrontLeft); got != watchdog.HealthStateHealthy {
		t.Errorf("Health = %v, want Healthy", got)
	}
}

// TestKeeper_UnknownZone_ReturnsFaulted verifies that an unregistered zone returns Faulted (REQ-WDG-002).
func TestKeeper_UnknownZone_ReturnsFaulted(t *testing.T) {
	k := watchdog.NewKeeper(testConfig(), nil)
	t.Cleanup(func() { _ = k.Close() })

	if got := k.Health(rcp.ZoneCentral); got != watchdog.HealthStateFaulted {
		t.Errorf("Health(unregistered) = %v, want Faulted", got)
	}
}

// TestKeeper_Degraded verifies Healthy→Degraded transition after consecutive failures (REQ-WDG-003).
func TestKeeper_Degraded(t *testing.T) {
	ctrl := &failCtrl{zone: rcp.ZoneFrontLeft}
	k := watchdog.NewKeeper(testConfig(), []rcp.Controller{ctrl})
	t.Cleanup(func() { _ = k.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	events, err := k.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	waitForState(t, events, watchdog.HealthStateDegraded, 300*time.Millisecond)
}

// TestKeeper_Faulted verifies Degraded→Faulted transition after enough failures (REQ-WDG-004).
func TestKeeper_Faulted(t *testing.T) {
	ctrl := &failCtrl{zone: rcp.ZoneRearLeft}
	k := watchdog.NewKeeper(testConfig(), []rcp.Controller{ctrl})
	t.Cleanup(func() { _ = k.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	events, err := k.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	waitForState(t, events, watchdog.HealthStateFaulted, 400*time.Millisecond)
}

// TestKeeper_Recovery verifies Faulted→Healthy recovery when kicks succeed again (REQ-WDG-005).
func TestKeeper_Recovery(t *testing.T) {
	ctrl := &toggleCtrl{zone: rcp.ZoneRearRight}
	ctrl.failing.Store(true)

	k := watchdog.NewKeeper(testConfig(), []rcp.Controller{ctrl})
	t.Cleanup(func() { _ = k.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	events, err := k.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// Wait until faulted.
	waitForState(t, events, watchdog.HealthStateFaulted, 500*time.Millisecond)

	// Recover the controller.
	ctrl.failing.Store(false)

	// Expect health to return to Healthy.
	waitForState(t, events, watchdog.HealthStateHealthy, 500*time.Millisecond)
}

// TestKeeper_HealthStatePriority verifies kicks use PriorityHigh (REQ-WDG-006).
func TestKeeper_KickUsesPriorityHigh(t *testing.T) {
	var observed rcp.Priority
	handler := func(cmd *rcp.Command) *rcp.Response {
		observed = cmd.Priority
		return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusOK}
	}
	ctrl := mock.NewController(rcp.ZoneFrontRight, handler)
	k := watchdog.NewKeeper(testConfig(), []rcp.Controller{ctrl})
	t.Cleanup(func() { _ = k.Close() })

	// Wait long enough for at least one kick.
	time.Sleep(20 * time.Millisecond)
	_ = k.Close()

	if observed != rcp.PriorityHigh {
		t.Errorf("kick priority = %v, want PriorityHigh", observed)
	}
}

// TestKeeper_KickUsesCmdWatchdog verifies kick commands use CmdWatchdog (REQ-WDG-007).
func TestKeeper_KickUsesCmdWatchdog(t *testing.T) {
	var observed rcp.CommandType
	handler := func(cmd *rcp.Command) *rcp.Response {
		observed = cmd.Type
		return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusOK}
	}
	ctrl := mock.NewController(rcp.ZoneCentral, handler)
	k := watchdog.NewKeeper(testConfig(), []rcp.Controller{ctrl})
	t.Cleanup(func() { _ = k.Close() })

	time.Sleep(20 * time.Millisecond)
	_ = k.Close()

	if observed != rcp.CmdWatchdog {
		t.Errorf("kick cmd type = %v, want CmdWatchdog", observed)
	}
}

// TestKeeper_Subscribe_ClosedOnKeeperClose verifies subscriber channel closes when Keeper.Close() is called (REQ-WDG-008).
func TestKeeper_Subscribe_ClosedOnKeeperClose(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	k := watchdog.NewKeeper(testConfig(), []rcp.Controller{ctrl})

	ctx := context.Background()
	events, err := k.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	_ = k.Close()

	// Channel should be closed within a short time.
	select {
	case _, ok := <-events:
		if ok {
			// A health event is fine — keep draining.
		drainLoop:
			for {
				select {
				case _, ok2 := <-events:
					if !ok2 {
						break drainLoop
					}
				case <-time.After(300 * time.Millisecond):
					t.Fatal("subscriber channel not closed after Keeper.Close()")
				}
			}
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("subscriber channel not closed after Keeper.Close()")
	}
}

// TestKeeper_Subscribe_ClosedOnContextCancel verifies subscriber channel closes when ctx is cancelled.
func TestKeeper_Subscribe_ClosedOnContextCancel(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	k := watchdog.NewKeeper(testConfig(), []rcp.Controller{ctrl})
	t.Cleanup(func() { _ = k.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	events, err := k.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	cancel()

	// Drain until closed.
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

// TestKeeper_Subscribe_AfterClose returns an error when Keeper is closed.
func TestKeeper_Subscribe_AfterClose(t *testing.T) {
	k := watchdog.NewKeeper(testConfig(), nil)
	_ = k.Close()

	_, err := k.Subscribe(context.Background())
	if err == nil {
		t.Error("Subscribe after Close should return error, got nil")
	}
}

// TestKeeper_Close_Idempotent verifies Close() can be called multiple times without panic (REQ-WDG-008).
func TestKeeper_Close_Idempotent(t *testing.T) {
	k := watchdog.NewKeeper(testConfig(), nil)
	if err := k.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := k.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

// TestDefaultConfig verifies DefaultConfig returns ASIL-B recommended values.
func TestDefaultConfig(t *testing.T) {
	cfg := watchdog.DefaultConfig()
	if cfg.Interval != 10*time.Millisecond {
		t.Errorf("Interval = %v, want 10ms", cfg.Interval)
	}
	if cfg.Timeout != 5*time.Millisecond {
		t.Errorf("Timeout = %v, want 5ms", cfg.Timeout)
	}
	if cfg.DegradeAfter != 3 {
		t.Errorf("DegradeAfter = %d, want 3", cfg.DegradeAfter)
	}
	if cfg.FaultAfter != 5 {
		t.Errorf("FaultAfter = %d, want 5", cfg.FaultAfter)
	}
}

// TestHealthState_String verifies HealthState.String() returns human-readable labels.
func TestHealthState_String(t *testing.T) {
	tests := []struct {
		state watchdog.HealthState
		want  string
	}{
		{watchdog.HealthStateHealthy, "healthy"},
		{watchdog.HealthStateDegraded, "degraded"},
		{watchdog.HealthStateFaulted, "faulted"},
		{watchdog.HealthState(99), "unknown"},
	}
	for _, tc := range tests {
		if got := tc.state.String(); got != tc.want {
			t.Errorf("HealthState(%d).String() = %q, want %q", tc.state, got, tc.want)
		}
	}
}
