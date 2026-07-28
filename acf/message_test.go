//fusa:test REQ-AVTP-011
//fusa:test REQ-AVTP-012
//fusa:test REQ-AVTP-013
//fusa:test REQ-AVTP-014

package acf_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
)

// ── REQ-AVTP-011: short-form (ACF_ABB) message round-trip ──────────────────

func TestMessage_ShortRoundTrip(t *testing.T) {
	m := acf.Message{
		Kind:              acf.KindShort,
		ByteBusID:         avtp.ByteBusID(5),
		TransactionNum:    avtp.TransactionNum(99),
		Control:           acf.FlagRead | acf.FlagAck,
		ReadSizeOrSegment: 16,
		Body:              []byte("hello"),
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
	// Short encoding must never carry the 8-byte timestamp slot on the wire.
	if len(b) != 10+len(m.Body) {
		t.Errorf("encoded length = %d, want %d (no timestamp slot)", len(b), 10+len(m.Body))
	}
}

// ── REQ-AVTP-012: long-form (ACF_GBB) message round-trip ───────────────────

func TestMessage_LongRoundTrip(t *testing.T) {
	m := acf.Message{
		Kind:              acf.KindLong,
		ByteBusID:         avtp.ByteBusID(9),
		TransactionNum:    avtp.TransactionNum(1),
		Control:           acf.FlagWrite,
		ReadSizeOrSegment: 0,
		Timestamp:         0x0123456789ABCDEF,
		Body:              []byte{0x01, 0x02, 0x03},
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
	if len(b) != 10+8+len(m.Body) {
		t.Errorf("encoded length = %d, want %d (with timestamp slot)", len(b), 10+8+len(m.Body))
	}
}

func TestMessage_EmptyBodyRoundTrip(t *testing.T) {
	for _, kind := range []acf.MessageKind{acf.KindShort, acf.KindLong} {
		m := acf.Message{Kind: kind, Control: acf.FlagAck}
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

func TestMessage_PadBytesRoundTrip(t *testing.T) {
	m := acf.Message{
		Kind: acf.KindShort,
		Pad:  3,
		Body: []byte{0xAB},
	}
	b, err := acf.EncodeMessage(m)
	if err != nil {
		t.Fatalf("EncodeMessage: %v", err)
	}
	wantLen := 10 + len(m.Body) + int(m.Pad)
	if len(b) != wantLen {
		t.Fatalf("encoded length = %d, want %d", len(b), wantLen)
	}
	for i := wantLen - int(m.Pad); i < wantLen; i++ {
		if b[i] != 0 {
			t.Errorf("pad byte at %d = %#x, want 0", i, b[i])
		}
	}
	got, err := acf.DecodeMessage(b)
	if err != nil {
		t.Fatalf("DecodeMessage: %v", err)
	}
	if got.Pad != m.Pad || !reflect.DeepEqual(got.Body, m.Body) {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, m)
	}
}

func TestMessage_PadOverflow(t *testing.T) {
	if _, err := acf.EncodeMessage(acf.Message{Kind: acf.KindShort, Pad: 4}); !errors.Is(err, acf.ErrPadOverflow) {
		t.Errorf("EncodeMessage = %v, want ErrPadOverflow", err)
	}
}

// ── REQ-AVTP-013: control flags + dual-purpose field semantics ─────────────

func TestControlFlags_Combinations(t *testing.T) {
	all := []acf.ControlFlags{
		acf.FlagAck, acf.FlagRead, acf.FlagWrite,
		acf.FlagResponse, acf.FlagError, acf.FlagMoreSegments,
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
	// Bit 0x01 is FlagExtended (claimed by the request package, Milestone
	// 49) and no longer reserved; only 0x02 remains reserved. See
	// TestEncodeMessage_FlagExtendedRoundTrip below for the now-valid case.
	m := acf.Message{Kind: acf.KindShort, Control: acf.ControlFlags(0x02)}
	if _, err := acf.EncodeMessage(m); !errors.Is(err, avtp.ErrReservedBitsSet) {
		t.Errorf("EncodeMessage = %v, want ErrReservedBitsSet", err)
	}
}

// TestEncodeMessage_FlagExtendedRoundTrip checks FlagExtended encodes and
// decodes like any other known control bit, now that Milestone 49 has
// claimed it.
func TestEncodeMessage_FlagExtendedRoundTrip(t *testing.T) {
	m := acf.Message{Kind: acf.KindShort, Control: acf.FlagWrite | acf.FlagExtended, Body: []byte{0x01, 0x02}}
	b, err := acf.EncodeMessage(m)
	if err != nil {
		t.Fatalf("EncodeMessage: %v", err)
	}
	got, err := acf.DecodeMessage(b)
	if err != nil {
		t.Fatalf("DecodeMessage: %v", err)
	}
	if !got.Control.Has(acf.FlagExtended) {
		t.Errorf("decoded Control = %v, want FlagExtended set", got.Control)
	}
}

// ── REQ-AVTP-014: message decode rejects malformed input ───────────────────

func TestDecodeMessage_ShortBuffer(t *testing.T) {
	full, err := acf.EncodeMessage(acf.Message{Kind: acf.KindLong, Body: []byte("xy")})
	if err != nil {
		t.Fatalf("EncodeMessage: %v", err)
	}
	for n := 0; n < 10; n++ { // below the shared descriptor length
		if _, err := acf.DecodeMessage(full[:n]); !errors.Is(err, acf.ErrShortMessage) {
			t.Errorf("DecodeMessage(len %d) = %v, want ErrShortMessage", n, err)
		}
	}
}

func TestDecodeMessage_UnknownKind(t *testing.T) {
	b, err := acf.EncodeMessage(acf.Message{Kind: acf.KindShort})
	if err != nil {
		t.Fatalf("EncodeMessage: %v", err)
	}
	b[0] = 0xFF
	if _, err := acf.DecodeMessage(b); !errors.Is(err, acf.ErrUnknownMessageKind) {
		t.Errorf("DecodeMessage = %v, want ErrUnknownMessageKind", err)
	}
}

func TestDecodeMessage_TruncatedLongTimestamp(t *testing.T) {
	b, err := acf.EncodeMessage(acf.Message{Kind: acf.KindLong, Body: []byte("z")})
	if err != nil {
		t.Fatalf("EncodeMessage: %v", err)
	}
	// Cut into the middle of the 8-byte timestamp slot.
	truncated := b[:13]
	if _, err := acf.DecodeMessage(truncated); !errors.Is(err, acf.ErrShortMessage) {
		t.Errorf("DecodeMessage(truncated timestamp) = %v, want ErrShortMessage", err)
	}
}

func TestDecodeMessage_DeclaredLengthExceedsBuffer(t *testing.T) {
	b, err := acf.EncodeMessage(acf.Message{Kind: acf.KindShort, Body: []byte("abcdef")})
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
