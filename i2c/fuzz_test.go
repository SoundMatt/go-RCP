//fusa:test REQ-I2C-008

package i2c_test

import (
	"testing"

	"github.com/SoundMatt/go-RCP/v9/i2c"
)

// ── REQ-I2C-008 (fuzz half): decoders never panic on arbitrary input ──────

func FuzzDecodeConfig(f *testing.F) {
	f.Add([]byte{})
	f.Add(goldenConfig)
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = i2c.DecodeConfig(b) // must not panic
	})
}

func FuzzDecodeTransferRequest(f *testing.F) {
	f.Add([]byte{})
	f.Add(goldenTransferRequest)
	f.Fuzz(func(t *testing.T, b []byte) {
		_ = i2c.DecodeTransferRequest(b) // must not panic
	})
}
