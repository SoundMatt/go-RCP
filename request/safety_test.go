//fusa:test REQ-REQ-022
//fusa:test REQ-REQ-023
//fusa:test REQ-REQ-024
//fusa:test REQ-REQ-025

package request_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/gpio"
	"github.com/SoundMatt/go-RCP/request"
)

// TestKind_SafetyTagging checks IsSafety/Base round-trip against each other,
// Valid recognizes exactly the three defined safety-request ("MSB-set")
// Kind values and rejects the flag applied to any other base Kind, and
// Priority ranks a safety-request Kind identically to its Base kind
// (ROADMAP.md Milestone 50; REQ-REQ-022).
func TestKind_SafetyTagging(t *testing.T) {
	safetyKinds := []request.Kind{
		request.KindCompoundSafety,
		request.KindCompoundWaitSafety,
		request.KindTriggeredSafety,
	}
	baseKinds := []request.Kind{
		request.KindCompound,
		request.KindCompoundWait,
		request.KindTriggered,
	}
	for i, sk := range safetyKinds {
		base := baseKinds[i]
		if !sk.IsSafety() {
			t.Errorf("%v.IsSafety() = false, want true", sk)
		}
		if sk.Base() != base {
			t.Errorf("%v.Base() = %v, want %v", sk, sk.Base(), base)
		}
		if !sk.Valid() {
			t.Errorf("%v.Valid() = false, want true", sk)
		}
		if base.IsSafety() {
			t.Errorf("%v.IsSafety() = true, want false", base)
		}
		if base.Base() != base {
			t.Errorf("%v.Base() = %v, want unchanged %v", base, base.Base(), base)
		}
		if sk.Priority() != base.Priority() {
			t.Errorf("%v.Priority() = %d, want same as %v.Priority() = %d", sk, sk.Priority(), base, base.Priority())
		}
	}

	// No safety counterpart is defined for KindPlain, KindChained,
	// KindTimed, or any cancellation variant.
	noCounterpart := []request.Kind{
		request.KindPlain, request.KindChained, request.KindTimed,
		request.KindCancelAll, request.KindCancelTransaction, request.KindCancelSequencer,
	}
	for _, k := range noCounterpart {
		tagged := k | request.KindSafetyFlag
		if tagged.Valid() {
			t.Errorf("(%v|KindSafetyFlag).Valid() = true, want false (no safety counterpart defined)", k)
		}
	}
}

// TestEnvelope_SafetyRoundTrip checks EncodeCompoundSafety/
// EncodeCompoundWaitSafety/EncodeTriggeredSafety produce envelopes their
// base kind's Decode function still parses (identical body layout, only the
// leading Kind byte differs), and that PeekKind reports the safety-tagged
// Kind, not the base one (REQ-REQ-023).
func TestEnvelope_SafetyRoundTrip(t *testing.T) {
	c := request.Conditional{Sequencer: 4, Op: request.CompareLessOrEqual, Operand: 7, AdvanceOnMatch: 2}

	compoundBody := request.EncodeCompoundSafety(c, acf.FlagWrite, []byte{0x01})
	if k, err := request.PeekKind(compoundBody); err != nil || k != request.KindCompoundSafety {
		t.Errorf("PeekKind(compound-safety) = (%v, %v), want (KindCompoundSafety, nil)", k, err)
	}
	gotC, gotControl, gotBody, err := request.DecodeCompound(compoundBody)
	if err != nil || gotC != c || gotControl != acf.FlagWrite || string(gotBody) != "\x01" {
		t.Errorf("DecodeCompound(safety) = (%+v, %v, % X, %v), want (%+v, FlagWrite, 01, nil)", gotC, gotControl, gotBody, err, c)
	}

	waitBody := request.EncodeCompoundWaitSafety(c)
	if k, err := request.PeekKind(waitBody); err != nil || k != request.KindCompoundWaitSafety {
		t.Errorf("PeekKind(compound-wait-safety) = (%v, %v), want (KindCompoundWaitSafety, nil)", k, err)
	}
	if got, err := request.DecodeCompoundWait(waitBody); err != nil || got != c {
		t.Errorf("DecodeCompoundWait(safety) = (%+v, %v), want (%+v, nil)", got, err, c)
	}

	triggeredBody := request.EncodeTriggeredSafety(avtp.ByteBusID(5), acf.FlagRead, nil)
	if k, err := request.PeekKind(triggeredBody); err != nil || k != request.KindTriggeredSafety {
		t.Errorf("PeekKind(triggered-safety) = (%v, %v), want (KindTriggeredSafety, nil)", k, err)
	}
	if src, ctrl, inner, err := request.DecodeTriggered(triggeredBody); err != nil || src != avtp.ByteBusID(5) || ctrl != acf.FlagRead || len(inner) != 0 {
		t.Errorf("DecodeTriggered(safety) = (%v, %v, %v, %v), want (5, FlagRead, empty, nil)", src, ctrl, inner, err)
	}
}

// TestDispatcher_SafeStateGate checks Submit refuses a safety-request Kind
// outright with ErrSafeStateNotConfigured when no SafeStateCheck is
// configured, and that once one is configured, a safety ticket stays
// pending across every Pump call until SafeStateCheck reports true for its
// requester (REQ-REQ-024).
func TestDispatcher_SafeStateGate(t *testing.T) {
	ep, root, addr := newGPIOEndpoint(t, gpio.Config{PinCount: 4, Direction: 0b1111})
	seq := request.NewSequencer()
	d := request.NewDispatcher(ep, addr, seq, nil)

	cond := request.Conditional{Sequencer: 1, Op: request.CompareEqual, Operand: 0, AdvanceOnMatch: 0}
	safeReq := acf.Message{
		Kind: acf.KindShort, ByteBusID: addr, TransactionNum: 1,
		Control: acf.FlagWrite | acf.FlagExtended,
		Body:    request.EncodeCompoundSafety(cond, acf.FlagWrite, gpio.EncodeWriteRequest(gpio.SemanticOr, 0b0001)),
	}

	// No SafeStateCheck configured yet: Submit must refuse outright rather
	// than admit a ticket that could never become ready.
	if _, err := d.Submit(root, safeReq); !errors.Is(err, request.ErrSafeStateNotConfigured) {
		t.Fatalf("Submit(safety, unconfigured) err = %v, want ErrSafeStateNotConfigured", err)
	}

	inSafeState := false
	d.SetSafeStateCheck(func(requester avtp.StreamID) bool {
		if requester != root {
			t.Errorf("SafeStateCheck called with requester = %v, want %v", requester, root)
		}
		return inSafeState
	})

	id, err := d.Submit(root, safeReq)
	if err != nil {
		t.Fatalf("Submit(safety, configured): %v", err)
	}

	// Not yet in safe state: stays pending across repeated Pump calls.
	for i := 0; i < 3; i++ {
		d.Pump(0)
		if _, pollErr := d.Response(id); !errors.Is(pollErr, request.ErrPending) {
			t.Fatalf("Response before safe state (pump %d) = %v, want ErrPending", i, pollErr)
		}
		if st, _ := d.StateOf(id); st != request.StateStarted {
			t.Errorf("StateOf before safe state = %v, want StateStarted", st)
		}
	}

	// Endpoint reports safe state entered: the ticket may now execute.
	inSafeState = true
	d.Pump(0)
	resp, err := d.Response(id)
	if err != nil {
		t.Fatalf("Response after safe state: %v", err)
	}
	res, _, err := request.DecodeConditionalResponse(resp.Body)
	if err != nil || !res.Matched {
		t.Errorf("DecodeConditionalResponse = (%+v, %v), want Matched=true", res, err)
	}
}

// TestDispatcher_PurgeNonSafety checks PurgeNonSafety finalizes every
// not-yet-finalized non-safety ticket with ErrPurgedByWatchdog while leaving
// a pending safety-request ticket completely untouched, and that the
// survivor still executes normally once its own readiness condition is
// later satisfied (REQ-REQ-025).
func TestDispatcher_PurgeNonSafety(t *testing.T) {
	ep, root, addr := newGPIOEndpoint(t, gpio.Config{PinCount: 4, Direction: 0b1111})
	seq := request.NewSequencer()
	d := request.NewDispatcher(ep, addr, seq, nil)
	d.SetSafeStateCheck(func(avtp.StreamID) bool { return true })

	// An ordinary pending Timed ticket, not yet due.
	timedBody := request.EncodeTimed(1000, acf.FlagWrite, gpio.EncodeWriteRequest(gpio.SemanticOr, 0b0001))
	timedID, err := d.Submit(root, acf.Message{Kind: acf.KindShort, ByteBusID: addr, TransactionNum: 1, Control: acf.FlagWrite | acf.FlagExtended, Body: timedBody})
	if err != nil {
		t.Fatalf("Submit(timed): %v", err)
	}

	// A safety-request CompoundWaitSafety ticket, gated ready (SafeStateCheck
	// always true here), but not yet executed by a Pump call.
	cond := request.Conditional{Sequencer: 1, Op: request.CompareEqual, Operand: 0, AdvanceOnMatch: 1}
	safeID, err := d.Submit(root, acf.Message{Kind: acf.KindShort, ByteBusID: addr, TransactionNum: 2, Control: acf.FlagExtended, Body: request.EncodeCompoundWaitSafety(cond)})
	if err != nil {
		t.Fatalf("Submit(safety): %v", err)
	}

	cleared := d.PurgeNonSafety()
	if len(cleared) != 1 || cleared[0] != timedID {
		t.Errorf("PurgeNonSafety() = %v, want [%d]", cleared, timedID)
	}
	if _, pollErr := d.Response(timedID); !errors.Is(pollErr, request.ErrPurgedByWatchdog) {
		t.Errorf("Response(timed) after purge = %v, want ErrPurgedByWatchdog", pollErr)
	}
	if _, pollErr := d.Response(safeID); !errors.Is(pollErr, request.ErrPending) {
		t.Errorf("Response(safety) after purge = %v, want still ErrPending (must survive the purge)", pollErr)
	}

	// The survivor still resolves normally on the next Pump call.
	d.Pump(0)
	resp, err := d.Response(safeID)
	if err != nil {
		t.Fatalf("Response(safety) after purge+pump: %v", err)
	}
	res, _, err := request.DecodeConditionalResponse(resp.Body)
	if err != nil || !res.Matched {
		t.Errorf("DecodeConditionalResponse = (%+v, %v), want Matched=true", res, err)
	}
}
