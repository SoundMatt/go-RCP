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

	// ErrLengthOverflow is returned when an encoded message's total length,
	// measured in quadlets, does not fit the 9-bit acf_msg_length field.
	ErrLengthOverflow = errors.New("rcp/acf: message length exceeds 9-bit quadlet field width")

	// ErrEVTOverflow is returned when a caller supplies an EVT value
	// outside the representable 4-bit range (0-15).
	ErrEVTOverflow = errors.New("rcp/acf: EVT out of range (0-15)")

	// ErrTransactionNumOverflow is returned when a caller supplies a
	// TransactionNum outside the representable 8-bit wire range (0-255).
	ErrTransactionNumOverflow = errors.New("rcp/acf: TransactionNum out of range for the 8-bit wire field (0-255)")

	// ErrByteBusIDOverflow is returned when a caller supplies a ByteBusID
	// above 2047 to EncodeMessage. avtp.ByteBusID is uint16, so this is
	// reachable (a caller can set any uint16 value), but decode can never
	// produce one: the wire field's 11-bit extraction is bounded to
	// 0-2047 by construction.
	ErrByteBusIDOverflow = errors.New("rcp/acf: ByteBusID out of range for the 11-bit wire field (0-2047)")

	// ErrReadSizeOverflow is returned when a caller supplies a
	// ReadSizeOrSegment value outside the representable 12-bit range
	// (0-4095).
	ErrReadSizeOverflow = errors.New("rcp/acf: ReadSizeOrSegment out of range for the 12-bit wire field (0-4095)")

	// ErrTrailingBytes is returned when a buffer is longer than the
	// message's own declared acf_msg_length (in quadlets): a message that
	// undershoots the buffer it was handed is exactly the kind of malformed
	// input a receiver must not paper over by silently dropping the excess,
	// the same strict-length posture DecodeFrame already takes at the
	// AVTPDU layer.
	ErrTrailingBytes = errors.New("rcp/acf: buffer longer than declared acf_msg_length")
)

// Frame-layer errors.
var (
	// ErrFrameLengthMismatch is returned when the AVTPDU header's
	// data-length field does not match the length of the RCP message that
	// actually follows it in the buffer.
	ErrFrameLengthMismatch = errors.New("rcp/acf: AVTPDU data length does not match enclosed message length")
)
