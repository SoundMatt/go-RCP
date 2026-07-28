package uart

import "encoding/binary"

// EncodeWriteRequest serializes a UART TX write request body: the raw bytes
// to transmit, with no framing of its own.
func EncodeWriteRequest(tx []byte) []byte {
	return append([]byte(nil), tx...)
}

// DecodeWriteRequest parses a TX write request body. Any byte sequence,
// including an empty one, is a structurally legal TX body, so this never
// fails — it exists for symmetry with this package's other Encode/Decode
// pairs. The returned slice aliases b — callers that need to retain it
// beyond the lifetime of b should copy it.
func DecodeWriteRequest(b []byte) []byte {
	return b
}

// EncodeWriteResponse serializes a UART TX write response body: a two-byte
// count of the bytes the endpoint actually accepted for transmission.
func EncodeWriteResponse(n int) []byte {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, uint16(n))
	return buf
}

// DecodeWriteResponse parses a TX write response body. It never panics on
// malformed input.
func DecodeWriteResponse(b []byte) (int, error) {
	if len(b) < 2 {
		return 0, ErrShortBuffer
	}
	if len(b) > 2 {
		return 0, ErrTrailingBytes
	}
	return int(binary.BigEndian.Uint16(b)), nil
}

// readResponseHeaderLen is the one-byte completeness flag every RX read
// response body leads with (see EncodeReadResponse).
const readResponseHeaderLen = 1

// EncodeReadResponse serializes a UART RX read response body: a one-byte
// completeness flag (1 when data holds every byte the request asked for, 0
// when the RX FIFO could not fill the full request before the read-timeout
// stand-in this package uses — see doc.go) followed by the drained bytes
// themselves. A caller that receives complete=false is expected to issue a
// follow-up read request to retrieve the remainder, the same fragmented-
// delivery-on-timeout posture ROADMAP.md Milestone 48 describes for UART.
func EncodeReadResponse(complete bool, data []byte) []byte {
	buf := make([]byte, readResponseHeaderLen+len(data))
	if complete {
		buf[0] = 1
	}
	copy(buf[readResponseHeaderLen:], data)
	return buf
}

// DecodeReadResponse parses an RX read response body. It never panics on
// malformed input. The returned slice aliases b — callers that need to
// retain it beyond the lifetime of b should copy it.
func DecodeReadResponse(b []byte) (complete bool, data []byte, err error) {
	if len(b) < readResponseHeaderLen {
		return false, nil, ErrShortBuffer
	}
	return b[0] != 0, b[readResponseHeaderLen:], nil
}
