//fusa:test REQ-MDIO-002
//fusa:test REQ-MDIO-003

package mdio_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/mdio"
)

// TestMode_Valid checks Valid recognizes exactly the four defined mdio_mode
// values (REQ-MDIO-002).
func TestMode_Valid(t *testing.T) {
	for _, m := range []mdio.Mode{
		mdio.ModeMMDSingleWord, mdio.ModeMMDMultiByte,
		mdio.ModeMMSSingleWord, mdio.ModeMMSMultiWord,
	} {
		if !m.Valid() {
			t.Errorf("Mode(%d).Valid() = false, want true", m)
		}
	}
	for _, m := range []mdio.Mode{4, 5, 255} {
		if m.Valid() {
			t.Errorf("Mode(%d).Valid() = true, want false", m)
		}
	}
}

// TestRequest_Validate checks DevAddr width enforcement, and mode
// recognition (REQ-MDIO-003).
func TestRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		r       mdio.Request
		wantErr error
	}{
		{"invalid mode", mdio.Request{Mode: 9}, mdio.ErrInvalidMode},
		{"mmd single-word ok", mdio.Request{Mode: mdio.ModeMMDSingleWord, DevAddr: 0x1F}, nil},
		{"mmd multi-byte dev addr out of range", mdio.Request{Mode: mdio.ModeMMDMultiByte, DevAddr: 0x20}, mdio.ErrDevAddrOutOfRange},
		{"mms single-word ok", mdio.Request{Mode: mdio.ModeMMSSingleWord, DevAddr: 0x1F}, nil},
		{"mms multi-word dev addr out of range", mdio.Request{Mode: mdio.ModeMMSMultiWord, DevAddr: 0x20}, mdio.ErrDevAddrOutOfRange},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.r.Validate()
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// TestRequest_DataWidth checks MMD accesses are always 16-bit, MMS accesses
// to MMS index 0 or 1 ("MMS0"/"MMS1") are 32-bit, and MMS accesses to every
// other index are 16-bit — for both the single-word and multiple-word
// variant of each (REQ-MDIO-003).
func TestRequest_DataWidth(t *testing.T) {
	tests := []struct {
		name string
		r    mdio.Request
		want int
	}{
		{"mmd single-word, dev 0", mdio.Request{Mode: mdio.ModeMMDSingleWord, DevAddr: 0}, 2},
		{"mmd single-word, dev 1", mdio.Request{Mode: mdio.ModeMMDSingleWord, DevAddr: 1}, 2},
		{"mmd multi-byte, dev 31", mdio.Request{Mode: mdio.ModeMMDMultiByte, DevAddr: 0x1F}, 2},
		{"mms single-word, MMS0", mdio.Request{Mode: mdio.ModeMMSSingleWord, DevAddr: 0}, 4},
		{"mms single-word, MMS1", mdio.Request{Mode: mdio.ModeMMSSingleWord, DevAddr: 1}, 4},
		{"mms single-word, MMS2", mdio.Request{Mode: mdio.ModeMMSSingleWord, DevAddr: 2}, 2},
		{"mms multi-word, MMS0", mdio.Request{Mode: mdio.ModeMMSMultiWord, DevAddr: 0}, 4},
		{"mms multi-word, MMS1", mdio.Request{Mode: mdio.ModeMMSMultiWord, DevAddr: 1}, 4},
		{"mms multi-word, MMS31", mdio.Request{Mode: mdio.ModeMMSMultiWord, DevAddr: 0x1F}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.r.DataWidth(); got != tt.want {
				t.Errorf("DataWidth() = %d, want %d", got, tt.want)
			}
		})
	}
}
