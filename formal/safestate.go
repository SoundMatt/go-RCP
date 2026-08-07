package formal

import (
	"time"

	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/e2e"
)

// SafeStateOf snapshots sup's current InSafeState verdict for stream as a
// formal.State.
func SafeStateOf(sup *e2e.Supervisor, stream avtp.StreamID) State {
	return State{"in_safe_state": sup.InSafeState(stream)}
}

// SafeStateAction is one step a caller drives while building a
// SafeStateTrace: an e2e.Supervisor.Observe call, an e2e.Supervisor.Reset
// call, or advancing the trace's injected clock with no Observe at all (to
// exercise the timeout path) — see NewTimeoutTrace and
// NewMonotonicityTrace.
type SafeStateAction func(sup *e2e.Supervisor) error

// SafeStateTrace drives sup through actions in order, recording
// SafeStateOf(sup, stream) both before the first action and after every
// action.
func SafeStateTrace(sup *e2e.Supervisor, stream avtp.StreamID, actions []SafeStateAction) []State {
	trace := make([]State, 0, len(actions)+1)
	trace = append(trace, SafeStateOf(sup, stream))
	for _, action := range actions {
		_ = action(sup)
		trace = append(trace, SafeStateOf(sup, stream))
	}
	return trace
}

// lastInSafeState reports the final trace state's in_safe_state value —
// used by the invariants below to assert what a trace ends in, distinct
// from Eventually's "at some point" and Always' "at every point."
func lastInSafeState(trace []State, want bool) bool {
	if len(trace) == 0 {
		return false
	}
	got, _ := trace[len(trace)-1]["in_safe_state"].(bool) //nolint:errcheck
	return got == want
}

// TimeoutInvariants returns the temporal properties this package verifies
// against a NewTimeoutTrace SafeStateTrace: the stream is not judged to
// require its safe state immediately after a fresh Observe
// (Eventually(¬in_safe_state), since e2e.Supervisor treats "never
// observed" as already-timed-out — see e2e/watchdog.go's StreamConfig.Timeout
// doc comment — a trace must show at least one state where activity has
// cleared that default), the watchdog eventually trips once the configured
// Timeout elapses with no further arrival (Eventually(in_safe_state)), and
// a fresh Observe after that trip clears it again by the trace's end —
// automatic safe-state entry on silence, automatic exit on resumed
// activity, matching e2e/doc.go's own integration example.
func TimeoutInvariants() []Invariant {
	inSafeState := func(s State) bool {
		v, _ := s["in_safe_state"].(bool) //nolint:errcheck
		return v
	}
	return []Invariant{
		{
			Name:  "clears at least once after activity resumes",
			Check: Eventually(func(s State) bool { return !inSafeState(s) }),
		},
		{
			Name:  "eventually trips once the configured timeout elapses",
			Check: Eventually(inSafeState),
		},
		{
			Name:  "clears again after the trip, by the trace's end",
			Check: func(trace []State) bool { return lastInSafeState(trace, false) },
		},
	}
}

// MonotonicityInvariants returns the temporal properties this package
// verifies against a NewMonotonicityTrace SafeStateTrace: a sequence
// violation is a *sticky* trip (Until: the safe-state verdict stays false
// until the violation, then stays true through every subsequent state up
// to Reset — checked here as "eventually true, and once true stays true
// until the trace's end" since Until itself requires q to hold exactly
// once and this trace's violation is immediately followed by more
// (still-tripped) arrivals rather than an immediate Reset), and Reset
// clears it again by the trace's end.
func MonotonicityInvariants() []Invariant {
	inSafeState := func(s State) bool {
		v, _ := s["in_safe_state"].(bool) //nolint:errcheck
		return v
	}
	return []Invariant{
		{
			Name:  "the violation eventually trips the safe-state verdict",
			Check: Eventually(inSafeState),
		},
		{
			Name: "the trip is sticky: once tripped, later valid arrivals don't clear it early",
			Check: func(trace []State) bool {
				// Locate the trip as a rising edge (false→true between
				// consecutive states) rather than raw truthiness, since a
				// stream Supervisor has never observed at all also reports
				// in_safe_state==true (see SafeStateOf's doc comment) —
				// that leading default is not itself "the trip" this
				// property is about.
				tripAt := -1
				for i := 1; i < len(trace); i++ {
					if !inSafeState(trace[i-1]) && inSafeState(trace[i]) {
						tripAt = i
						break
					}
				}
				if tripAt == -1 {
					return false // no violation-driven trip occurred at all
				}
				// Every state from the trip up to (but not including) the
				// trace's final state must stay tripped — Reset, which
				// NewMonotonicityTrace's last action performs, is the only
				// thing that may clear it, and only at the very end.
				for i := tripAt; i < len(trace)-1; i++ {
					if !inSafeState(trace[i]) {
						return false
					}
				}
				return true
			},
		},
		{
			Name:  "Reset clears the trip by the trace's end",
			Check: func(trace []State) bool { return lastInSafeState(trace, false) },
		},
	}
}

// NewTimeoutTrace returns an e2e.Supervisor driven by an injectable,
// action-advanced clock (no real-time sleeps) and the action sequence: an
// initial Observe, a clock advance past timeout with no further Observe
// (tripping InSafeState), and a fresh Observe (clearing it again).
func NewTimeoutTrace(stream avtp.StreamID, timeout time.Duration) (*e2e.Supervisor, []SafeStateAction) {
	now := time.Unix(0, 0)
	clock := func() time.Time { return now }
	sup := e2e.NewSupervisorWithClock(e2e.StreamConfig{Timeout: timeout}, clock)

	actions := []SafeStateAction{
		func(s *e2e.Supervisor) error { return s.Observe(stream, 0) },
		func(*e2e.Supervisor) error { now = now.Add(timeout / 2); return nil },
		func(*e2e.Supervisor) error { now = now.Add(timeout); return nil }, // total gap now exceeds timeout
		func(s *e2e.Supervisor) error { return s.Observe(stream, 1) },
	}
	return sup, actions
}

// NewMonotonicityTrace returns an e2e.Supervisor configured with
// RequireMonotonicSequence and a long Timeout (so only the monotonicity
// rule, not the timeout, can trip InSafeState here), and the action
// sequence: two consistent arrivals, a non-monotonic arrival (the sticky
// trip), one more arrival that is itself individually monotonic but arrives
// while still tripped, and a Reset.
func NewMonotonicityTrace(stream avtp.StreamID) (*e2e.Supervisor, []SafeStateAction) {
	now := time.Unix(0, 0)
	clock := func() time.Time { return now }
	sup := e2e.NewSupervisorWithClock(e2e.StreamConfig{
		Timeout:                  time.Hour,
		RequireMonotonicSequence: true,
	}, clock)

	actions := []SafeStateAction{
		func(s *e2e.Supervisor) error { return s.Observe(stream, 0) },
		func(s *e2e.Supervisor) error { return s.Observe(stream, 1) },
		func(s *e2e.Supervisor) error { return s.Observe(stream, 5) }, // violation: 5 != 1+1
		func(s *e2e.Supervisor) error { return s.Observe(stream, 6) }, // consistent with 5, but still tripped
		func(s *e2e.Supervisor) error { s.Reset(stream); return nil },
	}
	return sup, actions
}
