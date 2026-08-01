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

// Request-handling errors. Note that a request whose evt[2:0] selector is
// reserved for this endpoint type, or which sets a non-zero evt[2:0] without
// carrying any byte_msg_payload, is rejected with acf.ErrEVTReserved /
// acf.ErrEVTMissingPayload rather than a gpio-specific sentinel — those
// conditions and their mandated UNSUPPORTED_CMD error code are defined once,
// for every endpoint type, in acf/evt.go.
var (
	// ErrWrongEndpoint is returned when a request's ByteBusID does not match
	// the Endpoint it was handed to.
	ErrWrongEndpoint = errors.New("rcp/gpio: request addressed to a different endpoint")

	// ErrRequestMustReadOrWrite is returned when a request sets neither the
	// Read nor the Write control flag, so there is nothing for a GPIO
	// endpoint to do with it.
	ErrRequestMustReadOrWrite = errors.New("rcp/gpio: request must set the Read or Write control flag")
)
