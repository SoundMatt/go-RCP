package iseled

import "encoding/binary"

// dataLenFieldLen is the width of the length-prefix field EncodeCommand and
// EncodeDeviceResponse both use ahead of their variable-length Data.
const dataLenFieldLen = 2

// crcLen is the trailing ISELED-native CRC8 field width both encodings
// append.
const crcLen = 1

// addrHeaderLen is Address(1) + DataLen(2).
const addrHeaderLen = 1 + dataLenFieldLen

// EncodeCommand serializes cmd into its wire representation: Address(1),
// a 2-byte Data length, Data itself, and a trailing ISELED-native CRC8 (see
// ComputeCRC) computed over Address and Data.
func EncodeCommand(cmd Command) []byte {
	buf := make([]byte, addrHeaderLen+len(cmd.Data)+crcLen)
	buf[0] = cmd.Address
	binary.BigEndian.PutUint16(buf[1:3], uint16(len(cmd.Data)))
	copy(buf[addrHeaderLen:], cmd.Data)
	buf[len(buf)-1] = ComputeCRC(append([]byte{cmd.Address}, cmd.Data...))
	return buf
}

// DecodeCommand parses a Command from b. It never panics on malformed
// input, rejects a buffer whose declared Data length does not exactly
// account for every remaining byte before the trailing CRC, and returns
// ErrCRCMismatch (rather than a silently-corrupted Command) when the
// trailing CRC does not match the one ComputeCRC recomputes over the
// decoded Address and Data.
func DecodeCommand(b []byte) (Command, error) {
	addr, data, err := decodeAddressedPayload(b)
	if err != nil {
		return Command{}, err
	}
	return Command{Address: addr, Data: data}, nil
}

// EncodeDeviceResponse serializes r into its wire representation: the same
// Address(1) + 2-byte Data length + Data + trailing CRC8 shape
// EncodeCommand uses, computed over r.Address and r.Data.
func EncodeDeviceResponse(r DeviceResponse) []byte {
	return EncodeCommand(Command(r))
}

// DecodeDeviceResponse parses a DeviceResponse from b, with the same
// framing and CRC verification DecodeCommand performs.
func DecodeDeviceResponse(b []byte) (DeviceResponse, error) {
	addr, data, err := decodeAddressedPayload(b)
	if err != nil {
		return DeviceResponse{}, err
	}
	return DeviceResponse{Address: addr, Data: data}, nil
}

// decodeAddressedPayload is the shared Command/DeviceResponse decode
// implementation: both share the identical Address+length+Data+CRC framing.
func decodeAddressedPayload(b []byte) (addr uint8, data []byte, err error) {
	if len(b) < addrHeaderLen+crcLen {
		return 0, nil, ErrShortBuffer
	}
	addr = b[0]
	dataLen := int(binary.BigEndian.Uint16(b[1:3]))
	want := addrHeaderLen + dataLen + crcLen
	if len(b) < want {
		return 0, nil, ErrShortBuffer
	}
	if len(b) > want {
		return 0, nil, ErrTrailingBytes
	}

	if dataLen > 0 {
		data = make([]byte, dataLen)
		copy(data, b[addrHeaderLen:addrHeaderLen+dataLen])
	}

	gotCRC := b[want-1]
	wantCRC := ComputeCRC(append([]byte{addr}, data...))
	if gotCRC != wantCRC {
		return 0, nil, ErrCRCMismatch
	}
	return addr, data, nil
}

// EncodeAggregatedResponse serializes resp into its wire representation: a
// leading count byte, followed by each DeviceResponse's own
// EncodeDeviceResponse encoding back-to-back (each self-delimiting via its
// own length field, so no extra outer framing is needed between entries).
func EncodeAggregatedResponse(resp AggregatedResponse) []byte {
	buf := []byte{byte(len(resp))}
	for _, r := range resp {
		buf = append(buf, EncodeDeviceResponse(r)...)
	}
	return buf
}

// DecodeAggregatedResponse parses an AggregatedResponse from b. It never
// panics on malformed input, and rejects a count that does not exactly
// account for every remaining byte (either too few entries present, or
// trailing bytes past the declared count) — the same "don't silently ignore
// extra or missing input" posture the rest of this repo's decoders take.
func DecodeAggregatedResponse(b []byte) (AggregatedResponse, error) {
	if len(b) < 1 {
		return nil, ErrShortBuffer
	}
	count := int(b[0])
	rest := b[1:]

	resp := make(AggregatedResponse, 0, count)
	for i := 0; i < count; i++ {
		if len(rest) < addrHeaderLen+crcLen {
			return nil, ErrShortBuffer
		}
		dataLen := int(binary.BigEndian.Uint16(rest[1:3]))
		entryLen := addrHeaderLen + dataLen + crcLen
		if len(rest) < entryLen {
			return nil, ErrShortBuffer
		}
		r, err := DecodeDeviceResponse(rest[:entryLen])
		if err != nil {
			return nil, err
		}
		resp = append(resp, r)
		rest = rest[entryLen:]
	}
	if len(rest) > 0 {
		return nil, ErrTrailingBytes
	}
	return resp, nil
}
