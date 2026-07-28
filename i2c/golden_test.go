//fusa:test REQ-I2C-009

package i2c_test

import (
	"bytes"
	"testing"

	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/i2c"
)

// ── REQ-I2C-009 (golden-vector half): frozen I2C Config/request/response
// byte layouts ──
//
// These fixtures pin the exact wire bytes this package's encoders produce
// today, so Phase 15's conditional-request work and later endpoint types can
// regression-test against a frozen I2C encoding rather than re-deriving it
// from current behaviour, the same posture gpio/golden_test.go and
// spi/golden_test.go established.

// goldenConfig is Enabled=1, Speed=SpeedFast(1), TrailingTimeMicros=100.
var goldenConfig = []byte{0x01, 0x01, 0x00, 0x64}

func TestGolden_Config(t *testing.T) {
	cfg := i2c.Config{Enabled: true, Speed: i2c.SpeedFast, TrailingTimeMicros: 100}
	got := i2c.EncodeConfig(cfg)
	if !bytes.Equal(got, goldenConfig) {
		t.Fatalf("EncodeConfig changed:\n got  % X\n want % X", got, goldenConfig)
	}
	decoded, err := i2c.DecodeConfig(goldenConfig)
	if err != nil {
		t.Fatalf("DecodeConfig(golden): %v", err)
	}
	if decoded != cfg {
		t.Errorf("DecodeConfig(golden) = %+v, want %+v", decoded, cfg)
	}
}

// goldenTransferRequest is a write of address byte 0xA0 followed by two data
// bytes.
var goldenTransferRequest = []byte{0xA0, 0xDE, 0xAD}

func TestGolden_TransferRequest(t *testing.T) {
	got := i2c.EncodeTransferRequest([]byte{0xA0, 0xDE, 0xAD})
	if !bytes.Equal(got, goldenTransferRequest) {
		t.Fatalf("EncodeTransferRequest changed:\n got  % X\n want % X", got, goldenTransferRequest)
	}
	if got := i2c.DecodeTransferRequest(goldenTransferRequest); !bytes.Equal(got, goldenTransferRequest) {
		t.Errorf("DecodeTransferRequest(golden) = % X, want % X", got, goldenTransferRequest)
	}
}

func TestGolden_EndToEndDispatch(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if err := ep.Configure(root, i2c.Config{Enabled: true, Speed: i2c.SpeedStandard}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	req := avtp.Message{
		Kind:      avtp.KindShort,
		ByteBusID: avtp.ByteBusID(1),
		Control:   avtp.FlagWrite,
		Body:      goldenTransferRequest,
	}
	resp, err := ep.HandleRequest(root, req)
	if err != nil {
		t.Fatalf("HandleRequest(golden transfer): %v", err)
	}
	// Default loopback: response echoes the same bytes.
	if !bytes.Equal(resp.Body, goldenTransferRequest) {
		t.Fatalf("HandleRequest(golden transfer) response body = % X, want % X (loopback)", resp.Body, goldenTransferRequest)
	}
}
