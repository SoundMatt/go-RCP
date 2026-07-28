package uart

import "errors"

// Encoding errors.
var (
	// ErrShortBuffer is returned when a buffer is too short to hold the
	// structure a decoder was asked to parse.
	ErrShortBuffer = errors.New("rcp/uart: buffer too short")

	// ErrTrailingBytes is returned when a decoder is handed more bytes than
	// its fixed-length structure declares.
	ErrTrailingBytes = errors.New("rcp/uart: buffer longer than declared structure length")
)

// Configuration errors.
var (
	// ErrZeroBaudRate is returned when an enabled Config's BaudRate is zero.
	ErrZeroBaudRate = errors.New("rcp/uart: enabled endpoint must have a nonzero baud rate")

	// ErrInvalidDataBits is returned when an enabled Config's DataBits is
	// outside the 5-9 range.
	ErrInvalidDataBits = errors.New("rcp/uart: data bits must be between 5 and 9")

	// ErrInvalidParity is returned when an enabled Config's Parity is not
	// one of this package's five recognized values.
	ErrInvalidParity = errors.New("rcp/uart: unrecognized parity mode")

	// ErrInvalidStopBits is returned when an enabled Config's StopBits is
	// not one of this package's three recognized values.
	ErrInvalidStopBits = errors.New("rcp/uart: unrecognized stop-bit count")
)

// Request-handling errors.
var (
	// ErrWrongEndpoint is returned when a request's ByteBusID does not match
	// the Endpoint it was handed to.
	ErrWrongEndpoint = errors.New("rcp/uart: request addressed to a different endpoint")

	// ErrUARTNotConfigured is returned when a TX or RX request is handled
	// against an endpoint whose Config does not currently mark Enabled.
	ErrUARTNotConfigured = errors.New("rcp/uart: endpoint is not configured/enabled")

	// ErrReadRequestNotPayloadLess is returned when an RX read request
	// carries a nonempty body — a UART read request must be payload-less
	// (see doc.go's note on this asymmetry versus gpio/pwm's read requests).
	ErrReadRequestNotPayloadLess = errors.New("rcp/uart: read request body must be empty")

	// ErrRequestMustReadOrWrite is returned when a request sets neither the
	// Read nor the Write control flag, so there is nothing for a UART
	// endpoint to do with it.
	ErrRequestMustReadOrWrite = errors.New("rcp/uart: request must set the Read or Write control flag")
)
