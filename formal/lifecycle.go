package formal

import (
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/lifecycle"
	"github.com/SoundMatt/go-RCP/v9/regmap"
	"github.com/SoundMatt/go-RCP/v9/server"
)

// LifecycleRank orders lifecycle.LifecycleState values for the two
// transition-shape invariants LifecycleInvariants checks: server.Server
// advances forward one rank at a time, may fall back exactly one rank from
// StateHWLocked to StateUnconfigured via server.Server.DemoteToUnconfigured,
// and — once rank reaches lifecycle.StateFullyConfigured — never leaves it
// again (see lifecycle.LifecycleState's own doc comment). Every rejected
// transition or demotion attempt leaves the prior state unchanged rather
// than moving it at all.
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
// against any LifecycleTrace: every step's rank change has a legal shape
// (checked here at the trace level rather than trusted from server.Server's
// own unit tests alone), rank never regresses once it has reached
// lifecycle.StateFullyConfigured, and a trace built from a fully plausible
// action sequence eventually reaches lifecycle.StateFullyConfigured.
func LifecycleInvariants() []Invariant {
	unconfiguredRank := LifecycleRank(lifecycle.StateUnconfigured)
	hwLockedRank := LifecycleRank(lifecycle.StateHWLocked)
	fullyConfiguredRank := LifecycleRank(lifecycle.StateFullyConfigured)
	return []Invariant{
		{
			// A step either holds rank steady (an attempted transition or
			// demotion was rejected), advances it by exactly one (a
			// successful AdvanceToHWLocked/AdvanceToFullyConfigured), or
			// falls back by exactly one from StateHWLocked to
			// StateUnconfigured (the one legal reverse transition,
			// server.Server.DemoteToUnconfigured). Any other change —
			// skipping a rank forward, falling back from
			// StateFullyConfigured, or falling back more than one rank —
			// is illegal and fails this invariant.
			Name: "lifecycle rank only ever changes by a legal transition",
			Check: func(trace []State) bool {
				prev := -1
				for i, s := range trace {
					rank, _ := s["lifecycle_rank"].(int) //nolint:errcheck
					if i == 0 {
						prev = rank
						continue
					}
					switch {
					case rank == prev: // rejected attempt: no change
					case rank == prev+1: // legal forward advance
					case prev == hwLockedRank && rank == unconfiguredRank: // legal demotion
					default:
						return false
					}
					prev = rank
				}
				return true
			},
		},
		{
			Name: "lifecycle never regresses once fully-configured",
			Check: func(trace []State) bool {
				sawFullyConfigured := false
				for _, s := range trace {
					rank, _ := s["lifecycle_rank"].(int) //nolint:errcheck
					if sawFullyConfigured && rank != fullyConfiguredRank {
						return false
					}
					if rank == fullyConfiguredRank {
						sawFullyConfigured = true
					}
				}
				return true
			},
		},
		{
			Name: "lifecycle eventually reaches fully-configured",
			Check: Eventually(func(s State) bool {
				rank, _ := s["lifecycle_rank"].(int) //nolint:errcheck
				return rank == fullyConfiguredRank
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

// NewDemotionThenReconfigureLifecycleTrace returns a fresh *server.Server
// (root claimed) and an action sequence that reaches StateHWLocked, demotes
// back to StateUnconfigured via server.Server.DemoteToUnconfigured, then
// re-advances through StateHWLocked to StateFullyConfigured — exercising the
// one legal reverse transition LifecycleInvariants' "legal transition
// shape" property permits, followed by a normal forward re-run. The
// endpoint declaration and pin assignment from before the demotion are
// still in place afterward (DemoteToUnconfigured reopens them for writing;
// it does not clear them — see DemoteToUnconfigured's doc comment), so the
// re-advance only needs to repeat AdvanceToHWLocked itself, not
// AddEndpoint/SetPinAssignment.
func NewDemotionThenReconfigureLifecycleTrace(root avtp.StreamID, addr avtp.ByteBusID) (*server.Server, []LifecycleAction, error) {
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
		func(s *server.Server) error { return s.DemoteToUnconfigured(root) },
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
