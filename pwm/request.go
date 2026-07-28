package pwm

import "encoding/binary"

// waveformLen is PeriodMicros(4) + ActiveMicros(4): the symmetric two-field
// payload shape ROADMAP.md Milestone 48 specifies for both PWM output and
// PWM input.
const waveformLen = 4 + 4

// EncodeWaveform serializes a PWM waveform body: the period and active
// (high) duration, both in microseconds. This is used for a RoleOutput
// write/read request and response body alike, and for a RoleInput read
// response body — the same symmetric shape on both sides of the endpoint.
func EncodeWaveform(periodMicros, activeMicros uint32) []byte {
	buf := make([]byte, waveformLen)
	binary.BigEndian.PutUint32(buf[0:4], periodMicros)
	binary.BigEndian.PutUint32(buf[4:8], activeMicros)
	return buf
}

// DecodeWaveform parses a waveform body. It never panics on malformed input.
func DecodeWaveform(b []byte) (periodMicros, activeMicros uint32, err error) {
	if len(b) < waveformLen {
		return 0, 0, ErrShortBuffer
	}
	if len(b) > waveformLen {
		return 0, 0, ErrTrailingBytes
	}
	return binary.BigEndian.Uint32(b[0:4]), binary.BigEndian.Uint32(b[4:8]), nil
}
