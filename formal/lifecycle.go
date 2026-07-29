package formal

import (
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/lifecycle"
	"github.com/SoundMatt/go-RCP/regmap"
	"github.com/SoundMatt/go-RCP/server"
)

// LifecycleRank orders lifecycle.LifecycleState values for the
// monotonic-advance invariant LifecycleInvariants checks: a plausible
// trace's rank must never decrease from one state to the next, since
// server.Server only ever advances forward one step at a time (see
// lifecycle.LifecycleState's own doc comment) and every rejected
// transition attempt leaves the prior state unchanged rather than moving
// it backward.
func LifecycleRank(s lifecycle.LifecycleState) int {
	switch s {
	case lifecycle.StateUnconfigured:
		return 0
	case lifecycle.StateHWLocked:
		return 1
	case lifecycle.StateFullyConfigured:
		return 2
	default:
		return -1
	}
}

// LifecycleStateOf snapshots srv's current lifecycle state as a formal.State
// carrying both the human-readable name and LifecycleRank's numeric
// ordering, so LifecycleInvariants can compare successive snapshots without
// re-deriving the ordering itself.
func LifecycleStateOf(srv *server.Server) State {
	s := srv.State()
	return State{"lifecycle": s.String(), "lifecycle_rank": LifecycleRank(s)}
}

// LifecycleAction is one step a caller drives against srv while building a
// LifecycleTrace: a call through server.Server's own exported API (add an
// endpoint, set a pin assignment, attempt a state advance, ...). Its error
// return is intentionally not propagated by LifecycleTrace — see
// LifecycleTrace's own doc comment for why a rejected action is itself part
// of what a lifecycle trace needs to verify, not a reason to stop building
// one.
type LifecycleAction func(srv *server.Server) error

// LifecycleTrace drives srv through actions in order, recording
// LifecycleStateOf both before the first action and after every action — so
// a trace of n actions produces exactly n+1 states. An action's error is
// deliberately discarded here: this trace exists to verify what the
// server's *observable lifecycle state* does across a sequence of
// attempted transitions, valid or invalid, so a rejected transition
// attempt (which server.Server itself guarantees leaves the state
// unchanged) must appear in the trace as "no rank change" rather than
// short-circuit trace generation entirely.
func LifecycleTrace(srv *server.Server, actions []LifecycleAction) []State {
	trace := make([]State, 0, len(actions)+1)
	trace = append(trace, LifecycleStateOf(srv))
	for _, action := range actions {
		_ = action(srv)
		trace = append(trace, LifecycleStateOf(srv))
	}
	return trace
}

// LifecycleInvariants returns the temporal properties this package verifies
// against any LifecycleTrace: the observed rank never decreases (REQ-RCS's
// "states only ever advance one step at a time" rule, checked here at the
// trace level rather than trusted from server.Server's own unit tests
// alone), and a trace built from a fully plausible action sequence
// eventually reaches lifecycle.StateFullyConfigured.
func LifecycleInvariants() []Invariant {
	return []Invariant{
		{
			Name: "lifecycle rank never decreases",
			Check: func(trace []State) bool {
				prev := -1
				for _, s := range trace {
					rank, _ := s["lifecycle_rank"].(int) //nolint:errcheck
					if rank < prev {
						return false
					}
					prev = rank
				}
				return true
			},
		},
		{
			Name: "lifecycle eventually reaches fully-configured",
			Check: Eventually(func(s State) bool {
				rank, _ := s["lifecycle_rank"].(int) //nolint:errcheck
				return rank == LifecycleRank(lifecycle.StateFullyConfigured)
			}),
		},
	}
}

// NewValidLifecycleTrace returns a fresh *server.Server (with root claimed
// as its root client) and the plausible Unconfigured→HWLocked→
// FullyConfigured action sequence for one GPIO endpoint declared at addr —
// the same recipe server/lifecycle_test.go's own (server_test-internal)
// advanceToHWLocked helper uses, reproduced here since that helper isn't
// exported. Passing the returned actions to LifecycleTrace produces a trace
// that satisfies every LifecycleInvariants property.
func NewValidLifecycleTrace(root avtp.StreamID, addr avtp.ByteBusID) (*server.Server, []LifecycleAction, error) {
	srv := server.NewServer()
	if err := srv.ClaimRoot(root); err != nil {
		return nil, nil, err
	}
	actions := []LifecycleAction{
		func(s *server.Server) error { return s.AddEndpoint(root, addr, regmap.EndpointTypeGPIO) },
		func(s *server.Server) error {
			return s.SetPinAssignment(root, regmap.PinAssignment{Pin: 10, Endpoint: addr, SignalIndex: 0})
		},
		func(s *server.Server) error { return s.AdvanceToHWLocked() },
		func(s *server.Server) error { return s.WriteFunctional(root, addr, []byte{0x01}) },
		func(s *server.Server) error {
			return s.SetQueueConfig(root, regmap.QueueConfig{FlushThreshold: 4})
		},
		func(s *server.Server) error { return s.AdvanceToFullyConfigured() },
	}
	return srv, actions, nil
}

// NewOutOfOrderLifecycleTrace returns a fresh *server.Server (root claimed)
// and a single action that attempts AdvanceToFullyConfigured while still in
// lifecycle.StateUnconfigured — a rejected, out-of-order transition.
// LifecycleTrace over this action must produce a trace whose rank never
// moves off 0, exercising LifecycleInvariants' "never decreases" property
// against a trace that also never *increases*, distinguishing "rejected"
// from merely "never attempted."
func NewOutOfOrderLifecycleTrace(root avtp.StreamID) (*server.Server, []LifecycleAction, error) {
	srv := server.NewServer()
	if err := srv.ClaimRoot(root); err != nil {
		return nil, nil, err
	}
	actions := []LifecycleAction{
		func(s *server.Server) error { return s.AdvanceToFullyConfigured() },
	}
	return srv, actions, nil
}
