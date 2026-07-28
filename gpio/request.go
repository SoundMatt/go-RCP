package gpio

import "encoding/binary"

// requestLen is Semantic(1) + operand mask(4). A write request's body (and a
// reconfigure request's body, which reuses this same shape with
// SemanticReconfigure) is always exactly this many bytes.
const requestLen = 1 + 4

// EncodeWriteRequest serializes a GPIO write (or reconfigure) request body:
// which WriteSemantic to apply, and the 4-byte operand bitmask it combines
// with the endpoint's current state (or, for SemanticReconfigure, the new
// Direction bitmask).
func EncodeWriteRequest(sem WriteSemantic, operand uint32) []byte {
	buf := make([]byte, requestLen)
	buf[0] = byte(sem)
	binary.BigEndian.PutUint32(buf[1:5], operand)
	return buf
}

// DecodeWriteRequest parses a write (or reconfigure) request body. It never
// panics on malformed input.
func DecodeWriteRequest(b []byte) (WriteSemantic, uint32, error) {
	if len(b) < requestLen {
		return 0, 0, ErrShortBuffer
	}
	if len(b) > requestLen {
		return 0, 0, ErrTrailingBytes
	}
	sem := WriteSemantic(b[0])
	if !sem.Valid() {
		return 0, 0, ErrInvalidSemantic
	}
	return sem, binary.BigEndian.Uint32(b[1:5]), nil
}

// EncodeValue serializes a 32-bit pin-value bitmask — the body of a read
// response, or of a write/reconfigure response echoing the resulting value.
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
