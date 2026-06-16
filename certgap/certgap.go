//fusa:req REQ-CERT-001
//fusa:req REQ-CERT-002
//fusa:req REQ-CERT-003
//fusa:req REQ-CERT-004
//fusa:req REQ-CERT-005
//fusa:req REQ-CERT-006
//fusa:req REQ-CERT-007
//fusa:req REQ-CERT-008

// Package certgap provides ASIL-D gap analysis helpers for go-RCP.
//
// ISO 26262 defines four Automotive Safety Integrity Levels (ASIL A–D) with
// increasing rigor. go-RCP targets ASIL-B for its SEOOC (Safety Element out
// of Context) deployment. This package provides tooling to identify and track
// the additional measures required to reach ASIL-D in an integrated system.
//
// The gap analysis workflow:
//  1. Define a set of ASILDRequirements for the target system.
//  2. Mark each requirement as Met or Unmet.
//  3. Call Analyze to obtain a GapReport.
//  4. The GapReport lists unmet requirements and the current compliance ratio.
package certgap

import "fmt"

// ASIL represents an Automotive Safety Integrity Level.
type ASIL string

const (
	ASILA ASIL = "ASIL-A"
	ASILB ASIL = "ASIL-B"
	ASILC ASIL = "ASIL-C"
	ASILD ASIL = "ASIL-D"
)

// Requirement describes a single ISO 26262 / IEC 61508 requirement.
type Requirement struct {
	ID          string
	Description string
	TargetASIL  ASIL
	Category    Category
	Met         bool
}

// Category groups requirements by ISO 26262 topic area.
type Category string

const (
	CategorySoftware         Category = "Software"
	CategoryHardware         Category = "Hardware"
	CategorySafetyMgmt       Category = "Safety Management"
	CategoryFunctionalSafety Category = "Functional Safety"
	CategoryVerification     Category = "Verification & Validation"
)

// ─── Registry ─────────────────────────────────────────────────────────────────

// Registry holds all Requirements for a component.
type Registry struct {
	reqs map[string]Requirement
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{reqs: make(map[string]Requirement)}
}

// Add registers a Requirement. If a requirement with the same ID already
// exists, it is replaced.
func (r *Registry) Add(req Requirement) {
	r.reqs[req.ID] = req
}

// SetMet marks requirement id as met (true) or unmet (false).
// Returns an error if the requirement is not registered.
func (r *Registry) SetMet(id string, met bool) error {
	req, ok := r.reqs[id]
	if !ok {
		return fmt.Errorf("certgap: requirement %q not found", id)
	}
	req.Met = met
	r.reqs[id] = req
	return nil
}

// All returns a snapshot of all registered requirements.
func (r *Registry) All() []Requirement {
	out := make([]Requirement, 0, len(r.reqs))
	for _, req := range r.reqs {
		out = append(out, req)
	}
	return out
}

// ─── Gap Analysis ─────────────────────────────────────────────────────────────

// GapReport is the output of Analyze.
type GapReport struct {
	// Total is the count of all requirements analysed.
	Total int
	// MetCount is the count of requirements that are met.
	MetCount int
	// Gaps lists requirements that are not yet met.
	Gaps []Requirement
	// ComplianceRatio is MetCount/Total (0.0–1.0). Zero if Total == 0.
	ComplianceRatio float64
}

// Analyze generates a GapReport from all requirements in r.
// Optionally filter to a specific ASIL level; pass "" to include all.
func (r *Registry) Analyze(targetASIL ASIL) GapReport {
	var report GapReport
	for _, req := range r.reqs {
		if targetASIL != "" && req.TargetASIL != targetASIL {
			continue
		}
		report.Total++
		if req.Met {
			report.MetCount++
		} else {
			report.Gaps = append(report.Gaps, req)
		}
	}
	if report.Total > 0 {
		report.ComplianceRatio = float64(report.MetCount) / float64(report.Total)
	}
	return report
}

// ─── Predefined ASIL-D gaps relative to ASIL-B ───────────────────────────────

// StandardASILDGaps returns the set of standard requirements that an ASIL-B
// SEOOC must additionally satisfy to reach ASIL-D in its integrated context.
// These are provided as a convenience baseline; integrators should extend
// or replace them with project-specific requirements.
func StandardASILDGaps() []Requirement {
	return []Requirement{
		{
			ID:          "ASILD-SW-001",
			Description: "Formal methods or higher MC/DC coverage (100%) for safety-critical paths",
			TargetASIL:  ASILD,
			Category:    CategorySoftware,
		},
		{
			ID:          "ASILD-SW-002",
			Description: "Independent software review by a separate safety team",
			TargetASIL:  ASILD,
			Category:    CategorySoftware,
		},
		{
			ID:          "ASILD-HW-001",
			Description: "Hardware architectural metrics (SPFM ≥99%, LFM ≥90%) demonstrated",
			TargetASIL:  ASILD,
			Category:    CategoryHardware,
		},
		{
			ID:          "ASILD-HW-002",
			Description: "Hardware-software interface (HSI) specification verified",
			TargetASIL:  ASILD,
			Category:    CategoryHardware,
		},
		{
			ID:          "ASILD-MGT-001",
			Description: "Functional safety audit by an accredited third-party assessor",
			TargetASIL:  ASILD,
			Category:    CategorySafetyMgmt,
		},
		{
			ID:          "ASILD-MGT-002",
			Description: "Functional safety assessment report (FSA) accepted by OEM",
			TargetASIL:  ASILD,
			Category:    CategorySafetyMgmt,
		},
		{
			ID:          "ASILD-VV-001",
			Description: "Back-to-back testing between model and code at integration level",
			TargetASIL:  ASILD,
			Category:    CategoryVerification,
		},
		{
			ID:          "ASILD-VV-002",
			Description: "Fault injection testing covering all safety mechanisms",
			TargetASIL:  ASILD,
			Category:    CategoryVerification,
		},
	}
}
