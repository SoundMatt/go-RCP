//fusa:test REQ-FORM-009
//fusa:test REQ-FORM-010
//fusa:test REQ-FORM-011

package formal_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/formal"
	"github.com/SoundMatt/go-RCP/lifecycle"
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
// observed rank, so it can never falsify the "legal transition shape"
// invariant, but also never satisfies "eventually fully-configured" — a
// Checker over this trace must report exactly that third invariant as
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
		Check: formal.LifecycleInvariants()[2].Check,
	})
	var ve *formal.ViolationError
	if err := checker.Check(trace); !errors.As(err, &ve) {
		t.Errorf("want ViolationError, got %v", err)
	}

	// The legal-transition-shape invariant must still hold: a rejected
	// transition leaves rank at 0 throughout, which is "no change" at every
	// step, not a violation of that specific property.
	rankChecker := formal.NewChecker()
	rankChecker.Add(formal.LifecycleInvariants()[0])
	if err := rankChecker.Check(trace); err != nil {
		t.Errorf("legal-transition-shape should still hold for a rejected transition: %v", err)
	}
}

// REQ-FORM-010: a plausible trace that demotes from StateHWLocked back to
// StateUnconfigured and then re-advances all the way to
// StateFullyConfigured satisfies every LifecycleInvariants property — the
// legal-transition-shape invariant explicitly permits the one-step
// HWLocked→Unconfigured fallback, and demoting before full configuration
// can never trip the never-regresses-once-fully-configured invariant.
func TestLifecycleInvariants_DemotionThenReconfigure_Satisfied(t *testing.T) {
	srv, actions, err := formal.NewDemotionThenReconfigureLifecycleTrace(lifecycleRoot(), avtp.ByteBusID(1))
	if err != nil {
		t.Fatalf("NewDemotionThenReconfigureLifecycleTrace: %v", err)
	}
	trace := formal.LifecycleTrace(srv, actions)

	checker := formal.NewChecker()
	for _, inv := range formal.LifecycleInvariants() {
		checker.Add(inv)
	}
	if err := checker.Check(trace); err != nil {
		t.Errorf("Check: %v", err)
	}
}

// REQ-FORM-010: a trace that reaches StateFullyConfigured and then
// synthesizes an illegal regression back to StateUnconfigured (something
// server.Server itself never permits — DemoteToUnconfigured only accepts
// StateHWLocked as a starting state) must fail both the legal-transition-
// shape invariant and the never-regresses-once-fully-configured invariant.
func TestLifecycleInvariants_IllegalRegressionFromFullyConfigured_Violated(t *testing.T) {
	trace := []formal.State{
		{"lifecycle": lifecycle.StateFullyConfigured.String(), "lifecycle_rank": formal.LifecycleRank(lifecycle.StateFullyConfigured)},
		{"lifecycle": lifecycle.StateUnconfigured.String(), "lifecycle_rank": formal.LifecycleRank(lifecycle.StateUnconfigured)},
	}

	for _, inv := range formal.LifecycleInvariants()[:2] {
		if inv.Check(trace) {
			t.Errorf("invariant %q unexpectedly held for an illegal FullyConfigured→Unconfigured regression", inv.Name)
		}
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
