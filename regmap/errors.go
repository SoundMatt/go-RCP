package regmap

import "errors"

// Register-locking errors.
var (
	// ErrRegisterLocked is returned when a write targets a register field
	// that is not writable in the server's current lifecycle state. Once a
	// field's lock class closes for the current state, it stays closed for
	// every requester, including the root client.
	ErrRegisterLocked = errors.New("rcp/regmap: register field is locked in the current lifecycle state")

	// ErrGeneralBlockReadOnly is returned when a whole-map write attempts to
	// change any field of the general server register block. That block is
	// owned by the server itself; it has no client-facing write path in
	// this milestone.
	ErrGeneralBlockReadOnly = errors.New("rcp/regmap: general register block is not client-writable")

	// ErrPinMapInvalid is returned when the pin-mapping table fails its
	// pre-lock plausibility check: a duplicate physical pin assignment, a
	// reference to an endpoint address that was never declared, or a
	// named-signal index outside the range that endpoint's declared type
	// defines.
	ErrPinMapInvalid = errors.New("rcp/regmap: pin-mapping table failed plausibility check")

	// ErrQueueConfigInvalid is returned when the request-stream/response-
	// queue configuration fails its plausibility check (see
	// QueueConfig.Validate).
	ErrQueueConfigInvalid = errors.New("rcp/regmap: queue configuration failed plausibility check")
)

// Addressing and access-control errors.
var (
	// ErrReservedAddress is returned when a caller tries to declare a
	// functional endpoint at the EP0 pseudo-endpoint address.
	ErrReservedAddress = errors.New("rcp/regmap: byte_bus_id 0 is reserved for EP0")

	// ErrDuplicateEndpoint is returned when a caller declares an endpoint at
	// an address that already has one.
	ErrDuplicateEndpoint = errors.New("rcp/regmap: endpoint already declared at this address")

	// ErrUnknownEndpoint is returned when a caller addresses an endpoint
	// that has not been declared.
	ErrUnknownEndpoint = errors.New("rcp/regmap: no endpoint declared at this address")

	// ErrNotRootClient is returned when a stream that has not claimed the
	// root-client role attempts an operation reserved for it (whole-map
	// write, pin-map write, endpoint declaration).
	ErrNotRootClient = errors.New("rcp/regmap: operation requires the root-client stream")

	// ErrRootAlreadyClaimed is returned when a stream other than the
	// current root client attempts to claim the root-client role.
	ErrRootAlreadyClaimed = errors.New("rcp/regmap: root-client role already claimed by another stream")

	// ErrAccessDenied is returned when a restricted (non-root) stream
	// addresses an endpoint that was never granted to it.
	ErrAccessDenied = errors.New("rcp/regmap: stream has no grant for this endpoint")
)

// Wire (register-map encoding) errors.
var (
	// ErrShortBuffer is returned when a buffer is too short to hold the
	// register block a decoder was asked to parse.
	ErrShortBuffer = errors.New("rcp/regmap: buffer too short for register block")

	// ErrUnknownEndpointType is returned when a decoded generic endpoint
	// block's type tag is not one this package recognizes.
	ErrUnknownEndpointType = errors.New("rcp/regmap: unrecognized endpoint type")

	// ErrUnsupportedRegisterMapVersion is returned when a decoded
	// GeneralBlock's ProtocolVersion is not the version this package
	// implements.
	ErrUnsupportedRegisterMapVersion = errors.New("rcp/regmap: unsupported register-map version")

	// ErrBadMagic is returned when a decoded GeneralBlock's Magic field
	// does not equal GeneralBlockMagic — the buffer is not the front of a
	// genuine RC Server general register block.
	ErrBadMagic = errors.New("rcp/regmap: general register block has an unrecognized magic value")

	// ErrTrailingBytes is returned when a decoder is handed more bytes than
	// its register block declares — the same "don't silently ignore extra
	// input" posture acf.DecodeFrame takes on a length mismatch.
	ErrTrailingBytes = errors.New("rcp/regmap: buffer longer than declared register block length")
)
