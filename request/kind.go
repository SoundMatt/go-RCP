package request

// Kind selects which conditional-request shape a Message's envelope carries.
// KindPlain is the zero value and is never itself present on the wire — a
// Message with avtp.FlagExtended unset carries no envelope at all and is
// treated as KindPlain internally (see Dispatcher.Submit); every other value
// is a byte this package's own envelope encoding writes as Body's first
// byte.
type Kind uint8

const (
	// KindPlain is the synthetic Kind Dispatcher assigns to a Message with
	// avtp.FlagExtended unset: the unconditional, immediate request shape
	// every Phase 14 endpoint type already implements. It never appears as
	// an encoded envelope's leading byte.
	KindPlain Kind = iota

	// KindCompound gates a write (or read) against a sequencer register's
	// current value: the inner request only reaches the endpoint if the
	// configured comparison holds. See Conditional and EncodeCompound.
	KindCompound

	// KindCompoundWait evaluates the same sequencer-gated condition as
	// KindCompound but never touches endpoint output — it only reports
	// whether the condition currently holds (and optionally advances the
	// sequencer), regardless of the outcome. See EncodeCompoundWait.
	KindCompoundWait

	// KindTriggered defers execution until another endpoint (identified by
	// its ByteBusID) reports at least one new trigger event, per that
	// endpoint's own DrainTriggers-style signal. See EncodeTriggered and
	// Dispatcher.RegisterTriggerSource.
	KindTriggered

	// KindChained forces sequential, in-order execution of multiple
	// requests against one endpoint within a single frame. See
	// ChainedSegment and EncodeChained.
	KindChained

	// KindTimed defers execution until a caller-supplied presentation-time
	// clock reaches a body-carried target — scheduled execution without
	// relying on the AVTPDU's own timestamped (TSCF) header (see
	// avtp.Header.Disposition for that separate, wire-level mechanism).
	// See EncodeTimed.
	KindTimed

	// KindCancelAll is the mandatory cancellation variant: it clears every
	// pending (Queued/Started/Executing) ticket a Dispatcher currently
	// holds, regardless of Kind, sequencer, or transaction. See
	// EncodeCancelAll.
	KindCancelAll

	// KindCancelTransaction is an optional, narrower cancellation variant:
	// it clears only the single pending ticket matching one
	// avtp.TransactionNum. See EncodeCancelTransaction.
	KindCancelTransaction

	// KindCancelSequencer is the second optional, narrower cancellation
	// variant: it clears every pending Compound/CompoundWait ticket gated
	// on one specific SequencerID, leaving tickets gated on other
	// sequencers (and every non-conditional ticket) untouched. See
	// EncodeCancelSequencer.
	KindCancelSequencer

	// kindCount is a sentinel marking the end of this package's recognized
	// Kind values; keep it last.
	kindCount
)

// Valid reports whether k is a wire-representable envelope Kind — every
// value strictly between KindPlain and kindCount. KindPlain itself is
// excluded: it is never encoded as an envelope's leading byte, so a decoded
// envelope claiming KindPlain is exactly as malformed as one claiming
// kindCount or higher.
func (k Kind) Valid() bool {
	return k > KindPlain && k < kindCount
}

// IsCancellation reports whether k is one of the three cancellation-request
// variants (the mandatory clear-all plus the two optional narrower ones).
func (k Kind) IsCancellation() bool {
	return k == KindCancelAll || k == KindCancelTransaction || k == KindCancelSequencer
}

// String renders k for logs and test failure messages.
func (k Kind) String() string {
	switch k {
	case KindPlain:
		return "Plain"
	case KindCompound:
		return "Compound"
	case KindCompoundWait:
		return "CompoundWait"
	case KindTriggered:
		return "Triggered"
	case KindChained:
		return "Chained"
	case KindTimed:
		return "Timed"
	case KindCancelAll:
		return "CancelAll"
	case KindCancelTransaction:
		return "CancelTransaction"
	case KindCancelSequencer:
		return "CancelSequencer"
	default:
		return "Unknown"
	}
}

// priorityRank fixes the cross-type execution-priority ordering Dispatcher.Pump
// applies when more than one ticket on the same endpoint becomes eligible to
// advance from Started to Executing within a single Pump call. Lower ranks
// run first. This ordering is this implementation's own reasoned, documented
// default (see doc.go's spec-fidelity note) rather than a verified
// transcription of the source specification's own table, consistent with
// this repo's established posture on spec ambiguity (see e.g. gpio/doc.go's
// eight-write-semantics note):
//
//  1. Cancellation (all three variants, ranked equally) always runs first —
//     a pending cancellation must retire the requests it targets before any
//     of them is allowed to execute, mirroring this repo's general
//     "reject/stop rather than silently race" posture (server's register-
//     locking rules, discovery's untimed-header requirement).
//  2. Chained, because a forced-sequential multi-step request's atomicity
//     guarantee would be meaningless if another ticket could interleave
//     between its segments.
//  3. Triggered, because it exists specifically to respond to an external
//     event as soon as that event is observed.
//  4. Timed, because it carries its own explicit target execution time —
//     already-elapsed by definition once Pump finds it due.
//  5. CompoundWait, because it is read-only (never touches endpoint output)
//     and resolving it first lets a caller observe sequencer state before
//     any gated write below it can change that same state.
//  6. Compound, the gated write itself.
//  7. Plain, the unconditional baseline with no ordering constraint of its
//     own, so it yields to every conditional kind above it.
var priorityRank = map[Kind]int{
	KindCancelAll:         0,
	KindCancelTransaction: 0,
	KindCancelSequencer:   0,
	KindChained:           1,
	KindTriggered:         2,
	KindTimed:             3,
	KindCompoundWait:      4,
	KindCompound:          5,
	KindPlain:             6,
}

// Priority returns k's fixed cross-type execution-priority rank (lower runs
// first). See priorityRank's doc comment for the full ordering and its
// rationale.
func (k Kind) Priority() int {
	return priorityRank[k]
}

// CompareOp is the comparison a Compound or CompoundWait request evaluates
// between a sequencer's current value and the request's Operand.
type CompareOp uint8

const (
	CompareEqual CompareOp = iota
	CompareNotEqual
	CompareLess
	CompareLessOrEqual
	CompareGreater
	CompareGreaterOrEqual

	// compareOpCount is a sentinel marking the end of this package's
	// recognized CompareOp values; keep it last.
	compareOpCount
)

// Valid reports whether op is one of this package's six recognized
// comparison operators.
func (op CompareOp) Valid() bool {
	return op < compareOpCount
}

// Evaluate reports whether current op operand holds.
func (op CompareOp) Evaluate(current, operand uint32) bool {
	switch op {
	case CompareEqual:
		return current == operand
	case CompareNotEqual:
		return current != operand
	case CompareLess:
		return current < operand
	case CompareLessOrEqual:
		return current <= operand
	case CompareGreater:
		return current > operand
	case CompareGreaterOrEqual:
		return current >= operand
	default:
		return false
	}
}

// String renders op for logs and test failure messages.
func (op CompareOp) String() string {
	switch op {
	case CompareEqual:
		return "=="
	case CompareNotEqual:
		return "!="
	case CompareLess:
		return "<"
	case CompareLessOrEqual:
		return "<="
	case CompareGreater:
		return ">"
	case CompareGreaterOrEqual:
		return ">="
	default:
		return "?"
	}
}
