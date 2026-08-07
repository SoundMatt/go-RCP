//fusa:test REQ-FRAG-012

package fragment_test

import (
	"bytes"
	"testing"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/fragment"
)

// ── REQ-FRAG-012: frozen segment sequencing, end to end through avtp's own
// wire encoder ──
//
// These fixtures pin the exact wire bytes a representative multi-segment
// Split produces once each returned segment is itself encoded via
// acf.EncodeMessage, so later work regression-tests against a frozen
// combination of this package's segmenting policy and avtp's already-frozen
// message encoding, the same posture can/golden_test.go and
// uart/golden_test.go established for their own packages.

// goldenOriginal is a 12-byte KindShort write request split at a 5-byte
// per-segment budget: two non-terminal 5-byte segments plus a 2-byte
// terminal one. TransactionNum is 0x42 — the acf wire field is only 8 bits
// wide (see acf.ErrTransactionNumOverflow), unlike the pre-v2.0 16-bit
// field this fixture originally exercised.
var goldenOriginal = acf.Message{
	Kind:              acf.KindShort,
	ByteBusID:         avtp.ByteBusID(9),
	TransactionNum:    0x42,
	Control:           acf.FlagWrite,
	ReadSizeOrSegment: 0,
	Body:              []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11},
}

// Segment 0: byte0 acf_msg_type=0x0E<<1|len-top-bit=0x1C, byte1
// acf_msg_length(quadlets)=0x04 (16 bytes = 4 quadlets: 4-byte row1 + 4-byte
// row2 + 5-byte body + 3-byte pad), byte2 pad=3<<6|mtv=0|rsv=0|
// byte_bus_id-top3=0=0xC0, byte3 byte_bus_id-low8=0x09, byte4
// evt=0|hs=0|cs=0=0x00, byte5 transaction_num=0x42, byte6
// op=1(write)|rsp=0|err=0|ms=1(more segments)|read_size-top4=0=0x90, byte7
// read_size-low8=0x00 (segment number 0), bytes8-12 body=[0,1,2,3,4],
// bytes13-15 pad=0,0,0.
var goldenSegment0 = []byte{
	0x1C, 0x04, 0xC0, 0x09, 0x00, 0x42, 0x90, 0x00,
	0x00, 0x01, 0x02, 0x03, 0x04, 0x00, 0x00, 0x00,
}

// Segment 1: identical shape to segment 0, byte7 read_size-low8=0x01
// (segment number 1), body=[5,6,7,8,9].
var goldenSegment1 = []byte{
	0x1C, 0x04, 0xC0, 0x09, 0x00, 0x42, 0x90, 0x01,
	0x05, 0x06, 0x07, 0x08, 0x09, 0x00, 0x00, 0x00,
}

// Segment 2 (terminal): byte1 acf_msg_length(quadlets)=0x03 (12 bytes = 3
// quadlets: 4-byte row1 + 4-byte row2 + 2-byte body + 2-byte pad), byte2
// pad=2<<6|mtv=0|rsv=0|byte_bus_id-top3=0=0x80, byte6
// op=1(write)|rsp=0|err=0|ms=0(FlagMoreSegments clear)|read_size-top4=0=0x80,
// byte7 read_size-low8=0x00 (the original message's own read_size/segment
// value, restored on the terminal segment), body=[10,11], pad=0,0.
var goldenSegment2 = []byte{
	0x1C, 0x03, 0x80, 0x09, 0x00, 0x42, 0x80, 0x00,
	0x0A, 0x0B, 0x00, 0x00,
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
		got, err := acf.EncodeMessage(seg)
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
		msg, err := acf.DecodeMessage(raw)
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
