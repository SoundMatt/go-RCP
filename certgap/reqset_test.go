//fusa:test REQ-CERT-009
//fusa:test REQ-CERT-010

package certgap_test

import (
	"testing"

	"github.com/SoundMatt/go-RCP/v9/certgap"
)

// REQ-CERT-009: BuildRequirementFamilies returns unique, ASIL-B-targeted,
// Met requirements, and BuildRegistry's Analyze("") reports full compliance
// across them (no gaps below the ASIL-D baseline).
func TestBuildRequirementFamilies_AllMetAndUnique(t *testing.T) {
	families := certgap.BuildRequirementFamilies()
	if len(families) == 0 {
		t.Fatal("BuildRequirementFamilies returned nothing")
	}
	seen := make(map[string]bool)
	for _, f := range families {
		if !f.Req.Met {
			t.Errorf("%s: not Met", f.Req.ID)
		}
		if f.Req.TargetASIL != certgap.ASILB {
			t.Errorf("%s: TargetASIL = %v, want ASILB", f.Req.ID, f.Req.TargetASIL)
		}
		if seen[f.Req.ID] {
			t.Errorf("duplicate Requirement ID %q", f.Req.ID)
		}
		seen[f.Req.ID] = true
		if f.Prefix == "" {
			t.Errorf("%s: empty Prefix", f.Req.ID)
		}
	}
}

// REQ-CERT-010: BuildRegistry's Analyze(ASILB) reports zero gaps (every
// family requirement is Met at ASIL-B), but Analyze(ASILD) reports exactly
// StandardASILDGaps' eight items as the gap — this milestone regenerates
// the requirement set, not the ASIL-D uplift baseline itself.
func TestBuildRegistry_ASILBClean_ASILDGapsUnchanged(t *testing.T) {
	reg := certgap.BuildRegistry()

	bReport := reg.Analyze(certgap.ASILB)
	if len(bReport.Gaps) != 0 {
		t.Errorf("ASIL-B gaps = %d, want 0", len(bReport.Gaps))
	}
	if bReport.ComplianceRatio != 1.0 {
		t.Errorf("ASIL-B compliance ratio = %v, want 1.0", bReport.ComplianceRatio)
	}

	dReport := reg.Analyze(certgap.ASILD)
	wantGaps := len(certgap.StandardASILDGaps())
	if len(dReport.Gaps) != wantGaps {
		t.Errorf("ASIL-D gaps = %d, want %d (StandardASILDGaps)", len(dReport.Gaps), wantGaps)
	}
	if dReport.MetCount != 0 {
		t.Errorf("ASIL-D MetCount = %d, want 0", dReport.MetCount)
	}
}
