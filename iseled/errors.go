package iseled

import "errors"

// Encoding errors.
var (
	// ErrShortBuffer is returned when a buffer is too short to hold the
	// structure a decoder was asked to parse.
	ErrShortBuffer = errors.New("rcp/iseled: buffer too short")

	// ErrTrailingBytes is returned when a decoder is handed more bytes than
	// its declared structure length accounts for.
	ErrTrailingBytes = errors.New("rcp/iseled: buffer longer than declared structure length")

	// ErrCRCMismatch is returned when a decoded Command or DeviceResponse's
	// trailing ISELED-native CRC8 does not match the one ComputeCRC
	// recomputes over its decoded fields (see doc.go's "ISELED-native CRC"
	// section).
	ErrCRCMismatch = errors.New("rcp/iseled: ISELED-native CRC mismatch")
)

// Configuration errors.
var (
	// ErrInvalidDeviceCount is returned when an enabled Config's
	// DeviceCount is zero.
	ErrInvalidDeviceCount = errors.New("rcp/iseled: device count must be nonzero when enabled")
)

// Request-handling errors.
var (
	// ErrWrongEndpoint is returned when a request's ByteBusID does not match
	// the Endpoint it was handed to.
	ErrWrongEndpoint = errors.New("rcp/iseled: request addressed to a different endpoint")

	// ErrRequestMustWrite is returned when a command request does not set
	// the Write control flag — an ISELED command always carries an
	// outgoing payload, so there is nothing to send down the chain without
	// it.
	ErrRequestMustWrite = errors.New("rcp/iseled: command request must set the Write control flag")

	// ErrChainNotConfigured is returned when a command request is handled
	// against an endpoint whose Config does not currently mark Enabled.
	ErrChainNotConfigured = errors.New("rcp/iseled: chain is not configured/enabled")

	// ErrDeviceAddressOutOfRange is returned when a Command's Address is
	// neither DeviceBroadcast nor within the configured DeviceCount's
	// range.
	ErrDeviceAddressOutOfRange = errors.New("rcp/iseled: device address out of range for the configured chain")
)
