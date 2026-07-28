//fusa:test REQ-FRAG-012

package fragment_test

import (
	"bytes"
	"testing"

	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/fragment"
)

// ── REQ-FRAG-012: frozen segment sequencing, end to end through avtp's own
// wire encoder ──
//
// These fixtures pin the exact wire bytes a representative multi-segment
// Split produces once each returned segment is itself encoded via
// avtp.EncodeMessage, so later work regression-tests against a frozen
// combination of this package's segmenting policy and avtp's already-frozen
// message encoding, the same posture can/golden_test.go and
// uart/golden_test.go established for their own packages.

// goldenOriginal is a 12-byte KindShort write request split at a 5-byte
// per-segment budget: two non-terminal 5-byte segments plus a 2-byte
// terminal one.
var goldenOriginal = avtp.Message{
	Kind:              avtp.KindShort,
	ByteBusID:         avtp.ByteBusID(9),
	TransactionNum:    0x0102,
	Control:           avtp.FlagWrite,
	ReadSizeOrSegment: 0,
	Body:              []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11},
}

// Segment 0: kind(1)=0x01, pad/reserved(1)=0x00, length(2)=0x000F (10
// descriptor + 5 body), byte_bus_id(1)=0x09, transaction_num(2)=0x0102,
// control(1)=FlagWrite|FlagMoreSegments=0x20|0x04=0x24, read-size-or-
// segment(2)=0x0000 (segment number 0), body=[0,1,2,3,4].
var goldenSegment0 = []byte{
	0x01, 0x00, 0x00, 0x0F, 0x09, 0x01, 0x02, 0x24, 0x00, 0x00,
	0x00, 0x01, 0x02, 0x03, 0x04,
}

// Segment 1: identical shape, read-size-or-segment=0x0001, body=[5,6,7,8,9].
var goldenSegment1 = []byte{
	0x01, 0x00, 0x00, 0x0F, 0x09, 0x01, 0x02, 0x24, 0x00, 0x01,
	0x05, 0x06, 0x07, 0x08, 0x09,
}

// Segment 2 (terminal): length(2)=0x000C (10 descriptor + 2 body),
// control(1)=FlagWrite only=0x20 (FlagMoreSegments clear), read-size-or-
// segment(2) restored to the original message's own value (0x0000),
// body=[10,11].
var goldenSegment2 = []byte{
	0x01, 0x00, 0x00, 0x0C, 0x09, 0x01, 0x02, 0x20, 0x00, 0x00,
	0x0A, 0x0B,
}

func TestGolden_Split(t *testing.T) {
	segs, err := fragment.Split(goldenOriginal, 5)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	want := [][]byte{goldenSegment0, goldenSegment1, goldenSegment2}
	if len(segs) != len(want) {
		t.Fatalf("Split produced %d segments, want %d", len(segs), len(want))
	}
	for i, seg := range segs {
		got, err := avtp.EncodeMessage(seg)
		if err != nil {
			t.Fatalf("EncodeMessage(segment %d): %v", i, err)
		}
		if !bytes.Equal(got, want[i]) {
			t.Errorf("segment %d encoded = % X, want % X", i, got, want[i])
		}
	}
}

func TestGolden_ReassembleFromEncodedSegments(t *testing.T) {
	stream := testStream()
	re := fragment.NewReassembler(fragment.Config{})
	for i, raw := range [][]byte{goldenSegment0, goldenSegment1, goldenSegment2} {
		msg, err := avtp.DecodeMessage(raw)
		if err != nil {
			t.Fatalf("DecodeMessage(golden segment %d): %v", i, err)
		}
		if _, err := re.Add(stream, msg); err != nil {
			t.Fatalf("Add(golden segment %d): %v", i, err)
		}
	}
	out, err := re.Finish(fragment.KeyOf(stream, goldenOriginal))
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if !bytes.Equal(out.Body, goldenOriginal.Body) {
		t.Errorf("reassembled body = % X, want % X", out.Body, goldenOriginal.Body)
	}
}
