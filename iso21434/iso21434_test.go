//fusa:test REQ-I214-001
//fusa:test REQ-I214-002
//fusa:test REQ-I214-003
//fusa:test REQ-I214-004
//fusa:test REQ-I214-005
//fusa:test REQ-I214-006
//fusa:test REQ-I214-007
//fusa:test REQ-I214-008

package iso21434_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/iso21434"
)

// REQ-I214-001: ComputeRisk returns the correct risk value (impact × feasibility).
func TestComputeRisk_Value(t *testing.T) {
	rv, _, err := iso21434.ComputeRisk(iso21434.ImpactMajor, iso21434.FeasibilityHigh)
	if err != nil {
		t.Fatalf("ComputeRisk: %v", err)
	}
	if rv != 9 { // 3 × 3
		t.Errorf("risk value = %d, want 9", rv)
	}
}

// REQ-I214-002: ComputeRisk classifies Low (≤2), Medium (≤6), High (≤12), Critical (>12).
func TestComputeRisk_Levels(t *testing.T) {
	cases := []struct {
		impact      iso21434.ImpactRating
		feasibility iso21434.FeasibilityRating
		wantLevel   iso21434.RiskLevel
	}{
		{iso21434.ImpactNegligible, iso21434.FeasibilityLow, iso21434.RiskLevelLow},
		{iso21434.ImpactModerate, iso21434.FeasibilityMedium, iso21434.RiskLevelMedium},
		{iso21434.ImpactMajor, iso21434.FeasibilityHigh, iso21434.RiskLevelHigh},
		{iso21434.ImpactSevere, iso21434.FeasibilityVeryHigh, iso21434.RiskLevelCritical},
	}
	for _, tc := range cases {
		_, level, err := iso21434.ComputeRisk(tc.impact, tc.feasibility)
		if err != nil {
			t.Fatalf("ComputeRisk(%d,%d): %v", tc.impact, tc.feasibility, err)
		}
		if level != tc.wantLevel {
			t.Errorf("level = %s, want %s", level, tc.wantLevel)
		}
	}
}

// REQ-I214-003: ComputeRisk returns ErrInvalidRating for out-of-range impact.
func TestComputeRisk_InvalidImpact(t *testing.T) {
	_, _, err := iso21434.ComputeRisk(0, iso21434.FeasibilityLow)
	if !errors.Is(err, iso21434.ErrInvalidRating) {
		t.Errorf("want ErrInvalidRating, got %v", err)
	}
}

// REQ-I214-004: ComputeRisk returns ErrInvalidRating for out-of-range feasibility.
func TestComputeRisk_InvalidFeasibility(t *testing.T) {
	_, _, err := iso21434.ComputeRisk(iso21434.ImpactMajor, 5)
	if !errors.Is(err, iso21434.ErrInvalidRating) {
		t.Errorf("want ErrInvalidRating, got %v", err)
	}
}

// REQ-I214-005: ThreatScenario.RiskValue delegates to ComputeRisk correctly.
func TestThreatScenario_RiskValue(t *testing.T) {
	ts := iso21434.ThreatScenario{
		ID:          "T-001",
		Description: "Spoofed RCP command injection",
		Impact:      iso21434.ImpactSevere,
		Feasibility: iso21434.FeasibilityHigh,
	}
	rv, level, err := ts.RiskValue()
	if err != nil {
		t.Fatalf("RiskValue: %v", err)
	}
	if rv != 12 { // 4 × 3
		t.Errorf("risk = %d, want 12", rv)
	}
	if level != iso21434.RiskLevelHigh {
		t.Errorf("level = %s, want High", level)
	}
}

// REQ-I214-006: TARA.HighRiskThreats returns only High and Critical threats.
func TestTARA_HighRiskThreats(t *testing.T) {
	tara := iso21434.TARA{
		Component: "go-RCP zone controller",
		Threats: []iso21434.ThreatScenario{
			{ID: "T-001", Impact: iso21434.ImpactNegligible, Feasibility: iso21434.FeasibilityLow},    // Low
			{ID: "T-002", Impact: iso21434.ImpactMajor, Feasibility: iso21434.FeasibilityHigh},         // High
			{ID: "T-003", Impact: iso21434.ImpactSevere, Feasibility: iso21434.FeasibilityVeryHigh},    // Critical
		},
	}
	highs, err := tara.HighRiskThreats()
	if err != nil {
		t.Fatalf("HighRiskThreats: %v", err)
	}
	if len(highs) != 2 {
		t.Errorf("got %d high-risk threats, want 2", len(highs))
	}
}

// REQ-I214-007: GoalRegistry.Unsatisfied returns only goals with Satisfied=false.
func TestGoalRegistry_Unsatisfied(t *testing.T) {
	r := iso21434.NewGoalRegistry()
	r.Add(iso21434.CybersecurityGoal{ID: "CG-001", ThreatID: "T-001", Claim: "Authenticate all RCP commands", Satisfied: true})
	r.Add(iso21434.CybersecurityGoal{ID: "CG-002", ThreatID: "T-002", Claim: "Encrypt zone bus traffic", Satisfied: false})
	r.Add(iso21434.CybersecurityGoal{ID: "CG-003", ThreatID: "T-003", Claim: "Rate-limit diagnostic access", Satisfied: false})

	unsatisfied := r.Unsatisfied()
	if len(unsatisfied) != 2 {
		t.Errorf("got %d unsatisfied goals, want 2", len(unsatisfied))
	}
}

// REQ-I214-008: GoalRegistry.Unsatisfied returns empty slice when all goals are met.
func TestGoalRegistry_AllSatisfied(t *testing.T) {
	r := iso21434.NewGoalRegistry()
	r.Add(iso21434.CybersecurityGoal{ID: "CG-001", Satisfied: true})
	r.Add(iso21434.CybersecurityGoal{ID: "CG-002", Satisfied: true})

	unsatisfied := r.Unsatisfied()
	if len(unsatisfied) != 0 {
		t.Errorf("got %d unsatisfied goals, want 0", len(unsatisfied))
	}
}
