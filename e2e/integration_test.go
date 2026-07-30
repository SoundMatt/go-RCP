//fusa:test REQ-CRC-009

package e2e_test

import (
	"errors"
	"testing"
	"time"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/e2e"
	"github.com/SoundMatt/go-RCP/gpio"
	"github.com/SoundMatt/go-RCP/request"
	"github.com/SoundMatt/go-RCP/server"
)

// newGPIOEndpoint mirrors request_test's own helper of the same name
// (e2e_test is a separate external test package and cannot reuse it).
func newGPIOEndpoint(t *testing.T, cfg gpio.Config) (*gpio.Endpoint, avtp.StreamID, avtp.ByteBusID) {
	t.Helper()
	root := testStream()
	s := server.NewServer()
	if err := s.ClaimRoot(root); err != nil {
		t.Fatalf("ClaimRoot: %v", err)
	}
	addr := avtp.ByteBusID(1)
	if err := s.AddEndpoint(root, addr, gpio.EndpointType); err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}
	ep := gpio.NewEndpoint(s, addr)
	if err := ep.Configure(root, cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	return ep, root, addr
}

// TestIntegration_WatchdogDrivenSafeStateAndPurge wires a e2e.Supervisor
// into a request.Dispatcher via SetSafeStateCheck/PurgeNonSafety — the
// integration pattern doc.go documents — and checks the end-to-end
// behavior ROADMAP.md Milestone 50 describes: an ordinary pending request
// gets cleared once the stream's watchdog trips, while a safety-request
// ticket submitted on the same Dispatcher survives that purge and only
// executes once the endpoint is (per the same Supervisor) actually judged
// to be in its configured safe state (REQ-CRC-009).
func TestIntegration_WatchdogDrivenSafeStateAndPurge(t *testing.T) {
	ep, root, addr := newGPIOEndpoint(t, gpio.Config{PinCount: 4, Direction: 0b1111})
	seq := request.NewSequencer()
	d := request.NewDispatcher(ep, addr, seq, nil)

	clock := &fakeClock{t: time.Unix(0, 0)}
	sup := e2e.NewSupervisorWithClock(e2e.StreamConfig{Timeout: 100 * time.Millisecond}, clock.now)
	d.SetSafeStateCheck(sup.CheckFunc())

	// Establish a normal, recent arrival: the watchdog has not tripped yet.
	if err := sup.Observe(root, 1); err != nil {
		t.Fatalf("Observe: %v", err)
	}

	// An ordinary Timed ticket, not yet due.
	timedBody := request.EncodeTimed(1_000_000, acf.FlagWrite, gpio.EncodeWriteRequest(gpio.SemanticOr, 0b0001))
	timedID, err := d.Submit(root, acf.Message{Kind: acf.KindLong, ByteBusID: addr, TransactionNum: 1, Control: acf.FlagWrite, Body: timedBody})
	if err != nil {
		t.Fatalf("Submit(timed): %v", err)
	}

	// A safety-request CompoundWaitSafety ticket.
	cond := request.Conditional{Sequencer: 1, Op: request.CompareEqual, Operand: 0, AdvanceOnMatch: 1}
	safeID, err := d.Submit(root, acf.Message{Kind: acf.KindLong, ByteBusID: addr, TransactionNum: 2, Body: request.EncodeCompoundWaitSafety(cond)})
	if err != nil {
		t.Fatalf("Submit(safety): %v", err)
	}

	// Before the watchdog trips: neither ticket resolves. The safety ticket
	// stays pending because the endpoint is not (yet) in its configured
	// safe state; the timed ticket stays pending because it is not yet due.
	d.Pump(0)
	if sup.InSafeState(root) {
		t.Fatalf("InSafeState = true before Timeout elapsed, want false")
	}
	if _, pollErr := d.Response(safeID); !errors.Is(pollErr, request.ErrPending) {
		t.Fatalf("Response(safety) before safe state = %v, want ErrPending", pollErr)
	}
	if _, pollErr := d.Response(timedID); !errors.Is(pollErr, request.ErrPending) {
		t.Fatalf("Response(timed) before due = %v, want ErrPending", pollErr)
	}

	// The stream goes quiet past its configured Timeout: the watchdog trips.
	clock.advance(200 * time.Millisecond)
	if !sup.InSafeState(root) {
		t.Fatalf("InSafeState = false after Timeout elapsed, want true")
	}

	// A caller wires that trip to a purge, per doc.go's documented pattern.
	cleared := d.PurgeNonSafety()
	if len(cleared) != 1 || cleared[0] != timedID {
		t.Fatalf("PurgeNonSafety() = %v, want [%d] (only the ordinary timed ticket)", cleared, timedID)
	}
	if _, pollErr := d.Response(timedID); !errors.Is(pollErr, request.ErrPurgedByWatchdog) {
		t.Errorf("Response(timed) after purge = %v, want ErrPurgedByWatchdog", pollErr)
	}
	if _, pollErr := d.Response(safeID); !errors.Is(pollErr, request.ErrPending) {
		t.Errorf("Response(safety) immediately after purge = %v, want still ErrPending (must survive)", pollErr)
	}

	// The endpoint is now (per the same Supervisor) in its configured safe
	// state, so the surviving safety ticket may finally execute.
	d.Pump(0)
	resp, err := d.Response(safeID)
	if err != nil {
		t.Fatalf("Response(safety) after safe state reached: %v", err)
	}
	res, _, err := request.DecodeConditionalResponse(resp.Body)
	if err != nil || !res.Matched {
		t.Errorf("DecodeConditionalResponse = (%+v, %v), want Matched=true", res, err)
	}
}
