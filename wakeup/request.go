package wakeup

import "encoding/binary"

// powerStateLen is the width of an encoded PowerState.
const powerStateLen = 1

// EncodePowerStateRequest serializes a write request body: the PowerState
// target to transition to.
func EncodePowerStateRequest(target PowerState) []byte {
	return []byte{byte(target)}
}

// DecodePowerStateRequest parses a write request body. It never panics on
// malformed input, and rejects a buffer whose length isn't exactly
// powerStateLen.
func DecodePowerStateRequest(b []byte) (PowerState, error) {
	if len(b) < powerStateLen {
		return 0, ErrShortBuffer
	}
	if len(b) > powerStateLen {
		return 0, ErrTrailingBytes
	}
	return PowerState(b[0]), nil
}

// EncodePowerStateResponse serializes a read response, or a write
// response's echo, body: the current (or newly applied) PowerState.
func EncodePowerStateResponse(state PowerState) []byte {
	return EncodePowerStateRequest(state)
}

// DecodePowerStateResponse parses a read/write response body. It never
// panics on malformed input, for the same reason DecodePowerStateRequest
// doesn't.
func DecodePowerStateResponse(b []byte) (PowerState, error) {
	return DecodePowerStateRequest(b)
}

// wakeHandshakeLen is Start(1) + Sequence(2).
const wakeHandshakeLen = 1 + 2

// EncodeWakeHandshake serializes h into its wire representation.
func EncodeWakeHandshake(h WakeHandshake) []byte {
	buf := make([]byte, wakeHandshakeLen)
	buf[0] = byte(h.Start)
	binary.BigEndian.PutUint16(buf[1:3], h.Sequence)
	return buf
}

// DecodeWakeHandshake parses a WakeHandshake from b. It never panics on
// malformed input, and rejects a buffer whose length isn't exactly
// wakeHandshakeLen.
func DecodeWakeHandshake(b []byte) (WakeHandshake, error) {
	if len(b) < wakeHandshakeLen {
		return WakeHandshake{}, ErrShortBuffer
	}
	if len(b) > wakeHandshakeLen {
		return WakeHandshake{}, ErrTrailingBytes
	}
	return WakeHandshake{
		Start:    StartKind(b[0]),
		Sequence: binary.BigEndian.Uint16(b[1:3]),
	}, nil
}
