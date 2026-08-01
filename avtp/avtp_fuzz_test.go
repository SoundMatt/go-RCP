//fusa:test REQ-AVTP-016

package avtp_test

import (
	"testing"

	"github.com/SoundMatt/go-RCP/avtp"
)

// ── REQ-AVTP-016 (fuzz half, header-decoder share): decoders never panic ───
// on arbitrary input.
//
// FuzzDecodeMessage and FuzzDecodeFrame (originally alongside this fuzz
// target, and sharing its seed corpus) moved to the acf package's own fuzz
// test file when the message/frame layer split out of this package — see
// acf/doc.go and acf/acf_fuzz_test.go. REQ-AVTP-016 is jointly verified by
// FuzzDecodeHeader here and FuzzDecodeMessage/FuzzDecodeFrame there.

func FuzzDecodeHeader(f *testing.F) {
	f.Add([]byte{})
	// A header that declares a data_length far larger than what actually
	// follows — the classic out-of-bounds trigger a length-mismatch check
	// must guard against, mirroring wire's own fuzz seed for the same class
	// of bug (see wire_test.go's "huge" seed).
	huge, err := avtp.EncodeHeader(avtp.Header{StreamID: avtp.NewStreamID([6]byte{}, 0)})
	if err != nil {
		f.Fatalf("EncodeHeader: %v", err)
	}
	// ntscf_data_length is 11 bits straddling octets 1 and 2 (TC18 Figure
	// 6, bits 13-23), not a whole 16-bit word further along the header.
	huge[1] |= 0x07
	huge[2] = 0xFF // 0x7FF: max representable ntscf_data_length
	f.Add(huge)

	f.Fuzz(func(t *testing.T, b []byte) {
		_, _, _ = avtp.DecodeHeader(b) // must not panic
	})
}
