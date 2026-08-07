//fusa:test REQ-I214-009
//fusa:test REQ-I214-010
//fusa:test REQ-I214-011
//fusa:test REQ-I214-012

package iso21434_test

import (
	"testing"

	"github.com/SoundMatt/go-RCP/v9/iso21434"
)

// REQ-I214-009: GoalRegistry.All returns every registered goal, satisfied
// or not — distinct from Unsatisfied's filtered subset.
func TestGoalRegistry_All(t *testing.T) {
	r := iso21434.NewGoalRegistry()
	r.Add(iso21434.CybersecurityGoal{ID: "CG-001", Satisfied: true})
	r.Add(iso21434.CybersecurityGoal{ID: "CG-002", Satisfied: false})

	all := r.All()
	if len(all) != 2 {
		t.Fatalf("len(All()) = %d, want 2", len(all))
	}
}

// REQ-I214-010: BuildTARA's every ThreatScenario carries a valid Impact and
// Feasibility rating (ComputeRisk succeeds), a unique ID, and a non-empty
// DamageScenario.
func TestBuildTARA_AllThreatsValid(t *testing.T) {
	tara := iso21434.BuildTARA()
	if len(tara.Threats) == 0 {
		t.Fatal("BuildTARA returned no threats")
	}
	seen := make(map[string]bool)
	for _, ts := range tara.Threats {
		if ts.ID == "" {
			t.Error("threat with empty ID")
		}
		if seen[ts.ID] {
			t.Errorf("duplicate threat ID %q", ts.ID)
		}
		seen[ts.ID] = true
		if ts.DamageScenario == "" {
			t.Errorf("%s: empty DamageScenario", ts.ID)
		}
		if _, _, err := ts.RiskValue(); err != nil {
			t.Errorf("%s: RiskValue: %v", ts.ID, err)
		}
	}
}

// REQ-I214-011: BuildTARA identifies at least one High-or-Critical risk
// threat — the new attack surface is not rated uniformly low.
func TestBuildTARA_HasHighRiskThreats(t *testing.T) {
	tara := iso21434.BuildTARA()
	highs, err := tara.HighRiskThreats()
	if err != nil {
		t.Fatalf("HighRiskThreats: %v", err)
	}
	if len(highs) == 0 {
		t.Error("want at least one High/Critical threat, got none")
	}
}

// REQ-I214-012: BuildGoalRegistry maps a CybersecurityGoal to every threat
// BuildTARA defines, and reports at least one genuinely unsatisfied goal
// (this milestone documents real, currently-open gaps rather than claiming
// full mitigation).
func TestBuildGoalRegistry_CoversEveryThreatAndHasGaps(t *testing.T) {
	tara := iso21434.BuildTARA()
	goals := iso21434.BuildGoalRegistry()

	covered := make(map[string]bool)
	for _, g := range goals.All() {
		covered[g.ThreatID] = true
	}
	for _, ts := range tara.Threats {
		if !covered[ts.ID] {
			t.Errorf("threat %s has no mapped CybersecurityGoal", ts.ID)
		}
	}

	if len(goals.Unsatisfied()) == 0 {
		t.Error("want at least one unsatisfied goal (a real open gap), got none")
	}
	if len(goals.Unsatisfied()) == len(goals.All()) {
		t.Error("want at least one satisfied goal too, got all unsatisfied")
	}
}
