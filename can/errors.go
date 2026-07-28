package can

import "errors"

// Encoding errors.
var (
	// ErrShortBuffer is returned when a buffer is too short to hold the
	// structure a decoder was asked to parse.
	ErrShortBuffer = errors.New("rcp/can: buffer too short")

	// ErrTrailingBytes is returned when a decoder is handed more bytes than
	// its declared structure length accounts for.
	ErrTrailingBytes = errors.New("rcp/can: buffer longer than declared structure length")
)

// Configuration errors.
var (
	// ErrInvalidBitrate is returned when an enabled Config's
	// NominalBitrateKbps is zero.
	ErrInvalidBitrate = errors.New("rcp/can: nominal bitrate must be nonzero when enabled")
)

// Frame validation errors.
var (
	// ErrInvalidFormat is returned when a Frame's Format is not one of this
	// package's three recognized frame formats.
	ErrInvalidFormat = errors.New("rcp/can: unrecognized frame format")

	// ErrIDOutOfRange is returned when a Frame's ID exceeds the width its
	// Extended flag allows (11 bits standard, 29 bits extended).
	ErrIDOutOfRange = errors.New("rcp/can: identifier out of range for its standard/extended width")

	// ErrPayloadTooLarge is returned when a Frame's Data exceeds its
	// Format's maximum payload length.
	ErrPayloadTooLarge = errors.New("rcp/can: data payload exceeds this frame format's maximum length")

	// ErrBitRateSwitchNotSupported is returned when a FormatClassical
	// Frame sets BitRateSwitch, which Classical CAN has no data phase to
	// switch.
	ErrBitRateSwitchNotSupported = errors.New("rcp/can: bit-rate switch is not defined for FormatClassical")

	// ErrXLHeaderNotSupported is returned when a non-FormatXL Frame sets a
	// nonzero XLHeader, which only FormatXL defines.
	ErrXLHeaderNotSupported = errors.New("rcp/can: XL header fields are only defined for FormatXL")
)

// Request-handling errors.
var (
	// ErrWrongEndpoint is returned when a request's ByteBusID does not match
	// the Endpoint it was handed to.
	ErrWrongEndpoint = errors.New("rcp/can: request addressed to a different endpoint")

	// ErrNotConfigured is returned when a request is handled against an
	// endpoint whose Config does not currently mark Enabled.
	ErrNotConfigured = errors.New("rcp/can: endpoint is not configured/enabled")

	// ErrRequestMustReadOrWrite is returned when a request sets neither the
	// Read nor the Write control flag, so there is nothing for a CAN
	// endpoint to do with it.
	ErrRequestMustReadOrWrite = errors.New("rcp/can: request must set the Read or Write control flag")

	// ErrNoFrameReceived is returned by a read request when no frame has
	// ever been received on this bus (or none since the endpoint was last
	// configured) — this package fails explicitly rather than returning a
	// stale or zero-value Frame.
	ErrNoFrameReceived = errors.New("rcp/can: no frame has been received on this bus")
)
