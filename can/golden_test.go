//fusa:test REQ-CANEP-010

package can_test

import (
	"bytes"
	"testing"

	"github.com/SoundMatt/go-RCP/v9/can"
)

// ── REQ-CANEP-010 (golden-vector half): frozen CAN Config/Frame byte
// layouts ──
//
// These fixtures pin the exact wire bytes this package's encoders produce
// today, so later work can regression-test against a frozen CAN encoding
// rather than re-deriving it from current behaviour, the same posture
// i2c/golden_test.go established.

// goldenConfig is Enabled=1, NominalBitrateKbps=500, DataBitrateKbps=2000.
var goldenConfig = []byte{0x01, 0x00, 0x00, 0x01, 0xF4, 0x00, 0x00, 0x07, 0xD0}

func TestGolden_Config(t *testing.T) {
	cfg := can.Config{Enabled: true, NominalBitrateKbps: 500, DataBitrateKbps: 2000}
	got := can.EncodeConfig(cfg)
	if !bytes.Equal(got, goldenConfig) {
		t.Fatalf("EncodeConfig changed:\n got  % X\n want % X", got, goldenConfig)
	}
	decoded, err := can.DecodeConfig(goldenConfig)
	if err != nil {
		t.Fatalf("DecodeConfig(golden): %v", err)
	}
	if decoded != cfg {
		t.Errorf("DecodeConfig(golden) = %+v, want %+v", decoded, cfg)
	}
}

// goldenClassicalFrame is a standard-ID Classical frame: ID=0x123, two data
// bytes.
var goldenClassicalFrame = []byte{
	0x00,                   // Format = FormatClassical
	0x00,                   // flags: !Extended, !BitRateSwitch
	0x00, 0x00, 0x01, 0x23, // ID
	0x00, 0x02, // data length = 2
	0xDE, 0xAD, // data
}

func TestGolden_ClassicalFrame(t *testing.T) {
	f := can.Frame{Format: can.FormatClassical, ID: 0x123, Data: []byte{0xDE, 0xAD}}
	got := can.EncodeFrame(f)
	if !bytes.Equal(got, goldenClassicalFrame) {
		t.Fatalf("EncodeFrame(classical) changed:\n got  % X\n want % X", got, goldenClassicalFrame)
	}
	decoded, err := can.DecodeFrame(goldenClassicalFrame)
	if err != nil {
		t.Fatalf("DecodeFrame(golden classical): %v", err)
	}
	if decoded.ID != f.ID || !bytes.Equal(decoded.Data, f.Data) {
		t.Errorf("DecodeFrame(golden classical) = %+v, want %+v", decoded, f)
	}
}

func TestGolden_EndToEndDispatch(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if err := ep.Configure(root, can.Config{Enabled: true, NominalBitrateKbps: 500}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	req := writeReq(can.Frame{Format: can.FormatClassical, ID: 0x123, Data: []byte{0xDE, 0xAD}})
	req.Body = goldenClassicalFrame
	resp, err := ep.HandleRequest(root, req)
	if err != nil {
		t.Fatalf("HandleRequest(golden write): %v", err)
	}
	if !bytes.Equal(resp.Body, goldenClassicalFrame) {
		t.Fatalf("HandleRequest(golden write) response body = % X, want % X (echo)", resp.Body, goldenClassicalFrame)
	}
}
