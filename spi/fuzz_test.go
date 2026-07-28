//fusa:test REQ-SPI-010

package spi_test

import (
	"testing"

	"github.com/SoundMatt/go-RCP/spi"
)

// ── REQ-SPI-010 (fuzz half): decoders never panic on arbitrary input ──────

func FuzzDecodeConfig(f *testing.F) {
	f.Add([]byte{})
	f.Add(goldenConfig)
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = spi.DecodeConfig(b) // must not panic
	})
}

func FuzzDecodeTransferRequest(f *testing.F) {
	f.Add([]byte{})
	f.Add(goldenTransferRequest)
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _, _ = spi.DecodeTransferRequest(b) // must not panic
	})
}
