//fusa:test REQ-GPIO-012

package gpio_test

import (
	"bytes"
	"testing"

	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/gpio"
)

// ── REQ-GPIO-012 (golden-vector half): frozen GPIO Config/request/response
// byte layouts ──
//
// These fixtures pin the exact wire bytes this package's encoders produce
// today, so Phase 15's conditional-request work and Phase 16's remaining
// endpoint types can regression-test against a frozen GPIO encoding rather
// than re-deriving it from current behaviour, the same posture
// server/golden_test.go established for the register map itself.

// goldenConfig is PinCount=8, Direction=0x000000F0 (pins 4-7 output, 0-3
// input), TriggerEnable=0x00000001 (pin 0 only).
var goldenConfig = []byte{
	0x08,
	0x00, 0x00, 0x00, 0xF0,
	0x00, 0x00, 0x00, 0x01,
}

func TestGolden_Config(t *testing.T) {
	cfg := gpio.Config{PinCount: 8, Direction: 0x000000F0, TriggerEnable: 0x00000001}
	got := gpio.EncodeConfig(cfg)
	if !bytes.Equal(got, goldenConfig) {
		t.Fatalf("EncodeConfig changed:\n got  % X\n want % X", got, goldenConfig)
	}
	decoded, err := gpio.DecodeConfig(goldenConfig)
	if err != nil {
		t.Fatalf("DecodeConfig(golden): %v", err)
	}
	if decoded != cfg {
		t.Errorf("DecodeConfig(golden) = %+v, want %+v", decoded, cfg)
	}
}

// goldenWriteRequest is SemanticOr(1), operand=0x00000005.
var goldenWriteRequest = []byte{0x01, 0x00, 0x00, 0x00, 0x05}

// goldenResponseValue is pin value 0x00000005.
var goldenResponseValue = []byte{0x00, 0x00, 0x00, 0x05}

func TestGolden_WriteRequestAndResponse(t *testing.T) {
	got := gpio.EncodeWriteRequest(gpio.SemanticOr, 0x00000005)
	if !bytes.Equal(got, goldenWriteRequest) {
		t.Fatalf("EncodeWriteRequest changed:\n got  % X\n want % X", got, goldenWriteRequest)
	}
	sem, operand, err := gpio.DecodeWriteRequest(goldenWriteRequest)
	if err != nil {
		t.Fatalf("DecodeWriteRequest(golden): %v", err)
	}
	if sem != gpio.SemanticOr || operand != 0x00000005 {
		t.Errorf("DecodeWriteRequest(golden) = (%v, %#x), want (SemanticOr, 0x5)", sem, operand)
	}

	respBody := gpio.EncodeValue(0x00000005)
	if !bytes.Equal(respBody, goldenResponseValue) {
		t.Fatalf("EncodeValue changed:\n got  % X\n want % X", respBody, goldenResponseValue)
	}
}

func TestGolden_EndToEndDispatch(t *testing.T) {
	cfg := gpio.Config{PinCount: 8, Direction: 0x000000FF}
	ep, root := newConfiguredEndpoint(t, cfg)

	req := avtp.Message{
		Kind:      avtp.KindShort,
		ByteBusID: avtp.ByteBusID(1),
		Control:   avtp.FlagWrite,
		Body:      goldenWriteRequest,
	}
	resp, err := ep.HandleRequest(root, req)
	if err != nil {
		t.Fatalf("HandleRequest(golden write): %v", err)
	}
	if !bytes.Equal(resp.Body, goldenResponseValue) {
		t.Fatalf("HandleRequest(golden write) response body = % X, want % X", resp.Body, goldenResponseValue)
	}
}
