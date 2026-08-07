//fusa:test REQ-RCS-018

package regmap_test

import (
	"testing"

	"github.com/SoundMatt/go-RCP/v9/regmap"
)

// wantGeneralBlock is a GeneralBlock with a distinguishable value in every
// exported field, chosen so each field's encoded bytes equal its own byte
// offset within the block (e.g. the uint32 at offset 0x04 encodes as bytes
// 04 05 06 07) — the same convention goldenGeneralBlockBytes below verifies
// byte-for-byte, hand-computed straight from the go-RCP-N2-02 register-map
// table rather than derived by round-tripping EncodeGeneralBlock. The three
// reserved fields (offsets 0x17, 0x22-0x23, 0x2B) are unexported, so this
// external test package cannot set them; they stay the zero value, which is
// also their documented always-0x00 wire value.
var wantGeneralBlock = regmap.GeneralBlock{
	Magic:                           regmap.GeneralBlockMagic, // 0x52435030, offset 0x00
	ProtocolVersion:                 0x04050607,               // offset 0x04
	VendorID:                        0x0809,                   // offset 0x08
	DeviceID:                        0x0A0B,                   // offset 0x0A
	NumEndpoints:                    0x0C0D,                   // offset 0x0C
	MaxRequestStreams:               0x0E,                     // offset 0x0E
	MaxResponderStreams:             0x0F,                     // offset 0x0F
	MaxResponderQueueWords:          0x1011,                   // offset 0x10
	MaxRequestQueueWords:            0x1213,                   // offset 0x12
	NumSequencerStates:              0x14,                     // offset 0x14
	ConfigLock:                      0x15,                     // offset 0x15
	Options:                         0x16,                     // offset 0x16
	NumIOPins:                       0x1819,                   // offset 0x18
	HWConfigPointer:                 0x1A1B,                   // offset 0x1A
	MaxConfigurableRequestStreams:   0x1C,                     // offset 0x1C
	MaxConfigurableResponseStreams:  0x1D,                     // offset 0x1D
	ClientConfigPointer:             0x1E1F,                   // offset 0x1E
	QueueConfigPointer:              0x2021,                   // offset 0x20
	EndpointConfigPointer:           0x2425,                   // offset 0x24
	EndpointConfigLength:            0x2627,                   // offset 0x26
	EndpointMapPointer:              0x2829,                   // offset 0x28
	EndpointMapMaxEntries:           0x2A,                     // offset 0x2A
	EndpointFunctionalConfigPointer: 0x2C2D,                   // offset 0x2C
	SequencerStateMapPointer:        0x2E2F,                   // offset 0x2E
}

// goldenGeneralBlockBytes is the 48-byte (0x30) encoding wantGeneralBlock
// above must produce, computed by hand field-by-field from the go-RCP-N2-02
// register-map table rather than derived from EncodeGeneralBlock's own
// output — the rigor the issue asks for. Every non-reserved byte equals its
// own offset; the reserved bytes at 0x17, 0x22-0x23, and 0x2B are 0x00.
var goldenGeneralBlockBytes = []byte{
	0x52, 0x43, 0x50, 0x30, // 0x00 Magic ("RCP0")
	0x04, 0x05, 0x06, 0x07, // 0x04 ProtocolVersion
	0x08, 0x09, // 0x08 VendorID
	0x0A, 0x0B, // 0x0A DeviceID
	0x0C, 0x0D, // 0x0C NumEndpoints
	0x0E,       // 0x0E MaxRequestStreams
	0x0F,       // 0x0F MaxResponderStreams
	0x10, 0x11, // 0x10 MaxResponderQueueWords
	0x12, 0x13, // 0x12 MaxRequestQueueWords
	0x14,       // 0x14 NumSequencerStates
	0x15,       // 0x15 ConfigLock
	0x16,       // 0x16 Options (implemented-options bitfield)
	0x00,       // 0x17 reserved
	0x18, 0x19, // 0x18 NumIOPins
	0x1A, 0x1B, // 0x1A HWConfigPointer
	0x1C,       // 0x1C MaxConfigurableRequestStreams
	0x1D,       // 0x1D MaxConfigurableResponseStreams
	0x1E, 0x1F, // 0x1E ClientConfigPointer
	0x20, 0x21, // 0x20 QueueConfigPointer
	0x00, 0x00, // 0x22 reserved
	0x24, 0x25, // 0x24 EndpointConfigPointer
	0x26, 0x27, // 0x26 EndpointConfigLength
	0x28, 0x29, // 0x28 EndpointMapPointer
	0x2A,       // 0x2A EndpointMapMaxEntries
	0x00,       // 0x2B reserved
	0x2C, 0x2D, // 0x2C EndpointFunctionalConfigPointer
	0x2E, 0x2F, // 0x2E SequencerStateMapPointer
}

// TestEncodeGeneralBlock_MatchesHandComputedBytes checks EncodeGeneralBlock
// produces exactly the bytes go-RCP-N2-02's register-map table predicts,
// field by field, rather than merely round-tripping through
// DecodeGeneralBlock (REQ-RCS-018).
func TestEncodeGeneralBlock_MatchesHandComputedBytes(t *testing.T) {
	if len(goldenGeneralBlockBytes) != 0x30 {
		t.Fatalf("test fixture goldenGeneralBlockBytes is %d bytes, want 0x30 (48)", len(goldenGeneralBlockBytes))
	}
	got := regmap.EncodeGeneralBlock(wantGeneralBlock)
	if len(got) != len(goldenGeneralBlockBytes) {
		t.Fatalf("EncodeGeneralBlock produced %d bytes, want %d", len(got), len(goldenGeneralBlockBytes))
	}
	for i := range goldenGeneralBlockBytes {
		if got[i] != goldenGeneralBlockBytes[i] {
			t.Errorf("byte offset 0x%02X = 0x%02X, want 0x%02X", i, got[i], goldenGeneralBlockBytes[i])
		}
	}
}

// TestGeneralBlock_RoundTrips checks EncodeGeneralBlock followed by
// DecodeGeneralBlock, starting from the hand-verified golden bytes above,
// reproduces an identical GeneralBlock (REQ-RCS-018).
func TestGeneralBlock_RoundTrips(t *testing.T) {
	got, rest, err := regmap.DecodeGeneralBlock(goldenGeneralBlockBytes)
	if err != nil {
		t.Fatalf("DecodeGeneralBlock: %v", err)
	}
	if len(rest) != 0 {
		t.Errorf("DecodeGeneralBlock left %d trailing bytes, want 0", len(rest))
	}
	if got != wantGeneralBlock {
		t.Errorf("DecodeGeneralBlock = %+v, want %+v", got, wantGeneralBlock)
	}

	// And the reverse direction: re-encoding the decoded value must
	// reproduce the same golden bytes.
	reEncoded := regmap.EncodeGeneralBlock(got)
	for i := range goldenGeneralBlockBytes {
		if reEncoded[i] != goldenGeneralBlockBytes[i] {
			t.Errorf("re-encoded byte offset 0x%02X = 0x%02X, want 0x%02X", i, reEncoded[i], goldenGeneralBlockBytes[i])
		}
	}
}

// TestDecodeGeneralBlock_ShortBufferError checks a buffer shorter than the
// fixed general-block length is rejected rather than panicking.
func TestDecodeGeneralBlock_ShortBufferError(t *testing.T) {
	_, _, err := regmap.DecodeGeneralBlock(make([]byte, 5))
	if err == nil {
		t.Fatal("DecodeGeneralBlock on a short buffer: got nil error, want one")
	}
}

// TestDecodeGeneralBlock_BadMagicError checks a full-length buffer whose
// leading four bytes are not GeneralBlockMagic is rejected with
// ErrBadMagic, without any other field being trusted (REQ-RCS-018).
func TestDecodeGeneralBlock_BadMagicError(t *testing.T) {
	tampered := append([]byte(nil), goldenGeneralBlockBytes...)
	tampered[0] ^= 0xFF // corrupt the magic
	_, _, err := regmap.DecodeGeneralBlock(tampered)
	if err != regmap.ErrBadMagic {
		t.Fatalf("DecodeGeneralBlock(bad magic) err = %v, want ErrBadMagic", err)
	}
}
