package pwm

import "encoding/binary"

// waveformLen is PeriodTicks(2) + ActiveTicks(2): the symmetric two-field
// payload shape the governing OPEN Alliance TC18 Remote Control Protocol
// Specification specifies for both a PWM_OUT and a PWM_IN request/response
// body — period first, then active (high) time, each a big-endian 16-bit
// count of the endpoint's configured clock ticks.
const waveformLen = 2 + 2

// EncodeWaveform serializes a PWM waveform body: the period followed by the
// active (high) duration, both as a count of the endpoint's configured
// clock ticks (this package treats both as opaque tick counts — it has no
// clock-rate configuration to convert them to/from a time unit). This is
// used for a RoleOutput write/read request and response body alike, and for
// a RoleInput read response body — the same symmetric shape on both sides
// of the endpoint. Note the wire order (period, then active) does not match
// this function's parameter order (activeTicks, then periodTicks); the
// parameter order instead matches every other caller-facing spelling of a
// waveform in this package (see e.g. Endpoint.applyOutput).
func EncodeWaveform(activeTicks, periodTicks uint16) []byte {
	buf := make([]byte, waveformLen)
	binary.BigEndian.PutUint16(buf[0:2], periodTicks)
	binary.BigEndian.PutUint16(buf[2:4], activeTicks)
	return buf
}

// DecodeWaveform parses a waveform body. It never panics on malformed
// input, and rejects a buffer whose length isn't exactly waveformLen (4
// bytes). See EncodeWaveform's doc comment on the wire order (period, then
// active) versus this function's return order (active, then period).
func DecodeWaveform(b []byte) (activeTicks, periodTicks uint16, err error) {
	if len(b) < waveformLen {
		return 0, 0, ErrShortBuffer
	}
	if len(b) > waveformLen {
		return 0, 0, ErrTrailingBytes
	}
	return binary.BigEndian.Uint16(b[2:4]), binary.BigEndian.Uint16(b[0:2]), nil
}
