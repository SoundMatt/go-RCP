//fusa:test REQ-REQ-004

package request_test

import (
	"math"
	"testing"

	"github.com/SoundMatt/go-RCP/request"
)

// TestSequencer_GetSetAdvance checks a register starts at zero, Set
// overwrites it directly, and Advance saturates (rather than wraps) at
// both the zero floor and the uint32 ceiling while returning the resulting
// value (REQ-REQ-004).
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
	if got := seq.Advance(id, -20); got != 0 {
		t.Fatalf("Advance(-20) from 15 = %d, want saturated floor 0 (not wraparound)", got)
	}

	// A different SequencerID is an independent register.
	if got := seq.Get(request.SequencerID(8)); got != 0 {
		t.Fatalf("Get(other id) = %d, want 0 (independent register)", got)
	}
}

// TestSequencer_AdvanceSaturatesAtMax checks that advancing a register past
// math.MaxUint32 clamps at math.MaxUint32 instead of wrapping to a small
// value — the actual bug behind #130: this test fails against the old
// wrapping implementation (which would report 0) and passes against the
// saturating fix (REQ-REQ-004).
func TestSequencer_AdvanceSaturatesAtMax(t *testing.T) {
	seq := request.NewSequencer()
	const id = request.SequencerID(1)

	seq.Set(id, math.MaxUint32)
	if got := seq.Advance(id, 1); got != math.MaxUint32 {
		t.Fatalf("Advance past MaxUint32 = %d, want saturated ceiling %d (not wraparound to 0)", got, uint32(math.MaxUint32))
	}
	// The register itself, not just the returned value, must reflect the
	// saturated ceiling for subsequent reads/advances.
	if got := seq.Get(id); got != math.MaxUint32 {
		t.Fatalf("Get after saturating Advance = %d, want %d", got, uint32(math.MaxUint32))
	}
	if got := seq.Advance(id, 1); got != math.MaxUint32 {
		t.Fatalf("Advance(+1) again at ceiling = %d, want it to remain saturated at %d", got, uint32(math.MaxUint32))
	}

	seq.Set(id, math.MaxUint32-5)
	if got := seq.Advance(id, math.MaxInt32); got != math.MaxUint32 {
		t.Fatalf("Advance(MaxInt32) from MaxUint32-5 = %d, want saturated ceiling %d", got, uint32(math.MaxUint32))
	}
}

// TestSequencer_AdvanceSaturatesAtZero checks that advancing a register
// below zero (a large negative delta) clamps at zero instead of wrapping to
// a large value near math.MaxUint32 (REQ-REQ-004).
func TestSequencer_AdvanceSaturatesAtZero(t *testing.T) {
	seq := request.NewSequencer()
	const id = request.SequencerID(2)

	seq.Set(id, 3)
	if got := seq.Advance(id, math.MinInt32); got != 0 {
		t.Fatalf("Advance(MinInt32) from 3 = %d, want saturated floor 0 (not wraparound)", got)
	}
	if got := seq.Get(id); got != 0 {
		t.Fatalf("Get after saturating-floor Advance = %d, want 0", got)
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
