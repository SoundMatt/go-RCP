package request

import "sync"

// SequencerID addresses one persistent state register within a Sequencer
// bank. It is scoped to the Dispatcher (and therefore the endpoint) that
// owns the Sequencer instance — the same SequencerID value on two different
// endpoints' Dispatchers refers to two unrelated registers.
type SequencerID uint16

// Sequencer is the persistent per-sequencer state-register bank that
// KindCompound and KindCompoundWait requests read and advance (ROADMAP.md
// Milestone 49). Every register starts at zero and exists implicitly the
// first time it's addressed — there is no separate "declare a sequencer"
// step, mirroring how gpio's pin-value word needs no allocation beyond
// Configure. All exported methods are safe for concurrent use.
type Sequencer struct {
	mu     sync.Mutex
	values map[SequencerID]uint32
}

// NewSequencer returns an empty Sequencer bank; every register reads as
// zero until first Set or Advance.
func NewSequencer() *Sequencer {
	return &Sequencer{values: make(map[SequencerID]uint32)}
}

// Get returns id's current value (zero if never set or advanced).
func (s *Sequencer) Get(id SequencerID) uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.values[id]
}

// Set overwrites id's value directly, bypassing the advance-by-delta path
// Compound/CompoundWait requests use. Intended for test setup and operator
// tooling, not for the request-handling path itself.
func (s *Sequencer) Set(id SequencerID, v uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[id] = v
}

// Advance adds delta (which may be negative) to id's current value and
// returns the result. The addition wraps modulo 2^32 rather than
// saturating — unlike gpio's saturating-arithmetic write semantics, a
// sequencer register is a free-running counter with no declared active-bit
// mask to clamp against, so wraparound is this package's own reasoned,
// documented choice (see doc.go's spec-fidelity note) for what "advance by a
// (possibly negative) count" means with no declared bound.
func (s *Sequencer) Advance(id SequencerID, delta int32) uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.values[id] + uint32(delta)
	s.values[id] = next
	return next
}

// Conditional is the shared sequencer-gating shape KindCompound and
// KindCompoundWait requests both carry: which register to read, the
// comparison to evaluate against Operand, and how far to advance the
// register when that comparison holds.
type Conditional struct {
	// Sequencer identifies which register this request reads and
	// (conditionally) advances.
	Sequencer SequencerID

	// Op is the comparison evaluated as (current-value Op Operand).
	Op CompareOp

	// Operand is the right-hand side of the comparison.
	Operand uint32

	// AdvanceOnMatch is added to the sequencer's value (see
	// Sequencer.Advance) only when Op.Evaluate(current, Operand) is true.
	// A false-evaluating request leaves the register untouched — this
	// package's own reasoned choice (see doc.go) for what "gated" means:
	// an unmet condition has no side effect at all, matching how a plain
	// read request never mutates state either.
	AdvanceOnMatch int32
}

// Valid reports whether c's Op is one of this package's recognized
// comparison operators. It does not (and cannot) validate Sequencer, Operand,
// or AdvanceOnMatch — every SequencerID and every int32/uint32 value is a
// legal Conditional field.
func (c Conditional) Valid() bool {
	return c.Op.Valid()
}

// evaluate reports whether c's condition holds against seq's current value
// for c.Sequencer, and — only when it holds — advances that register by
// c.AdvanceOnMatch. It returns whether the condition matched and the
// register's value immediately after this call (advanced on a match,
// unchanged otherwise).
func (c Conditional) evaluate(seq *Sequencer) (matched bool, value uint32) {
	current := seq.Get(c.Sequencer)
	if !c.Op.Evaluate(current, c.Operand) {
		return false, current
	}
	return true, seq.Advance(c.Sequencer, c.AdvanceOnMatch)
}
