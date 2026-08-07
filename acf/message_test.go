//fusa:test REQ-AVTP-011
//fusa:test REQ-AVTP-012
//fusa:test REQ-AVTP-013
//fusa:test REQ-AVTP-014

package acf_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
)

// ── REQ-AVTP-011: short-form (ACF_ABB) message round-trip ──────────────────

func TestMessage_ShortRoundTrip(t *testing.T) {
	m := acf.Message{
		Kind:              acf.KindShort,
		ByteBusID:         avtp.ByteBusID(5),
		TransactionNum:    avtp.TransactionNum(99),
		Control:           acf.FlagRead,
		EVT:               0x08,
		HS:                true,
		CS:                true,
		ReadSizeOrSegment: 16,
		Body:              []byte("halo"), // already quadlet-aligned with the header; see TestMessage_PadIsComputedNotTrusted for the padding case
	}
	b, err := acf.EncodeMessage(m)
	if err != nil {
		t.Fatalf("EncodeMessage: %v", err)
	}
	got, err := acf.DecodeMessage(b)
	if err != nil {
		t.Fatalf("DecodeMessage: %v", err)
	}
	if !reflect.DeepEqual(got, m) {
		t.Errorf("round-trip mismatch:\n got  %+v\n want %+v", got, m)
	}
	// Short encoding must never carry the 8-byte message_timestamp slot on
	// the wire: 4-byte row1 + 4-byte row2 + Body.
	if len(b) != 8+len(m.Body) {
		t.Errorf("encoded length = %d, want %d (no message_timestamp slot)", len(b), 8+len(m.Body))
	}
}

// ── REQ-AVTP-012: long-form (ACF_GBB) message round-trip ───────────────────

func TestMessage_LongRoundTrip(t *testing.T) {
	m := acf.Message{
		Kind:              acf.KindLong,
		ByteBusID:         avtp.ByteBusID(9),
		TransactionNum:    avtp.TransactionNum(1),
		Control:           acf.FlagWrite,
		MTV:               true,
		ReadSizeOrSegment: 0,
		Timestamp:         0x0123456789ABCDEF,
		Body:              []byte{0x01, 0x02, 0x03, 0x04}, // already quadlet-aligned with the header
	}
	b, err := acf.EncodeMessage(m)
	if err != nil {
		t.Fatalf("EncodeMessage: %v", err)
	}
	got, err := acf.DecodeMessage(b)
	if err != nil {
		t.Fatalf("DecodeMessage: %v", err)
	}
	if !reflect.DeepEqual(got, m) {
		t.Errorf("round-trip mismatch:\n got  %+v\n want %+v", got, m)
	}
	// Long encoding: 4-byte row1 + 8-byte message_timestamp slot (inserted
	// between row1 and row2, not appended after both) + 4-byte row2 + Body.
	if len(b) != 4+8+4+len(m.Body) {
		t.Errorf("encoded length = %d, want %d (with message_timestamp slot)", len(b), 4+8+4+len(m.Body))
	}
	// The message_timestamp slot must sit immediately after row1 (byte
	// offset 4), before row2 (evt/transaction_num/op-rsp-err-ms/read_size).
	if got, want := b[4:12], []byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF}; !reflect.DeepEqual(got, want) {
		t.Errorf("message_timestamp slot = % X, want % X", got, want)
	}
}

func TestMessage_EmptyBodyRoundTrip(t *testing.T) {
	for _, kind := range []acf.MessageKind{acf.KindShort, acf.KindLong} {
		m := acf.Message{Kind: kind, Control: acf.FlagRead, EVT: 0x08}
		b, err := acf.EncodeMessage(m)
		if err != nil {
			t.Fatalf("EncodeMessage(kind %v): %v", kind, err)
		}
		got, err := acf.DecodeMessage(b)
		if err != nil {
			t.Fatalf("DecodeMessage(kind %v): %v", kind, err)
		}
		if len(got.Body) != 0 {
			t.Errorf("kind %v: Body = %v, want empty", kind, got.Body)
		}
	}
}

// TestMessage_PadIsComputedNotTrusted checks EncodeMessage computes Pad
// itself from Body's length — the one correct value that brings the
// encoded message to a quadlet boundary — rather than trusting whatever a
// caller happened to set. This matters because virtually every existing
// call site across this repo constructs a Message without ever setting Pad
// (its zero value), and most endpoint payload lengths are not already a
// multiple of 4 bytes; requiring callers to compute the exactly-correct
// value themselves would have broken all of them.
func TestMessage_PadIsComputedNotTrusted(t *testing.T) {
	for _, tc := range []struct {
		name      string
		body      []byte
		callerPad uint8 // deliberately wrong/ignored input
		wantPad   uint8
		wantLen   int
	}{
		{"already aligned (empty body)", nil, 0, 0, 8},
		{"needs 3 bytes of pad", []byte{0x01}, 0, 3, 12},
		{"needs 2 bytes of pad", []byte{0x01, 0x02}, 1, 2, 12},
		{"needs 1 byte of pad", []byte{0x01, 0x02, 0x03}, 3, 1, 12},
		{"already aligned (4-byte body)", []byte{0x01, 0x02, 0x03, 0x04}, 2, 0, 12},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := acf.Message{Kind: acf.KindShort, Control: acf.FlagRead, Pad: tc.callerPad, Body: tc.body}
			b, err := acf.EncodeMessage(m)
			if err != nil {
				t.Fatalf("EncodeMessage: %v", err)
			}
			if len(b) != tc.wantLen {
				t.Fatalf("encoded length = %d, want %d", len(b), tc.wantLen)
			}
			for i := len(b) - int(tc.wantPad); i < len(b); i++ {
				if b[i] != 0 {
					t.Errorf("pad byte at %d = %#x, want 0", i, b[i])
				}
			}
			got, err := acf.DecodeMessage(b)
			if err != nil {
				t.Fatalf("DecodeMessage: %v", err)
			}
			if got.Pad != tc.wantPad {
				t.Errorf("decoded Pad = %d, want %d (caller-supplied Pad %d must be ignored on encode)", got.Pad, tc.wantPad, tc.callerPad)
			}
			if !reflect.DeepEqual(got.Body, tc.body) && (len(got.Body) != 0 || len(tc.body) != 0) {
				t.Errorf("decoded Body = % X, want % X", got.Body, tc.body)
			}
		})
	}
}

// ── REQ-AVTP-013: control flags + dual-purpose field semantics ─────────────

func TestControlFlags_Combinations(t *testing.T) {
	all := []acf.ControlFlags{
		acf.FlagWrite, acf.FlagResponse, acf.FlagError, acf.FlagMoreSegments,
	}
	var combo acf.ControlFlags
	for _, f := range all {
		combo |= f
	}
	m := acf.Message{Kind: acf.KindShort, Control: combo, ReadSizeOrSegment: 7}
	b, err := acf.EncodeMessage(m)
	if err != nil {
		t.Fatalf("EncodeMessage: %v", err)
	}
	got, err := acf.DecodeMessage(b)
	if err != nil {
		t.Fatalf("DecodeMessage: %v", err)
	}
	for _, f := range all {
		if !got.Control.Has(f) {
			t.Errorf("flag %#x lost across round-trip", f)
		}
	}
}

func TestMessage_DualPurposeField(t *testing.T) {
	read := acf.Message{Control: acf.FlagRead, ReadSizeOrSegment: 64}
	if n, ok := read.ReadSize(); !ok || n != 64 {
		t.Errorf("ReadSize() = (%d, %v), want (64, true)", n, ok)
	}
	if _, ok := read.SegmentNumber(); ok {
		t.Error("SegmentNumber() ok = true for a plain read, want false")
	}

	seg := acf.Message{Control: acf.FlagRead | acf.FlagMoreSegments, ReadSizeOrSegment: 3}
	if n, ok := seg.SegmentNumber(); !ok || n != 3 {
		t.Errorf("SegmentNumber() = (%d, %v), want (3, true)", n, ok)
	}
	if _, ok := seg.ReadSize(); ok {
		t.Error("ReadSize() ok = true once MoreSegments is set, want false")
	}
}

func TestEncodeMessage_ReservedControlBits(t *testing.T) {
	m := acf.Message{Kind: acf.KindShort, Control: acf.ControlFlags(0x01)}
	if _, err := acf.EncodeMessage(m); !errors.Is(err, avtp.ErrReservedBitsSet) {
		t.Errorf("EncodeMessage = %v, want ErrReservedBitsSet", err)
	}
}

func TestEncodeMessage_FieldRangeValidation(t *testing.T) {
	base := acf.Message{Kind: acf.KindShort, Control: acf.FlagRead}

	tooBigEVT := base
	tooBigEVT.EVT = 0x10
	if _, err := acf.EncodeMessage(tooBigEVT); !errors.Is(err, acf.ErrEVTOverflow) {
		t.Errorf("EncodeMessage(EVT overflow) = %v, want ErrEVTOverflow", err)
	}

	tooBigTxn := base
	tooBigTxn.TransactionNum = 256
	if _, err := acf.EncodeMessage(tooBigTxn); !errors.Is(err, acf.ErrTransactionNumOverflow) {
		t.Errorf("EncodeMessage(TransactionNum overflow) = %v, want ErrTransactionNumOverflow", err)
	}

	tooBigReadSize := base
	tooBigReadSize.ReadSizeOrSegment = 4096
	if _, err := acf.EncodeMessage(tooBigReadSize); !errors.Is(err, acf.ErrReadSizeOverflow) {
		t.Errorf("EncodeMessage(ReadSizeOrSegment overflow) = %v, want ErrReadSizeOverflow", err)
	}
}

// ── REQ-AVTP-014: message decode rejects malformed input ───────────────────

func TestDecodeMessage_ShortBuffer(t *testing.T) {
	full, err := acf.EncodeMessage(acf.Message{Kind: acf.KindLong, Control: acf.FlagRead, Body: []byte("xy")})
	if err != nil {
		t.Fatalf("EncodeMessage: %v", err)
	}
	for n := 0; n < 4; n++ { // below row1's own length
		if _, err := acf.DecodeMessage(full[:n]); !errors.Is(err, acf.ErrShortMessage) {
			t.Errorf("DecodeMessage(len %d) = %v, want ErrShortMessage", n, err)
		}
	}
}

func TestDecodeMessage_UnknownKind(t *testing.T) {
	b, err := acf.EncodeMessage(acf.Message{Kind: acf.KindShort, Control: acf.FlagRead})
	if err != nil {
		t.Fatalf("EncodeMessage: %v", err)
	}
	b[0] = 0xFF
	if _, err := acf.DecodeMessage(b); !errors.Is(err, acf.ErrUnknownMessageKind) {
		t.Errorf("DecodeMessage = %v, want ErrUnknownMessageKind", err)
	}
}

func TestDecodeMessage_TruncatedLongTimestamp(t *testing.T) {
	b, err := acf.EncodeMessage(acf.Message{Kind: acf.KindLong, Control: acf.FlagRead, Body: []byte("z")})
	if err != nil {
		t.Fatalf("EncodeMessage: %v", err)
	}
	// Cut into the middle of the 8-byte message_timestamp slot (starts at
	// offset 4 for KindLong).
	truncated := b[:9]
	if _, err := acf.DecodeMessage(truncated); !errors.Is(err, acf.ErrShortMessage) {
		t.Errorf("DecodeMessage(truncated timestamp) = %v, want ErrShortMessage", err)
	}
}

func TestDecodeMessage_DeclaredLengthExceedsBuffer(t *testing.T) {
	b, err := acf.EncodeMessage(acf.Message{Kind: acf.KindShort, Control: acf.FlagRead, Body: []byte("abcdef")})
	if err != nil {
		t.Fatalf("EncodeMessage: %v", err)
	}
	if _, err := acf.DecodeMessage(b[:len(b)-1]); !errors.Is(err, acf.ErrShortMessage) {
		t.Errorf("DecodeMessage(short by one) = %v, want ErrShortMessage", err)
	}
}

func TestUnknownMessageKind_EncodeRejected(t *testing.T) {
	if _, err := acf.EncodeMessage(acf.Message{Kind: acf.MessageKind(0)}); !errors.Is(err, acf.ErrUnknownMessageKind) {
		t.Errorf("EncodeMessage = %v, want ErrUnknownMessageKind", err)
	}
}
