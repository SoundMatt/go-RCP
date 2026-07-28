package server

import "errors"

// Lifecycle errors.
var (
	// ErrLifecycleOutOfOrder is returned when a caller asks the server to
	// advance to a lifecycle state that does not immediately follow its
	// current one. States only ever advance one step at a time.
	ErrLifecycleOutOfOrder = errors.New("rcp/server: lifecycle states must advance one step at a time")

	// ErrPinMapInvalid is returned when the pin-mapping table fails its
	// pre-lock plausibility check: a duplicate physical pin assignment, a
	// reference to an endpoint address that was never declared, or a
	// named-signal index outside the range that endpoint's declared type
	// defines.
	ErrPinMapInvalid = errors.New("rcp/server: pin-mapping table failed plausibility check")

	// ErrFunctionalBlockIncomplete is returned when advancing to the fully-
	// configured state while at least one declared endpoint still has an
	// empty functional (type-specific) configuration block.
	ErrFunctionalBlockIncomplete = errors.New("rcp/server: endpoint has no functional configuration set")

	// ErrQueueConfigInvalid is returned when the request-stream/response-
	// queue configuration fails its plausibility check (see
	// QueueConfig.Validate).
	ErrQueueConfigInvalid = errors.New("rcp/server: queue configuration failed plausibility check")
)

// Register-locking errors.
var (
	// ErrRegisterLocked is returned when a write targets a register field
	// that is not writable in the server's current lifecycle state. Once a
	// field's lock class closes for the current state, it stays closed for
	// every requester, including the root client.
	ErrRegisterLocked = errors.New("rcp/server: register field is locked in the current lifecycle state")

	// ErrGeneralBlockReadOnly is returned when a whole-map write attempts to
	// change any field of the general server register block. That block is
	// owned by the server itself; it has no client-facing write path in
	// this milestone.
	ErrGeneralBlockReadOnly = errors.New("rcp/server: general register block is not client-writable")
)

// Addressing and access-control errors.
var (
	// ErrReservedAddress is returned when a caller tries to declare a
	// functional endpoint at the EP0 pseudo-endpoint address.
	ErrReservedAddress = errors.New("rcp/server: byte_bus_id 0 is reserved for EP0")

	// ErrDuplicateEndpoint is returned when a caller declares an endpoint at
	// an address that already has one.
	ErrDuplicateEndpoint = errors.New("rcp/server: endpoint already declared at this address")

	// ErrUnknownEndpoint is returned when a caller addresses an endpoint
	// that has not been declared.
	ErrUnknownEndpoint = errors.New("rcp/server: no endpoint declared at this address")

	// ErrNotRootClient is returned when a stream that has not claimed the
	// root-client role attempts an operation reserved for it (whole-map
	// write, pin-map write, endpoint declaration).
	ErrNotRootClient = errors.New("rcp/server: operation requires the root-client stream")

	// ErrRootAlreadyClaimed is returned when a stream other than the
	// current root client attempts to claim the root-client role.
	ErrRootAlreadyClaimed = errors.New("rcp/server: root-client role already claimed by another stream")

	// ErrAccessDenied is returned when a restricted (non-root) stream
	// addresses an endpoint that was never granted to it.
	ErrAccessDenied = errors.New("rcp/server: stream has no grant for this endpoint")
)

// Discovery errors (ROADMAP.md Milestone 46).
var (
	// ErrDiscoveryRequiresUntimedHeader is returned by HandleDiscoveryRequest
	// when a discovery request arrives framed in a presentation-timestamped
	// (TSCF) AVTPDU header. Discovery has no scheduled-execution concept, so
	// a timestamped discovery request is dropped outright rather than
	// folded down to best-effort.
	ErrDiscoveryRequiresUntimedHeader = errors.New("rcp/server: discovery request must use the untimed AVTPDU header")

	// ErrConfigurationClaimed is returned by ClaimConfiguration when a
	// different stream currently holds an unexpired Discovery-stream
	// configuration claim.
	ErrConfigurationClaimed = errors.New("rcp/server: configuration rights are currently reserved by another stream")

	// ErrNotConfigurationClaimant is returned by ReleaseConfigurationClaim
	// when the calling stream does not currently hold an active Discovery-
	// stream configuration claim.
	ErrNotConfigurationClaimant = errors.New("rcp/server: stream does not hold the active configuration claim")
)

// Wire (register-map encoding) errors.
var (
	// ErrShortBuffer is returned when a buffer is too short to hold the
	// register block a decoder was asked to parse.
	ErrShortBuffer = errors.New("rcp/server: buffer too short for register block")

	// ErrUnknownEndpointType is returned when a decoded generic endpoint
	// block's type tag is not one this package recognizes.
	ErrUnknownEndpointType = errors.New("rcp/server: unrecognized endpoint type")

	// ErrUnsupportedRegisterMapVersion is returned when a decoded
	// GeneralBlock's RegisterMapVersion is not the version this package
	// implements.
	ErrUnsupportedRegisterMapVersion = errors.New("rcp/server: unsupported register-map version")

	// ErrTrailingBytes is returned when a decoder is handed more bytes than
	// its register block declares — the same "don't silently ignore extra
	// input" posture avtp.DecodeFrame takes on a length mismatch.
	ErrTrailingBytes = errors.New("rcp/server: buffer longer than declared register block length")
)
