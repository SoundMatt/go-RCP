//fusa:test REQ-PWM-009

package pwm_test

import (
	"testing"

	"github.com/SoundMatt/go-RCP/pwm"
)

// ── REQ-PWM-009 (fuzz half): decoders never panic on arbitrary input ──────

func FuzzDecodeConfig(f *testing.F) {
	f.Add([]byte{})
	f.Add(goldenConfig)
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = pwm.DecodeConfig(b) // must not panic
	})
}

func FuzzDecodeWaveform(f *testing.F) {
	f.Add([]byte{})
	f.Add(goldenWaveform)
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _, _ = pwm.DecodeWaveform(b) // must not panic
	})
}
