//fusa:test REQ-ADC-010

package adc_test

import (
	"bytes"
	"testing"

	"github.com/SoundMatt/go-RCP/adc"
	"github.com/SoundMatt/go-RCP/avtp"
)

// ── REQ-ADC-010 (golden-vector half): frozen ADC Config/response byte
// layouts ──
//
// These fixtures pin the exact wire bytes this package's encoders produce
// today, so Phase 15's conditional-request work and later endpoint types can
// regression-test against a frozen ADC encoding rather than re-deriving it
// from current behaviour, the same posture gpio/golden_test.go and
// spi/golden_test.go established.

// goldenConfig is Enabled=1, ResolutionBits=12, SampleCount=8,
// Combine=CombineRollingAverage(1), TriggerMode=TriggerModeSelf(2).
var goldenConfig = []byte{0x01, 0x0C, 0x08, 0x01, 0x02}

func TestGolden_Config(t *testing.T) {
	cfg := adc.Config{Enabled: true, ResolutionBits: 12, SampleCount: 8, Combine: adc.CombineRollingAverage, TriggerMode: adc.TriggerModeSelf}
	got := adc.EncodeConfig(cfg)
	if !bytes.Equal(got, goldenConfig) {
		t.Fatalf("EncodeConfig changed:\n got  % X\n want % X", got, goldenConfig)
	}
	decoded, err := adc.DecodeConfig(goldenConfig)
	if err != nil {
		t.Fatalf("DecodeConfig(golden): %v", err)
	}
	if decoded != cfg {
		t.Errorf("DecodeConfig(golden) = %+v, want %+v", decoded, cfg)
	}
}

// goldenValue is sample value 0x0ABC.
var goldenValue = []byte{0x0A, 0xBC}

func TestGolden_Value(t *testing.T) {
	got := adc.EncodeValue(0x0ABC)
	if !bytes.Equal(got, goldenValue) {
		t.Fatalf("EncodeValue changed:\n got  % X\n want % X", got, goldenValue)
	}
	v, err := adc.DecodeValue(goldenValue)
	if err != nil {
		t.Fatalf("DecodeValue(golden): %v", err)
	}
	if v != 0x0ABC {
		t.Errorf("DecodeValue(golden) = %#x, want 0xABC", v)
	}
}

func TestGolden_EndToEndDispatch(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	cfg := adc.Config{Enabled: true, ResolutionBits: 12, SampleCount: 1, Combine: adc.CombineReplace, TriggerMode: adc.TriggerModeOnDemand}
	if err := ep.Configure(root, cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	ep.SetTransport(&sequenceTransport{samples: []uint16{0x0ABC}})

	req := avtp.Message{Kind: avtp.KindShort, ByteBusID: avtp.ByteBusID(1), Control: avtp.FlagRead}
	resp, err := ep.HandleRequest(root, req)
	if err != nil {
		t.Fatalf("HandleRequest(golden read): %v", err)
	}
	if !bytes.Equal(resp.Body, goldenValue) {
		t.Fatalf("HandleRequest(golden read) response body = % X, want % X", resp.Body, goldenValue)
	}
}
