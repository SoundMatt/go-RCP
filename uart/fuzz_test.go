//fusa:test REQ-UART-009

package uart_test

import (
	"testing"

	"github.com/SoundMatt/go-RCP/uart"
)

// ── REQ-UART-009 (fuzz half): decoders never panic on arbitrary input ─────

func FuzzDecodeConfig(f *testing.F) {
	f.Add([]byte{})
	f.Add(goldenConfig)
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = uart.DecodeConfig(b) // must not panic
	})
}

func FuzzDecodeReadResponse(f *testing.F) {
	f.Add([]byte{})
	f.Add(goldenReadResponse)
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _, _ = uart.DecodeReadResponse(b) // must not panic
	})
}

func FuzzDecodeWriteResponse(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0x00, 0x03})
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = uart.DecodeWriteResponse(b) // must not panic
	})
}
