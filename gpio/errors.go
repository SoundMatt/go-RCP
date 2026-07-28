package gpio

import "errors"

// Encoding errors.
var (
	// ErrShortBuffer is returned when a buffer is too short to hold the
	// structure a decoder was asked to parse.
	ErrShortBuffer = errors.New("rcp/gpio: buffer too short")

	// ErrTrailingBytes is returned when a decoder is handed more bytes than
	// its structure declares, the same "don't silently ignore extra input"
	// posture the rest of this repo's decoders take.
	ErrTrailingBytes = errors.New("rcp/gpio: buffer longer than declared structure length")
)

// Configuration errors.
var (
	// ErrPinCountOutOfRange is returned when a Config's PinCount is zero or
	// exceeds MaxPins.
	ErrPinCountOutOfRange = errors.New("rcp/gpio: pin count must be between 1 and MaxPins")
)

// Request-handling errors.
var (
	// ErrInvalidSemantic is returned when a decoded WriteSemantic is not one
	// of this package's eight recognized values.
	ErrInvalidSemantic = errors.New("rcp/gpio: unrecognized write semantic")

	// ErrWrongEndpoint is returned when a request's ByteBusID does not match
	// the Endpoint it was handed to.
	ErrWrongEndpoint = errors.New("rcp/gpio: request addressed to a different endpoint")

	// ErrRequestMustReadOrWrite is returned when a request sets neither the
	// Read nor the Write control flag, so there is nothing for a GPIO
	// endpoint to do with it.
	ErrRequestMustReadOrWrite = errors.New("rcp/gpio: request must set the Read or Write control flag")
)
