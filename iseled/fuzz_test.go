package iseled_test

import (
	"testing"

	"github.com/SoundMatt/go-RCP/v9/iseled"
)

// ── decoders never panic on arbitrary input (covered under REQ-ISELED-003/
// REQ-ISELED-004's round-trip tests; this file adds fuzzing on top) ───────

func FuzzDecodeConfig(f *testing.F) {
	f.Add([]byte{})
	f.Add(goldenConfig)
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = iseled.DecodeConfig(b) // must not panic
	})
}

func FuzzDecodeCommand(f *testing.F) {
	f.Add([]byte{})
	f.Add(goldenCommand)
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = iseled.DecodeCommand(b) // must not panic
	})
}

func FuzzDecodeAggregatedResponse(f *testing.F) {
	f.Add([]byte{})
	f.Add(iseled.EncodeAggregatedResponse(iseled.AggregatedResponse{{Address: 1, Data: []byte{0x01}}}))
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = iseled.DecodeAggregatedResponse(b) // must not panic
	})
}
