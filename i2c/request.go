package i2c

// EncodeTransferRequest serializes an I2C transfer request body: the raw
// bytes to place on the bus, including the address byte(s) themselves. This
// protocol layer performs no address parsing at all — see doc.go's Scope
// section — so there is nothing to validate here beyond copying the input.
func EncodeTransferRequest(tx []byte) []byte {
	return append([]byte(nil), tx...)
}

// DecodeTransferRequest parses a transfer request body. Any byte sequence,
// including an empty one, is a structurally legal raw transfer body at this
// layer, so this never fails — it exists for symmetry with this package's
// other Encode/Decode pairs and with the sibling gpio/spi packages' request
// shape. The returned slice aliases b — callers that need to retain it beyond
// the lifetime of b should copy it.
func DecodeTransferRequest(b []byte) []byte {
	return b
}

// EncodeTransferResponse serializes an I2C transfer response body: the raw
// bytes the controller received back over the bus (empty for a
// write-only transaction).
func EncodeTransferResponse(rx []byte) []byte {
	return EncodeTransferRequest(rx)
}

// DecodeTransferResponse parses a transfer response body. It never fails,
// for the same reason DecodeTransferRequest doesn't.
func DecodeTransferResponse(b []byte) []byte {
	return DecodeTransferRequest(b)
}
