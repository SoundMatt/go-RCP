//fusa:test REQ-RL-001
//fusa:test REQ-RL-002
//fusa:test REQ-RL-003
//fusa:test REQ-RL-004
//fusa:test REQ-RL-005
//fusa:test REQ-RL-006
//fusa:test REQ-RL-007
//fusa:test REQ-RL-008

package ratelimit_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/mock"
	"github.com/SoundMatt/go-RCP/ratelimit"
)

// fakeClock provides a controllable clock for testing without real-time sleeps.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Now()}
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *fakeClock) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}

// TestRateLimit_BucketStartsFull verifies bucket starts full so burst sends succeed (REQ-RL-008).
func TestRateLimit_BucketStartsFull(t *testing.T) {
	fc := newFakeClock()
	inner := mock.NewController(rcp.ZoneFrontLeft, nil)
	cfg := ratelimit.Config{Rate: 1, Burst: 5, ExemptCritical: false}
	ctrl := ratelimit.NewControllerWithClock(inner, cfg, fc.Now)
	t.Cleanup(func() { _ = ctrl.Close() })

	ctx := context.Background()
	for i := 0; i < 5; i++ {
		_, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft})
		if err != nil {
			t.Errorf("send %d of %d failed: %v", i+1, 5, err)
		}
	}
}

// TestRateLimit_ErrBusyWhenExhausted verifies Send returns ErrBusy when bucket is empty (REQ-RL-003).
func TestRateLimit_ErrBusyWhenExhausted(t *testing.T) {
	fc := newFakeClock()
	inner := mock.NewController(rcp.ZoneFrontLeft, nil)
	cfg := ratelimit.Config{Rate: 1, Burst: 3, ExemptCritical: false}
	ctrl := ratelimit.NewControllerWithClock(inner, cfg, fc.Now)
	t.Cleanup(func() { _ = ctrl.Close() })

	ctx := context.Background()
	// Drain the bucket.
	for i := 0; i < 3; i++ {
		if _, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft}); err != nil {
			t.Fatalf("drain send %d: %v", i+1, err)
		}
	}
	// Next send must fail immediately.
	_, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft})
	if err == nil {
		t.Fatal("expected ErrBusy, got nil")
	}
	if !errors.Is(err, rcp.ErrBusy) {
		t.Errorf("err = %v, want ErrBusy", err)
	}
}

// TestRateLimit_RefillAllowsNextSend verifies tokens refill at Config.Rate (REQ-RL-001).
func TestRateLimit_RefillAllowsNextSend(t *testing.T) {
	fc := newFakeClock()
	// 10 tokens/sec, burst=1 → starts with 1 token; need 100ms advance for +1 token.
	inner := mock.NewController(rcp.ZoneFrontLeft, nil)
	cfg := ratelimit.Config{Rate: 10, Burst: 1, ExemptCritical: false}
	ctrl := ratelimit.NewControllerWithClock(inner, cfg, fc.Now)
	t.Cleanup(func() { _ = ctrl.Close() })

	ctx := context.Background()
	// Drain.
	if _, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft}); err != nil {
		t.Fatalf("initial send: %v", err)
	}
	// Exhausted.
	if _, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft}); !errors.Is(err, rcp.ErrBusy) {
		t.Fatalf("expected ErrBusy before refill")
	}
	// Advance 100ms → +1 token.
	fc.Advance(100 * time.Millisecond)
	if _, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft}); err != nil {
		t.Errorf("send after refill: %v", err)
	}
}

// TestRateLimit_BurstCapIsCapped verifies tokens never exceed Burst (REQ-RL-002).
func TestRateLimit_BurstCapIsCapped(t *testing.T) {
	fc := newFakeClock()
	inner := mock.NewController(rcp.ZoneFrontLeft, nil)
	cfg := ratelimit.Config{Rate: 10, Burst: 3, ExemptCritical: false}
	ctrl := ratelimit.NewControllerWithClock(inner, cfg, fc.Now)
	t.Cleanup(func() { _ = ctrl.Close() })

	ctx := context.Background()
	// Advance 10 seconds — would accumulate 100 tokens if uncapped.
	fc.Advance(10 * time.Second)

	// Should only be able to send Burst (3) times before ErrBusy.
	for i := 0; i < 3; i++ {
		if _, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft}); err != nil {
			t.Errorf("send %d: %v", i+1, err)
		}
	}
	if _, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft}); !errors.Is(err, rcp.ErrBusy) {
		t.Error("expected ErrBusy after Burst exhausted")
	}
}

// TestRateLimit_ExemptCritical verifies PriorityCritical bypasses bucket (REQ-RL-004).
func TestRateLimit_ExemptCritical(t *testing.T) {
	fc := newFakeClock()
	inner := mock.NewController(rcp.ZoneFrontLeft, nil)
	cfg := ratelimit.Config{Rate: 1, Burst: 1, ExemptCritical: true}
	ctrl := ratelimit.NewControllerWithClock(inner, cfg, fc.Now)
	t.Cleanup(func() { _ = ctrl.Close() })

	ctx := context.Background()
	// Drain with Normal.
	if _, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft, Priority: rcp.PriorityNormal}); err != nil {
		t.Fatalf("normal send: %v", err)
	}
	// Bucket exhausted for Normal.
	if _, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft, Priority: rcp.PriorityNormal}); !errors.Is(err, rcp.ErrBusy) {
		t.Fatal("expected ErrBusy for Normal when exhausted")
	}
	// Critical still goes through — 10 times.
	for i := 0; i < 10; i++ {
		_, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft, Priority: rcp.PriorityCritical})
		if err != nil {
			t.Errorf("critical send %d: %v", i+1, err)
		}
	}
}

// TestRateLimit_NonExemptCriticalIsThrottled verifies Critical is throttled when ExemptCritical=false (REQ-RL-004).
func TestRateLimit_NonExemptCriticalIsThrottled(t *testing.T) {
	fc := newFakeClock()
	inner := mock.NewController(rcp.ZoneFrontLeft, nil)
	cfg := ratelimit.Config{Rate: 1, Burst: 1, ExemptCritical: false}
	ctrl := ratelimit.NewControllerWithClock(inner, cfg, fc.Now)
	t.Cleanup(func() { _ = ctrl.Close() })

	ctx := context.Background()
	if _, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft, Priority: rcp.PriorityCritical}); err != nil {
		t.Fatalf("first critical send: %v", err)
	}
	_, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft, Priority: rcp.PriorityCritical})
	if !errors.Is(err, rcp.ErrBusy) {
		t.Errorf("expected ErrBusy when ExemptCritical=false, got %v", err)
	}
}

// TestRateLimit_Zone verifies Zone() delegates to inner (REQ-RL-005).
func TestRateLimit_Zone(t *testing.T) {
	inner := mock.NewController(rcp.ZoneRearRight, nil)
	ctrl := ratelimit.NewController(inner, ratelimit.DefaultConfig())
	t.Cleanup(func() { _ = ctrl.Close() })

	if got := ctrl.Zone(); got != rcp.ZoneRearRight {
		t.Errorf("Zone() = %v, want ZoneRearRight", got)
	}
}

// TestRateLimit_Subscribe verifies Subscribe() delegates to inner (REQ-RL-005).
func TestRateLimit_Subscribe(t *testing.T) {
	inner := mock.NewController(rcp.ZoneFrontLeft, nil)
	ctrl := ratelimit.NewController(inner, ratelimit.DefaultConfig())
	t.Cleanup(func() { _ = ctrl.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := ctrl.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	_ = ch
}

// TestRateLimit_Close_Idempotent verifies Close() is safe to call multiple times (REQ-RL-006).
func TestRateLimit_Close_Idempotent(t *testing.T) {
	ctrl := ratelimit.NewController(mock.NewController(rcp.ZoneFrontLeft, nil), ratelimit.DefaultConfig())
	if err := ctrl.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := ctrl.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

// TestRateLimit_Close_RejectsNewSends verifies Send after Close returns ErrClosed (REQ-RL-006).
func TestRateLimit_Close_RejectsNewSends(t *testing.T) {
	ctrl := ratelimit.NewController(mock.NewController(rcp.ZoneFrontLeft, nil), ratelimit.DefaultConfig())
	_ = ctrl.Close()

	_, err := ctrl.Send(context.Background(), &rcp.Command{Zone: rcp.ZoneFrontLeft})
	if err == nil {
		t.Error("expected error after Close, got nil")
	}
	if !errors.Is(err, rcp.ErrClosed) {
		t.Errorf("err = %v, want ErrClosed", err)
	}
}

// TestRateLimit_Concurrent verifies concurrent Sends don't race (REQ-RL-007).
func TestRateLimit_Concurrent(t *testing.T) {
	inner := mock.NewController(rcp.ZoneFrontLeft, nil)
	cfg := ratelimit.Config{Rate: 1e9, Burst: 1000, ExemptCritical: false} // effectively unlimited
	ctrl := ratelimit.NewController(inner, cfg)
	t.Cleanup(func() { _ = ctrl.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_, _ = ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft})
		}()
	}
	wg.Wait()
}

// TestRateLimit_DefaultConfig verifies DefaultConfig returns sensible ASIL-B values (REQ-RL-008).
func TestRateLimit_DefaultConfig(t *testing.T) {
	cfg := ratelimit.DefaultConfig()
	if cfg.Rate <= 0 {
		t.Errorf("DefaultConfig.Rate = %v, want > 0", cfg.Rate)
	}
	if cfg.Burst <= 0 {
		t.Errorf("DefaultConfig.Burst = %d, want > 0", cfg.Burst)
	}
	if !cfg.ExemptCritical {
		t.Error("DefaultConfig.ExemptCritical = false, want true")
	}
}
