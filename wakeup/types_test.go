//fusa:test REQ-WAKEUP-003

package wakeup_test

import (
	"testing"

	"github.com/SoundMatt/go-RCP/v9/wakeup"
)

// TestPowerState_Valid checks Valid recognizes exactly the four defined
// power states (REQ-WAKEUP-003).
func TestPowerState_Valid(t *testing.T) {
	for _, s := range []wakeup.PowerState{wakeup.PowerNormal, wakeup.PowerStandBy, wakeup.PowerSleep, wakeup.PowerUnpowered} {
		if !s.Valid() {
			t.Errorf("PowerState(%d).Valid() = false, want true", s)
		}
	}
	for _, s := range []wakeup.PowerState{4, 5, 255} {
		if s.Valid() {
			t.Errorf("PowerState(%d).Valid() = true, want false", s)
		}
	}
}

// TestPowerState_String and TestStartKind_String check every defined value
// renders a distinct, non-empty name.
func TestPowerState_String(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range []wakeup.PowerState{wakeup.PowerNormal, wakeup.PowerStandBy, wakeup.PowerSleep, wakeup.PowerUnpowered} {
		str := s.String()
		if str == "" || str == "Unknown" {
			t.Errorf("PowerState(%d).String() = %q, want a distinct non-empty name", s, str)
		}
		if seen[str] {
			t.Errorf("PowerState(%d).String() = %q, want a name not already used by another state", s, str)
		}
		seen[str] = true
	}
	if got := wakeup.PowerState(99).String(); got != "Unknown" {
		t.Errorf("PowerState(99).String() = %q, want Unknown", got)
	}
}

func TestStartKind_String(t *testing.T) {
	if got := wakeup.StartHot.String(); got != "Hot" {
		t.Errorf("StartHot.String() = %q, want Hot", got)
	}
	if got := wakeup.StartCold.String(); got != "Cold" {
		t.Errorf("StartCold.String() = %q, want Cold", got)
	}
	if got := wakeup.StartUnknown.String(); got != "Unknown" {
		t.Errorf("StartUnknown.String() = %q, want Unknown", got)
	}
}
