package mdio

import "encoding/binary"

// requestHeaderLen is Mode(1) + PhyAddr(1) + DevAddr(1) + RegAddr(2).
const requestHeaderLen = 1 + 1 + 1 + 2

// dataLen is the width of a register value: MDIO registers are 16 bits
// wide in both Clause 22 and Clause 45.
const dataLen = 2

// EncodeReadRequest serializes a read Request into its wire representation.
func EncodeReadRequest(r Request) []byte {
	return encodeRequest(r)
}

// DecodeReadRequest parses a read Request from b. It never panics on
// malformed input.
func DecodeReadRequest(b []byte) (Request, error) {
	return decodeRequest(b)
}

// EncodeWriteRequest serializes a write Request and the 16-bit value to
// write into its wire representation: the same Request header
// EncodeReadRequest produces, followed by a 2-byte data field.
func EncodeWriteRequest(r Request, data uint16) []byte {
	buf := make([]byte, requestHeaderLen+dataLen)
	copy(buf, encodeRequest(r))
	binary.BigEndian.PutUint16(buf[requestHeaderLen:], data)
	return buf
}

// DecodeWriteRequest parses a write Request and its 16-bit data value from
// b. It never panics on malformed input, and rejects a buffer whose length
// isn't exactly requestHeaderLen+dataLen.
func DecodeWriteRequest(b []byte) (Request, uint16, error) {
	if len(b) < requestHeaderLen+dataLen {
		return Request{}, 0, ErrShortBuffer
	}
	if len(b) > requestHeaderLen+dataLen {
		return Request{}, 0, ErrTrailingBytes
	}
	r, err := decodeRequest(b[:requestHeaderLen])
	if err != nil {
		return Request{}, 0, err
	}
	return r, binary.BigEndian.Uint16(b[requestHeaderLen:]), nil
}

// EncodeResponse serializes a 16-bit register value response body.
func EncodeResponse(data uint16) []byte {
	buf := make([]byte, dataLen)
	binary.BigEndian.PutUint16(buf, data)
	return buf
}

// DecodeResponse parses a 16-bit register value response body from b. It
// never panics on malformed input, and rejects a buffer whose length isn't
// exactly dataLen.
func DecodeResponse(b []byte) (uint16, error) {
	if len(b) < dataLen {
		return 0, ErrShortBuffer
	}
	if len(b) > dataLen {
		return 0, ErrTrailingBytes
	}
	return binary.BigEndian.Uint16(b), nil
}

// encodeRequest is the shared Request header encoding EncodeReadRequest and
// EncodeWriteRequest both use.
func encodeRequest(r Request) []byte {
	buf := make([]byte, requestHeaderLen)
	buf[0] = byte(r.Mode)
	buf[1] = r.PhyAddr
	buf[2] = r.DevAddr
	binary.BigEndian.PutUint16(buf[3:5], r.RegAddr)
	return buf
}

// decodeRequest is the shared Request header decoding DecodeReadRequest and
// DecodeWriteRequest both use. Unlike DecodeReadRequest, this does not
// itself reject trailing bytes (DecodeWriteRequest needs the trailing data
// field DecodeReadRequest doesn't have), so DecodeReadRequest performs that
// check itself.
func decodeRequest(b []byte) (Request, error) {
	if len(b) < requestHeaderLen {
		return Request{}, ErrShortBuffer
	}
	if len(b) > requestHeaderLen {
		return Request{}, ErrTrailingBytes
	}
	return Request{
		Mode:    AddressMode(b[0]),
		PhyAddr: b[1],
		DevAddr: b[2],
		RegAddr: binary.BigEndian.Uint16(b[3:5]),
	}, nil
}
