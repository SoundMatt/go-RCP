//fusa:test REQ-AVTP-016

package acf_test

import (
	"testing"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
)

// ── REQ-AVTP-016 (fuzz half, message/frame-decoder share): decoders never
// panic on arbitrary input. See avtp/avtp_fuzz_test.go's FuzzDecodeHeader
// for the header-decoder share of this same requirement.

func seedFrames(f *testing.F) {
	f.Helper()
	f.Add([]byte{})
	f.Add(goldenUntimedShortRead)
	f.Add(goldenTimedLongWrite)

	// A header that declares a data_length far larger than what actually
	// follows — the classic out-of-bounds trigger a length-mismatch check
	// must guard against, mirroring wire's own fuzz seed for the same class
	// of bug (see wire_test.go's "huge" seed), and avtp/avtp_fuzz_test.go's
	// own copy of this same seed for FuzzDecodeHeader.
	huge, err := avtp.EncodeHeader(avtp.Header{StreamID: avtp.NewStreamID([6]byte{}, 0)})
	if err != nil {
		f.Fatalf("EncodeHeader: %v", err)
	}
	huge[3], huge[4] = 0x07, 0xFF // max representable 11-bit data_length
	f.Add(huge)
}

func FuzzDecodeMessage(f *testing.F) {
	seedFrames(f)
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = acf.DecodeMessage(b) // must not panic
	})
}

func FuzzDecodeFrame(f *testing.F) {
	seedFrames(f)
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = acf.DecodeFrame(b) // must not panic
	})
}
