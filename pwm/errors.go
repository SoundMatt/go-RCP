package pwm

import "errors"

// Encoding errors.
var (
	// ErrShortBuffer is returned when a buffer is too short to hold the
	// structure a decoder was asked to parse.
	ErrShortBuffer = errors.New("rcp/pwm: buffer too short")

	// ErrTrailingBytes is returned when a decoder is handed more bytes than
	// its fixed-length structure declares.
	ErrTrailingBytes = errors.New("rcp/pwm: buffer longer than declared structure length")
)

// Configuration errors.
var (
	// ErrInvalidRole is returned when an enabled Config's Role is not one of
	// this package's two recognized values.
	ErrInvalidRole = errors.New("rcp/pwm: unrecognized role")

	// ErrActiveExceedsPeriod is returned when a waveform's active-duration
	// exceeds its own period.
	ErrActiveExceedsPeriod = errors.New("rcp/pwm: active duration exceeds period")
)

// Request-handling errors.
var (
	// ErrWrongEndpoint is returned when a request's ByteBusID does not match
	// the Endpoint it was handed to.
	ErrWrongEndpoint = errors.New("rcp/pwm: request addressed to a different endpoint")

	// ErrNotConfigured is returned when a request is handled against an
	// endpoint whose Config does not currently mark Enabled.
	ErrNotConfigured = errors.New("rcp/pwm: endpoint is not configured/enabled")

	// ErrRequestMustReadOrWrite is returned when a request sets neither the
	// Read nor the Write control flag, so there is nothing for a PWM
	// endpoint to do with it.
	ErrRequestMustReadOrWrite = errors.New("rcp/pwm: request must set the Read or Write control flag")

	// ErrWriteNotSupportedForInput is returned when a write request is
	// handled against a RoleInput endpoint — PWM input is response-only per
	// ROADMAP.md Milestone 48, so there is nothing for a write request to
	// command.
	ErrWriteNotSupportedForInput = errors.New("rcp/pwm: write request not supported for a RoleInput endpoint")

	// ErrSignalLost is returned by a RoleInput endpoint's read request when
	// no valid incoming waveform has been captured (or capture has been
	// explicitly marked lost) — this package fails explicitly on signal
	// loss rather than returning stale data or hanging, per ROADMAP.md
	// Milestone 48.
	ErrSignalLost = errors.New("rcp/pwm: incoming signal lost")
)
