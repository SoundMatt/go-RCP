package wakeup

import "errors"

// Encoding errors.
var (
	// ErrShortBuffer is returned when a buffer is too short to hold the
	// structure a decoder was asked to parse.
	ErrShortBuffer = errors.New("rcp/wakeup: buffer too short")

	// ErrTrailingBytes is returned when a decoder is handed more bytes than
	// its fixed-length structure declares.
	ErrTrailingBytes = errors.New("rcp/wakeup: buffer longer than declared structure length")
)

// Configuration errors.
var (
	// ErrInvalidHandshakeInterval is returned when an enabled Config's
	// WakeHandshakeIntervalMillis is zero.
	ErrInvalidHandshakeInterval = errors.New("rcp/wakeup: wake-handshake interval must be nonzero when enabled")

	// ErrInvalidHandshakeRepeatCount is returned when an enabled Config's
	// WakeHandshakeRepeatCount is zero.
	ErrInvalidHandshakeRepeatCount = errors.New("rcp/wakeup: wake-handshake repeat count must be nonzero when enabled")
)

// Request-handling errors.
var (
	// ErrWrongEndpoint is returned when a request's ByteBusID does not match
	// the Endpoint it was handed to.
	ErrWrongEndpoint = errors.New("rcp/wakeup: request addressed to a different endpoint")

	// ErrNotConfigured is returned when a request is handled against an
	// endpoint whose Config does not currently mark Enabled.
	ErrNotConfigured = errors.New("rcp/wakeup: endpoint is not configured/enabled")

	// ErrRequestMustReadOrWrite is returned when a request sets neither the
	// Read nor the Write control flag, so there is nothing for a Wakeup
	// endpoint to do with it.
	ErrRequestMustReadOrWrite = errors.New("rcp/wakeup: request must set the Read or Write control flag")

	// ErrInvalidPowerState is returned when a write request's target
	// PowerState is not one of this package's four recognized values.
	ErrInvalidPowerState = errors.New("rcp/wakeup: unrecognized power state")

	// ErrCannotRequestUnpowered is returned when a write request targets
	// PowerUnpowered — per doc.go's Scope section, a server cannot request
	// its own total power loss through a protocol request it would not be
	// running to process afterward.
	ErrCannotRequestUnpowered = errors.New("rcp/wakeup: PowerUnpowered cannot be requested")
)
