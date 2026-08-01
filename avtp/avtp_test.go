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
//fusa:test REQ-AVTP-017
//fusa:test REQ-AVTP-018
//fusa:test REQ-AVTP-019
//fusa:test REQ-AVTP-020

// Message-, ControlFlags-, and Frame-level tests (originally REQ-AVTP-011
// through REQ-AVTP-016) moved to the acf package's own test files when the
// message/frame layer split out of this package — see acf/doc.go and
// acf/message_test.go / acf/frame_test.go.
package avtp_test

import (
	"bytes"
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
	// TC18 Figure 6: NTSCF's sole reserved bit "r" is bit 12 — octet 1,
	// bit 3 (0x08). Bits 13-15 of that octet are ntscf_data_length's top
	// three, and bits 0-2 are sv/version, so 0x08 is the only bit here a
	// conformant sender must leave clear.
	untimed[1] |= 0x08
	if _, _, decErr := avtp.DecodeHeader(untimed); !errors.Is(decErr, avtp.ErrReservedBitsSet) {
		t.Errorf("untimed r bit: DecodeHeader = %v, want ErrReservedBitsSet", decErr)
	}

	// TC18 Figure 5: TSCF's "rsv" field is bits 13-14 — octet 1, mask 0x06.
	timed, err := avtp.EncodeHeader(avtp.Header{Timed: true, StreamID: testStreamID()})
	if err != nil {
		t.Fatalf("EncodeHeader: %v", err)
	}
	timed[1] |= 0x02
	if _, _, decErr := avtp.DecodeHeader(timed); !errors.Is(decErr, avtp.ErrReservedBitsSet) {
		t.Errorf("timed rsv field: DecodeHeader = %v, want ErrReservedBitsSet", decErr)
	}

	// TC18 Figure 5: bits 24-30 — octet 3, mask 0xFE — are reserved; only
	// bit 31 ("tu") carries meaning there.
	timed2, err := avtp.EncodeHeader(avtp.Header{Timed: true, StreamID: testStreamID()})
	if err != nil {
		t.Fatalf("EncodeHeader: %v", err)
	}
	timed2[3] |= 0x80
	if _, _, decErr := avtp.DecodeHeader(timed2); !errors.Is(decErr, avtp.ErrReservedBitsSet) {
		t.Errorf("timed octet-3 reserved: DecodeHeader = %v, want ErrReservedBitsSet", decErr)
	}
}

// ── REQ-AVTP-017: encoded headers match TC18 Figures 5 and 6 byte-for-byte ─
//
// The byte expectations below are laid out by hand from the specification's
// own figures — "OPEN Alliance TC18 Remote Control Protocol Specification
// v0.5.1_RC" §11.1 p.22, Figure 6 (NTSCF-Header Version 0) and Figure 5
// (TSCF-Header Version 0) — not copied back out of EncodeHeader. Both
// figures were read from a 600-DPI render of p.22 and cross-checked against
// the worked examples on p.79 (Figure 20 NTSCF, Figure 19 TSCF).
func TestEncodeHeader_UntimedWireLayout(t *testing.T) {
	h := avtp.Header{
		StreamIDValid: true,
		SequenceNum:   0x5A,
		DataLength:    0x123, // 291: exercises all 11 bits' straddle
		StreamID:      testStreamID(),
	}
	// Figure 6, one quadlet then stream_id (12 octets total):
	//   octet 0      subtype                     = 0x82
	//   octet 1      sv=1 | version=000 | r=0 | ntscf_data_length[10:8]=001
	//                -> 1 000 0 001              = 0x81
	//   octet 2      ntscf_data_length[7:0]      = 0x23
	//   octet 3      sequence_num                = 0x5A
	//   octets 4-11  stream_id
	want := []byte{
		0x82, 0x81, 0x23, 0x5A,
		0x02, 0x11, 0x22, 0x33, 0x44, 0x55, 0x12, 0x34,
	}
	got, err := avtp.EncodeHeader(h)
	if err != nil {
		t.Fatalf("EncodeHeader: %v", err)
	}
	if len(got) != 12 {
		t.Errorf("NTSCF header length = %d, want 12 (TC18 Figure 6)", len(got))
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("NTSCF wire layout:\n got  % X\n want % X", got, want)
	}
}

func TestEncodeHeader_TimedWireLayout(t *testing.T) {
	h := avtp.Header{
		Timed:           true,
		StreamIDValid:   true,
		SequenceNum:     0x5A,
		DataLength:      0x123,
		StreamID:        testStreamID(),
		Timestamp:       0xDEADBEEF,
		TimestampStatus: avtp.TimestampUncertain, // tv=1, tu=1
	}
	// Figure 5, six quadlets (24 octets total):
	//   octet 0        subtype                        = 0x05
	//   octet 1        sv=1|version=000|mr=0|rsv=00|tv=1 -> 1 000 0 00 1 = 0x81
	//   octet 2        sequence_num                   = 0x5A
	//   octet 3        reserved=0000000 | tu=1        = 0x01
	//   octets 4-11    stream_id
	//   octets 12-15   avtp_timestamp                 = 0xDEADBEEF
	//   octets 16-19   "Format specific" reserved     = 0
	//   octets 20-21   stream_data_length             = 0x0123
	//   octets 22-23   reserved                       = 0
	want := []byte{
		0x05, 0x81, 0x5A, 0x01,
		0x02, 0x11, 0x22, 0x33, 0x44, 0x55, 0x12, 0x34,
		0xDE, 0xAD, 0xBE, 0xEF,
		0x00, 0x00, 0x00, 0x00,
		0x01, 0x23, 0x00, 0x00,
	}
	got, err := avtp.EncodeHeader(h)
	if err != nil {
		t.Fatalf("EncodeHeader: %v", err)
	}
	if len(got) != 24 {
		t.Errorf("TSCF header length = %d, want 24 (TC18 Figure 5)", len(got))
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("TSCF wire layout:\n got  % X\n want % X", got, want)
	}
}

// ── REQ-AVTP-018: subtype tags carry the specification's own values ────────

func TestSubtypeValues(t *testing.T) {
	// TC18 §11.1 p.22 Figure 6 labels the NTSCF header's first octet
	// "subtype(0x82)"; Figure 5 labels the TSCF header's "subtype(0x05)".
	// Both are repeated by the p.79 worked examples (Figures 19 and 20).
	if avtp.SubtypeNTSCF != 0x82 {
		t.Errorf("SubtypeNTSCF = %#x, want 0x82 (TC18 Figure 6)", avtp.SubtypeNTSCF)
	}
	if avtp.SubtypeTSCF != 0x05 {
		t.Errorf("SubtypeTSCF = %#x, want 0x05 (TC18 Figure 5)", avtp.SubtypeTSCF)
	}
}

// ── REQ-AVTP-019: TSCF's tv/tu bit pair carries the timestamp marker ───────

func TestHeader_TimestampMarkerBits(t *testing.T) {
	tests := []struct {
		status     avtp.TimestampStatus
		wantTV     bool
		wantTU     bool
		wantDecode avtp.TimestampStatus
	}{
		// TC18 Figure 5: tv is bit 15 (octet 1, mask 0x01), tu is bit 31
		// (octet 3, mask 0x01).
		{avtp.TimestampValid, true, false, avtp.TimestampValid},
		{avtp.TimestampUncertain, true, true, avtp.TimestampUncertain},
		{avtp.TimestampMissing, false, false, avtp.TimestampMissing},
		// Invalid and Missing are indistinguishable on the wire (both
		// tv=0); decode reports the zero value. Disposition treats them
		// identically, so nothing downstream observes the collapse.
		{avtp.TimestampInvalid, false, false, avtp.TimestampMissing},
	}
	for _, tt := range tests {
		b, err := avtp.EncodeHeader(avtp.Header{Timed: true, TimestampStatus: tt.status})
		if err != nil {
			t.Fatalf("EncodeHeader(%v): %v", tt.status, err)
		}
		if gotTV := b[1]&0x01 != 0; gotTV != tt.wantTV {
			t.Errorf("%v: tv = %v, want %v", tt.status, gotTV, tt.wantTV)
		}
		if gotTU := b[3]&0x01 != 0; gotTU != tt.wantTU {
			t.Errorf("%v: tu = %v, want %v", tt.status, gotTU, tt.wantTU)
		}
		h, _, err := avtp.DecodeHeader(b)
		if err != nil {
			t.Fatalf("DecodeHeader(%v): %v", tt.status, err)
		}
		if h.TimestampStatus != tt.wantDecode {
			t.Errorf("%v: decoded status = %v, want %v", tt.status, h.TimestampStatus, tt.wantDecode)
		}
	}
}

// ── REQ-AVTP-020: TSCF's stream_data_length is a full 16 bits on decode ────

func TestDecodeHeader_TimedFullWidthDataLength(t *testing.T) {
	// TC18 Figure 5's "Packet Info" row gives stream_data_length bits 0-15
	// of its quadlet — 16 bits, unlike NTSCF's 11 — so a decoder must not
	// mask it down to 11 or a conformant peer's larger frame is misparsed.
	b, err := avtp.EncodeHeader(avtp.Header{Timed: true, StreamID: testStreamID()})
	if err != nil {
		t.Fatalf("EncodeHeader: %v", err)
	}
	b[20], b[21] = 0xAB, 0xCD
	h, _, err := avtp.DecodeHeader(b)
	if err != nil {
		t.Fatalf("DecodeHeader: %v", err)
	}
	if h.DataLength != 0xABCD {
		t.Errorf("DataLength = %#x, want 0xABCD", h.DataLength)
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
