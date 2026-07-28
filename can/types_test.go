//fusa:test REQ-CANEP-003
//fusa:test REQ-CANEP-004

package can_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/can"
)

// TestFormat_Valid checks Valid recognizes exactly the three defined frame
// formats (REQ-CANEP-003).
func TestFormat_Valid(t *testing.T) {
	for _, f := range []can.Format{can.FormatClassical, can.FormatFD, can.FormatXL} {
		if !f.Valid() {
			t.Errorf("Format(%d).Valid() = false, want true", f)
		}
	}
	for _, f := range []can.Format{3, 4, 255} {
		if f.Valid() {
			t.Errorf("Format(%d).Valid() = true, want false", f)
		}
	}
}

// TestFrame_Validate checks each frame format's payload cap, standard/
// extended ID width, and that BitRateSwitch/XLHeader are rejected outside
// the formats that define them (REQ-CANEP-004).
func TestFrame_Validate(t *testing.T) {
	tests := []struct {
		name    string
		f       can.Frame
		wantErr error
	}{
		{"invalid format", can.Frame{Format: 9}, can.ErrInvalidFormat},
		{"classical ok", can.Frame{Format: can.FormatClassical, Data: make([]byte, 8)}, nil},
		{"classical over cap", can.Frame{Format: can.FormatClassical, Data: make([]byte, 9)}, can.ErrPayloadTooLarge},
		{"classical rejects bit-rate switch", can.Frame{Format: can.FormatClassical, BitRateSwitch: true}, can.ErrBitRateSwitchNotSupported},
		{"classical rejects XL header", can.Frame{Format: can.FormatClassical, XL: can.XLHeader{SDT: 1}}, can.ErrXLHeaderNotSupported},
		{"fd ok", can.Frame{Format: can.FormatFD, Data: make([]byte, 64), BitRateSwitch: true}, nil},
		{"fd over cap", can.Frame{Format: can.FormatFD, Data: make([]byte, 65)}, can.ErrPayloadTooLarge},
		{"fd rejects XL header", can.Frame{Format: can.FormatFD, XL: can.XLHeader{VCID: 1}}, can.ErrXLHeaderNotSupported},
		{"xl ok", can.Frame{Format: can.FormatXL, Data: make([]byte, 2048), XL: can.XLHeader{SDT: 1, VCID: 2, AF: 3}}, nil},
		{"xl over cap", can.Frame{Format: can.FormatXL, Data: make([]byte, 2049)}, can.ErrPayloadTooLarge},
		{"standard id ok", can.Frame{Format: can.FormatClassical, ID: 0x7FF}, nil},
		{"standard id out of range", can.Frame{Format: can.FormatClassical, ID: 0x800}, can.ErrIDOutOfRange},
		{"extended id ok", can.Frame{Format: can.FormatClassical, Extended: true, ID: 0x1FFFFFFF}, nil},
		{"extended id out of range", can.Frame{Format: can.FormatClassical, Extended: true, ID: 0x20000000}, can.ErrIDOutOfRange},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.f.Validate()
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
