//fusa:req REQ-FORM-001
//fusa:req REQ-FORM-002
//fusa:req REQ-FORM-003
//fusa:req REQ-FORM-004
//fusa:req REQ-FORM-005
//fusa:req REQ-FORM-006
//fusa:req REQ-FORM-007
//fusa:req REQ-FORM-008

// Package formal provides lightweight formal-verification helpers for go-RCP.
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
	Name    string
	Check   func([]State) bool
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
