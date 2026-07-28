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

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/ratelimit"
	"github.com/SoundMatt/go-RCP/request"
	"github.com/SoundMatt/go-RCP/server"
	"github.com/SoundMatt/go-RCP/udp"
)

type stubHandler struct{}

func (stubHandler) HandleRequest(_ avtp.StreamID, req acf.Message) (acf.Message, error) {
	return acf.Message{
		Kind:           req.Kind,
		ByteBusID:      req.ByteBusID,
		TransactionNum: req.TransactionNum,
		Control:        acf.FlagResponse | (req.Control & (acf.FlagRead | acf.FlagWrite)),
		Body:           req.Body,
	}, nil
}

const (
	endpointA = avtp.ByteBusID(1)
	endpointB = avtp.ByteBusID(2)
)

func newHarness(t *testing.T) *udp.Controller {
	t.Helper()
	router := udp.NewRouter(udp.NewEP0Handler(server.NewServer()), false)
	for _, addr := range []avtp.ByteBusID{endpointA, endpointB} {
		if err := router.Register(addr, stubHandler{}); err != nil {
			t.Fatalf("Register: %v", err)
		}
	}
	srv, err := udp.NewServer(avtp.NewStreamID([6]byte{0x02, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE}, 1), "127.0.0.1:0", router)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	ctrl, err := udp.NewController(avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55}, 1), srv.Addr())
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	t.Cleanup(func() { _ = ctrl.Close() })
	return ctrl
}

// fakeClock is a manually-advanced clock for deterministic tests.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// TestRateLimit_BucketStartsFull a new Controller's bucket starts full and
// DefaultConfig returns positive ASIL-B values (REQ-RL-008).
func TestRateLimit_BucketStartsFull(t *testing.T) {
	cfg := ratelimit.DefaultConfig()
	if cfg.Rate <= 0 || cfg.Burst <= 0 || !cfg.ExemptCancellation {
		t.Fatalf("DefaultConfig = %+v, want positive Rate/Burst and ExemptCancellation=true", cfg)
	}

	inner := newHarness(t)
	clock := &fakeClock{t: time.Now()}
	ctrl := ratelimit.NewControllerWithClock(inner, ratelimit.Config{Rate: 1, Burst: 3}, clock.now)

	for i := 0; i < 3; i++ {
		if _, err := ctrl.Read(context.Background(), endpointA); err != nil {
			t.Fatalf("Read %d: %v", i, err)
		}
	}
}

// TestRateLimit_ExhaustedReturnsErrBusy Request returns ErrBusy immediately
// once the bucket is exhausted (REQ-RL-003).
func TestRateLimit_ExhaustedReturnsErrBusy(t *testing.T) {
	inner := newHarness(t)
	clock := &fakeClock{t: time.Now()}
	ctrl := ratelimit.NewControllerWithClock(inner, ratelimit.Config{Rate: 1, Burst: 1}, clock.now)

	if _, err := ctrl.Read(context.Background(), endpointA); err != nil {
		t.Fatalf("first Read: %v", err)
	}
	if _, err := ctrl.Read(context.Background(), endpointA); !errors.Is(err, ratelimit.ErrBusy) {
		t.Errorf("second Read err = %v, want ErrBusy", err)
	}
}

// TestRateLimit_RefillsOverTime the bucket refills at Config.Rate tokens per
// second (REQ-RL-001) capped at Config.Burst (REQ-RL-002).
func TestRateLimit_RefillsOverTime(t *testing.T) {
	inner := newHarness(t)
	clock := &fakeClock{t: time.Now()}
	ctrl := ratelimit.NewControllerWithClock(inner, ratelimit.Config{Rate: 10, Burst: 2}, clock.now)

	if _, err := ctrl.Read(context.Background(), endpointA); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if _, err := ctrl.Read(context.Background(), endpointA); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if _, err := ctrl.Read(context.Background(), endpointA); !errors.Is(err, ratelimit.ErrBusy) {
		t.Fatalf("Read before refill err = %v, want ErrBusy", err)
	}

	// 200ms at 10 tok/s refills 2 tokens, capped at Burst=2.
	clock.advance(200 * time.Millisecond)
	if _, err := ctrl.Read(context.Background(), endpointA); err != nil {
		t.Errorf("Read after refill: %v", err)
	}
	if _, err := ctrl.Read(context.Background(), endpointA); err != nil {
		t.Errorf("second Read after refill: %v", err)
	}
	if _, err := ctrl.Read(context.Background(), endpointA); !errors.Is(err, ratelimit.ErrBusy) {
		t.Errorf("Read beyond cap err = %v, want ErrBusy (refill capped at Burst)", err)
	}
}

// TestRateLimit_PerEndpoint each endpoint tracks an independent bucket, so
// exhausting one does not starve another (REQ-RL-001, re-keying).
func TestRateLimit_PerEndpoint(t *testing.T) {
	inner := newHarness(t)
	clock := &fakeClock{t: time.Now()}
	ctrl := ratelimit.NewControllerWithClock(inner, ratelimit.Config{Rate: 1, Burst: 1}, clock.now)

	if _, err := ctrl.Read(context.Background(), endpointA); err != nil {
		t.Fatalf("endpointA Read: %v", err)
	}
	if _, err := ctrl.Read(context.Background(), endpointA); !errors.Is(err, ratelimit.ErrBusy) {
		t.Fatalf("endpointA second Read err = %v, want ErrBusy", err)
	}
	if _, err := ctrl.Read(context.Background(), endpointB); err != nil {
		t.Errorf("endpointB Read = %v, want success (independent bucket)", err)
	}
}

// TestRateLimit_ExemptCancellation a cancellation-kind request bypasses the
// bucket when ExemptCancellation is true (REQ-RL-004).
func TestRateLimit_ExemptCancellation(t *testing.T) {
	inner := newHarness(t)
	clock := &fakeClock{t: time.Now()}
	ctrl := ratelimit.NewControllerWithClock(inner, ratelimit.Config{Rate: 1, Burst: 1, ExemptCancellation: true}, clock.now)

	// Exhaust the bucket with a plain read.
	if _, err := ctrl.Read(context.Background(), endpointA); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if _, err := ctrl.Read(context.Background(), endpointA); !errors.Is(err, ratelimit.ErrBusy) {
		t.Fatalf("expected ErrBusy before exemption check")
	}

	// A cancellation-kind request still bypasses the exhausted bucket.
	if _, err := ctrl.Request(context.Background(), endpointA, acf.FlagWrite, nil, request.KindCancelAll); err != nil {
		t.Errorf("cancellation Request = %v, want bypass", err)
	}

	// With ExemptCancellation false, the same request kind is subject to
	// the bucket like any other.
	clock2 := &fakeClock{t: time.Now()}
	ctrl2 := ratelimit.NewControllerWithClock(inner, ratelimit.Config{Rate: 1, Burst: 1, ExemptCancellation: false}, clock2.now)
	if _, err := ctrl2.Request(context.Background(), endpointA, acf.FlagWrite, nil, request.KindCancelAll); err != nil {
		t.Fatalf("first cancellation Request: %v", err)
	}
	if _, err := ctrl2.Request(context.Background(), endpointA, acf.FlagWrite, nil, request.KindCancelAll); !errors.Is(err, ratelimit.ErrBusy) {
		t.Errorf("second cancellation Request err = %v, want ErrBusy (not exempt)", err)
	}
}

// TestRateLimit_StreamID delegates to the inner controller (REQ-RL-005).
func TestRateLimit_StreamID(t *testing.T) {
	inner := newHarness(t)
	ctrl := ratelimit.NewController(inner, ratelimit.DefaultConfig())
	if got, want := ctrl.StreamID(), inner.StreamID(); got != want {
		t.Errorf("StreamID() = %v, want %v", got, want)
	}
}

// TestRateLimit_Close_Idempotent Close is safe to call multiple times and
// Request after Close returns ErrClosed (REQ-RL-006).
func TestRateLimit_Close_Idempotent(t *testing.T) {
	inner := newHarness(t)
	ctrl := ratelimit.NewController(inner, ratelimit.DefaultConfig())
	if err := ctrl.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := ctrl.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
	if _, err := ctrl.Read(context.Background(), endpointA); !errors.Is(err, udp.ErrClosed) {
		t.Errorf("Read after Close err = %v, want ErrClosed", err)
	}
}

// TestRateLimit_Concurrent verifies no race under concurrent Sends
// (REQ-RL-007).
func TestRateLimit_Concurrent(t *testing.T) {
	inner := newHarness(t)
	ctrl := ratelimit.NewController(inner, ratelimit.Config{Rate: 1000, Burst: 1000})

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_, _ = ctrl.Read(context.Background(), endpointA)
		}()
	}
	wg.Wait()
}
