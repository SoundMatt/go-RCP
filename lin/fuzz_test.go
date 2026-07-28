//fusa:test REQ-LINEP-007

package lin_test

import (
	"testing"

	"github.com/SoundMatt/go-RCP/lin"
)

// ── REQ-LINEP-007 (fuzz half): decoders never panic on arbitrary input ────

func FuzzDecodeConfig(f *testing.F) {
	f.Add([]byte{})
	f.Add(goldenConfig)
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = lin.DecodeConfig(b) // must not panic
	})
}

func FuzzDecodeTransferRequest(f *testing.F) {
	f.Add([]byte{})
	f.Add(goldenTransferRequest)
	f.Fuzz(func(t *testing.T, b []byte) {
		_ = lin.DecodeTransferRequest(b) // must not panic
	})
}
