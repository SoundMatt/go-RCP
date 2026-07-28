//fusa:test REQ-MDIO-002
//fusa:test REQ-MDIO-003

package mdio_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/mdio"
)

// TestAddressMode_Valid checks Valid recognizes exactly the two defined
// address modes (REQ-MDIO-002).
func TestAddressMode_Valid(t *testing.T) {
	for _, m := range []mdio.AddressMode{mdio.ModeClause22, mdio.ModeClause45} {
		if !m.Valid() {
			t.Errorf("AddressMode(%d).Valid() = false, want true", m)
		}
	}
	for _, m := range []mdio.AddressMode{2, 3, 255} {
		if m.Valid() {
			t.Errorf("AddressMode(%d).Valid() = true, want false", m)
		}
	}
}

// TestRequest_Validate checks PhyAddr/DevAddr/RegAddr width enforcement,
// mode-dependently (REQ-MDIO-003).
func TestRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		r       mdio.Request
		wantErr error
	}{
		{"invalid mode", mdio.Request{Mode: 9}, mdio.ErrInvalidAddressMode},
		{"phy addr out of range", mdio.Request{PhyAddr: 0x20}, mdio.ErrPhyAddrOutOfRange},
		{"c22 ok", mdio.Request{Mode: mdio.ModeClause22, PhyAddr: 3, RegAddr: 0x1F}, nil},
		{"c22 rejects dev addr", mdio.Request{Mode: mdio.ModeClause22, DevAddr: 1}, mdio.ErrDevAddrNotSupported},
		{"c22 reg addr out of range", mdio.Request{Mode: mdio.ModeClause22, RegAddr: 0x20}, mdio.ErrRegAddrOutOfRange},
		{"c45 ok", mdio.Request{Mode: mdio.ModeClause45, PhyAddr: 1, DevAddr: 0x1F, RegAddr: 0xFFFF}, nil},
		{"c45 dev addr out of range", mdio.Request{Mode: mdio.ModeClause45, DevAddr: 0x20}, mdio.ErrDevAddrOutOfRange},
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
