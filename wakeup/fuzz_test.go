package wakeup_test

import (
	"testing"

	"github.com/SoundMatt/go-RCP/v9/wakeup"
)

// ── decoders never panic on arbitrary input (covered under REQ-WAKEUP-002/
// REQ-WAKEUP-004's round-trip tests; this file adds fuzzing on top) ────────

func FuzzDecodeConfig(f *testing.F) {
	f.Add([]byte{})
	f.Add(goldenConfig)
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = wakeup.DecodeConfig(b) // must not panic
	})
}

func FuzzDecodePowerStateRequest(f *testing.F) {
	f.Add([]byte{})
	f.Add(goldenSleepRequest)
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = wakeup.DecodePowerStateRequest(b) // must not panic
	})
}

func FuzzDecodeWakeHandshake(f *testing.F) {
	f.Add([]byte{})
	f.Add(wakeup.EncodeWakeHandshake(wakeup.WakeHandshake{}))
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = wakeup.DecodeWakeHandshake(b) // must not panic
	})
}
