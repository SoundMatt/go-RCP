package request

// Kind selects which conditional-request shape a Message's envelope carries.
// KindPlain is the zero value and is never itself present on the wire — a
// Message with acf.FlagExtended unset carries no envelope at all and is
// treated as KindPlain internally (see Dispatcher.Submit); every other value
// is a byte this package's own envelope encoding writes as Body's first
// byte.
type Kind uint8

const (
	// KindPlain is the synthetic Kind Dispatcher assigns to a Message with
	// acf.FlagExtended unset: the unconditional, immediate request shape
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

// KindSafetyFlag is the "MSB-set" tag ROADMAP.md Milestone 50 adds on top
// of the base Kind byte: set on a request envelope's leading byte, it marks
// that request as the safety-request variant of whichever base Kind the
// remaining bits (see Base) identify. It is a strict superset encoding, not
// a fourth taxonomy alongside conditional/cancellation/plain — every
// safety-request Kind value is some base Kind ORed with this bit, and
// Base/IsSafety round-trip losslessly against each other. Only three base
// kinds have a defined safety counterpart (see the KindXxxSafety constants
// below and Valid); KindSafetyFlag set on any other base Kind byte is not a
// wire-representable value.
const KindSafetyFlag Kind = 0x80

// The safety-request ("MSB-set") variants of the three request kinds
// ROADMAP.md Milestone 50 calls out by name. Each is only ever executed by
// Dispatcher.Pump once the requester's addressed endpoint reports its
// configured safe state (see Dispatcher.SetSafeStateCheck) is currently
// active — a readiness gate layered on top of each kind's own existing
// readiness condition, the same way KindTimed's target-time gate composes
// with Kind.Priority rather than replacing it. Unlike every other pending
// ticket, these specifically survive Dispatcher.PurgeNonSafety, the
// watchdog-driven purge this milestone introduces (see doc.go's "Safety
// requests and the watchdog-driven purge" section). KindChained, KindTimed,
// and every cancellation variant have no safety counterpart: chained and
// timed requests already have their own scheduling gate, and a cancellation
// request's purpose is to retire other tickets, not to be gated on one
// itself.
const (
	KindCompoundSafety     Kind = KindCompound | KindSafetyFlag
	KindCompoundWaitSafety Kind = KindCompoundWait | KindSafetyFlag
	KindTriggeredSafety    Kind = KindTriggered | KindSafetyFlag
)

// IsSafety reports whether k carries KindSafetyFlag — i.e. is the
// safety-request variant of its Base kind.
func (k Kind) IsSafety() bool {
	return k&KindSafetyFlag != 0
}

// Base returns k with KindSafetyFlag cleared: the underlying conditional-
// request kind a safety-request variant is tagging. Base is a no-op on a
// Kind that was never safety-tagged in the first place, so callers that
// don't care about the safety distinction can always switch on Base(k)
// rather than k itself.
func (k Kind) Base() Kind {
	return k &^ KindSafetyFlag
}

// Valid reports whether k is a wire-representable envelope Kind: every
// non-safety value strictly between KindPlain and kindCount, plus exactly
// the three safety-tagged values KindCompoundSafety, KindCompoundWaitSafety,
// and KindTriggeredSafety. KindPlain itself is excluded (safety-tagged or
// not): it is never encoded as an envelope's leading byte, so a decoded
// envelope claiming KindPlain — or KindPlain|KindSafetyFlag — is exactly as
// malformed as one claiming kindCount or higher.
func (k Kind) Valid() bool {
	if k.IsSafety() {
		switch k.Base() {
		case KindCompound, KindCompoundWait, KindTriggered:
			return true
		default:
			return false
		}
	}
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
	case KindCompoundSafety:
		return "CompoundSafety"
	case KindCompoundWaitSafety:
		return "CompoundWaitSafety"
	case KindTriggeredSafety:
		return "TriggeredSafety"
	default:
		return "Unknown"
	}
}

// priorityRank fixes the cross-type execution-priority ordering Dispatcher.Pump
// applies when more than one ticket on the same endpoint becomes eligible to
// advance from Started to Executing within a single Pump call. Lower ranks
// run first. This ordering is a verbatim transcription of the governing OPEN
// Alliance TC18 Remote Control Protocol Specification's §12.9.2 "Priorities in
// execution" table (1 highest):
//
//  1. Cancellation (all three variants, ranked equally) — a pending
//     cancellation must retire the requests it targets before any of them is
//     allowed to execute.
//  2. Triggered, because it exists specifically to respond to an external
//     event as soon as that event is observed.
//  3. Timed, because it carries its own explicit target execution time —
//     already-elapsed by definition once Pump finds it due.
//  4. Compound, the sequencer-gated write.
//  5. CompoundWait, the read-only sequencer condition check.
//  6. Chained, the forced-sequential multi-step request.
//  7. Standard (Plain), the unconditional baseline with no ordering
//     constraint of its own, so it yields to every conditional kind above it.
//
// Within equal priority, Dispatcher.Pump preserves arrival order per §12.9.2.
var priorityRank = map[Kind]int{
	KindCancelAll:         0,
	KindCancelTransaction: 0,
	KindCancelSequencer:   0,
	KindTriggered:         1,
	KindTimed:             2,
	KindCompound:          3,
	KindCompoundWait:      4,
	KindChained:           5,
	KindPlain:             6,
}

// Priority returns k's fixed cross-type execution-priority rank (lower runs
// first). A safety-request variant ranks identically to its Base kind — this
// milestone's roadmap text distinguishes safety requests from their base
// kind purely by the configured-safe-state readiness gate and their
// immunity to Dispatcher.PurgeNonSafety (see doc.go), not by a separate
// priority tier, so this implementation does not invent one. See
// priorityRank's doc comment for the full base ordering and its rationale.
func (k Kind) Priority() int {
	return priorityRank[k.Base()]
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
