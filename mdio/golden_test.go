//fusa:test REQ-MDIO-008

package mdio_test

import (
	"bytes"
	"testing"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/mdio"
)

// ── REQ-MDIO-008 (golden-vector half): frozen MDIO Config/Request/Response
// byte layouts ──
//
// These fixtures pin the exact wire bytes this package's encoders produce
// today, so later work can regression-test against a frozen MDIO encoding
// rather than re-deriving it from current behaviour, the same posture
// i2c/golden_test.go established.

// goldenConfig is Enabled=1.
var goldenConfig = []byte{0x01}

func TestGolden_Config(t *testing.T) {
	cfg := mdio.Config{Enabled: true}
	got := mdio.EncodeConfig(cfg)
	if !bytes.Equal(got, goldenConfig) {
		t.Fatalf("EncodeConfig changed:\n got  % X\n want % X", got, goldenConfig)
	}
	decoded, err := mdio.DecodeConfig(goldenConfig)
	if err != nil {
		t.Fatalf("DecodeConfig(golden): %v", err)
	}
	if decoded != cfg {
		t.Errorf("DecodeConfig(golden) = %+v, want %+v", decoded, cfg)
	}
}

// goldenReadRequest is a Clause 45 read of PHY 1, device (MMD) 3, register
// 0x0006.
var goldenReadRequest = []byte{0x01, 0x01, 0x03, 0x00, 0x06}

func TestGolden_ReadRequest(t *testing.T) {
	r := mdio.Request{Mode: mdio.ModeClause45, PhyAddr: 1, DevAddr: 3, RegAddr: 0x0006}
	got := mdio.EncodeReadRequest(r)
	if !bytes.Equal(got, goldenReadRequest) {
		t.Fatalf("EncodeReadRequest changed:\n got  % X\n want % X", got, goldenReadRequest)
	}
	decoded, err := mdio.DecodeReadRequest(goldenReadRequest)
	if err != nil {
		t.Fatalf("DecodeReadRequest(golden): %v", err)
	}
	if decoded != r {
		t.Errorf("DecodeReadRequest(golden) = %+v, want %+v", decoded, r)
	}
}

func TestGolden_EndToEndDispatch(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if err := ep.Configure(root, mdio.Config{Enabled: true}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	req := acf.Message{
		Kind:      acf.KindShort,
		ByteBusID: avtp.ByteBusID(1),
		Control:   acf.FlagRead,
		Body:      goldenReadRequest,
	}
	resp, err := ep.HandleRequest(root, req)
	if err != nil {
		t.Fatalf("HandleRequest(golden read): %v", err)
	}
	got, err := mdio.DecodeResponse(resp.Body)
	if err != nil {
		t.Fatalf("DecodeResponse: %v", err)
	}
	if got != 0 { // default store: unset register reads as zero
		t.Fatalf("HandleRequest(golden read) = %#x, want 0", got)
	}
}
