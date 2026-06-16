//fusa:test REQ-CERT-001
//fusa:test REQ-CERT-002
//fusa:test REQ-CERT-003
//fusa:test REQ-CERT-004
//fusa:test REQ-CERT-005
//fusa:test REQ-CERT-006
//fusa:test REQ-CERT-007
//fusa:test REQ-CERT-008

package certgap_test

import (
	"testing"

	"github.com/SoundMatt/go-RCP/certgap"
)

func populatedRegistry() *certgap.Registry {
	r := certgap.NewRegistry()
	for _, req := range certgap.StandardASILDGaps() {
		r.Add(req)
	}
	return r
}

// REQ-CERT-001: Registry.Add registers a requirement and All returns it.
func TestRegistry_Add(t *testing.T) {
	r := certgap.NewRegistry()
	r.Add(certgap.Requirement{ID: "TEST-001", TargetASIL: certgap.ASILD, Category: certgap.CategorySoftware})
	all := r.All()
	if len(all) != 1 {
		t.Fatalf("All() len = %d, want 1", len(all))
	}
	if all[0].ID != "TEST-001" {
		t.Errorf("ID = %q, want TEST-001", all[0].ID)
	}
}

// REQ-CERT-002: Registry.SetMet marks a requirement as met.
func TestRegistry_SetMet(t *testing.T) {
	r := populatedRegistry()
	if err := r.SetMet("ASILD-SW-001", true); err != nil {
		t.Fatalf("SetMet: %v", err)
	}
	report := r.Analyze(certgap.ASILD)
	if report.MetCount == 0 {
		t.Error("MetCount should be > 0 after SetMet")
	}
}

// REQ-CERT-003: Registry.SetMet returns error for unknown requirement ID.
func TestRegistry_SetMet_Unknown(t *testing.T) {
	r := certgap.NewRegistry()
	if err := r.SetMet("NONEXISTENT", true); err == nil {
		t.Error("want error for unknown requirement, got nil")
	}
}

// REQ-CERT-004: Analyze returns Total=0 for an empty registry.
func TestAnalyze_EmptyRegistry(t *testing.T) {
	r := certgap.NewRegistry()
	report := r.Analyze(certgap.ASILD)
	if report.Total != 0 {
		t.Errorf("Total = %d, want 0", report.Total)
	}
	if report.ComplianceRatio != 0.0 {
		t.Errorf("ComplianceRatio = %f, want 0", report.ComplianceRatio)
	}
}

// REQ-CERT-005: Analyze with targetASIL="" includes all requirements.
func TestAnalyze_AllASILs(t *testing.T) {
	r := certgap.NewRegistry()
	r.Add(certgap.Requirement{ID: "A", TargetASIL: certgap.ASILB})
	r.Add(certgap.Requirement{ID: "B", TargetASIL: certgap.ASILD})
	report := r.Analyze("") // all
	if report.Total != 2 {
		t.Errorf("Total = %d, want 2", report.Total)
	}
}

// REQ-CERT-006: Analyze with specific ASIL filters correctly.
func TestAnalyze_FilteredASIL(t *testing.T) {
	r := certgap.NewRegistry()
	r.Add(certgap.Requirement{ID: "A", TargetASIL: certgap.ASILB})
	r.Add(certgap.Requirement{ID: "B", TargetASIL: certgap.ASILD})
	report := r.Analyze(certgap.ASILD)
	if report.Total != 1 {
		t.Errorf("Total = %d, want 1 (only ASIL-D)", report.Total)
	}
}

// REQ-CERT-007: GapReport.ComplianceRatio equals MetCount/Total.
func TestAnalyze_ComplianceRatio(t *testing.T) {
	r := populatedRegistry()
	_ = r.SetMet("ASILD-SW-001", true)
	_ = r.SetMet("ASILD-SW-002", true)
	report := r.Analyze(certgap.ASILD)
	want := float64(2) / float64(report.Total)
	if report.ComplianceRatio != want {
		t.Errorf("ComplianceRatio = %f, want %f", report.ComplianceRatio, want)
	}
}

// REQ-CERT-008: StandardASILDGaps returns exactly 8 predefined requirements.
func TestStandardASILDGaps(t *testing.T) {
	gaps := certgap.StandardASILDGaps()
	if len(gaps) != 8 {
		t.Errorf("StandardASILDGaps len = %d, want 8", len(gaps))
	}
	for _, g := range gaps {
		if g.TargetASIL != certgap.ASILD {
			t.Errorf("gap %q has ASIL %s, want ASIL-D", g.ID, g.TargetASIL)
		}
	}
}
