package request

import "errors"

// Encoding errors.
var (
	// ErrShortBuffer is returned when a buffer is too short to hold the
	// structure a decoder was asked to parse.
	ErrShortBuffer = errors.New("rcp/request: buffer too short")

	// ErrTrailingBytes is returned when a decoder is handed more bytes than
	// its structure declares, the same "don't silently ignore extra input"
	// posture the rest of this repo's decoders take.
	ErrTrailingBytes = errors.New("rcp/request: buffer longer than declared structure length")

	// ErrInvalidKind is returned when a decoded envelope Kind byte is not
	// one of this package's recognized, wire-representable values (i.e. not
	// KindPlain, and not one of the Kind constants above kindCount).
	ErrInvalidKind = errors.New("rcp/request: unrecognized conditional-request kind")

	// ErrWrongKind is returned when a Decode function for one specific Kind
	// is handed a buffer whose leading Kind byte declares a different one.
	ErrWrongKind = errors.New("rcp/request: buffer's kind tag does not match the decoder called")

	// ErrInvalidCompareOp is returned when a decoded CompareOp is not one of
	// this package's six recognized comparison operators.
	ErrInvalidCompareOp = errors.New("rcp/request: unrecognized sequencer compare operator")

	// ErrInvalidSegmentCount is returned when a KindChained envelope
	// declares zero segments, or more than maxChainedSegments.
	ErrInvalidSegmentCount = errors.New("rcp/request: chained request segment count out of range")
)

// Submission errors.
var (
	// ErrNotExtended is returned by Submit when a caller explicitly asks for
	// conditional-request decoding (DecodeAny) on a Message that does not
	// have acf.FlagExtended set — there is no envelope to decode.
	ErrNotExtended = errors.New("rcp/request: message does not carry the FlagExtended conditional-request envelope")

	// ErrUnknownTicket is returned when a caller addresses a TicketID this
	// Dispatcher has never issued, or has already garbage-collected after
	// its response was collected.
	ErrUnknownTicket = errors.New("rcp/request: unrecognized ticket id")

	// ErrPending is returned by Dispatch when the submitted request's Kind
	// cannot resolve synchronously (Triggered or Timed, until its condition
	// is satisfied by a later Pump call). The caller's TicketID is still
	// valid and can be polled via Dispatcher.Response.
	ErrPending = errors.New("rcp/request: request accepted but not yet resolved; poll Dispatcher.Response after Pump")

	// ErrChainedSegmentFailed is returned (wrapped with fmt.Errorf's %w) when
	// one segment of a chained request fails; the segments after it are
	// deliberately not executed — see ChainedResult's doc comment.
	ErrChainedSegmentFailed = errors.New("rcp/request: chained request segment failed; remaining segments not executed")

	// ErrTicketCancelled is the error Dispatcher.Response reports for a
	// ticket a cancellation request (KindCancelAll/CancelTransaction/
	// CancelSequencer) cleared before it reached StateExecuting.
	ErrTicketCancelled = errors.New("rcp/request: ticket cancelled before execution")

	// ErrSafeStateNotConfigured is returned by Submit when a request's Kind
	// is a safety-request ("MSB-set") variant (ROADMAP.md Milestone 50) but
	// this Dispatcher has no SafeStateCheck configured (see
	// Dispatcher.SetSafeStateCheck). Safety requests are only meaningful
	// against an endpoint that has actually opted into a configured safe
	// state; admitting one with no way to ever evaluate that gate would
	// leave it pending forever instead of surfacing the misconfiguration.
	ErrSafeStateNotConfigured = errors.New("rcp/request: safety-request kind submitted but no SafeStateCheck configured")

	// ErrPurgedByWatchdog is the error Dispatcher.Response reports for a
	// ticket Dispatcher.PurgeNonSafety cleared. It is deliberately distinct
	// from ErrTicketCancelled: a watchdog-driven purge is this Dispatcher's
	// own fault-response action, not a request the client itself made (see
	// PurgeNonSafety's doc comment).
	ErrPurgedByWatchdog = errors.New("rcp/request: ticket purged by watchdog-driven safe-state entry")
)
