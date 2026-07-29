//fusa:req REQ-FORM-001
//fusa:req REQ-FORM-002
//fusa:req REQ-FORM-003
//fusa:req REQ-FORM-004
//fusa:req REQ-FORM-005
//fusa:req REQ-FORM-006
//fusa:req REQ-FORM-007
//fusa:req REQ-FORM-008
//fusa:req REQ-FORM-009
//fusa:req REQ-FORM-010
//fusa:req REQ-FORM-011
//fusa:req REQ-FORM-012
//fusa:req REQ-FORM-013
//fusa:req REQ-FORM-014
//fusa:req REQ-FORM-015
//fusa:req REQ-FORM-016
//fusa:req REQ-FORM-017

// Package formal provides lightweight formal-verification helpers for go-RCP,
// and — as of ROADMAP.md Milestone 58 (v0.71.0) — the concrete
// Invariant/Checker wiring that applies them to this program's actual
// safety-relevant state machines.
//
// Automotive safety standards (ISO 26262, IEC 61508) require that safety
// mechanisms be verified beyond unit testing. This package implements:
//
//   - Invariant: a predicate that must hold over a sequence of states
//   - LTL (Linear Temporal Logic) operators: Always, Eventually, Until
//   - StateSequence: generates state traces for model checking
//   - Checker: runs an invariant against a generated trace
//
// These primitives allow engineers to express and verify temporal safety
// properties without an external model checker.
//
// # Milestone 58: concrete state-machine proofs
//
// Through Milestone 57 (v0.70.0) this package was pure engine: the
// primitives above, exercised only by a trivial integer-counter trace in
// formal_test.go. Milestone 41 (v0.41.0), under the pre-TC18 bespoke
// Zone/Command protocol, produced TLA+ (`.tla`/`.cfg`) proofs of that
// protocol's own zone-health, client-push-watchdog, and anti-replay-window
// state machines — none of which exist in this program anymore (the last
// of that protocol's satellite packages migrated off by Milestone 57; the
// legacy API itself is scheduled for removal at Milestone 59's Phase 18
// cutover). This package never carried a Go-native equivalent for either
// era's machines until now.
//
// Milestone 58 adds three files, each wiring this package's own primitives
// (not a TLA+/TLC toolchain — see the note below) to one of this program's
// three safety-relevant state machines, and each following the same
// three-part shape: a StateOf snapshot function, an Action type plus a
// Trace driver, and an Invariants function returning the temporal
// properties a plausible Trace must satisfy:
//
//   - lifecycle.go: server.Server's Unconfigured→HWLocked→FullyConfigured
//     configuration-readiness axis (ROADMAP.md Milestone 44/45,
//     v0.58.0-era). LifecycleInvariants checks the observed rank never
//     decreases and a plausible action sequence eventually reaches
//     lifecycle.StateFullyConfigured.
//   - power.go: wakeup.Endpoint's Normal/StandBy/Sleep power model plus
//     powerstate.Driver's wake-handshake retransmission pacing (Milestone
//     51/53, v0.66.0-era). PowerInvariants checks
//     wakeup.PowerUnpowered is never Endpoint's own observed state, the
//     retransmission queue eventually drains, and a Sleep→Normal wake
//     eventually determines a cold/hot-start kind.
//   - safestate.go: e2e.Supervisor's automatic safe-state-entry watchdog
//     (Milestone 50, v0.63.0-era). TimeoutInvariants and
//     MonotonicityInvariants together check both ways InSafeState can
//     trip (inter-arrival timeout, sequence-monotonicity violation), that
//     a monotonicity trip is sticky until Reset, and that resumed activity
//     (a fresh Observe) clears a timeout trip.
//
// Each file's own doc comments carry the reasoning specific to that state
// machine; this comment only records why they exist and what each proves.
//
// # A note on this package's proof method (Guiding Principle 10)
//
// The original Milestone 41 proofs used TLA+ and the TLC model checker —
// external, industry-standard tools that exhaustively explore a specified
// state space. This package's Invariant/Checker primitives, by contrast,
// check named temporal properties against *specific, code-driven* traces
// (the exact action sequences NewValidLifecycleTrace, NewWakeCycleTrace,
// NewTimeoutTrace, and NewMonotonicityTrace each build) — closer to
// property-based testing over real API calls than to exhaustive model
// checking over an abstract specification. This is a deliberate,
// documented scope reduction, not an oversight: ROADMAP.md's own framing
// for this milestone says the modeling *methodology* carries over, not
// that a TLA+/TLC toolchain must be reintroduced, and nothing in this
// program depends on TLC-specific tooling (no `tla/` directory, no
// TLC-formatted counter-example traces) the way the retired proofs did.
// A future pass wanting exhaustive state-space coverage (rather than
// targeted, hand-built traces) would need to reintroduce an external model
// checker; this package's own Checker only ever tells you whether the
// specific trace you handed it satisfies the specific invariants you
// registered.
package formal

import "fmt"

// State is a snapshot of observable system state.
type State map[string]any

// Predicate is a function that evaluates to true or false for a given State.
type Predicate func(State) bool

// ─── LTL operators ────────────────────────────────────────────────────────────

// Always returns a Predicate that holds iff p holds in every state of the trace.
// This models the LTL □ (box) operator.
func Always(p Predicate) func([]State) bool {
	return func(trace []State) bool {
		for _, s := range trace {
			if !p(s) {
				return false
			}
		}
		return true
	}
}

// Eventually returns a Predicate that holds iff p holds in at least one state.
// This models the LTL ◇ (diamond) operator.
func Eventually(p Predicate) func([]State) bool {
	return func(trace []State) bool {
		for _, s := range trace {
			if p(s) {
				return true
			}
		}
		return false
	}
}

// Until returns a trace predicate that holds iff p holds continuously until q
// becomes true. q must eventually become true.
// This models the LTL p U q operator.
func Until(p, q Predicate) func([]State) bool {
	return func(trace []State) bool {
		for i, s := range trace {
			if q(s) {
				// q holds now: verify p held in all prior states
				for _, prior := range trace[:i] {
					if !p(prior) {
						return false
					}
				}
				return true
			}
			if !p(s) {
				return false
			}
		}
		return false // q never held
	}
}

// ─── StateSequence ────────────────────────────────────────────────────────────

// Generator produces the next State given the current one.
type Generator func(current State) State

// StateSequence generates a trace of n states starting from initial,
// applying gen at each step.
func StateSequence(initial State, gen Generator, n int) []State {
	trace := make([]State, 0, n)
	s := initial
	for i := 0; i < n; i++ {
		trace = append(trace, s)
		s = gen(s)
	}
	return trace
}

// ─── Invariant ────────────────────────────────────────────────────────────────

// Invariant describes a named temporal property over a state trace.
type Invariant struct {
	Name  string
	Check func([]State) bool
}

// ─── Checker ─────────────────────────────────────────────────────────────────

// ViolationError is returned when an invariant is falsified.
type ViolationError struct {
	Invariant string
}

func (e *ViolationError) Error() string {
	return fmt.Sprintf("formal: invariant %q falsified", e.Invariant)
}

// Checker runs a set of Invariants against a generated trace.
type Checker struct {
	invariants []Invariant
}

// NewChecker returns an empty Checker.
func NewChecker() *Checker { return &Checker{} }

// Add registers an Invariant with the Checker.
func (c *Checker) Add(inv Invariant) { c.invariants = append(c.invariants, inv) }

// Check runs all invariants against trace. Returns the first ViolationError
// found, or nil if all invariants hold.
func (c *Checker) Check(trace []State) error {
	for _, inv := range c.invariants {
		if !inv.Check(trace) {
			return &ViolationError{Invariant: inv.Name}
		}
	}
	return nil
}
