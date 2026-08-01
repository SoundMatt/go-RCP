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

// evt-field (TC18 §13.5 Table 30 / §12.9.1) errors. Every one of these maps
// to error code UNSUPPORTED_CMD in a wire-level error response — see
// udp.errorCodeFor — because that is the code the specification names for
// each of the conditions they represent.
var (
	// ErrEVTReserved is returned when a request's evt[2:0] value is one
	// Table 30 marks reserved for the addressed endpoint type: 110b for
	// SPI, 001b-110b for the ADC/PWM_IN/I²C/LIN/CAN/UART/ISELED/MDIO row
	// (see ClassifyEVT's documented deviation for that row's 000b entry),
	// and 100b for GPIO/PWM_OUT. Table 30 requires each such request be
	// "rejected with error code = UNSUPPORTED_CMD".
	ErrEVTReserved = errors.New("rcp/acf: evt[2:0] value is reserved for this endpoint type (TC18 §13.5 Table 30)")

	// ErrEVTMissingPayload is returned when a request sets evt[2:0] to a
	// non-zero value but carries no byte_msg_payload at all, which TC18
	// §12.9.1 requires be answered with error code UNSUPPORTED_CMD: "If
	// evt[2:0] ≠ 0 and no byte_msg_payload is present, then an error
	// response shall be sent with the error code = UNSUPPORTED_CMD". This
	// rule is endpoint-type-independent — it applies before Table 30's
	// per-type interpretation.
	ErrEVTMissingPayload = errors.New("rcp/acf: evt[2:0] is non-zero but the request carries no byte_msg_payload (TC18 §12.9.1)")

	// ErrEVTUnknownClass is returned when ClassifyEVT is handed an EVTClass
	// that is not one of Table 30's three defined endpoint-type rows. It is
	// a caller programming error rather than a wire condition.
	ErrEVTUnknownClass = errors.New("rcp/acf: unrecognized evt endpoint-type class")

	// ErrShortConfigRequest is returned when a configuration request's
	// byte_msg_payload (the evt[2:0] = 111b shape) is too short to hold the
	// leading relative EP_func register start address (TC18 §12.7.1
	// Figure 18).
	ErrShortConfigRequest = errors.New("rcp/acf: configuration request body too short for its EP_func start address")
)
