//fusa:test REQ-SIM-001
//fusa:test REQ-SIM-002
//fusa:test REQ-SIM-003
//fusa:test REQ-SIM-004
//fusa:test REQ-SIM-005
//fusa:test REQ-SIM-006
//fusa:test REQ-SIM-007
//fusa:test REQ-SIM-008
//fusa:test REQ-SIM-009

package sim_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/sim"
)

// TestSim_RoundTrip verifies Send returns StatusOK after latency (REQ-SIM-001).
func TestSim_RoundTrip(t *testing.T) {
	cfg := sim.Config{
		Zone:         rcp.ZoneFrontLeft,
		BaseLatency:  0,
		LatencyModel: sim.LatencyConstant,
	}
	ctrl := sim.NewController(cfg)
	t.Cleanup(func() { _ = ctrl.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdNoop})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Status != rcp.StatusOK {
		t.Errorf("Status = %v, want OK", resp.Status)
	}
}

// TestSim_LatencyIsApplied verifies BaseLatency actually delays Send (REQ-SIM-001).
func TestSim_LatencyIsApplied(t *testing.T) {
	cfg := sim.Config{
		Zone:         rcp.ZoneFrontLeft,
		BaseLatency:  20 * time.Millisecond,
		LatencyModel: sim.LatencyConstant,
	}
	ctrl := sim.NewController(cfg)
	t.Cleanup(func() { _ = ctrl.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	start := time.Now()
	_, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 15*time.Millisecond {
		t.Errorf("elapsed %v, want >= 15ms (BaseLatency=20ms)", elapsed)
	}
}

// TestSim_FaultCausesSendError verifies Fault() causes StatusError (REQ-SIM-002).
func TestSim_FaultCausesSendError(t *testing.T) {
	cfg := sim.Config{Zone: rcp.ZoneFrontLeft, LatencyModel: sim.LatencyConstant}
	ctrl := sim.NewController(cfg)
	t.Cleanup(func() { _ = ctrl.Close() })

	ctrl.Fault(errors.New("injected fault"))

	ctx := context.Background()
	resp, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if resp.Status != rcp.StatusError {
		t.Errorf("Status = %v, want StatusError", resp.Status)
	}
}

// TestSim_RecoverRestoresOK verifies Recover() clears fault (REQ-SIM-003).
func TestSim_RecoverRestoresOK(t *testing.T) {
	cfg := sim.Config{Zone: rcp.ZoneFrontLeft, LatencyModel: sim.LatencyConstant}
	ctrl := sim.NewController(cfg)
	t.Cleanup(func() { _ = ctrl.Close() })

	ctrl.Fault(errors.New("transient fault"))
	ctrl.Recover()

	ctx := context.Background()
	resp, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Status != rcp.StatusOK {
		t.Errorf("Status = %v after Recover, want OK", resp.Status)
	}
}

// TestSim_WatchdogMissedAfterTimeout verifies WatchdogMissed() fires (REQ-SIM-004).
func TestSim_WatchdogMissedAfterTimeout(t *testing.T) {
	cfg := sim.Config{
		Zone:            rcp.ZoneFrontLeft,
		WatchdogTimeout: 30 * time.Millisecond,
		LatencyModel:    sim.LatencyConstant,
	}
	ctrl := sim.NewController(cfg)
	t.Cleanup(func() { _ = ctrl.Close() })

	deadline := time.Now().Add(200 * time.Millisecond)
	for !ctrl.WatchdogMissed() {
		if time.Now().After(deadline) {
			t.Fatal("WatchdogMissed() never became true within 200ms")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestSim_WatchdogKickResetsTimer verifies CmdWatchdog prevents WatchdogMissed (REQ-SIM-005).
func TestSim_WatchdogKickResetsTimer(t *testing.T) {
	cfg := sim.Config{
		Zone:            rcp.ZoneFrontLeft,
		WatchdogTimeout: 50 * time.Millisecond,
		LatencyModel:    sim.LatencyConstant,
	}
	ctrl := sim.NewController(cfg)
	t.Cleanup(func() { _ = ctrl.Close() })

	ctx := context.Background()
	// Kick watchdog every 20ms for 150ms — should never miss.
	end := time.Now().Add(150 * time.Millisecond)
	for time.Now().Before(end) {
		_, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdWatchdog})
		if err != nil {
			t.Fatalf("CmdWatchdog Send: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if ctrl.WatchdogMissed() {
		t.Error("WatchdogMissed() = true despite regular kicks")
	}
}

// TestSim_WatchdogKickClearsMissed verifies a kick clears the missed flag (REQ-SIM-005).
func TestSim_WatchdogKickClearsMissed(t *testing.T) {
	cfg := sim.Config{
		Zone:            rcp.ZoneFrontLeft,
		WatchdogTimeout: 20 * time.Millisecond,
		LatencyModel:    sim.LatencyConstant,
	}
	ctrl := sim.NewController(cfg)
	t.Cleanup(func() { _ = ctrl.Close() })

	// Wait for miss.
	deadline := time.Now().Add(200 * time.Millisecond)
	for !ctrl.WatchdogMissed() {
		if time.Now().After(deadline) {
			t.Fatal("WatchdogMissed never became true")
		}
		time.Sleep(5 * time.Millisecond)
	}
	// Kick clears it.
	ctx := context.Background()
	_, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdWatchdog})
	if err != nil {
		t.Fatalf("CmdWatchdog: %v", err)
	}
	if ctrl.WatchdogMissed() {
		t.Error("WatchdogMissed() = true immediately after kick")
	}
}

// TestSim_StatusPublishedAtInterval verifies periodic Status publishing (REQ-SIM-006).
func TestSim_StatusPublishedAtInterval(t *testing.T) {
	cfg := sim.Config{
		Zone:           rcp.ZoneFrontLeft,
		StatusInterval: 10 * time.Millisecond,
		LatencyModel:   sim.LatencyConstant,
	}
	ctrl := sim.NewController(cfg)
	t.Cleanup(func() { _ = ctrl.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := ctrl.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// Expect at least 3 Status events within 100ms.
	count := 0
	deadline := time.After(100 * time.Millisecond)
	for count < 3 {
		select {
		case _, ok := <-ch:
			if !ok {
				t.Fatal("channel closed unexpectedly")
			}
			count++
		case <-deadline:
			t.Fatalf("only received %d Status events in 100ms (want >= 3)", count)
		}
	}
}

// TestSim_SubscriberClosedOnControllerClose verifies channel closes on Close() (REQ-SIM-007).
func TestSim_SubscriberClosedOnControllerClose(t *testing.T) {
	cfg := sim.Config{Zone: rcp.ZoneFrontLeft, StatusInterval: 5 * time.Millisecond}
	ctrl := sim.NewController(cfg)

	ctx := context.Background()
	ch, err := ctrl.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	_ = ctrl.Close()

	select {
	case <-ch:
		// may receive a buffered event before the close; drain below
	case <-time.After(200 * time.Millisecond):
		t.Error("subscriber channel not closed within 200ms of Close()")
		return
	}

	// Drain until closed.
	timeout := time.After(200 * time.Millisecond)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return // closed
			}
		case <-timeout:
			t.Error("subscriber channel still open after 200ms")
			return
		}
	}
}

// TestSim_SubscriberClosedOnContextCancel verifies channel closes on ctx cancel (REQ-SIM-007).
func TestSim_SubscriberClosedOnContextCancel(t *testing.T) {
	cfg := sim.Config{Zone: rcp.ZoneFrontLeft, StatusInterval: 5 * time.Millisecond}
	ctrl := sim.NewController(cfg)
	t.Cleanup(func() { _ = ctrl.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := ctrl.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	cancel()

	timeout := time.After(200 * time.Millisecond)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-timeout:
			t.Error("subscriber channel still open 200ms after context cancel")
			return
		}
	}
}

// TestSim_SubscribeAfterCloseReturnsError verifies Subscribe on closed ctrl (REQ-SIM-007).
func TestSim_SubscribeAfterCloseReturnsError(t *testing.T) {
	ctrl := sim.NewController(sim.Config{Zone: rcp.ZoneFrontLeft})
	_ = ctrl.Close()

	_, err := ctrl.Subscribe(context.Background())
	if err == nil {
		t.Error("Subscribe after Close should return error")
	}
	if !errors.Is(err, rcp.ErrClosed) {
		t.Errorf("err = %v, want ErrClosed", err)
	}
}

// TestSim_CloseIdempotent verifies Close() is safe to call twice (REQ-SIM-008).
func TestSim_CloseIdempotent(t *testing.T) {
	ctrl := sim.NewController(sim.DefaultConfig(rcp.ZoneFrontLeft))
	if err := ctrl.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := ctrl.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

// TestSim_DefaultConfig verifies DefaultConfig returns sensible ASIL-B values (REQ-SIM-008).
func TestSim_DefaultConfig(t *testing.T) {
	cfg := sim.DefaultConfig(rcp.ZoneFrontLeft)
	if cfg.Zone != rcp.ZoneFrontLeft {
		t.Errorf("Zone = %v, want ZoneFrontLeft", cfg.Zone)
	}
	if cfg.BaseLatency <= 0 {
		t.Errorf("BaseLatency = %v, want > 0", cfg.BaseLatency)
	}
	if cfg.WatchdogTimeout <= 0 {
		t.Errorf("WatchdogTimeout = %v, want > 0", cfg.WatchdogTimeout)
	}
	if cfg.StatusInterval <= 0 {
		t.Errorf("StatusInterval = %v, want > 0", cfg.StatusInterval)
	}
}

// TestSim_Concurrent verifies no data race under concurrent Sends (REQ-SIM-001).
func TestSim_Concurrent(t *testing.T) {
	cfg := sim.Config{Zone: rcp.ZoneFrontLeft, LatencyModel: sim.LatencyConstant}
	ctrl := sim.NewController(cfg)
	t.Cleanup(func() { _ = ctrl.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_, _ = ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdNoop})
		}()
	}
	wg.Wait()
}

// TestSim_PublishCloseRace stresses the publish/close path: many subscribers
// with a fast StatusInterval are created and torn down (via context cancel)
// while statusLoop publishes, then Close() races the same channels. Before the
// fix this panicked with "send on closed channel" (REQ-SIM-009). Run with -race.
func TestSim_PublishCloseRace(t *testing.T) {
	cfg := sim.Config{
		Zone:           rcp.ZoneFrontLeft,
		LatencyModel:   sim.LatencyConstant,
		StatusInterval: 200 * time.Microsecond,
	}
	ctrl := sim.NewController(cfg)

	var wg sync.WaitGroup
	const n = 50
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithCancel(context.Background())
			ch, err := ctrl.Subscribe(ctx)
			if err != nil {
				cancel()
				return
			}
			// Drain briefly so the buffer fills and publish keeps sending.
			go func() {
				for range ch { //nolint:revive // intentional drain
				}
			}()
			time.Sleep(time.Millisecond)
			cancel() // closes this subscriber concurrently with publish
		}()
	}
	time.Sleep(2 * time.Millisecond)
	_ = ctrl.Close() // races publish + subscriber closes
	wg.Wait()
}
