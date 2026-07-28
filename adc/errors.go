package adc

import "errors"

// Encoding errors.
var (
	// ErrShortBuffer is returned when a buffer is too short to hold the
	// structure a decoder was asked to parse.
	ErrShortBuffer = errors.New("rcp/adc: buffer too short")

	// ErrTrailingBytes is returned when a decoder is handed more bytes than
	// its fixed-length structure declares.
	ErrTrailingBytes = errors.New("rcp/adc: buffer longer than declared structure length")
)

// Configuration errors.
var (
	// ErrInvalidResolution is returned when an enabled Config's
	// ResolutionBits is zero or exceeds MaxResolutionBits.
	ErrInvalidResolution = errors.New("rcp/adc: resolution must be between 1 and MaxResolutionBits")

	// ErrInvalidSampleCount is returned when an enabled Config's SampleCount
	// is zero.
	ErrInvalidSampleCount = errors.New("rcp/adc: enabled channel must sample at least once per measurement")

	// ErrInvalidCombineMode is returned when an enabled Config's Combine is
	// not one of this package's two recognized values.
	ErrInvalidCombineMode = errors.New("rcp/adc: unrecognized combine mode")

	// ErrInvalidTriggerMode is returned when an enabled Config's
	// TriggerMode is not one of this package's three recognized values.
	ErrInvalidTriggerMode = errors.New("rcp/adc: unrecognized trigger mode")
)

// Request-handling errors.
var (
	// ErrWrongEndpoint is returned when a request's ByteBusID does not match
	// the Endpoint it was handed to.
	ErrWrongEndpoint = errors.New("rcp/adc: request addressed to a different endpoint")

	// ErrChannelNotConfigured is returned when Endpoint.Trigger or
	// HandleRequest is called against an endpoint whose Config does not
	// currently mark Enabled.
	ErrChannelNotConfigured = errors.New("rcp/adc: channel is not configured/enabled")

	// ErrRequestMustReadOrWrite is returned when a request sets neither the
	// Read nor the Write control flag, so there is nothing for an ADC
	// endpoint to do with it.
	ErrRequestMustReadOrWrite = errors.New("rcp/adc: request must set the Read or Write control flag")
)
