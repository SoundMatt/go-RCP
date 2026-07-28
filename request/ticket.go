package request

import (
	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
)

// TicketID identifies one in-flight or resolved request within a
// Dispatcher's own bookkeeping. It has no meaning outside the Dispatcher
// instance that issued it, and no wire representation — the wire-level
// correlation handle is (and remains) acf.Message.TransactionNum.
type TicketID uint64

// State is one stage of the request lifecycle state machine every request
// Kind — including KindPlain, the already-shipped Phase 14 shape — is
// retrofitted onto (ROADMAP.md Milestone 49).
type State uint8

const (
	// StateQueued is a ticket's initial state: Dispatcher.Submit accepted
	// and decoded it, but has not yet begun evaluating whether it can run.
	StateQueued State = iota

	// StateStarted means admission is complete and this ticket is now
	// waiting on (or immediately eligible for) its kind-specific readiness
	// condition: KindTriggered waits on a trigger source's pump, KindTimed
	// waits on a target time, and every other kind is immediately ready.
	StateStarted

	// StateExecuting means this ticket's readiness condition is satisfied
	// and Dispatcher is running its kind-specific action (an inner
	// Handler.HandleRequest call, a sequencer evaluation, or a
	// cancellation sweep). This package always resolves StateExecuting to
	// StateFinalized synchronously within one Dispatcher.Pump call — there
	// is no observable window where a ticket sits in this state across two
	// Pump calls (see dispatcher.go's design note on why Handler calls are
	// made in Pump's own critical section) — but it remains a distinct,
	// named stage because it is exactly where the roadmap's per-kind
	// "type-specific behaviour at each transition" lives.
	StateExecuting

	// StateFinalized is the terminal state: Dispatcher.Response returns
	// this ticket's outcome (success or error) rather than ErrPending.
	// Includes tickets a cancellation request cleared before they ever
	// reached StateExecuting.
	StateFinalized
)

// String renders s for logs and test failure messages.
func (s State) String() string {
	switch s {
	case StateQueued:
		return "Queued"
	case StateStarted:
		return "Started"
	case StateExecuting:
		return "Executing"
	case StateFinalized:
		return "Finalized"
	default:
		return "Unknown"
	}
}

// ticket is one Dispatcher-internal request instance. Callers never see this
// type directly — they address a ticket by its TicketID through Dispatcher's
// own methods (Submit, Pump, Response, StateOf, Forget).
type ticket struct {
	id        TicketID
	requester avtp.StreamID
	original  acf.Message
	kind      Kind
	state     State

	// Decoded kind-specific fields. Only the ones relevant to kind are
	// populated; the rest sit at their zero value.
	conditional     Conditional
	innerControl    acf.ControlFlags
	innerBody       []byte
	triggerSource   avtp.ByteBusID
	executeAtMicros uint64
	segments        []ChainedSegment
	cancelTxn       avtp.TransactionNum
	cancelSeq       SequencerID

	// Outcome, populated once state reaches StateFinalized.
	response acf.Message
	err      error
}
