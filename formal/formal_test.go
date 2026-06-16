//fusa:test REQ-FORM-001
//fusa:test REQ-FORM-002
//fusa:test REQ-FORM-003
//fusa:test REQ-FORM-004
//fusa:test REQ-FORM-005
//fusa:test REQ-FORM-006
//fusa:test REQ-FORM-007
//fusa:test REQ-FORM-008

package formal_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/formal"
)

// stateN extracts the "n" int counter from a State.
func stateN(s formal.State) int {
	v, _ := s["n"].(int) //nolint:errcheck
	return v
}

// incGen advances the counter by 1.
func incGen(s formal.State) formal.State {
	return formal.State{"n": stateN(s) + 1}
}

// REQ-FORM-001: StateSequence generates exactly n states.
func TestStateSequence_Length(t *testing.T) {
	trace := formal.StateSequence(formal.State{"n": 0}, incGen, 10)
	if len(trace) != 10 {
		t.Fatalf("len(trace) = %d, want 10", len(trace))
	}
}

// REQ-FORM-002: StateSequence applies the generator correctly.
func TestStateSequence_Values(t *testing.T) {
	trace := formal.StateSequence(formal.State{"n": 0}, incGen, 5)
	for i, s := range trace {
		if stateN(s) != i {
			t.Errorf("trace[%d][n] = %v, want %d", i, stateN(s), i)
		}
	}
}

// REQ-FORM-003: Always returns true when predicate holds in every state.
func TestAlways_HoldsAll(t *testing.T) {
	trace := formal.StateSequence(formal.State{"n": 0}, incGen, 10)
	nonNeg := formal.Predicate(func(s formal.State) bool { return stateN(s) >= 0 })
	if !formal.Always(nonNeg)(trace) {
		t.Error("Always(n>=0) should hold for n=0..9")
	}
}

// REQ-FORM-004: Always returns false when predicate fails in some state.
func TestAlways_Falsified(t *testing.T) {
	trace := formal.StateSequence(formal.State{"n": 0}, incGen, 10)
	lessThan5 := formal.Predicate(func(s formal.State) bool { return stateN(s) < 5 })
	if formal.Always(lessThan5)(trace) {
		t.Error("Always(n<5) should be false for trace 0..9")
	}
}

// REQ-FORM-005: Eventually returns true when predicate holds at least once.
func TestEventually_Holds(t *testing.T) {
	trace := formal.StateSequence(formal.State{"n": 0}, incGen, 10)
	eq7 := formal.Predicate(func(s formal.State) bool { return stateN(s) == 7 })
	if !formal.Eventually(eq7)(trace) {
		t.Error("Eventually(n==7) should hold for trace 0..9")
	}
}

// REQ-FORM-006: Eventually returns false when predicate never holds.
func TestEventually_NeverHolds(t *testing.T) {
	trace := formal.StateSequence(formal.State{"n": 0}, incGen, 5)
	eq99 := formal.Predicate(func(s formal.State) bool { return stateN(s) == 99 })
	if formal.Eventually(eq99)(trace) {
		t.Error("Eventually(n==99) should be false for trace 0..4")
	}
}

// REQ-FORM-007: Until holds when p holds until q becomes true.
func TestUntil_HoldsWhenQReached(t *testing.T) {
	trace := formal.StateSequence(formal.State{"n": 0}, incGen, 10)
	lessThan5 := formal.Predicate(func(s formal.State) bool { return stateN(s) < 5 })
	eq5 := formal.Predicate(func(s formal.State) bool { return stateN(s) == 5 })
	if !formal.Until(lessThan5, eq5)(trace) {
		t.Error("Until(n<5, n==5) should hold for trace 0..9")
	}
}

// REQ-FORM-008: Checker.Check returns ViolationError when an invariant is falsified.
func TestChecker_Violation(t *testing.T) {
	trace := formal.StateSequence(formal.State{"n": 0}, incGen, 10)
	checker := formal.NewChecker()
	checker.Add(formal.Invariant{
		Name:  "n < 5 always",
		Check: formal.Always(formal.Predicate(func(s formal.State) bool { return stateN(s) < 5 })),
	})
	err := checker.Check(trace)
	var ve *formal.ViolationError
	if !errors.As(err, &ve) {
		t.Errorf("want ViolationError, got %v", err)
	}
	if ve.Invariant != "n < 5 always" {
		t.Errorf("invariant name = %q, want %q", ve.Invariant, "n < 5 always")
	}
}
