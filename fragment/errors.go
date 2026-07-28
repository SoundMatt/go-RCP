package fragment

import "errors"

// Segmentation (send-side) errors.
var (
	// ErrInvalidMaxSegmentBody is returned by Split when maxBody is not a
	// positive number of bytes.
	ErrInvalidMaxSegmentBody = errors.New("rcp/fragment: maxBody must be positive")

	// ErrAlreadyFragmented is returned by Split when the Message it was
	// asked to split already carries avtp.FlagMoreSegments — Split's
	// contract is to fragment one complete logical message, not to
	// re-fragment a segment that is itself already part of a sequence.
	ErrAlreadyFragmented = errors.New("rcp/fragment: message already carries FlagMoreSegments")

	// ErrTooManySegments is returned by Split when a Message's Body would
	// require more segments than fit the wire's 16-bit segment-number field
	// (avtp.Message.ReadSizeOrSegment), and by Reassembler.Add when an
	// in-progress sequence's configured Config.MaxSegments bound is
	// exceeded before a terminal segment arrives.
	ErrTooManySegments = errors.New("rcp/fragment: message requires more segments than this package supports")
)

// Reassembly (receive-side) errors.
var (
	// ErrOutOfOrderSegment is returned by Reassembler.Add when a segment's
	// own ReadSizeOrSegment-as-segment-number does not match the next
	// segment number the sequence expects — see Reassembler's doc comment
	// for this package's own reasoned, documented in-order-only policy and
	// why a genuine gap or reordering abandons the sequence rather than
	// buffering out of order.
	ErrOutOfOrderSegment = errors.New("rcp/fragment: segment arrived out of order")

	// ErrDuplicateSegment is returned by Reassembler.Add when a segment
	// whose number matches one already accepted for its sequence arrives
	// again with different content than the copy already buffered. An
	// exact byte-for-byte repeat of the most recently accepted segment (or
	// of an already-completed sequence's terminal segment) is tolerated as
	// a harmless retransmission and does not return this error — see
	// Reassembler.Add's doc comment.
	ErrDuplicateSegment = errors.New("rcp/fragment: duplicate segment does not match the one already buffered")

	// ErrHeaderMismatch is returned by Reassembler.Add when a segment's
	// shared request-descriptor fields (Kind, Timestamp, and Control apart
	// from FlagMoreSegments) disagree with the first segment already
	// buffered for the same Key.
	ErrHeaderMismatch = errors.New("rcp/fragment: segment's shared descriptor fields do not match this sequence's first segment")

	// ErrSequenceComplete is returned by Reassembler.Add when a segment
	// (other than an exact repeat of the stored terminal segment) arrives
	// for a Key whose sequence has already received its terminal segment
	// and is only awaiting Finish/FinishProtected.
	ErrSequenceComplete = errors.New("rcp/fragment: sequence already received its terminal segment")

	// ErrUnknownSequence is returned by Finish and FinishProtected when Key
	// names no sequence this Reassembler currently holds (never started,
	// already finished, or swept away as stale).
	ErrUnknownSequence = errors.New("rcp/fragment: unrecognized reassembly sequence")

	// ErrIncomplete is returned by Finish and FinishProtected when Key
	// names a sequence that has not yet received its terminal segment.
	ErrIncomplete = errors.New("rcp/fragment: reassembly sequence has not yet received its terminal segment")
)

// Dispatcher-integration (gateway.go) errors.
var (
	// ErrAwaitingSegments is returned by Gateway.Submit when the inbound
	// Message was accepted as one segment of a still-incomplete sequence:
	// it has been buffered, but not yet handed to the wrapped
	// request.Dispatcher, so there is no request.TicketID to return yet.
	// It is deliberately distinct from request.ErrPending, which describes
	// an already-admitted ticket that has not yet resolved — this error
	// describes a message that has not been admitted at all.
	ErrAwaitingSegments = errors.New("rcp/fragment: segment buffered; awaiting this sequence's terminal segment before submission")
)
