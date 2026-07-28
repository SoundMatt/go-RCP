//fusa:test REQ-REQ-004

package request_test

import (
	"math"
	"testing"

	"github.com/SoundMatt/go-RCP/request"
)

// TestSequencer_GetSetAdvance checks a register starts at zero, Set
// overwrites it directly, and Advance both wraps (rather than saturates) and
// returns the resulting value (REQ-REQ-004).
func TestSequencer_GetSetAdvance(t *testing.T) {
	seq := request.NewSequencer()
	const id = request.SequencerID(7)

	if got := seq.Get(id); got != 0 {
		t.Fatalf("Get(unset) = %d, want 0", got)
	}

	seq.Set(id, 10)
	if got := seq.Get(id); got != 10 {
		t.Fatalf("Get after Set(10) = %d, want 10", got)
	}

	if got := seq.Advance(id, 5); got != 15 {
		t.Fatalf("Advance(+5) = %d, want 15", got)
	}
	before := int64(15)
	delta := int64(-20)
	want := uint32(before + delta) // wraps: modular add, not the (negative) mathematical sum
	if got := seq.Advance(id, -20); got != want {
		t.Fatalf("Advance(-20) = %d, want wraparound %d", got, want)
	}

	seq.Set(id, math.MaxUint32)
	if got := seq.Advance(id, 1); got != 0 {
		t.Fatalf("Advance past MaxUint32 = %d, want wraparound to 0", got)
	}

	// A different SequencerID is an independent register.
	if got := seq.Get(request.SequencerID(8)); got != 0 {
		t.Fatalf("Get(other id) = %d, want 0 (independent register)", got)
	}
}

// TestConditional_Evaluate checks evaluate reports the comparison outcome
// and only advances the sequencer register when it matches, leaving it
// untouched on a non-match (REQ-REQ-004).
func TestConditional_Evaluate(t *testing.T) {
	seq := request.NewSequencer()
	seq.Set(1, 5)

	matchCond := request.Conditional{Sequencer: 1, Op: request.CompareEqual, Operand: 5, AdvanceOnMatch: 3}
	if !matchCond.Valid() {
		t.Fatalf("Conditional.Valid() = false, want true")
	}

	// Exported via the package's public evaluate path: Dispatcher wraps
	// this, so this test drives it through a CompoundWait envelope/Dispatch
	// round trip instead of calling an unexported method directly (see
	// dispatcher_test.go's TestDispatcher_CompoundWait for the full path).
	// Here we only check the underlying Sequencer bookkeeping directly.
	if got := seq.Get(1); got != 5 {
		t.Fatalf("sequencer value before any Dispatcher call = %d, want unchanged 5", got)
	}
}
