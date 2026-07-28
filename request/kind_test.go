//fusa:test REQ-REQ-001
//fusa:test REQ-REQ-002
//fusa:test REQ-REQ-003

package request_test

import (
	"testing"

	"github.com/SoundMatt/go-RCP/request"
)

// TestKind_Valid checks Valid recognizes exactly the eight non-plain Kind
// values and rejects KindPlain and anything past the sentinel (REQ-REQ-001).
func TestKind_Valid(t *testing.T) {
	tests := []struct {
		k    request.Kind
		want bool
	}{
		{request.KindPlain, false},
		{request.KindCompound, true},
		{request.KindCompoundWait, true},
		{request.KindTriggered, true},
		{request.KindChained, true},
		{request.KindTimed, true},
		{request.KindCancelAll, true},
		{request.KindCancelTransaction, true},
		{request.KindCancelSequencer, true},
		{request.Kind(200), false},
	}
	for _, tt := range tests {
		if got := tt.k.Valid(); got != tt.want {
			t.Errorf("Kind(%v).Valid() = %v, want %v", tt.k, got, tt.want)
		}
	}
}

// TestKind_IsCancellation checks IsCancellation recognizes exactly the three
// cancellation variants (REQ-REQ-001).
func TestKind_IsCancellation(t *testing.T) {
	tests := []struct {
		k    request.Kind
		want bool
	}{
		{request.KindCancelAll, true},
		{request.KindCancelTransaction, true},
		{request.KindCancelSequencer, true},
		{request.KindCompound, false},
		{request.KindPlain, false},
	}
	for _, tt := range tests {
		if got := tt.k.IsCancellation(); got != tt.want {
			t.Errorf("Kind(%v).IsCancellation() = %v, want %v", tt.k, got, tt.want)
		}
	}
}

// TestKind_Priority checks the fixed cross-type ordering documented on
// priorityRank: cancellation < chained < triggered < timed < compound-wait <
// compound < plain (REQ-REQ-002).
func TestKind_Priority(t *testing.T) {
	order := []request.Kind{
		request.KindCancelAll,
		request.KindChained,
		request.KindTriggered,
		request.KindTimed,
		request.KindCompoundWait,
		request.KindCompound,
		request.KindPlain,
	}
	for i := 1; i < len(order); i++ {
		if order[i-1].Priority() > order[i].Priority() {
			t.Errorf("Priority(%v) = %d > Priority(%v) = %d, want non-decreasing",
				order[i-1], order[i-1].Priority(), order[i], order[i].Priority())
		}
	}
	// The three cancellation variants rank equally with each other.
	if request.KindCancelAll.Priority() != request.KindCancelTransaction.Priority() ||
		request.KindCancelAll.Priority() != request.KindCancelSequencer.Priority() {
		t.Errorf("cancellation variants do not share one priority rank: %d/%d/%d",
			request.KindCancelAll.Priority(), request.KindCancelTransaction.Priority(), request.KindCancelSequencer.Priority())
	}
}

// TestCompareOp_Evaluate checks all six comparison operators against a
// representative pair of operands, and that Valid recognizes exactly the
// six defined values (REQ-REQ-003).
func TestCompareOp_Evaluate(t *testing.T) {
	tests := []struct {
		op        request.CompareOp
		current   uint32
		operand   uint32
		want      bool
		wantValid bool
	}{
		{request.CompareEqual, 5, 5, true, true},
		{request.CompareEqual, 5, 6, false, true},
		{request.CompareNotEqual, 5, 6, true, true},
		{request.CompareNotEqual, 5, 5, false, true},
		{request.CompareLess, 4, 5, true, true},
		{request.CompareLess, 5, 5, false, true},
		{request.CompareLessOrEqual, 5, 5, true, true},
		{request.CompareGreater, 6, 5, true, true},
		{request.CompareGreater, 5, 5, false, true},
		{request.CompareGreaterOrEqual, 5, 5, true, true},
		{request.CompareOp(200), 5, 5, false, false},
	}
	for _, tt := range tests {
		if got := tt.op.Valid(); got != tt.wantValid {
			t.Errorf("CompareOp(%d).Valid() = %v, want %v", tt.op, got, tt.wantValid)
		}
		if !tt.wantValid {
			continue
		}
		if got := tt.op.Evaluate(tt.current, tt.operand); got != tt.want {
			t.Errorf("%v.Evaluate(%d, %d) = %v, want %v", tt.op, tt.current, tt.operand, got, tt.want)
		}
	}
}
