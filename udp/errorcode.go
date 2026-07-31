package udp

import (
	"errors"

	"github.com/SoundMatt/go-RCP/e2e"
	"github.com/SoundMatt/go-RCP/pwm"
	"github.com/SoundMatt/go-RCP/regmap"
	"github.com/SoundMatt/go-RCP/request"
)

// ErrorCode is the numeric error-response code errorResponse carries as the
// first byte of a wire-level error response's Body, drawn from the OPEN
// Alliance TC18 Remote Control Protocol Specification's fixed
// request-response error-code enumeration (§12.9.6 Table 27) rather than an
// internal Go error's own free-text message. See errorResponse and
// errorCodeFor.
type ErrorCode uint8

// This package implements the specification's full 17-entry error-code
// enumeration (Table 27); the numeric value of each is its exact assignment
// in that table, not a locally invented sequence, so the values below are
// intentionally non-contiguous and not in declaration order. Not every code
// has a corresponding Go sentinel error to map from today — see
// errorCodeFor's doc comment for which are currently reachable and which
// exist only for forward compatibility/completeness.
const (
	// ErrorCodeUnsupportedCommand (UNSUPPORTED_CMD) means the addressed
	// endpoint (or this server's routing for it) does not support the
	// request it was asked to perform — e.g. a KindCompound,
	// KindCompoundWait, KindTriggered, or KindChained request reaching a
	// Handler with no support for that request kind, a safety-request
	// variant submitted against a Dispatcher with no SafeStateCheck
	// configured (see request.ErrSafeStateNotConfigured), or a
	// byte_bus_id with no registered Handler at all (see
	// ErrUnknownEndpoint).
	ErrorCodeUnsupportedCommand ErrorCode = 1

	// ErrorCodeSequencerNotKnown (SEQUENCER_NOT_KNOWN) means a request
	// referenced a sequencer id this server has no record of. This
	// package's request.Sequencer bank auto-vivifies every SequencerID on
	// first use (see request/sequencer.go), so there is currently no
	// "unknown sequencer" condition to map to it; defined for forward
	// compatibility.
	ErrorCodeSequencerNotKnown ErrorCode = 2

	// ErrorCodeUnauthorizedAccess (UNAUTHORIZED_ACCESS) means the
	// requester stream is not permitted to reach the addressed endpoint or
	// register, e.g. regmap.ErrAccessDenied (no grant for this endpoint)
	// or regmap.ErrNotRootClient (operation reserved for the root-client
	// stream).
	ErrorCodeUnauthorizedAccess ErrorCode = 3

	// ErrorCodeLockedMemAccess (LOCKED_MEM_ACCESS) means a write targeted
	// a register field that is not writable in the server's current
	// lifecycle state (see regmap.ErrRegisterLocked).
	ErrorCodeLockedMemAccess ErrorCode = 4

	// ErrorCodeRequestCancelled (REQUEST_CANCELED) means the request this
	// response answers was itself cancelled while pending — either by an
	// explicit cancellation request (see request.ErrTicketCancelled) or by
	// the watchdog-driven safe-state purge (see
	// request.ErrPurgedByWatchdog).
	ErrorCodeRequestCancelled ErrorCode = 5

	// ErrorCodeRequestNotFound (REQUEST_NOT_FOUND) means a cancellation
	// request targeted a transaction this server has no record of (see
	// request.ErrUnknownTicket).
	ErrorCodeRequestNotFound ErrorCode = 6

	// ErrorCodeEPError (EP_ERROR) means an error occurred during request
	// execution at the endpoint itself (e.g. a HW failure); per Table 27,
	// details are expected to be available via the endpoint's ep_status
	// register. No single Go sentinel maps to this generically across
	// every endpoint package today; defined for forward compatibility.
	ErrorCodeEPError ErrorCode = 7

	// ErrorCodeEPNotFound (EP_NOT_FOUND) means a request referenced an
	// endpoint that was never declared — most notably a Triggered
	// request's trigger_source_ep (see regmap.ErrUnknownEndpoint).
	ErrorCodeEPNotFound ErrorCode = 8

	// ErrorCodePWMInNoSignal (PWM_IN_NO_SIGNAL) means a pwm.RoleInput
	// endpoint's read request found no incoming signal (see
	// pwm.ErrSignalLost).
	ErrorCodePWMInNoSignal ErrorCode = 9

	// ErrorCodeRequestStorageOverflow (REQ_storage_OVFL) means the
	// addressed endpoint's pending-request storage is full. This package's
	// request.Dispatcher keeps an unbounded ticket map with no configured
	// storage ceiling, so there is currently no overflow condition to map
	// to it; defined for forward compatibility.
	ErrorCodeRequestStorageOverflow ErrorCode = 10

	// ErrorCodeRequestRejected (REQUEST_REJECTED) means a request other
	// than a standard request arrived during the RC Server's initial
	// configuration phase. This package has no explicit "initial config
	// phase" gate distinct from regmap's lifecycle/lock machinery today;
	// defined for forward compatibility.
	ErrorCodeRequestRejected ErrorCode = 11

	// ErrorCodePOCIFailure (POCI_FAILURE) means the request's CRC did not
	// match (see e2e.ErrCRCMismatch). A conformant client may treat this
	// very differently from a merely malformed request — e.g. as a safety
	// event — so this is deliberately not folded into
	// ErrorCodeInvalidParameter.
	ErrorCodePOCIFailure ErrorCode = 12

	// ErrorCodePresentationTimeTooFarInFuture (PRESENTATION_TIME_TOO_FAR)
	// means a KindTimed request's target presentation time was rejected as
	// unreasonably distant. No caller-facing scheduler exists in this repo
	// yet to perform that check (see doc.go's "Explicit non-goals"), so
	// this code is defined for forward compatibility but errorCodeFor
	// never currently selects it.
	ErrorCodePresentationTimeTooFarInFuture ErrorCode = 13

	// ErrorCodeGPTPFailure (GPTP_FAIL) means a timed request arrived
	// before this server established time synchronization. Router.Route
	// currently handles that case by dropping the AVTPDU silently rather
	// than sending any reply at all (see avtp.DispositionDrop and Route's
	// own doc comment), so this code is defined for forward compatibility
	// but errorCodeFor never currently selects it.
	ErrorCodeGPTPFailure ErrorCode = 14

	// ErrorCodeInvalidParameter (INVALID_PARAMETER) means the request body
	// was malformed, the wrong size, or otherwise failed structural
	// validation for the kind it declared. errorCodeFor falls back to this
	// code for any error it does not recognize as one of the more specific
	// codes above.
	ErrorCodeInvalidParameter ErrorCode = 15

	// ErrorCodeChainAborted (CHAIN_ABORTED) means a KindChained request's
	// predecessor segment failed and the chain's abort-on-error option was
	// set, so the remaining segments were not executed (see
	// request.ErrChainedSegmentFailed).
	ErrorCodeChainAborted ErrorCode = 16

	// ErrorCodeChainError (CHAIN_ERROR) means a KindChained request had no
	// valid predecessor segment to build on — e.g. it declared zero
	// segments (see request.ErrInvalidSegmentCount).
	ErrorCodeChainError ErrorCode = 17
)

// Valid reports whether c is one of Table 27's 17 defined error-response
// codes.
func (c ErrorCode) Valid() bool {
	switch c {
	case ErrorCodeUnsupportedCommand,
		ErrorCodeSequencerNotKnown,
		ErrorCodeUnauthorizedAccess,
		ErrorCodeLockedMemAccess,
		ErrorCodeRequestCancelled,
		ErrorCodeRequestNotFound,
		ErrorCodeEPError,
		ErrorCodeEPNotFound,
		ErrorCodePWMInNoSignal,
		ErrorCodeRequestStorageOverflow,
		ErrorCodeRequestRejected,
		ErrorCodePOCIFailure,
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
	case ErrorCodeSequencerNotKnown:
		return "sequencer not known"
	case ErrorCodeUnauthorizedAccess:
		return "unauthorized access"
	case ErrorCodeLockedMemAccess:
		return "locked memory access"
	case ErrorCodeChainError:
		return "chain error"
	case ErrorCodeChainAborted:
		return "chain aborted"
	case ErrorCodeRequestNotFound:
		return "request not found"
	case ErrorCodeRequestCancelled:
		return "request cancelled"
	case ErrorCodeEPError:
		return "endpoint error"
	case ErrorCodeEPNotFound:
		return "endpoint not found"
	case ErrorCodePWMInNoSignal:
		return "pwm input has no signal"
	case ErrorCodeRequestStorageOverflow:
		return "request storage overflow"
	case ErrorCodeRequestRejected:
		return "request rejected"
	case ErrorCodePOCIFailure:
		return "POCI failure (CRC mismatch)"
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
// most specific sentinel this repo defines and falling back to
// ErrorCodeInvalidParameter — a malformed/rejected request is the most
// common shape for an error this package cannot classify more precisely.
// Some mappings here are this implementation's own reasoned choice among
// several plausible fits (most notably ErrUnknownEndpoint and
// request.ErrSafeStateNotConfigured, neither of which has an exact
// counterpart in the fixed error-code enumeration) rather than an
// unambiguous one-to-one correspondence. Every internally-detected error
// condition that has a corresponding Table 27 code is mapped to that exact
// code here rather than folded into the generic fallback — this matters
// most for ErrorCodePOCIFailure (CRC/integrity failures), which a
// conformant client may treat very differently (e.g. as a safety event)
// than a malformed-request error.
//
// authz.ErrDenied is deliberately not mapped here: authz is a client-side
// policy layer wrapping a *udp.Controller (see authz's own doc comment) —
// it rejects a request locally before it is ever sent, so it never reaches
// this server-side error-response path, and this package cannot import
// authz to check for it anyway without an import cycle (authz imports
// udp).
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
	case errors.Is(err, e2e.ErrCRCMismatch):
		return ErrorCodePOCIFailure
	case errors.Is(err, regmap.ErrAccessDenied),
		errors.Is(err, regmap.ErrNotRootClient),
		errors.Is(err, regmap.ErrRootAlreadyClaimed):
		return ErrorCodeUnauthorizedAccess
	case errors.Is(err, regmap.ErrRegisterLocked),
		errors.Is(err, regmap.ErrGeneralBlockReadOnly):
		return ErrorCodeLockedMemAccess
	case errors.Is(err, regmap.ErrUnknownEndpoint):
		return ErrorCodeEPNotFound
	case errors.Is(err, pwm.ErrSignalLost):
		return ErrorCodePWMInNoSignal
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
