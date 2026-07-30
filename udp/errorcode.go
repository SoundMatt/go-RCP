package udp

import (
	"errors"

	"github.com/SoundMatt/go-RCP/request"
)

// ErrorCode is the numeric error-response code errorResponse carries as the
// first byte of a wire-level error response's Body, drawn from the OPEN
// Alliance TC18 Remote Control Protocol Specification's fixed
// request-response error-code enumeration rather than an internal Go
// error's own free-text message. See errorResponse and errorCodeFor.
type ErrorCode uint8

// This package implements only the subset of the specification's full
// error-code enumeration its own errorCodeFor mapping currently selects
// (eight of the specification's defined codes); the numeric value of each
// is its exact assignment in that enumeration, not a locally invented
// sequence, so the values below are intentionally non-contiguous and not
// in declaration order.
const (
	// ErrorCodeUnsupportedCommand means the addressed endpoint (or this
	// server's routing for it) does not support the request it was asked
	// to perform — e.g. a KindCompound, KindCompoundWait, KindTriggered,
	// or KindChained request reaching a Handler with no support for that
	// request kind, a safety-request variant submitted against a
	// Dispatcher with no SafeStateCheck configured (see
	// request.ErrSafeStateNotConfigured), or a byte_bus_id with no
	// registered Handler at all (see ErrUnknownEndpoint).
	ErrorCodeUnsupportedCommand ErrorCode = 1

	// ErrorCodeRequestCancelled means the request this response answers
	// was itself cancelled while pending — either by an explicit
	// cancellation request (see request.ErrTicketCancelled) or by the
	// watchdog-driven safe-state purge (see request.ErrPurgedByWatchdog).
	ErrorCodeRequestCancelled ErrorCode = 5

	// ErrorCodeRequestNotFound means a cancellation request targeted a
	// transaction this server has no record of (see
	// request.ErrUnknownTicket).
	ErrorCodeRequestNotFound ErrorCode = 6

	// ErrorCodePresentationTimeTooFarInFuture means a KindTimed request's
	// target presentation time was rejected as unreasonably distant. No
	// caller-facing scheduler exists in this repo yet to perform that
	// check (see doc.go's "Explicit non-goals"), so this code is defined
	// for forward compatibility but errorCodeFor never currently selects
	// it.
	ErrorCodePresentationTimeTooFarInFuture ErrorCode = 13

	// ErrorCodeGPTPFailure means a timed request arrived before this
	// server established time synchronization. Router.Route currently
	// handles that case by dropping the AVTPDU silently rather than
	// sending any reply at all (see avtp.DispositionDrop and Route's own
	// doc comment), so this code is defined for forward compatibility but
	// errorCodeFor never currently selects it.
	ErrorCodeGPTPFailure ErrorCode = 14

	// ErrorCodeInvalidParameter means the request body was malformed, the
	// wrong size, or otherwise failed structural validation for the kind
	// it declared. errorCodeFor falls back to this code for any error it
	// does not recognize as one of the more specific codes above.
	ErrorCodeInvalidParameter ErrorCode = 15

	// ErrorCodeChainAborted means a KindChained request's predecessor
	// segment failed and the chain's abort-on-error option was set, so
	// the remaining segments were not executed (see
	// request.ErrChainedSegmentFailed).
	ErrorCodeChainAborted ErrorCode = 16

	// ErrorCodeChainError means a KindChained request had no valid
	// predecessor segment to build on — e.g. it declared zero segments
	// (see request.ErrInvalidSegmentCount).
	ErrorCodeChainError ErrorCode = 17
)

// Valid reports whether c is one of this package's recognized
// error-response codes. This package implements only a subset of the
// specification's full error-code enumeration (see the const block above),
// so this deliberately checks membership in that subset rather than a
// contiguous range.
func (c ErrorCode) Valid() bool {
	switch c {
	case ErrorCodeUnsupportedCommand,
		ErrorCodeRequestCancelled,
		ErrorCodeRequestNotFound,
		ErrorCodePresentationTimeTooFarInFuture,
		ErrorCodeGPTPFailure,
		ErrorCodeInvalidParameter,
		ErrorCodeChainAborted,
		ErrorCodeChainError:
		return true
	default:
		return false
	}
}

// String renders c for logs and test failure messages.
func (c ErrorCode) String() string {
	switch c {
	case ErrorCodeUnsupportedCommand:
		return "unsupported command"
	case ErrorCodeChainError:
		return "chain error"
	case ErrorCodeChainAborted:
		return "chain aborted"
	case ErrorCodeRequestNotFound:
		return "request not found"
	case ErrorCodeRequestCancelled:
		return "request cancelled"
	case ErrorCodePresentationTimeTooFarInFuture:
		return "presentation time too far in the future"
	case ErrorCodeGPTPFailure:
		return "gPTP failure"
	case ErrorCodeInvalidParameter:
		return "invalid parameter"
	default:
		return "?"
	}
}

// errorCodeFor maps err to the closest matching ErrorCode, preferring the
// most specific sentinel this repo's request package (or this package's own
// Router) defines and falling back to ErrorCodeInvalidParameter — a
// malformed/rejected request is the most common shape for an error this
// package cannot classify more precisely. Some mappings here are this
// implementation's own reasoned choice among several plausible fits (most
// notably ErrUnknownEndpoint and request.ErrSafeStateNotConfigured, neither
// of which has an exact counterpart in the fixed error-code enumeration)
// rather than an unambiguous one-to-one correspondence.
func errorCodeFor(err error) ErrorCode {
	switch {
	case errors.Is(err, ErrUnknownEndpoint):
		return ErrorCodeUnsupportedCommand
	case errors.Is(err, request.ErrSafeStateNotConfigured):
		return ErrorCodeUnsupportedCommand
	case errors.Is(err, request.ErrInvalidSegmentCount):
		return ErrorCodeChainError
	case errors.Is(err, request.ErrChainedSegmentFailed):
		return ErrorCodeChainAborted
	case errors.Is(err, request.ErrUnknownTicket):
		return ErrorCodeRequestNotFound
	case errors.Is(err, request.ErrTicketCancelled),
		errors.Is(err, request.ErrPurgedByWatchdog):
		return ErrorCodeRequestCancelled
	default:
		return ErrorCodeInvalidParameter
	}
}

// EncodeErrorBody builds the wire-level error-response Body: code as the
// leading byte, followed by diagnostic's raw UTF-8 bytes (if any) as an
// optional trailing human-readable detail — present for local debugging
// only, never required to interpret the response, since code alone is the
// primary, authoritative payload.
func EncodeErrorBody(code ErrorCode, diagnostic string) []byte {
	buf := make([]byte, 1+len(diagnostic))
	buf[0] = byte(code)
	copy(buf[1:], diagnostic)
	return buf
}

// DecodeErrorBody parses a Body built by EncodeErrorBody, returning the
// leading ErrorCode and any trailing diagnostic text. It returns
// ErrShortBuffer for an empty body.
func DecodeErrorBody(body []byte) (ErrorCode, string, error) {
	if len(body) < 1 {
		return 0, "", ErrShortBuffer
	}
	return ErrorCode(body[0]), string(body[1:]), nil
}
