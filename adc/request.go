package adc

import "encoding/binary"

// EncodeValue serializes a 16-bit sample value — the body of a read
// response, or of a manual-trigger write response echoing the resulting
// value.
func EncodeValue(v uint16) []byte {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, v)
	return buf
}

// DecodeValue parses a 16-bit sample value. It never panics on malformed
// input.
func DecodeValue(b []byte) (uint16, error) {
	if len(b) < 2 {
		return 0, ErrShortBuffer
	}
	if len(b) > 2 {
		return 0, ErrTrailingBytes
	}
	return binary.BigEndian.Uint16(b), nil
}
