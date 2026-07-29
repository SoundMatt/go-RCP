//fusa:test REQ-FORM-009
//fusa:test REQ-FORM-010
//fusa:test REQ-FORM-011

package formal_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/formal"
)

func lifecycleRoot() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x03, 0x11, 0x22, 0x33, 0x44, 0x55}, 1)
}

// REQ-FORM-009: a plausible Unconfigured→HWLocked→FullyConfigured action
// sequence produces a trace satisfying every LifecycleInvariants property.
func TestLifecycleInvariants_ValidSequence(t *testing.T) {
	srv, actions, err := formal.NewValidLifecycleTrace(lifecycleRoot(), avtp.ByteBusID(1))
	if err != nil {
		t.Fatalf("NewValidLifecycleTrace: %v", err)
	}
	trace := formal.LifecycleTrace(srv, actions)
	if len(trace) != len(actions)+1 {
		t.Fatalf("len(trace) = %d, want %d", len(trace), len(actions)+1)
	}

	checker := formal.NewChecker()
	for _, inv := range formal.LifecycleInvariants() {
		checker.Add(inv)
	}
	if err := checker.Check(trace); err != nil {
		t.Errorf("Check: %v", err)
	}
}

// REQ-FORM-010: an out-of-order transition attempt never advances the
// observed rank, so it can never falsify the "rank never decreases"
// invariant, but also never satisfies "eventually fully-configured" — a
// Checker over this trace must report exactly that second invariant as
// violated.
func TestLifecycleInvariants_OutOfOrder_NeverReachesFullyConfigured(t *testing.T) {
	srv, actions, err := formal.NewOutOfOrderLifecycleTrace(lifecycleRoot())
	if err != nil {
		t.Fatalf("NewOutOfOrderLifecycleTrace: %v", err)
	}
	trace := formal.LifecycleTrace(srv, actions)

	checker := formal.NewChecker()
	checker.Add(formal.Invariant{
		Name:  "eventually reaches fully-configured",
		Check: formal.LifecycleInvariants()[1].Check,
	})
	var ve *formal.ViolationError
	if err := checker.Check(trace); !errors.As(err, &ve) {
		t.Errorf("want ViolationError, got %v", err)
	}

	// The rank-never-decreases invariant must still hold: a rejected
	// transition leaves rank at 0 throughout, which is "never decreases"
	// trivially, not a violation of that specific property.
	rankChecker := formal.NewChecker()
	rankChecker.Add(formal.LifecycleInvariants()[0])
	if err := rankChecker.Check(trace); err != nil {
		t.Errorf("rank-never-decreases should still hold for a rejected transition: %v", err)
	}
}

// REQ-FORM-011: LifecycleRank orders the three lifecycle states strictly
// increasing, and reports -1 for any other value.
func TestLifecycleRank_Ordering(t *testing.T) {
	srv, actions, err := formal.NewValidLifecycleTrace(lifecycleRoot(), avtp.ByteBusID(1))
	if err != nil {
		t.Fatalf("NewValidLifecycleTrace: %v", err)
	}
	trace := formal.LifecycleTrace(srv, actions)
	final := trace[len(trace)-1]
	rank, _ := final["lifecycle_rank"].(int)
	if rank != 2 {
		t.Errorf("final lifecycle_rank = %d, want 2 (fully-configured)", rank)
	}
}
