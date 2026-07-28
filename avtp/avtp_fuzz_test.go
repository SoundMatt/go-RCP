//fusa:test REQ-AVTP-016

package avtp_test

import (
	"testing"

	"github.com/SoundMatt/go-RCP/avtp"
)

// ── REQ-AVTP-016 (fuzz half): decoders never panic on arbitrary input ──────

func seedFrames(f *testing.F) {
	f.Helper()
	f.Add([]byte{})
	f.Add(goldenUntimedShortRead)
	f.Add(goldenTimedLongWrite)

	// A header that declares a data_length far larger than what actually
	// follows — the classic out-of-bounds trigger ErrFrameLengthMismatch /
	// ErrShortMessage must guard against, mirroring wire's own fuzz seed
	// for the same class of bug (see wire_test.go's "huge" seed).
	huge, err := avtp.EncodeHeader(avtp.Header{StreamID: avtp.NewStreamID([6]byte{}, 0)})
	if err != nil {
		f.Fatalf("EncodeHeader: %v", err)
	}
	huge[3], huge[4] = 0x07, 0xFF // max representable 11-bit data_length
	f.Add(huge)
}

func FuzzDecodeHeader(f *testing.F) {
	seedFrames(f)
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _, _ = avtp.DecodeHeader(b) // must not panic
	})
}

func FuzzDecodeMessage(f *testing.F) {
	seedFrames(f)
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = avtp.DecodeMessage(b) // must not panic
	})
}

func FuzzDecodeFrame(f *testing.F) {
	seedFrames(f)
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = avtp.DecodeFrame(b) // must not panic
	})
}
