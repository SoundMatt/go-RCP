//fusa:test REQ-PWR-001
//fusa:test REQ-PWR-002
//fusa:test REQ-PWR-003
//fusa:test REQ-PWR-004
//fusa:test REQ-PWR-005
//fusa:test REQ-PWR-006
//fusa:test REQ-PWR-007
//fusa:test REQ-PWR-008

package powerstate_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/mock"
	"github.com/SoundMatt/go-RCP/powerstate"
)

// fastConfig returns a fast recovery config for tests.
func fastConfig() powerstate.Config {
	return powerstate.Config{
		RecoveryInterval: 5 * time.Millisecond,
		RecoveryTimeout:  2 * time.Millisecond,
	}
}

// failCtrl always returns an error from Send.
type failCtrl struct{ zone rcp.Zone }

func (f *failCtrl) Zone() rcp.Zone { return f.zone }
func (f *failCtrl) Send(_ context.Context, _ *rcp.Command) (*rcp.Response, error) {
	return nil, errors.New("simulated send failure")
}
func (f *failCtrl) Subscribe(_ context.Context) (<-chan *rcp.Status, error) {
	return make(chan *rcp.Status, 1), nil
}
func (f *failCtrl) Close() error { return nil }

// toggleCtrl switches between success and failure via an atomic flag.
type toggleCtrl struct {
	zone    rcp.Zone
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

// waitForPower blocks until the target PowerState is received or deadline exceeded.
func waitForPower(t *testing.T, events <-chan powerstate.PowerEvent, want powerstate.PowerState, d time.Duration) {
	t.Helper()
	timer := time.NewTimer(d)
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
			t.Fatalf("timed out after %v waiting for state %v", d, want)
		}
	}
}

// TestPowerState_CmdSleepWake_Defined verifies CmdSleep and CmdWake exist and are distinct (REQ-PWR-001).
func TestPowerState_CmdSleepWake_Defined(t *testing.T) {
	if rcp.CmdSleep == rcp.CmdWake {
		t.Error("CmdSleep and CmdWake must be distinct")
	}
	if rcp.CmdSleep == rcp.CmdNoop || rcp.CmdWake == rcp.CmdNoop {
		t.Error("CmdSleep/CmdWake must differ from CmdNoop")
	}
}

// TestManager_InitiallyActive verifies zones start Active (REQ-PWR-002).
func TestManager_InitiallyActive(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	mgr := powerstate.NewManager(fastConfig(), []rcp.Controller{ctrl})
	t.Cleanup(func() { _ = mgr.Close() })

	if got := mgr.State(rcp.ZoneFrontLeft); got != powerstate.PowerStateActive {
		t.Errorf("State = %v, want Active", got)
	}
}

// TestManager_Sleep_TransitionsToSleeping verifies CmdSleep → Sleeping (REQ-PWR-002).
func TestManager_Sleep_TransitionsToSleeping(t *testing.T) {
	cmdCh := make(chan rcp.CommandType, 4)
	handler := func(cmd *rcp.Command) *rcp.Response {
		select {
		case cmdCh <- cmd.Type:
		default:
		}
		return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusOK}
	}
	ctrl := mock.NewController(rcp.ZoneFrontLeft, handler)
	mgr := powerstate.NewManager(fastConfig(), []rcp.Controller{ctrl})
	t.Cleanup(func() { _ = mgr.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := mgr.Sleep(ctx, rcp.ZoneFrontLeft); err != nil {
		t.Fatalf("Sleep: %v", err)
	}

	// Find the CmdSleep in the observed commands.
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case tp := <-cmdCh:
			if tp == rcp.CmdSleep {
				goto found
			}
		case <-timer.C:
			t.Fatal("CmdSleep not observed")
		}
	}
found:
	if got := mgr.State(rcp.ZoneFrontLeft); got != powerstate.PowerStateSleeping {
		t.Errorf("State after Sleep = %v, want Sleeping", got)
	}
}

// TestManager_Wake_TransitionsToActive verifies CmdWake → Active (REQ-PWR-003).
func TestManager_Wake_TransitionsToActive(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneRearLeft, nil)
	mgr := powerstate.NewManager(fastConfig(), []rcp.Controller{ctrl})
	t.Cleanup(func() { _ = mgr.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := mgr.Sleep(ctx, rcp.ZoneRearLeft); err != nil {
		t.Fatalf("Sleep: %v", err)
	}
	if err := mgr.Wake(ctx, rcp.ZoneRearLeft); err != nil {
		t.Fatalf("Wake: %v", err)
	}

	if got := mgr.State(rcp.ZoneRearLeft); got != powerstate.PowerStateActive {
		t.Errorf("State after Wake = %v, want Active", got)
	}
}

// TestManager_SleepFailure_TransitionsToBusOff verifies command failure → BusOff (REQ-PWR-004).
func TestManager_SleepFailure_TransitionsToBusOff(t *testing.T) {
	ctrl := &failCtrl{zone: rcp.ZoneFrontRight}
	mgr := powerstate.NewManager(fastConfig(), []rcp.Controller{ctrl})
	t.Cleanup(func() { _ = mgr.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := mgr.Sleep(ctx, rcp.ZoneFrontRight)
	if err == nil {
		t.Fatal("Sleep on failing ctrl should return error")
	}
	if got := mgr.State(rcp.ZoneFrontRight); got != powerstate.PowerStateBusOff {
		t.Errorf("State after failure = %v, want BusOff", got)
	}
}

// TestManager_Recovery_Loop verifies BusOff→Active recovery when CmdWake succeeds (REQ-PWR-005, REQ-PWR-006).
func TestManager_Recovery_Loop(t *testing.T) {
	ctrl := &toggleCtrl{zone: rcp.ZoneCentral}
	ctrl.failing.Store(true) // start failing

	mgr := powerstate.NewManager(fastConfig(), []rcp.Controller{ctrl})
	t.Cleanup(func() { _ = mgr.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	events, err := mgr.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// Sleep fails → BusOff.
	sleepErr := mgr.Sleep(ctx, rcp.ZoneCentral)
	if sleepErr == nil {
		t.Fatal("Sleep should fail on toggleCtrl in failing mode")
	}
	waitForPower(t, events, powerstate.PowerStateBusOff, 100*time.Millisecond)

	// Allow recovery to succeed.
	ctrl.failing.Store(false)

	// Recovery loop should transition back to Active.
	waitForPower(t, events, powerstate.PowerStateActive, 300*time.Millisecond)
}

// TestManager_Subscribe_EmitsEvents verifies events are delivered on transitions (REQ-PWR-002, REQ-PWR-003).
func TestManager_Subscribe_EmitsEvents(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	mgr := powerstate.NewManager(fastConfig(), []rcp.Controller{ctrl})
	t.Cleanup(func() { _ = mgr.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	events, err := mgr.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	if err := mgr.Sleep(ctx, rcp.ZoneFrontLeft); err != nil {
		t.Fatalf("Sleep: %v", err)
	}
	waitForPower(t, events, powerstate.PowerStateSleeping, 100*time.Millisecond)

	if err := mgr.Wake(ctx, rcp.ZoneFrontLeft); err != nil {
		t.Fatalf("Wake: %v", err)
	}
	waitForPower(t, events, powerstate.PowerStateActive, 100*time.Millisecond)
}

// TestManager_Subscribe_ClosedOnManagerClose verifies channel closes on Manager.Close() (REQ-PWR-007).
func TestManager_Subscribe_ClosedOnManagerClose(t *testing.T) {
	mgr := powerstate.NewManager(fastConfig(), nil)
	ctx := context.Background()
	events, err := mgr.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	_ = mgr.Close()

	timer := time.NewTimer(300 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case _, ok := <-events:
			if !ok {
				return
			}
		case <-timer.C:
			t.Fatal("channel not closed after Manager.Close()")
		}
	}
}

// TestManager_Subscribe_ClosedOnContextCancel verifies channel closes on ctx cancel (REQ-PWR-007).
func TestManager_Subscribe_ClosedOnContextCancel(t *testing.T) {
	mgr := powerstate.NewManager(fastConfig(), nil)
	t.Cleanup(func() { _ = mgr.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	events, err := mgr.Subscribe(ctx)
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
			t.Fatal("channel not closed after ctx cancel")
		}
	}
}

// TestManager_Subscribe_AfterClose returns error when Manager is closed (REQ-PWR-007).
func TestManager_Subscribe_AfterClose(t *testing.T) {
	mgr := powerstate.NewManager(fastConfig(), nil)
	_ = mgr.Close()
	_, err := mgr.Subscribe(context.Background())
	if err == nil {
		t.Error("Subscribe after Close should return error")
	}
}

// TestManager_Close_Idempotent verifies Close() is safe to call multiple times (REQ-PWR-008).
func TestManager_Close_Idempotent(t *testing.T) {
	mgr := powerstate.NewManager(fastConfig(), nil)
	if err := mgr.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := mgr.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

// TestManager_UnknownZone_ReturnsBusOff verifies State() for unregistered zone (REQ-PWR-004).
func TestManager_UnknownZone_ReturnsBusOff(t *testing.T) {
	mgr := powerstate.NewManager(fastConfig(), nil)
	t.Cleanup(func() { _ = mgr.Close() })
	if got := mgr.State(rcp.ZoneCentral); got != powerstate.PowerStateBusOff {
		t.Errorf("State(unregistered) = %v, want BusOff", got)
	}
}

// TestPowerState_String verifies PowerState.String() values.
func TestPowerState_String(t *testing.T) {
	cases := []struct {
		state powerstate.PowerState
		want  string
	}{
		{powerstate.PowerStateActive, "active"},
		{powerstate.PowerStateSleeping, "sleeping"},
		{powerstate.PowerStateBusOff, "bus-off"},
		{powerstate.PowerState(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.state.String(); got != tc.want {
			t.Errorf("PowerState(%d).String() = %q, want %q", tc.state, got, tc.want)
		}
	}
}

// TestDefaultConfig verifies DefaultConfig returns expected values.
func TestDefaultConfig(t *testing.T) {
	cfg := powerstate.DefaultConfig()
	if cfg.RecoveryInterval != 100*time.Millisecond {
		t.Errorf("RecoveryInterval = %v, want 100ms", cfg.RecoveryInterval)
	}
	if cfg.RecoveryTimeout != 50*time.Millisecond {
		t.Errorf("RecoveryTimeout = %v, want 50ms", cfg.RecoveryTimeout)
	}
}

// TestManager_Sleep_AlreadySleeping returns error when already sleeping.
func TestManager_Sleep_AlreadySleeping(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	mgr := powerstate.NewManager(fastConfig(), []rcp.Controller{ctrl})
	t.Cleanup(func() { _ = mgr.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := mgr.Sleep(ctx, rcp.ZoneFrontLeft); err != nil {
		t.Fatalf("first Sleep: %v", err)
	}
	if err := mgr.Sleep(ctx, rcp.ZoneFrontLeft); err == nil {
		t.Error("second Sleep should fail when already sleeping")
	}
}

// TestManager_Wake_AlreadyActive returns error when already active.
func TestManager_Wake_AlreadyActive(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	mgr := powerstate.NewManager(fastConfig(), []rcp.Controller{ctrl})
	t.Cleanup(func() { _ = mgr.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := mgr.Wake(ctx, rcp.ZoneFrontLeft); err == nil {
		t.Error("Wake when already active should return error")
	}
}
