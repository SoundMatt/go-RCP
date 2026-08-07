//fusa:test REQ-FI-001
//fusa:test REQ-FI-002
//fusa:test REQ-FI-003
//fusa:test REQ-FI-004
//fusa:test REQ-FI-005
//fusa:test REQ-FI-006
//fusa:test REQ-FI-007
//fusa:test REQ-FI-008
//fusa:test REQ-FI-009

package faultinject_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/discovery"
	"github.com/SoundMatt/go-RCP/v9/e2e"
	"github.com/SoundMatt/go-RCP/v9/faultinject"
	"github.com/SoundMatt/go-RCP/v9/request"
)

func testStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55}, 1)
}

type countingHandler struct {
	calls atomic.Int32
}

func (c *countingHandler) HandleRequest(_ avtp.StreamID, _ acf.Message) (acf.Message, error) {
	c.calls.Add(1)
	return acf.Message{Control: acf.FlagResponse}, nil
}

// TestHandler_NoRules_Passthrough verifies HandleRequest delegates directly
// when no rules are configured (REQ-FI-001).
func TestHandler_NoRules_Passthrough(t *testing.T) {
	inner := &countingHandler{}
	h := faultinject.NewHandler(inner)
	if _, err := h.HandleRequest(testStream(), acf.Message{}); err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	if inner.calls.Load() != 1 {
		t.Errorf("inner.calls = %d, want 1", inner.calls.Load())
	}
}

// TestHandler_FaultDrop_DoesNotReachInner verifies FaultDrop returns an
// error without calling the wrapped Handler (REQ-FI-002).
func TestHandler_FaultDrop_DoesNotReachInner(t *testing.T) {
	inner := &countingHandler{}
	h := faultinject.NewHandler(inner)
	h.AddRule(faultinject.Rule{Type: faultinject.FaultDrop, Count: -1})

	_, err := h.HandleRequest(testStream(), acf.Message{})
	if err == nil {
		t.Fatal("expected error for FaultDrop")
	}
	if inner.calls.Load() != 0 {
		t.Errorf("inner.calls = %d, want 0", inner.calls.Load())
	}
}

// TestHandler_FaultSlow_DelaysThenForwards verifies FaultSlow sleeps at
// least Latency before delegating to the wrapped Handler (REQ-FI-003).
func TestHandler_FaultSlow_DelaysThenForwards(t *testing.T) {
	inner := &countingHandler{}
	h := faultinject.NewHandler(inner)
	h.AddRule(faultinject.Rule{Type: faultinject.FaultSlow, Latency: 20 * time.Millisecond, Count: -1})

	start := time.Now()
	if _, err := h.HandleRequest(testStream(), acf.Message{}); err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 20*time.Millisecond {
		t.Errorf("elapsed = %v, want >= 20ms", elapsed)
	}
	if inner.calls.Load() != 1 {
		t.Errorf("inner.calls = %d, want 1", inner.calls.Load())
	}
}

// TestHandler_FaultCRCFailure_ReturnsE2EError verifies FaultCRCFailure
// wraps e2e.ErrCRCMismatch (REQ-FI-004).
func TestHandler_FaultCRCFailure_ReturnsE2EError(t *testing.T) {
	inner := &countingHandler{}
	h := faultinject.NewHandler(inner)
	h.AddRule(faultinject.Rule{Type: faultinject.FaultCRCFailure, Count: -1})

	_, err := h.HandleRequest(testStream(), acf.Message{})
	if !errors.Is(err, e2e.ErrCRCMismatch) {
		t.Errorf("err = %v, want e2e.ErrCRCMismatch", err)
	}
	if inner.calls.Load() != 0 {
		t.Errorf("inner.calls = %d, want 0", inner.calls.Load())
	}
}

// TestHandler_FaultSafeStateEntry_ReturnsPurgedByWatchdog verifies
// FaultSafeStateEntry wraps request.ErrPurgedByWatchdog (REQ-FI-005).
func TestHandler_FaultSafeStateEntry_ReturnsPurgedByWatchdog(t *testing.T) {
	inner := &countingHandler{}
	h := faultinject.NewHandler(inner)
	h.AddRule(faultinject.Rule{Type: faultinject.FaultSafeStateEntry, Count: -1})

	_, err := h.HandleRequest(testStream(), acf.Message{})
	if !errors.Is(err, request.ErrPurgedByWatchdog) {
		t.Errorf("err = %v, want request.ErrPurgedByWatchdog", err)
	}
}

// TestHandler_FaultDiscoveryClaimTimeout_ReturnsNotConfigurationClaimant
// verifies FaultDiscoveryClaimTimeout wraps
// discovery.ErrNotConfigurationClaimant (REQ-FI-006).
func TestHandler_FaultDiscoveryClaimTimeout_ReturnsNotConfigurationClaimant(t *testing.T) {
	inner := &countingHandler{}
	h := faultinject.NewHandler(inner)
	h.AddRule(faultinject.Rule{Type: faultinject.FaultDiscoveryClaimTimeout, Count: -1})

	_, err := h.HandleRequest(testStream(), acf.Message{})
	if !errors.Is(err, discovery.ErrNotConfigurationClaimant) {
		t.Errorf("err = %v, want discovery.ErrNotConfigurationClaimant", err)
	}
}

// TestHandler_FaultCancellation_ReturnsTicketCancelled verifies
// FaultCancellation wraps request.ErrTicketCancelled (REQ-FI-007).
func TestHandler_FaultCancellation_ReturnsTicketCancelled(t *testing.T) {
	inner := &countingHandler{}
	h := faultinject.NewHandler(inner)
	h.AddRule(faultinject.Rule{Type: faultinject.FaultCancellation, Count: -1})

	_, err := h.HandleRequest(testStream(), acf.Message{})
	if !errors.Is(err, request.ErrTicketCancelled) {
		t.Errorf("err = %v, want request.ErrTicketCancelled", err)
	}
}

// TestHandler_CountBasedRule_AutoExpires verifies a Count>0 rule fires
// exactly Count times, then falls through to the wrapped Handler, and
// ClearRules removes everything immediately (REQ-FI-008).
func TestHandler_CountBasedRule_AutoExpires(t *testing.T) {
	inner := &countingHandler{}
	h := faultinject.NewHandler(inner)
	h.AddRule(faultinject.Rule{Type: faultinject.FaultDrop, Count: 2})

	for i := range 2 {
		if _, err := h.HandleRequest(testStream(), acf.Message{}); err == nil {
			t.Fatalf("call %d: expected FaultDrop error", i)
		}
	}
	// Rule exhausted; third call should pass through.
	if _, err := h.HandleRequest(testStream(), acf.Message{}); err != nil {
		t.Fatalf("call after exhaustion: %v", err)
	}
	if inner.calls.Load() != 1 {
		t.Errorf("inner.calls = %d, want 1", inner.calls.Load())
	}

	h.AddRule(faultinject.Rule{Type: faultinject.FaultDrop, Count: -1})
	h.ClearRules()
	if _, err := h.HandleRequest(testStream(), acf.Message{}); err != nil {
		t.Fatalf("after ClearRules: %v", err)
	}
}

// TestHandler_Concurrent verifies concurrent HandleRequest and AddRule
// calls are data-race free (REQ-FI-009).
func TestHandler_Concurrent(t *testing.T) {
	inner := &countingHandler{}
	h := faultinject.NewHandler(inner)
	h.AddRule(faultinject.Rule{Type: faultinject.FaultSlow, Latency: 0, Count: -1})

	var wg sync.WaitGroup
	for range 30 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = h.HandleRequest(testStream(), acf.Message{})
		}()
	}
	wg.Wait()
}
