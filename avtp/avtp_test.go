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

// Message-, ControlFlags-, and Frame-level tests (originally REQ-AVTP-011
// through REQ-AVTP-016) moved to the acf package's own test files when the
// message/frame layer split out of this package — see acf/doc.go and
// acf/message_test.go / acf/frame_test.go.
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
