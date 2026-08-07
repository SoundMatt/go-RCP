//fusa:test REQ-ADC-009

package adc_test

import (
	"testing"

	"github.com/SoundMatt/go-RCP/v9/adc"
)

// ── REQ-ADC-009 (fuzz half): decoders never panic on arbitrary input ──────

func FuzzDecodeConfig(f *testing.F) {
	f.Add([]byte{})
	f.Add(goldenConfig)
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = adc.DecodeConfig(b) // must not panic
	})
}

func FuzzDecodeValue(f *testing.F) {
	f.Add([]byte{})
	f.Add(goldenValue)
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = adc.DecodeValue(b) // must not panic
	})
}
