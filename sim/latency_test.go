//fusa:test REQ-SIM-007
//fusa:test REQ-SIM-008
//fusa:test REQ-SIM-009
//fusa:test REQ-SIM-010

package sim_test

import (
	"sync"
	"testing"
	"time"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/sim"
)

type stubHandler struct {
	body []byte
}

func (h *stubHandler) HandleRequest(_ avtp.StreamID, req acf.Message) (acf.Message, error) {
	return acf.Message{
		Kind:           req.Kind,
		ByteBusID:      req.ByteBusID,
		TransactionNum: req.TransactionNum,
		Control:        acf.FlagResponse,
		Body:           h.body,
	}, nil
}

// TestLatencyHandler_ConstantModel_DelaysAtLeastBase verifies
// LatencyConstant adds at least Base delay before delegating (REQ-SIM-007).
func TestLatencyHandler_ConstantModel_DelaysAtLeastBase(t *testing.T) {
	h := sim.NewLatencyHandler(&stubHandler{body: []byte("ok")}, 20*time.Millisecond, 0, sim.LatencyConstant)
	start := time.Now()
	resp, err := h.HandleRequest(testStream(), acf.Message{ByteBusID: 1})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	if elapsed < 20*time.Millisecond {
		t.Errorf("elapsed = %v, want >= 20ms", elapsed)
	}
	if string(resp.Body) != "ok" {
		t.Errorf("resp.Body = %q, want %q", resp.Body, "ok")
	}
}

// TestLatencyHandler_JitterModel_StaysWithinBounds verifies LatencyJitter
// adds no more than Base+Jitter delay (REQ-SIM-008).
func TestLatencyHandler_JitterModel_StaysWithinBounds(t *testing.T) {
	h := sim.NewLatencyHandler(&stubHandler{}, 5*time.Millisecond, 10*time.Millisecond, sim.LatencyJitter)
	for range 5 {
		start := time.Now()
		if _, err := h.HandleRequest(testStream(), acf.Message{ByteBusID: 1}); err != nil {
			t.Fatalf("HandleRequest: %v", err)
		}
		elapsed := time.Since(start)
		if elapsed < 5*time.Millisecond {
			t.Errorf("elapsed = %v, want >= base 5ms", elapsed)
		}
		if elapsed > 200*time.Millisecond {
			t.Errorf("elapsed = %v, suspiciously far past base+jitter (15ms)", elapsed)
		}
	}
}

// TestLatencyHandler_ZeroLatency_NoDelay verifies a zero Base/Jitter
// configuration adds negligible delay (REQ-SIM-009).
func TestLatencyHandler_ZeroLatency_NoDelay(t *testing.T) {
	h := sim.NewLatencyHandler(&stubHandler{}, 0, 0, sim.LatencyConstant)
	start := time.Now()
	if _, err := h.HandleRequest(testStream(), acf.Message{ByteBusID: 1}); err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Errorf("elapsed = %v, want near-zero", elapsed)
	}
}

// TestLatencyHandler_Concurrent verifies concurrent HandleRequest calls are
// data-race free (REQ-SIM-010).
func TestLatencyHandler_Concurrent(t *testing.T) {
	h := sim.NewLatencyHandler(&stubHandler{}, time.Millisecond, time.Millisecond, sim.LatencyJitter)
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = h.HandleRequest(testStream(), acf.Message{ByteBusID: 1})
		}()
	}
	wg.Wait()
}
