package mdio

import "encoding/binary"

// requestHeaderLen is Mode(1) + PhyAddr(1) + DevAddr(1) + RegAddr(2).
const requestHeaderLen = 1 + 1 + 1 + 2

// EncodeReadRequest serializes a read Request into its wire representation.
func EncodeReadRequest(r Request) []byte {
	return encodeRequest(r)
}

// DecodeReadRequest parses a read Request from b. It never panics on
// malformed input.
func DecodeReadRequest(b []byte) (Request, error) {
	return decodeRequest(b)
}

// EncodeWriteRequest serializes a write Request and the value to write
// into its wire representation: the same Request header EncodeReadRequest
// produces, followed by a data field r.DataWidth() bytes wide (2 or 4 —
// see Request.DataWidth). Only data's low DataWidth()*8 bits are used; any
// higher bits are discarded.
func EncodeWriteRequest(r Request, data uint32) []byte {
	width := r.DataWidth()
	buf := make([]byte, requestHeaderLen+width)
	copy(buf, encodeRequest(r))
	putData(buf[requestHeaderLen:], width, data)
	return buf
}

// DecodeWriteRequest parses a write Request and its data value from b. It
// never panics on malformed input, and rejects a buffer whose length isn't
// exactly requestHeaderLen plus the decoded Request's DataWidth().
func DecodeWriteRequest(b []byte) (Request, uint32, error) {
	if len(b) < requestHeaderLen {
		return Request{}, 0, ErrShortBuffer
	}
	r, err := decodeRequest(b[:requestHeaderLen])
	if err != nil {
		return Request{}, 0, err
	}
	rest := b[requestHeaderLen:]
	width := r.DataWidth()
	if len(rest) < width {
		return Request{}, 0, ErrShortBuffer
	}
	if len(rest) > width {
		return Request{}, 0, ErrTrailingBytes
	}
	return r, getData(rest, width), nil
}

// EncodeResponse serializes a register-value response body for a request
// shaped like r: r.DataWidth() bytes wide (2 or 4 — see Request.DataWidth).
// Only data's low DataWidth()*8 bits are used; any higher bits are
// discarded.
func EncodeResponse(r Request, data uint32) []byte {
	width := r.DataWidth()
	buf := make([]byte, width)
	putData(buf, width, data)
	return buf
}

// DecodeResponse parses a register-value response body from b for a
// request shaped like r. It never panics on malformed input, and rejects a
// buffer whose length isn't exactly r.DataWidth().
func DecodeResponse(r Request, b []byte) (uint32, error) {
	width := r.DataWidth()
	if len(b) < width {
		return 0, ErrShortBuffer
	}
	if len(b) > width {
		return 0, ErrTrailingBytes
	}
	return getData(b, width), nil
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
// DecodeWriteRequest both use. DecodeReadRequest passes the whole buffer,
// so this rejects any trailing bytes on its own; DecodeWriteRequest instead
// passes an exact requestHeaderLen slice and performs its own length check
// against the full write-request length, since only it knows the trailing
// data field's width (it depends on the decoded Request's DataWidth).
func decodeRequest(b []byte) (Request, error) {
	if len(b) < requestHeaderLen {
		return Request{}, ErrShortBuffer
	}
	if len(b) > requestHeaderLen {
		return Request{}, ErrTrailingBytes
	}
	return Request{
		Mode:    Mode(b[0]),
		PhyAddr: b[1],
		DevAddr: b[2],
		RegAddr: binary.BigEndian.Uint16(b[3:5]),
	}, nil
}

// putData writes the low width bytes of data into buf, big-endian. width
// must be 2 or 4 (see Request.DataWidth) and buf must be exactly that long.
func putData(buf []byte, width int, data uint32) {
	if width == 4 {
		binary.BigEndian.PutUint32(buf, data)
		return
	}
	binary.BigEndian.PutUint16(buf, uint16(data))
}

// getData reads a big-endian width-byte value from b (width is 2 or 4 —
// see Request.DataWidth), zero-extended to uint32.
func getData(b []byte, width int) uint32 {
	if width == 4 {
		return binary.BigEndian.Uint32(b)
	}
	return uint32(binary.BigEndian.Uint16(b))
}
