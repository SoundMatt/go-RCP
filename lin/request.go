package lin

// EncodeTransferRequest serializes a LIN transfer request body: the raw
// bytes to place on the bus. Per doc.go's Scope section, this protocol layer
// performs no frame ID/checksum/schedule-table parsing at all, so there is
// nothing to validate here beyond copying the input.
func EncodeTransferRequest(tx []byte) []byte {
	return append([]byte(nil), tx...)
}

// DecodeTransferRequest parses a transfer request body. Any byte sequence,
// including an empty one, is a structurally legal raw transfer body at this
// layer, so this never fails — it exists for symmetry with this package's
// other Encode/Decode pairs and with the sibling i2c package's request
// shape. The returned slice aliases b — callers that need to retain it beyond
// the lifetime of b should copy it.
func DecodeTransferRequest(b []byte) []byte {
	return b
}

// EncodeTransferResponse serializes a LIN transfer response body: the raw
// bytes the commander received back over the bus (empty for a
// write-only/no-response frame).
func EncodeTransferResponse(rx []byte) []byte {
	return EncodeTransferRequest(rx)
}

// DecodeTransferResponse parses a transfer response body. It never fails,
// for the same reason DecodeTransferRequest doesn't.
func DecodeTransferResponse(b []byte) []byte {
	return DecodeTransferRequest(b)
}
