package mdio_test

import (
	"testing"

	"github.com/SoundMatt/go-RCP/mdio"
)

// ── decoders never panic on arbitrary input (covered under REQ-MDIO-004's
// round-trip tests; this file adds fuzzing on top) ─────────────────────────

func FuzzDecodeConfig(f *testing.F) {
	f.Add([]byte{})
	f.Add(goldenConfig)
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = mdio.DecodeConfig(b) // must not panic
	})
}

func FuzzDecodeReadRequest(f *testing.F) {
	f.Add([]byte{})
	f.Add(goldenReadRequest)
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = mdio.DecodeReadRequest(b) // must not panic
	})
}

func FuzzDecodeWriteRequest(f *testing.F) {
	f.Add([]byte{})
	f.Add(mdio.EncodeWriteRequest(mdio.Request{}, 0))
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _, _ = mdio.DecodeWriteRequest(b) // must not panic
	})
}
