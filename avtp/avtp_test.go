//fusa:test REQ-AVTP-001
//fusa:test REQ-AVTP-002
//fusa:test REQ-AVTP-003
//fusa:test REQ-AVTP-004
//fusa:test REQ-AVTP-005
//fusa:test REQ-AVTP-006
//fusa:test REQ-AVTP-007
//fusa:test REQ-AVTP-008
//fusa:test REQ-AVTP-009
//fusa:test REQ-AVTP-010
//fusa:test REQ-AVTP-011
//fusa:test REQ-AVTP-012
//fusa:test REQ-AVTP-013
//fusa:test REQ-AVTP-014
//fusa:test REQ-AVTP-015

package avtp_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/SoundMatt/go-RCP/avtp"
)

func testStreamID() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55}, 0x1234)
}

// ── REQ-AVTP-001: untimed (NTSCF) header round-trip ────────────────────────

func TestHeader_UntimedRoundTrip(t *testing.T) {
	h := avtp.Header{
		Timed:         false,
		StreamIDValid: true,
		SequenceNum:   7,
		DataLength:    42,
		StreamID:      testStreamID(),
	}
	b, err := avtp.EncodeHeader(h)
	if err != nil {
		t.Fatalf("EncodeHeader: %v", err)
	}
	got, rest, err := avtp.DecodeHeader(b)
	if err != nil {
		t.Fatalf("DecodeHeader: %v", err)
	}
	if len(rest) != 0 {
		t.Errorf("rest = %d bytes, want 0", len(rest))
	}
	if !reflect.DeepEqual(got, h) {
		t.Errorf("round-trip mismatch:\n got  %+v\n want %+v", got, h)
	}
}

// ── REQ-AVTP-002: timestamped (TSCF) header round-trip ─────────────────────

func TestHeader_TimedRoundTrip(t *testing.T) {
	h := avtp.Header{
		Timed:           true,
		StreamIDValid:   true,
		SequenceNum:     200,
		DataLength:      1000,
		StreamID:        testStreamID(),
		Timestamp:       0xDEADBEEF,
		TimestampStatus: avtp.TimestampValid,
	}
	b, err := avtp.EncodeHeader(h)
	if err != nil {
		t.Fatalf("EncodeHeader: %v", err)
	}
	got, rest, err := avtp.DecodeHeader(b)
	if err != nil {
		t.Fatalf("DecodeHeader: %v", err)
	}
	if len(rest) != 0 {
		t.Errorf("rest = %d bytes, want 0", len(rest))
	}
	if !reflect.DeepEqual(got, h) {
		t.Errorf("round-trip mismatch:\n got  %+v\n want %+v", got, h)
	}
}

func TestHeader_DataLengthOverflow(t *testing.T) {
	h := avtp.Header{DataLength: 0x0800} // one past the 11-bit max
	if _, err := avtp.EncodeHeader(h); !errors.Is(err, avtp.ErrDataLengthOverflow) {
		t.Errorf("EncodeHeader = %v, want ErrDataLengthOverflow", err)
	}
}

// ── REQ-AVTP-003: header decode rejects short buffers ──────────────────────

func TestDecodeHeader_ShortBuffer(t *testing.T) {
	full, err := avtp.EncodeHeader(avtp.Header{Timed: true, StreamID: testStreamID()})
	if err != nil {
		t.Fatalf("EncodeHeader: %v", err)
	}
	for n := 0; n < len(full); n++ {
		if _, _, err := avtp.DecodeHeader(full[:n]); !errors.Is(err, avtp.ErrShortHeader) {
			t.Errorf("DecodeHeader(len %d) = %v, want ErrShortHeader", n, err)
		}
	}
}

func TestDecodeHeader_UnknownSubtype(t *testing.T) {
	b := make([]byte, 20)
	b[0] = 0x00
	if _, _, err := avtp.DecodeHeader(b); !errors.Is(err, avtp.ErrUnknownSubtype) {
		t.Errorf("DecodeHeader = %v, want ErrUnknownSubtype", err)
	}
}

// ── REQ-AVTP-004: header decode rejects unsupported version ────────────────

func TestDecodeHeader_UnsupportedVersion(t *testing.T) {
	b, err := avtp.EncodeHeader(avtp.Header{StreamID: testStreamID()})
	if err != nil {
		t.Fatalf("EncodeHeader: %v", err)
	}
	b[1] |= 0x10 // set a nonzero version into bits 6-4
	if _, _, err := avtp.DecodeHeader(b); !errors.Is(err, avtp.ErrUnsupportedVersion) {
		t.Errorf("DecodeHeader = %v, want ErrUnsupportedVersion", err)
	}
}

// ── REQ-AVTP-005: header decode rejects reserved bits set ──────────────────

func TestDecodeHeader_ReservedBitsSet(t *testing.T) {
	untimed, err := avtp.EncodeHeader(avtp.Header{StreamID: testStreamID()})
	if err != nil {
		t.Fatalf("EncodeHeader: %v", err)
	}
	untimed[1] |= 0x01
	if _, _, decErr := avtp.DecodeHeader(untimed); !errors.Is(decErr, avtp.ErrReservedBitsSet) {
		t.Errorf("untimed reserved bit: DecodeHeader = %v, want ErrReservedBitsSet", decErr)
	}

	timed, err := avtp.EncodeHeader(avtp.Header{Timed: true, StreamID: testStreamID()})
	if err != nil {
		t.Fatalf("EncodeHeader: %v", err)
	}
	timed[1] |= 0x01
	if _, _, err := avtp.DecodeHeader(timed); !errors.Is(err, avtp.ErrReservedBitsSet) {
		t.Errorf("timed reserved bit: DecodeHeader = %v, want ErrReservedBitsSet", err)
	}
}

// ── REQ-AVTP-006: StreamID round-trips MAC + locally-assigned suffix ───────

func TestStreamID_MACAndSuffix(t *testing.T) {
	mac := [6]byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF}
	id := avtp.NewStreamID(mac, 0xBEEF)
	if got := id.MAC(); got != mac {
		t.Errorf("MAC() = %x, want %x", got, mac)
	}
	if got := id.Suffix(); got != 0xBEEF {
		t.Errorf("Suffix() = %#x, want 0xBEEF", got)
	}
	if s := id.String(); s == "" {
		t.Error("String() returned empty string")
	}
}

// ── REQ-AVTP-007/008/009/010: timestamp Disposition rules ──────────────────

func TestHeader_Disposition(t *testing.T) {
	tests := []struct {
		name              string
		timed             bool
		status            avtp.TimestampStatus
		timeSyncSupported bool
		want              avtp.Disposition
	}{
		{"untimed, sync supported", false, avtp.TimestampValid, true, avtp.DispositionBestEffort},
		{"untimed, no sync support", false, avtp.TimestampMissing, false, avtp.DispositionBestEffort},
		{"timed valid, sync supported", true, avtp.TimestampValid, true, avtp.DispositionScheduled},
		{"timed missing, sync supported", true, avtp.TimestampMissing, true, avtp.DispositionBestEffort},
		{"timed invalid, sync supported", true, avtp.TimestampInvalid, true, avtp.DispositionBestEffort},
		{"timed uncertain, sync supported", true, avtp.TimestampUncertain, true, avtp.DispositionBestEffort},
		{"timed valid, no sync support", true, avtp.TimestampValid, false, avtp.DispositionDrop},
		{"timed invalid, no sync support", true, avtp.TimestampInvalid, false, avtp.DispositionDrop},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := avtp.Header{Timed: tt.timed, TimestampStatus: tt.status}
			if got := h.Disposition(tt.timeSyncSupported); got != tt.want {
				t.Errorf("Disposition(%v) = %v, want %v", tt.timeSyncSupported, got, tt.want)
			}
		})
	}
}

// ── REQ-AVTP-011: short-form (ACF_ABB) message round-trip ──────────────────

func TestMessage_ShortRoundTrip(t *testing.T) {
	m := avtp.Message{
		Kind:              avtp.KindShort,
		ByteBusID:         avtp.ByteBusID(5),
		TransactionNum:    avtp.TransactionNum(99),
		Control:           avtp.FlagRead | avtp.FlagAck,
		ReadSizeOrSegment: 16,
		Body:              []byte("hello"),
	}
	b, err := avtp.EncodeMessage(m)
	if err != nil {
		t.Fatalf("EncodeMessage: %v", err)
	}
	got, err := avtp.DecodeMessage(b)
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
	m := avtp.Message{
		Kind:              avtp.KindLong,
		ByteBusID:         avtp.ByteBusID(9),
		TransactionNum:    avtp.TransactionNum(1),
		Control:           avtp.FlagWrite,
		ReadSizeOrSegment: 0,
		Timestamp:         0x0123456789ABCDEF,
		Body:              []byte{0x01, 0x02, 0x03},
	}
	b, err := avtp.EncodeMessage(m)
	if err != nil {
		t.Fatalf("EncodeMessage: %v", err)
	}
	got, err := avtp.DecodeMessage(b)
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
	for _, kind := range []avtp.MessageKind{avtp.KindShort, avtp.KindLong} {
		m := avtp.Message{Kind: kind, Control: avtp.FlagAck}
		b, err := avtp.EncodeMessage(m)
		if err != nil {
			t.Fatalf("EncodeMessage(kind %v): %v", kind, err)
		}
		got, err := avtp.DecodeMessage(b)
		if err != nil {
			t.Fatalf("DecodeMessage(kind %v): %v", kind, err)
		}
		if len(got.Body) != 0 {
			t.Errorf("kind %v: Body = %v, want empty", kind, got.Body)
		}
	}
}

func TestMessage_PadBytesRoundTrip(t *testing.T) {
	m := avtp.Message{
		Kind: avtp.KindShort,
		Pad:  3,
		Body: []byte{0xAB},
	}
	b, err := avtp.EncodeMessage(m)
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
	got, err := avtp.DecodeMessage(b)
	if err != nil {
		t.Fatalf("DecodeMessage: %v", err)
	}
	if got.Pad != m.Pad || !reflect.DeepEqual(got.Body, m.Body) {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, m)
	}
}

func TestMessage_PadOverflow(t *testing.T) {
	if _, err := avtp.EncodeMessage(avtp.Message{Kind: avtp.KindShort, Pad: 4}); !errors.Is(err, avtp.ErrPadOverflow) {
		t.Errorf("EncodeMessage = %v, want ErrPadOverflow", err)
	}
}

// ── REQ-AVTP-013: control flags + dual-purpose field semantics ─────────────

func TestControlFlags_Combinations(t *testing.T) {
	all := []avtp.ControlFlags{
		avtp.FlagAck, avtp.FlagRead, avtp.FlagWrite,
		avtp.FlagResponse, avtp.FlagError, avtp.FlagMoreSegments,
	}
	var combo avtp.ControlFlags
	for _, f := range all {
		combo |= f
	}
	m := avtp.Message{Kind: avtp.KindShort, Control: combo, ReadSizeOrSegment: 7}
	b, err := avtp.EncodeMessage(m)
	if err != nil {
		t.Fatalf("EncodeMessage: %v", err)
	}
	got, err := avtp.DecodeMessage(b)
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
	read := avtp.Message{Control: avtp.FlagRead, ReadSizeOrSegment: 64}
	if n, ok := read.ReadSize(); !ok || n != 64 {
		t.Errorf("ReadSize() = (%d, %v), want (64, true)", n, ok)
	}
	if _, ok := read.SegmentNumber(); ok {
		t.Error("SegmentNumber() ok = true for a plain read, want false")
	}

	seg := avtp.Message{Control: avtp.FlagRead | avtp.FlagMoreSegments, ReadSizeOrSegment: 3}
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
	m := avtp.Message{Kind: avtp.KindShort, Control: avtp.ControlFlags(0x02)}
	if _, err := avtp.EncodeMessage(m); !errors.Is(err, avtp.ErrReservedBitsSet) {
		t.Errorf("EncodeMessage = %v, want ErrReservedBitsSet", err)
	}
}

// TestEncodeMessage_FlagExtendedRoundTrip checks FlagExtended encodes and
// decodes like any other known control bit, now that Milestone 49 has
// claimed it.
func TestEncodeMessage_FlagExtendedRoundTrip(t *testing.T) {
	m := avtp.Message{Kind: avtp.KindShort, Control: avtp.FlagWrite | avtp.FlagExtended, Body: []byte{0x01, 0x02}}
	b, err := avtp.EncodeMessage(m)
	if err != nil {
		t.Fatalf("EncodeMessage: %v", err)
	}
	got, err := avtp.DecodeMessage(b)
	if err != nil {
		t.Fatalf("DecodeMessage: %v", err)
	}
	if !got.Control.Has(avtp.FlagExtended) {
		t.Errorf("decoded Control = %v, want FlagExtended set", got.Control)
	}
}

// ── REQ-AVTP-014: message decode rejects malformed input ───────────────────

func TestDecodeMessage_ShortBuffer(t *testing.T) {
	full, err := avtp.EncodeMessage(avtp.Message{Kind: avtp.KindLong, Body: []byte("xy")})
	if err != nil {
		t.Fatalf("EncodeMessage: %v", err)
	}
	for n := 0; n < 10; n++ { // below the shared descriptor length
		if _, err := avtp.DecodeMessage(full[:n]); !errors.Is(err, avtp.ErrShortMessage) {
			t.Errorf("DecodeMessage(len %d) = %v, want ErrShortMessage", n, err)
		}
	}
}

func TestDecodeMessage_UnknownKind(t *testing.T) {
	b, err := avtp.EncodeMessage(avtp.Message{Kind: avtp.KindShort})
	if err != nil {
		t.Fatalf("EncodeMessage: %v", err)
	}
	b[0] = 0xFF
	if _, err := avtp.DecodeMessage(b); !errors.Is(err, avtp.ErrUnknownMessageKind) {
		t.Errorf("DecodeMessage = %v, want ErrUnknownMessageKind", err)
	}
}

func TestDecodeMessage_TruncatedLongTimestamp(t *testing.T) {
	b, err := avtp.EncodeMessage(avtp.Message{Kind: avtp.KindLong, Body: []byte("z")})
	if err != nil {
		t.Fatalf("EncodeMessage: %v", err)
	}
	// Cut into the middle of the 8-byte timestamp slot.
	truncated := b[:13]
	if _, err := avtp.DecodeMessage(truncated); !errors.Is(err, avtp.ErrShortMessage) {
		t.Errorf("DecodeMessage(truncated timestamp) = %v, want ErrShortMessage", err)
	}
}

func TestDecodeMessage_DeclaredLengthExceedsBuffer(t *testing.T) {
	b, err := avtp.EncodeMessage(avtp.Message{Kind: avtp.KindShort, Body: []byte("abcdef")})
	if err != nil {
		t.Fatalf("EncodeMessage: %v", err)
	}
	if _, err := avtp.DecodeMessage(b[:len(b)-1]); !errors.Is(err, avtp.ErrShortMessage) {
		t.Errorf("DecodeMessage(short by one) = %v, want ErrShortMessage", err)
	}
}

func TestUnknownMessageKind_EncodeRejected(t *testing.T) {
	if _, err := avtp.EncodeMessage(avtp.Message{Kind: avtp.MessageKind(0)}); !errors.Is(err, avtp.ErrUnknownMessageKind) {
		t.Errorf("EncodeMessage = %v, want ErrUnknownMessageKind", err)
	}
}

// ── REQ-AVTP-015: full Frame round-trip across header × message-kind ───────

func TestFrame_RoundTrip(t *testing.T) {
	for _, timed := range []bool{false, true} {
		for _, kind := range []avtp.MessageKind{avtp.KindShort, avtp.KindLong} {
			hdr := avtp.Header{
				Timed:           timed,
				StreamIDValid:   true,
				SequenceNum:     3,
				StreamID:        testStreamID(),
				Timestamp:       0x11223344,
				TimestampStatus: avtp.TimestampValid,
			}
			msg := avtp.Message{
				Kind:              kind,
				ByteBusID:         avtp.ByteBusID(2),
				TransactionNum:    avtp.TransactionNum(55),
				Control:           avtp.FlagWrite | avtp.FlagAck,
				ReadSizeOrSegment: 0,
				Timestamp:         0xFEEDFACECAFEBEEF,
				Body:              []byte("payload"),
			}
			b, err := avtp.EncodeFrame(hdr, msg)
			if err != nil {
				t.Fatalf("timed=%v kind=%v: EncodeFrame: %v", timed, kind, err)
			}
			frame, err := avtp.DecodeFrame(b)
			if err != nil {
				t.Fatalf("timed=%v kind=%v: DecodeFrame: %v", timed, kind, err)
			}
			wantMsg := msg
			if kind == avtp.KindShort {
				wantMsg.Timestamp = 0 // short encoding carries no timestamp field
			}
			if !reflect.DeepEqual(frame.Message, wantMsg) {
				t.Errorf("timed=%v kind=%v: message mismatch:\n got  %+v\n want %+v",
					timed, kind, frame.Message, wantMsg)
			}
			if frame.Header.StreamID != hdr.StreamID || frame.Header.Timed != hdr.Timed {
				t.Errorf("timed=%v kind=%v: header mismatch: %+v", timed, kind, frame.Header)
			}
		}
	}
}

func TestDecodeFrame_LengthMismatch(t *testing.T) {
	b, err := avtp.EncodeFrame(avtp.Header{StreamID: testStreamID()}, avtp.Message{Kind: avtp.KindShort, Body: []byte("x")})
	if err != nil {
		t.Fatalf("EncodeFrame: %v", err)
	}
	// Append a stray trailing byte the header's DataLength doesn't account for.
	corrupt := append(b, 0x00)
	if _, err := avtp.DecodeFrame(corrupt); !errors.Is(err, avtp.ErrFrameLengthMismatch) {
		t.Errorf("DecodeFrame(extra byte) = %v, want ErrFrameLengthMismatch", err)
	}
}
