//fusa:test REQ-CANEP-005

package can_test

import (
	"testing"

	"github.com/SoundMatt/go-RCP/can"
)

// ── REQ-CANEP-005 (fuzz half): decoders never panic on arbitrary input ────

func FuzzDecodeConfig(f *testing.F) {
	f.Add([]byte{})
	f.Add(goldenConfig)
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = can.DecodeConfig(b) // must not panic
	})
}

func FuzzDecodeFrame(f *testing.F) {
	f.Add([]byte{})
	f.Add(goldenClassicalFrame)
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = can.DecodeFrame(b) // must not panic
	})
}
