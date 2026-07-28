package lin

import "errors"

// Encoding errors.
var (
	// ErrShortBuffer is returned when a buffer is too short to hold the
	// structure a decoder was asked to parse.
	ErrShortBuffer = errors.New("rcp/lin: buffer too short")

	// ErrTrailingBytes is returned when a decoder is handed more bytes than
	// its fixed-length structure declares.
	ErrTrailingBytes = errors.New("rcp/lin: buffer longer than declared structure length")
)

// Configuration errors.
var (
	// ErrInvalidBaudRate is returned when an enabled Config's BaudRate is
	// zero.
	ErrInvalidBaudRate = errors.New("rcp/lin: baud rate must be nonzero when enabled")
)

// Request-handling errors.
var (
	// ErrWrongEndpoint is returned when a request's ByteBusID does not match
	// the Endpoint it was handed to.
	ErrWrongEndpoint = errors.New("rcp/lin: request addressed to a different endpoint")

	// ErrRequestMustWrite is returned when a transfer request does not set
	// the Write control flag — a LIN commander transfer always carries an
	// outgoing frame body (even a zero-length one), so there is nothing to
	// transfer without it.
	ErrRequestMustWrite = errors.New("rcp/lin: transfer request must set the Write control flag")

	// ErrBusNotConfigured is returned when a transfer request is handled
	// against an endpoint whose Config does not currently mark Enabled.
	ErrBusNotConfigured = errors.New("rcp/lin: bus is not configured/enabled")
)
