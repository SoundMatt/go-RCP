package acf

import "errors"

// Message-layer (ACF_ABB/ACF_GBB) errors. ErrReservedBitsSet, shared with
// the avtp package's header-layer decoding, is defined there — see
// avtp.ErrReservedBitsSet — since "a field documented as reserved was
// nonzero on decode" is the same failure mode at both layers.
var (
	// ErrShortMessage is returned when a buffer is too short to hold the
	// declared message, or too short for the request-descriptor header of
	// the message kind it claims to be.
	ErrShortMessage = errors.New("rcp/acf: buffer too short for RCP message")

	// ErrUnknownMessageKind is returned when a message's kind tag is
	// neither KindShort (ACF_ABB) nor KindLong (ACF_GBB).
	ErrUnknownMessageKind = errors.New("rcp/acf: unrecognized RCP message kind")

	// ErrLengthOverflow is returned when an encoded message's total length
	// does not fit the 16-bit message-length field.
	ErrLengthOverflow = errors.New("rcp/acf: message length exceeds 16-bit field width")

	// ErrPadOverflow is returned when a caller supplies a pad-byte count
	// outside the representable 0-3 range.
	ErrPadOverflow = errors.New("rcp/acf: pad byte count out of range (0-3)")
)

// Frame-layer errors.
var (
	// ErrFrameLengthMismatch is returned when the AVTPDU header's
	// data-length field does not match the length of the RCP message that
	// actually follows it in the buffer.
	ErrFrameLengthMismatch = errors.New("rcp/acf: AVTPDU data length does not match enclosed message length")
)
