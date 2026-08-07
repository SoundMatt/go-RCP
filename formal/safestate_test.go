//fusa:test REQ-FORM-015
//fusa:test REQ-FORM-016
//fusa:test REQ-FORM-017

package formal_test

import (
	"testing"
	"time"

	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/formal"
)

func safeStateStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x05, 0x11, 0x22, 0x33, 0x44, 0x55}, 1)
}

// REQ-FORM-015: an Observe → silence-past-timeout → Observe sequence
// satisfies every TimeoutInvariants property.
func TestTimeoutInvariants_TripsAndClears(t *testing.T) {
	stream := safeStateStream()
	sup, actions := formal.NewTimeoutTrace(stream, 100*time.Millisecond)
	trace := formal.SafeStateTrace(sup, stream, actions)

	checker := formal.NewChecker()
	for _, inv := range formal.TimeoutInvariants() {
		checker.Add(inv)
	}
	if err := checker.Check(trace); err != nil {
		t.Errorf("Check: %v", err)
	}
}

// REQ-FORM-016: a monotonicity violation followed by Reset satisfies every
// MonotonicityInvariants property.
func TestMonotonicityInvariants_TripsStickyThenResets(t *testing.T) {
	stream := safeStateStream()
	sup, actions := formal.NewMonotonicityTrace(stream)
	trace := formal.SafeStateTrace(sup, stream, actions)

	checker := formal.NewChecker()
	for _, inv := range formal.MonotonicityInvariants() {
		checker.Add(inv)
	}
	if err := checker.Check(trace); err != nil {
		t.Errorf("Check: %v", err)
	}
}

// REQ-FORM-017: a stream Supervisor has never observed at all is judged
// in-safe-state from its very first snapshot (e2e.Supervisor's own
// documented "never observed = already timed out" default), so
// SafeStateOf's initial trace entry (before any action runs) must already
// report true.
func TestSafeStateOf_NeverObserved_StartsTrue(t *testing.T) {
	stream := safeStateStream()
	sup, _ := formal.NewTimeoutTrace(stream, time.Hour)
	initial := formal.SafeStateOf(sup, stream)
	inSafe, _ := initial["in_safe_state"].(bool)
	if !inSafe {
		t.Error("in_safe_state = false for a never-observed stream, want true")
	}
}
