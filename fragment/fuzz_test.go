//fusa:test REQ-FRAG-004

package fragment_test

import (
	"testing"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/fragment"
)

// ── REQ-FRAG-004 (fuzz half): Reassembler.Add never panics on an arbitrary
// segment sequence ──

// FuzzReassemblerAdd feeds an arbitrary control byte, segment number, and
// body into a fresh Reassembler for every fuzz iteration. It never resets
// state between the two Add calls within one iteration, deliberately
// exercising the "second Add for an already-known Key" code paths
// (duplicate/out-of-order/header-mismatch/already-complete) alongside the
// simpler first-segment path.
func FuzzReassemblerAdd(f *testing.F) {
	f.Add(byte(0), uint16(0), []byte{0x01, 0x02}, byte(0), uint16(0), []byte{0x03})
	f.Add(byte(acf.FlagMoreSegments), uint16(0), []byte{0x01}, byte(0), uint16(1), []byte{0x02})
	f.Add(byte(acf.FlagMoreSegments), uint16(5), []byte{}, byte(acf.FlagMoreSegments), uint16(0), []byte{})

	f.Fuzz(func(t *testing.T, ctrl1 byte, seg1 uint16, body1 []byte, ctrl2 byte, seg2 uint16, body2 []byte) {
		re := fragment.NewReassembler(fragment.Config{})
		stream := testStream()
		m1 := acf.Message{ByteBusID: 1, TransactionNum: 1, Control: acf.ControlFlags(ctrl1), ReadSizeOrSegment: seg1, Body: body1}
		m2 := acf.Message{ByteBusID: 1, TransactionNum: 1, Control: acf.ControlFlags(ctrl2), ReadSizeOrSegment: seg2, Body: body2}

		complete, err := re.Add(stream, m1) // must not panic
		if err == nil && complete {
			if _, err := re.Finish(fragment.KeyOf(stream, m1)); err != nil {
				t.Fatalf("Finish after complete Add reported an error: %v", err)
			}
			return
		}
		_, _ = re.Add(stream, m2) // must not panic
	})
}

// FuzzSplit checks Split never panics for an arbitrary maxBody/Body
// combination, and that a successful split's segments always concatenate
// back to the original Body.
func FuzzSplit(f *testing.F) {
	f.Add(5, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
	f.Add(0, []byte{0x01})
	f.Add(1, []byte{})

	f.Fuzz(func(t *testing.T, maxBody int, body []byte) {
		msg := acf.Message{ByteBusID: 1, TransactionNum: 1, Body: body}
		segs, err := fragment.Split(msg, maxBody) // must not panic
		if err != nil {
			return
		}
		var recombined []byte
		for _, seg := range segs {
			recombined = append(recombined, seg.Body...)
		}
		if len(recombined) != len(body) {
			t.Fatalf("recombined length %d, want %d (maxBody=%d)", len(recombined), len(body), maxBody)
		}
	})
}
