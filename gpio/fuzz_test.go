//fusa:test REQ-GPIO-011

package gpio_test

import (
	"testing"

	"github.com/SoundMatt/go-RCP/gpio"
)

// ── REQ-GPIO-011 (fuzz half): decoders never panic on arbitrary input ─────

func FuzzDecodeConfig(f *testing.F) {
	f.Add([]byte{})
	f.Add(goldenConfig)
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = gpio.DecodeConfig(b) // must not panic
	})
}

func FuzzDecodeWriteRequest(f *testing.F) {
	f.Add([]byte{})
	f.Add(goldenWriteRequest)
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = gpio.DecodeWriteRequest(b) // must not panic
	})
}

func FuzzDecodeValue(f *testing.F) {
	f.Add([]byte{})
	f.Add(goldenResponseValue)
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = gpio.DecodeValue(b) // must not panic
	})
}
