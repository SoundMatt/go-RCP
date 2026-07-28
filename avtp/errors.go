package avtp

import "errors"

// Header-layer (AVTPDU) errors.
var (
	// ErrShortHeader is returned when a buffer is too short to hold the
	// applicable AVTPDU header variant (untimed or timestamped).
	ErrShortHeader = errors.New("rcp/avtp: buffer too short for AVTPDU header")

	// ErrUnsupportedVersion is returned when the header's protocol-version
	// field is not the version this package implements.
	ErrUnsupportedVersion = errors.New("rcp/avtp: unsupported AVTPDU protocol version")

	// ErrUnknownSubtype is returned when the header's subtype byte is
	// neither SubtypeNTSCF nor SubtypeTSCF.
	ErrUnknownSubtype = errors.New("rcp/avtp: unrecognized AVTPDU subtype")

	// ErrReservedBitsSet is returned when a field documented as reserved
	// (and required to be zero) is nonzero on decode.
	ErrReservedBitsSet = errors.New("rcp/avtp: reserved bits set in wire field")

	// ErrDataLengthOverflow is returned when a caller-supplied length value
	// does not fit the 11-bit AVTPDU data-length field.
	ErrDataLengthOverflow = errors.New("rcp/avtp: data length exceeds 11-bit field width")
)

// Message-layer (RCP-over-ACF) errors.
var (
	// ErrShortMessage is returned when a buffer is too short to hold the
	// declared message, or too short for the request-descriptor header of
	// the message kind it claims to be.
	ErrShortMessage = errors.New("rcp/avtp: buffer too short for RCP message")

	// ErrUnknownMessageKind is returned when a message's kind tag is
	// neither KindShort (ACF_ABB) nor KindLong (ACF_GBB).
	ErrUnknownMessageKind = errors.New("rcp/avtp: unrecognized RCP message kind")

	// ErrLengthOverflow is returned when an encoded message's total length
	// does not fit the 16-bit message-length field.
	ErrLengthOverflow = errors.New("rcp/avtp: message length exceeds 16-bit field width")

	// ErrPadOverflow is returned when a caller supplies a pad-byte count
	// outside the representable 0-3 range.
	ErrPadOverflow = errors.New("rcp/avtp: pad byte count out of range (0-3)")
)

// Frame-layer errors.
var (
	// ErrFrameLengthMismatch is returned when the AVTPDU header's
	// data-length field does not match the length of the RCP message that
	// actually follows it in the buffer.
	ErrFrameLengthMismatch = errors.New("rcp/avtp: AVTPDU data length does not match enclosed message length")
)
