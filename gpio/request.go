package gpio

import "encoding/binary"

// requestLen is the exact length TC18 §13.7.4.1 fixes for a GPIO request's
// byte_msg_payload: the 4-byte IO bitmask of Figure 24, one bit per pin,
// least-significant bits first for an endpoint with fewer than MaxPins pins.
// The selector that says what to DO with that bitmask is not in the body at
// all — it is evt[2:0] in the request-descriptor header (Table 30). §13.7.4.1
// is explicit about the length: "A request not having exactly four bytes is
// rejected and an error response with error code = INVALID_PARAMETER will be
// sent."
const requestLen = 4

// EncodeWriteRequest serializes a GPIO write request's byte_msg_payload: the
// 4-byte operand bitmask, and nothing else. Which combining rule the
// receiving endpoint applies it under, and whether it is presented at the
// pins at all or routed to the endpoint's configuration instead, is carried
// by the request's evt[2:0] field rather than by this body — see
// acf.EVTClassArithmetic and EVTClass.
func EncodeWriteRequest(operand uint32) []byte {
	buf := make([]byte, requestLen)
	binary.BigEndian.PutUint32(buf, operand)
	return buf
}

// DecodeWriteRequest parses a write request's byte_msg_payload. It never
// panics on malformed input, and enforces §13.7.4.1's exactly-four-bytes
// rule: a shorter body returns ErrShortBuffer and a longer one
// ErrTrailingBytes, both of which the transport layer answers with error
// code INVALID_PARAMETER.
func DecodeWriteRequest(b []byte) (uint32, error) {
	if len(b) < requestLen {
		return 0, ErrShortBuffer
	}
	if len(b) > requestLen {
		return 0, ErrTrailingBytes
	}
	return binary.BigEndian.Uint32(b), nil
}

// EncodeValue serializes a 32-bit pin-value bitmask — the body of a read
// response, or of a write response echoing the resulting value. It is the
// same 4-byte shape as a request body (Figure 24 uses one layout for both
// directions).
func EncodeValue(v uint32) []byte {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, v)
	return buf
}

// DecodeValue parses a 32-bit pin-value bitmask. It never panics on
// malformed input.
func DecodeValue(b []byte) (uint32, error) {
	if len(b) < 4 {
		return 0, ErrShortBuffer
	}
	if len(b) > 4 {
		return 0, ErrTrailingBytes
	}
	return binary.BigEndian.Uint32(b), nil
}
