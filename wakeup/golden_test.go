//fusa:test REQ-WAKEUP-009

package wakeup_test

import (
	"bytes"
	"testing"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/wakeup"
)

// ── REQ-WAKEUP-009 (golden-vector half): frozen Wakeup Config/request byte
// layouts ──
//
// These fixtures pin the exact wire bytes this package's encoders produce
// today, so later work can regression-test against a frozen Wakeup
// encoding rather than re-deriving it from current behaviour, the same
// posture i2c/golden_test.go established.

// goldenConfig is Enabled=1, WakeHandshakeIntervalMillis=50,
// WakeHandshakeRepeatCount=3.
var goldenConfig = []byte{0x01, 0x00, 0x00, 0x00, 0x32, 0x00, 0x03}

func TestGolden_Config(t *testing.T) {
	cfg := wakeup.Config{Enabled: true, WakeHandshakeIntervalMillis: 50, WakeHandshakeRepeatCount: 3}
	got := wakeup.EncodeConfig(cfg)
	if !bytes.Equal(got, goldenConfig) {
		t.Fatalf("EncodeConfig changed:\n got  % X\n want % X", got, goldenConfig)
	}
	decoded, err := wakeup.DecodeConfig(goldenConfig)
	if err != nil {
		t.Fatalf("DecodeConfig(golden): %v", err)
	}
	if decoded != cfg {
		t.Errorf("DecodeConfig(golden) = %+v, want %+v", decoded, cfg)
	}
}

// goldenSleepRequest is a write request targeting PowerSleep.
var goldenSleepRequest = []byte{0x02}

func TestGolden_PowerStateRequest(t *testing.T) {
	got := wakeup.EncodePowerStateRequest(wakeup.PowerSleep)
	if !bytes.Equal(got, goldenSleepRequest) {
		t.Fatalf("EncodePowerStateRequest changed:\n got  % X\n want % X", got, goldenSleepRequest)
	}
	decoded, err := wakeup.DecodePowerStateRequest(goldenSleepRequest)
	if err != nil {
		t.Fatalf("DecodePowerStateRequest(golden): %v", err)
	}
	if decoded != wakeup.PowerSleep {
		t.Errorf("DecodePowerStateRequest(golden) = %v, want PowerSleep", decoded)
	}
}

func TestGolden_EndToEndDispatch(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if err := ep.Configure(root, defaultConfig()); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	req := acf.Message{
		Kind:      acf.KindShort,
		ByteBusID: avtp.ByteBusID(1),
		Control:   acf.FlagWrite,
		Body:      goldenSleepRequest,
	}
	resp, err := ep.HandleRequest(root, req)
	if err != nil {
		t.Fatalf("HandleRequest(golden write): %v", err)
	}
	if !bytes.Equal(resp.Body, goldenSleepRequest) {
		t.Fatalf("HandleRequest(golden write) response body = % X, want % X (echo)", resp.Body, goldenSleepRequest)
	}
	if got := ep.State(); got != wakeup.PowerSleep {
		t.Errorf("State() after golden write = %v, want PowerSleep", got)
	}
}
