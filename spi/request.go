package spi

// EncodeTransferRequest serializes a SPI transfer request's
// byte_msg_payload: the raw bytes to transmit, and nothing else. Which
// chip-select channel the transfer targets is carried by the request's
// evt[2:0] field (TC18 §13.5 Table 30's SPI row), not by a sub-opcode byte
// in the body — §13.7.3's Figure 23 shows the byte_msg_payload as the SPI
// payload in full, and states that "The byte_msg_payload will be presented
// on PICO in full", which leaves no room for an in-band selector byte.
//
// This function exists as the named codec for the request body even though
// it is now a plain copy: the transfer request's body shape is a wire
// contract this package pins with golden vectors, and a caller building one
// should not have to know whether it happens to be a copy today.
func EncodeTransferRequest(tx []byte) []byte {
	return append([]byte(nil), tx...)
}

// DecodeTransferRequest parses a transfer request's byte_msg_payload. It
// never panics on malformed input — every byte sequence, including the empty
// one, is a structurally valid SPI payload. The returned slice aliases b;
// callers that need to retain it beyond the lifetime of b should copy it.
func DecodeTransferRequest(b []byte) []byte {
	return b
}

// EncodeTransferResponse serializes a SPI transfer response's
// byte_msg_payload: the raw bytes the controller received, in the same
// bare-payload shape as the request.
func EncodeTransferResponse(rx []byte) []byte {
	return EncodeTransferRequest(rx)
}

// DecodeTransferResponse parses a transfer response's byte_msg_payload. It
// never panics on malformed input.
func DecodeTransferResponse(b []byte) []byte {
	return DecodeTransferRequest(b)
}
