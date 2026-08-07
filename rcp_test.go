package rcp_test

//fusa:test REQ-ERR-001
//fusa:test REQ-ERR-002
//fusa:test REQ-ERR-003
//fusa:test REQ-ERR-004
//fusa:test REQ-ERR-005
//fusa:test REQ-ERR-006
//fusa:test REQ-ERR-007
//fusa:test REQ-ERR-008
//fusa:test REQ-ERR-009
//fusa:test REQ-ERR-010
//fusa:test REQ-ERR-011
//fusa:test REQ-SPEC-001

import (
	"errors"
	"testing"

	relay "github.com/SoundMatt/RELAY/v2"
	rcp "github.com/SoundMatt/go-RCP/v9"
)

// ── Sentinel errors ───────────────────────────────────────────────────────────

func TestErrors_NonNil(t *testing.T) {
	errs := []struct {
		name string
		err  error
	}{
		{"ErrClosed", rcp.ErrClosed},
		{"ErrNotConnected", rcp.ErrNotConnected},
		{"ErrTimeout", rcp.ErrTimeout},
		{"ErrPayloadTooLarge", rcp.ErrPayloadTooLarge},
		{"ErrNotFound", rcp.ErrNotFound},
	}
	for _, tc := range errs {
		if tc.err == nil {
			t.Errorf("%s is nil", tc.name)
		}
	}
}

// TestMandatoryErrors_AllDistinct verifies the four mandatory RELAY sentinels
// (spec §5.1) are pairwise distinct (none wraps another).
//
//fusa:test REQ-ERR-006
func TestMandatoryErrors_AllDistinct(t *testing.T) {
	sentinels := []struct {
		name string
		err  error
	}{
		{"ErrClosed", rcp.ErrClosed},
		{"ErrNotConnected", rcp.ErrNotConnected},
		{"ErrTimeout", rcp.ErrTimeout},
		{"ErrPayloadTooLarge", rcp.ErrPayloadTooLarge},
	}
	for i := range sentinels {
		for j := range sentinels {
			if i == j {
				continue
			}
			if errors.Is(sentinels[i].err, sentinels[j].err) {
				t.Errorf("%s matches %s via errors.Is — mandatory sentinels must be distinct",
					sentinels[i].name, sentinels[j].name)
			}
		}
	}
}

// TestErrors_WrapRelayChain verifies each mandatory rcp sentinel reaches its
// RELAY counterpart via errors.Is, and ErrNotFound reaches ErrNotConnected.
//
//fusa:test REQ-ERR-007
//fusa:test REQ-ERR-008
//fusa:test REQ-ERR-009
//fusa:test REQ-ERR-010
//fusa:test REQ-ERR-011
func TestErrors_WrapRelayChain(t *testing.T) {
	cases := []struct {
		child  error
		parent error
		name   string
	}{
		{rcp.ErrClosed, relay.ErrClosed, "ErrClosed→relay.ErrClosed"},
		{rcp.ErrNotConnected, relay.ErrNotConnected, "ErrNotConnected→relay.ErrNotConnected"},
		{rcp.ErrTimeout, relay.ErrTimeout, "ErrTimeout→relay.ErrTimeout"},
		{rcp.ErrPayloadTooLarge, relay.ErrPayloadTooLarge, "ErrPayloadTooLarge→relay.ErrPayloadTooLarge"},
		{rcp.ErrNotFound, rcp.ErrNotConnected, "ErrNotFound→ErrNotConnected"},
		{rcp.ErrNotFound, relay.ErrNotConnected, "ErrNotFound→relay.ErrNotConnected (transitive)"},
	}
	for _, tc := range cases {
		if !errors.Is(tc.child, tc.parent) {
			t.Errorf("errors.Is(%s) = false, want true", tc.name)
		}
	}
}

func TestErrors_IsDetectableWhenWrapped(t *testing.T) {
	wrap := func(sentinel error) error {
		return errors.Join(errors.New("outer context"), sentinel)
	}
	cases := []struct {
		name     string
		sentinel error
	}{
		{"ErrClosed", rcp.ErrClosed},
		{"ErrNotConnected", rcp.ErrNotConnected},
		{"ErrTimeout", rcp.ErrTimeout},
		{"ErrPayloadTooLarge", rcp.ErrPayloadTooLarge},
		{"ErrNotFound", rcp.ErrNotFound},
	}
	for _, tc := range cases {
		wrapped := wrap(tc.sentinel)
		if !errors.Is(wrapped, tc.sentinel) {
			t.Errorf("errors.Is(wrap(%s)) = false, want true", tc.name)
		}
	}
}

// ── SpecVersion ───────────────────────────────────────────────────────────────

func TestSpecVersion_MatchesRELAY(t *testing.T) {
	if rcp.SpecVersion != relay.SpecVersion {
		t.Errorf("SpecVersion = %q, want relay.SpecVersion %q", rcp.SpecVersion, relay.SpecVersion)
	}
}
