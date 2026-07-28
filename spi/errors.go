package spi

import "errors"

// Encoding errors.
var (
	// ErrShortBuffer is returned when a buffer is too short to hold the
	// structure a decoder was asked to parse.
	ErrShortBuffer = errors.New("rcp/spi: buffer too short")

	// ErrTrailingBytes is returned when a decoder is handed more bytes than
	// its fixed-length structure declares.
	ErrTrailingBytes = errors.New("rcp/spi: buffer longer than declared structure length")
)

// Configuration errors.
var (
	// ErrInvalidChannel is returned when a decoded Channel value is not one
	// of this package's MaxChannels recognized channels.
	ErrInvalidChannel = errors.New("rcp/spi: unrecognized chip-select channel")

	// ErrInvalidMode is returned when an enabled ChannelConfig's Mode is not
	// one of the four recognized clock-polarity/phase modes.
	ErrInvalidMode = errors.New("rcp/spi: unrecognized SPI mode")

	// ErrZeroClock is returned when an enabled ChannelConfig's ClockHz is
	// zero.
	ErrZeroClock = errors.New("rcp/spi: enabled channel must have a nonzero clock rate")
)

// Request-handling errors.
var (
	// ErrWrongEndpoint is returned when a request's ByteBusID does not match
	// the Endpoint it was handed to.
	ErrWrongEndpoint = errors.New("rcp/spi: request addressed to a different endpoint")

	// ErrRequestMustWrite is returned when a transfer request does not set
	// the Write control flag — a SPI transfer always carries an outgoing
	// payload, even one of length zero, so there is nothing to transfer
	// without it.
	ErrRequestMustWrite = errors.New("rcp/spi: transfer request must set the Write control flag")

	// ErrChannelNotConfigured is returned when a transfer request selects a
	// channel that Config does not currently mark Enabled.
	ErrChannelNotConfigured = errors.New("rcp/spi: selected channel is not configured/enabled")
)
