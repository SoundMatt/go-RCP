package can

import "encoding/binary"

const (
	flagExtended      = 1 << 0
	flagBitRateSwitch = 1 << 1
)

// frameHeaderLen is Format(1) + flags(1) + ID(4).
const frameHeaderLen = 1 + 1 + 4

// xlHeaderLen is SDT(1) + VCID(1) + AF(4).
const xlHeaderLen = 1 + 1 + 4

// dataLenFieldLen is the width of the trailing data-length-prefix field.
// XLMaxPayload (2048) does not fit a single byte, so this is 2 bytes wide
// for every format, not just FormatXL.
const dataLenFieldLen = 2

// EncodeFrame serializes f into its wire representation: a fixed header
// (Format, Extended/BitRateSwitch flags, ID), XLHeader's three fields when
// Format is FormatXL, a 2-byte data length, and Data itself. EncodeFrame
// does not itself validate f — call Frame.Validate first; callers that skip
// that step get whatever (possibly non-conformant) bytes f's fields encode
// to.
func EncodeFrame(f Frame) []byte {
	headerLen := frameHeaderLen
	if f.Format == FormatXL {
		headerLen += xlHeaderLen
	}
	buf := make([]byte, headerLen+dataLenFieldLen+len(f.Data))

	buf[0] = byte(f.Format)
	var flags uint8
	if f.Extended {
		flags |= flagExtended
	}
	if f.BitRateSwitch {
		flags |= flagBitRateSwitch
	}
	buf[1] = flags
	binary.BigEndian.PutUint32(buf[2:6], f.ID)

	off := frameHeaderLen
	if f.Format == FormatXL {
		buf[off] = f.XL.SDT
		buf[off+1] = f.XL.VCID
		binary.BigEndian.PutUint32(buf[off+2:off+6], f.XL.AF)
		off += xlHeaderLen
	}

	binary.BigEndian.PutUint16(buf[off:off+dataLenFieldLen], uint16(len(f.Data)))
	off += dataLenFieldLen
	copy(buf[off:], f.Data)
	return buf
}

// DecodeFrame parses a Frame from b. It never panics on malformed input, and
// rejects a buffer whose declared data length does not exactly account for
// every remaining byte, the same "don't silently ignore extra or missing
// input" posture the rest of this repo's decoders take. It does not itself
// call Validate — a caller that needs a plausibility-checked Frame should
// call it explicitly on the result.
func DecodeFrame(b []byte) (Frame, error) {
	if len(b) < frameHeaderLen {
		return Frame{}, ErrShortBuffer
	}
	format := Format(b[0])
	flags := b[1]
	f := Frame{
		Format:        format,
		Extended:      flags&flagExtended != 0,
		BitRateSwitch: flags&flagBitRateSwitch != 0,
		ID:            binary.BigEndian.Uint32(b[2:6]),
	}

	off := frameHeaderLen
	if format == FormatXL {
		if len(b) < off+xlHeaderLen {
			return Frame{}, ErrShortBuffer
		}
		f.XL = XLHeader{
			SDT:  b[off],
			VCID: b[off+1],
			AF:   binary.BigEndian.Uint32(b[off+2 : off+6]),
		}
		off += xlHeaderLen
	}

	if len(b) < off+dataLenFieldLen {
		return Frame{}, ErrShortBuffer
	}
	dataLen := int(binary.BigEndian.Uint16(b[off : off+dataLenFieldLen]))
	off += dataLenFieldLen

	if len(b) < off+dataLen {
		return Frame{}, ErrShortBuffer
	}
	if len(b) > off+dataLen {
		return Frame{}, ErrTrailingBytes
	}
	if dataLen > 0 {
		f.Data = make([]byte, dataLen)
		copy(f.Data, b[off:off+dataLen])
	}
	return f, nil
}
