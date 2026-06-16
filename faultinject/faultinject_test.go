//fusa:test REQ-FI-001
//fusa:test REQ-FI-002
//fusa:test REQ-FI-003
//fusa:test REQ-FI-004
//fusa:test REQ-FI-005
//fusa:test REQ-FI-006
//fusa:test REQ-FI-007
//fusa:test REQ-FI-008

package faultinject_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/faultinject"
	"github.com/SoundMatt/go-RCP/mock"
)

func newCtrl() (*faultinject.Controller, *mock.Controller) {
	inner := mock.NewController(rcp.ZoneFrontLeft, nil)
	return faultinject.NewController(inner), inner
}

// TestFaultInject_NoRule_PassThrough verifies clean path without rules (REQ-FI-008).
func TestFaultInject_NoRule_PassThrough(t *testing.T) {
	ctrl, _ := newCtrl()
	t.Cleanup(func() { _ = ctrl.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	resp, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Status != rcp.StatusOK {
		t.Errorf("Status = %v, want OK", resp.Status)
	}
}

// TestFaultInject_FaultDrop returns error without inner Send (REQ-FI-001).
func TestFaultInject_FaultDrop(t *testing.T) {
	ctrl, _ := newCtrl()
	t.Cleanup(func() { _ = ctrl.Close() })

	ctrl.AddRule(faultinject.Rule{Type: faultinject.FaultDrop, Count: -1})

	_, err := ctrl.Send(context.Background(), &rcp.Command{Zone: rcp.ZoneFrontLeft})
	if err == nil {
		t.Fatal("expected error from FaultDrop, got nil")
	}
}

// TestFaultInject_FaultSlow adds latency then forwards (REQ-FI-002).
func TestFaultInject_FaultSlow(t *testing.T) {
	ctrl, _ := newCtrl()
	t.Cleanup(func() { _ = ctrl.Close() })

	ctrl.AddRule(faultinject.Rule{Type: faultinject.FaultSlow, Latency: 30 * time.Millisecond, Count: -1})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	start := time.Now()
	resp, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Status != rcp.StatusOK {
		t.Errorf("Status = %v, want OK", resp.Status)
	}
	if elapsed := time.Since(start); elapsed < 25*time.Millisecond {
		t.Errorf("elapsed %v, want >= 25ms (FaultSlow latency=30ms)", elapsed)
	}
}

// TestFaultInject_FaultSlow_Timeout verifies ctx cancel during slow fault (REQ-FI-002).
func TestFaultInject_FaultSlow_Timeout(t *testing.T) {
	ctrl, _ := newCtrl()
	t.Cleanup(func() { _ = ctrl.Close() })

	ctrl.AddRule(faultinject.Rule{Type: faultinject.FaultSlow, Latency: 200 * time.Millisecond, Count: -1})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft})
	if !errors.Is(err, rcp.ErrTimeout) {
		t.Errorf("err = %v, want ErrTimeout", err)
	}
}

// TestFaultInject_FaultError returns StatusError without inner Send (REQ-FI-003).
func TestFaultInject_FaultError(t *testing.T) {
	ctrl, _ := newCtrl()
	t.Cleanup(func() { _ = ctrl.Close() })

	ctrl.AddRule(faultinject.Rule{Type: faultinject.FaultError, Count: -1})

	resp, err := ctrl.Send(context.Background(), &rcp.Command{Zone: rcp.ZoneFrontLeft})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != rcp.StatusError {
		t.Errorf("Status = %v, want StatusError", resp.Status)
	}
}

// TestFaultInject_FaultTimeout returns ErrTimeout without inner Send (REQ-FI-004).
func TestFaultInject_FaultTimeout(t *testing.T) {
	ctrl, _ := newCtrl()
	t.Cleanup(func() { _ = ctrl.Close() })

	ctrl.AddRule(faultinject.Rule{Type: faultinject.FaultTimeout, Count: -1})

	_, err := ctrl.Send(context.Background(), &rcp.Command{Zone: rcp.ZoneFrontLeft})
	if !errors.Is(err, rcp.ErrTimeout) {
		t.Errorf("err = %v, want ErrTimeout", err)
	}
}

// TestFaultInject_CountedRule_AutoExpires verifies Count>0 rules expire (REQ-FI-005).
func TestFaultInject_CountedRule_AutoExpires(t *testing.T) {
	ctrl, _ := newCtrl()
	t.Cleanup(func() { _ = ctrl.Close() })

	ctrl.AddRule(faultinject.Rule{Type: faultinject.FaultError, Count: 2})

	ctx := context.Background()
	cmd := &rcp.Command{Zone: rcp.ZoneFrontLeft}

	// First two sends hit the fault rule.
	for i := 0; i < 2; i++ {
		resp, err := ctrl.Send(ctx, cmd)
		if err != nil {
			t.Fatalf("send %d: unexpected error: %v", i+1, err)
		}
		if resp.Status != rcp.StatusError {
			t.Errorf("send %d: Status = %v, want StatusError", i+1, resp.Status)
		}
	}
	// Third send: rule expired, goes to inner → StatusOK.
	resp, err := ctrl.Send(ctx, cmd)
	if err != nil {
		t.Fatalf("send 3 after expiry: %v", err)
	}
	if resp.Status != rcp.StatusOK {
		t.Errorf("send 3: Status = %v after rule expired, want OK", resp.Status)
	}
}

// TestFaultInject_ClearRules removes all rules (REQ-FI-006).
func TestFaultInject_ClearRules(t *testing.T) {
	ctrl, _ := newCtrl()
	t.Cleanup(func() { _ = ctrl.Close() })

	ctrl.AddRule(faultinject.Rule{Type: faultinject.FaultDrop, Count: -1})
	ctrl.ClearRules()

	resp, err := ctrl.Send(context.Background(), &rcp.Command{Zone: rcp.ZoneFrontLeft})
	if err != nil {
		t.Fatalf("Send after ClearRules: %v", err)
	}
	if resp.Status != rcp.StatusOK {
		t.Errorf("Status = %v after ClearRules, want OK", resp.Status)
	}
}

// TestFaultInject_Concurrent verifies no data race under concurrent Sends (REQ-FI-007).
func TestFaultInject_Concurrent(t *testing.T) {
	ctrl, _ := newCtrl()
	t.Cleanup(func() { _ = ctrl.Close() })
	ctrl.AddRule(faultinject.Rule{Type: faultinject.FaultError, Count: -1})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const n = 30
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_, _ = ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft})
		}()
	}
	// Concurrently mutate rules mid-flight.
	go func() {
		time.Sleep(5 * time.Millisecond)
		ctrl.ClearRules()
	}()
	wg.Wait()
}

// TestFaultInject_Zone verifies Zone() delegates to inner (REQ-FI-008).
func TestFaultInject_Zone(t *testing.T) {
	inner := mock.NewController(rcp.ZoneRearRight, nil)
	ctrl := faultinject.NewController(inner)
	t.Cleanup(func() { _ = ctrl.Close() })

	if got := ctrl.Zone(); got != rcp.ZoneRearRight {
		t.Errorf("Zone() = %v, want ZoneRearRight", got)
	}
}

// TestFaultInject_Subscribe delegates to inner (REQ-FI-008).
func TestFaultInject_Subscribe(t *testing.T) {
	ctrl, _ := newCtrl()
	t.Cleanup(func() { _ = ctrl.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := ctrl.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	_ = ch
}

// TestFaultInject_Close_Idempotent verifies Close() is safe to call twice (REQ-FI-008).
func TestFaultInject_Close_Idempotent(t *testing.T) {
	ctrl, _ := newCtrl()
	if err := ctrl.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := ctrl.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

// TestFaultInject_Close_RejectsSend verifies Send after Close returns ErrClosed (REQ-FI-008).
func TestFaultInject_Close_RejectsSend(t *testing.T) {
	ctrl, _ := newCtrl()
	_ = ctrl.Close()

	_, err := ctrl.Send(context.Background(), &rcp.Command{Zone: rcp.ZoneFrontLeft})
	if !errors.Is(err, rcp.ErrClosed) {
		t.Errorf("err = %v, want ErrClosed", err)
	}
}
