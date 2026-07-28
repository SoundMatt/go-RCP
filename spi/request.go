package spi

// EncodeTransferRequest serializes a SPI transfer request body: a one-byte
// Channel sub-opcode selecting which pre-configured chip-select channel this
// transfer targets, followed by the raw bytes to transmit.
func EncodeTransferRequest(ch Channel, tx []byte) []byte {
	buf := make([]byte, 1+len(tx))
	buf[0] = byte(ch)
	copy(buf[1:], tx)
	return buf
}

// DecodeTransferRequest parses a transfer request body. It never panics on
// malformed input. The returned slice aliases b — callers that need to
// retain it beyond the lifetime of b should copy it.
func DecodeTransferRequest(b []byte) (Channel, []byte, error) {
	if len(b) < 1 {
		return 0, nil, ErrShortBuffer
	}
	ch := Channel(b[0])
	if !ch.Valid() {
		return 0, nil, ErrInvalidChannel
	}
	return ch, b[1:], nil
}

// EncodeTransferResponse serializes a SPI transfer response body: the same
// Channel sub-opcode echoed back, followed by the raw bytes the controller
// received.
func EncodeTransferResponse(ch Channel, rx []byte) []byte {
	return EncodeTransferRequest(ch, rx)
}

// DecodeTransferResponse parses a transfer response body. It never panics
// on malformed input.
func DecodeTransferResponse(b []byte) (Channel, []byte, error) {
	return DecodeTransferRequest(b)
}
