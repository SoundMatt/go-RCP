package i2c

import "errors"

// Encoding errors.
var (
	// ErrShortBuffer is returned when a buffer is too short to hold the
	// structure a decoder was asked to parse.
	ErrShortBuffer = errors.New("rcp/i2c: buffer too short")

	// ErrTrailingBytes is returned when a decoder is handed more bytes than
	// its fixed-length structure declares.
	ErrTrailingBytes = errors.New("rcp/i2c: buffer longer than declared structure length")
)

// Configuration errors.
var (
	// ErrInvalidSpeed is returned when an enabled Config's Speed is not one
	// of this package's five recognized bus speed classes.
	ErrInvalidSpeed = errors.New("rcp/i2c: unrecognized bus speed")
)

// Request-handling errors.
var (
	// ErrWrongEndpoint is returned when a request's ByteBusID does not match
	// the Endpoint it was handed to.
	ErrWrongEndpoint = errors.New("rcp/i2c: request addressed to a different endpoint")

	// ErrRequestMustWrite is returned when a transfer request does not set
	// the Write control flag — an I2C transfer always carries an outgoing
	// payload (even a zero-length one, since the address byte itself is part
	// of that payload at this layer), so there is nothing to transfer
	// without it.
	ErrRequestMustWrite = errors.New("rcp/i2c: transfer request must set the Write control flag")

	// ErrBusNotConfigured is returned when a transfer request is handled
	// against an endpoint whose Config does not currently mark Enabled.
	ErrBusNotConfigured = errors.New("rcp/i2c: bus is not configured/enabled")
)
