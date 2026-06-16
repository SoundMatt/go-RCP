//fusa:req REQ-I214-001
//fusa:req REQ-I214-002
//fusa:req REQ-I214-003
//fusa:req REQ-I214-004
//fusa:req REQ-I214-005
//fusa:req REQ-I214-006
//fusa:req REQ-I214-007
//fusa:req REQ-I214-008

// Package iso21434 provides cybersecurity engineering artifacts for go-RCP
// aligned with ISO/SAE 21434 (Road Vehicle Cybersecurity Engineering).
//
// ISO 21434 defines a risk-based cybersecurity engineering process for
// road vehicles. This package provides the Go-side data structures and
// analysis helpers for:
//
//   - TARA (Threat Analysis and Risk Assessment) items
//   - Attack paths and feasibility ratings
//   - Cybersecurity goals and claims
//   - Risk value computation using impact and attack feasibility matrices
//
// Risk = Impact × Attack-Feasibility (both rated 1–4 per ISO 21434 Annex E).
package iso21434

import (
	"errors"
	"fmt"
)

// ImpactRating maps to ISO 21434 impact categories (1=Negligible, 4=Severe).
type ImpactRating int

const (
	ImpactNegligible ImpactRating = 1
	ImpactModerate   ImpactRating = 2
	ImpactMajor      ImpactRating = 3
	ImpactSevere     ImpactRating = 4
)

// FeasibilityRating maps to ISO 21434 attack feasibility (1=Low, 4=Very High).
type FeasibilityRating int

const (
	FeasibilityLow      FeasibilityRating = 1
	FeasibilityMedium   FeasibilityRating = 2
	FeasibilityHigh     FeasibilityRating = 3
	FeasibilityVeryHigh FeasibilityRating = 4
)

// RiskValue is the composite risk score (1–16).
type RiskValue int

// RiskLevel classifies a RiskValue into Low/Medium/High/Critical.
type RiskLevel string

const (
	RiskLevelLow      RiskLevel = "Low"
	RiskLevelMedium   RiskLevel = "Medium"
	RiskLevelHigh     RiskLevel = "High"
	RiskLevelCritical RiskLevel = "Critical"
)

// ErrInvalidRating is returned when an impact or feasibility value is out of range.
var ErrInvalidRating = errors.New("iso21434: rating must be 1–4")

// ComputeRisk returns the ISO 21434 risk value (impact × feasibility) and its level.
func ComputeRisk(impact ImpactRating, feasibility FeasibilityRating) (RiskValue, RiskLevel, error) {
	if impact < 1 || impact > 4 {
		return 0, "", fmt.Errorf("%w: impact %d", ErrInvalidRating, impact)
	}
	if feasibility < 1 || feasibility > 4 {
		return 0, "", fmt.Errorf("%w: feasibility %d", ErrInvalidRating, feasibility)
	}
	rv := RiskValue(int(impact) * int(feasibility))
	var level RiskLevel
	switch {
	case rv <= 2:
		level = RiskLevelLow
	case rv <= 6:
		level = RiskLevelMedium
	case rv <= 12:
		level = RiskLevelHigh
	default:
		level = RiskLevelCritical
	}
	return rv, level, nil
}

// ─── TARA ─────────────────────────────────────────────────────────────────────

// ThreatScenario describes one identified threat in the TARA.
type ThreatScenario struct {
	ID          string
	Description string
	DamageScenario string
	Impact      ImpactRating
	Feasibility FeasibilityRating
}

// RiskValue computes the composite risk for this threat scenario.
func (ts ThreatScenario) RiskValue() (RiskValue, RiskLevel, error) {
	return ComputeRisk(ts.Impact, ts.Feasibility)
}

// TARA holds the complete Threat Analysis and Risk Assessment for a component.
type TARA struct {
	Component string
	Threats   []ThreatScenario
}

// HighRiskThreats returns all ThreatScenarios rated High or Critical.
func (t *TARA) HighRiskThreats() ([]ThreatScenario, error) {
	var out []ThreatScenario
	for _, ts := range t.Threats {
		_, level, err := ts.RiskValue()
		if err != nil {
			return nil, err
		}
		if level == RiskLevelHigh || level == RiskLevelCritical {
			out = append(out, ts)
		}
	}
	return out, nil
}

// ─── Cybersecurity Goal ───────────────────────────────────────────────────────

// CybersecurityGoal links a threat to a mitigation claim.
type CybersecurityGoal struct {
	ID        string
	ThreatID  string
	Claim     string
	Satisfied bool
}

// GoalRegistry holds a set of CybersecurityGoals.
type GoalRegistry struct {
	goals map[string]CybersecurityGoal
}

// NewGoalRegistry returns an empty registry.
func NewGoalRegistry() *GoalRegistry {
	return &GoalRegistry{goals: make(map[string]CybersecurityGoal)}
}

// Add registers a goal.
func (r *GoalRegistry) Add(g CybersecurityGoal) {
	r.goals[g.ID] = g
}

// Unsatisfied returns all goals that are not yet satisfied.
func (r *GoalRegistry) Unsatisfied() []CybersecurityGoal {
	var out []CybersecurityGoal
	for _, g := range r.goals {
		if !g.Satisfied {
			out = append(out, g)
		}
	}
	return out
}
