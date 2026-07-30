//fusa:test REQ-PWM-010

package pwm_test

import (
	"bytes"
	"testing"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/pwm"
)

// ── REQ-PWM-010 (golden-vector half): frozen PWM Config/waveform byte
// layouts ──
//
// These fixtures pin the exact wire bytes this package's encoders produce
// today, so Phase 15's conditional-request work and later endpoint types can
// regression-test against a frozen PWM encoding rather than re-deriving it
// from current behaviour, the same posture gpio/golden_test.go and
// spi/golden_test.go established.

// goldenConfig is Enabled=1, Role=RoleOutput(0), DefaultActiveTicks=1500
// (0x05DC), DefaultPeriodTicks=20000 (0x4E20) — hand-computed: 1500 decimal
// is 0x05DC, 20000 decimal is 0x4E20, each a big-endian 16-bit field, active
// before period per the governing specification's PWM_OUT/PWM_IN body
// layout.
var goldenConfig = []byte{
	0x01, 0x00,
	0x05, 0xDC,
	0x4E, 0x20,
}

func TestGolden_Config(t *testing.T) {
	cfg := pwm.Config{Enabled: true, Role: pwm.RoleOutput, DefaultActiveTicks: 1500, DefaultPeriodTicks: 20000}
	got := pwm.EncodeConfig(cfg)
	if !bytes.Equal(got, goldenConfig) {
		t.Fatalf("EncodeConfig changed:\n got  % X\n want % X", got, goldenConfig)
	}
	decoded, err := pwm.DecodeConfig(goldenConfig)
	if err != nil {
		t.Fatalf("DecodeConfig(golden): %v", err)
	}
	if decoded != cfg {
		t.Errorf("DecodeConfig(golden) = %+v, want %+v", decoded, cfg)
	}
}

// goldenWaveform is active=1500 (0x05DC), period=20000 (0x4E20), each a
// hand-computed big-endian 16-bit field, active first per the governing
// specification's PWM_OUT/PWM_IN body layout.
var goldenWaveform = []byte{0x05, 0xDC, 0x4E, 0x20}

func TestGolden_Waveform(t *testing.T) {
	got := pwm.EncodeWaveform(1500, 20000)
	if !bytes.Equal(got, goldenWaveform) {
		t.Fatalf("EncodeWaveform changed:\n got  % X\n want % X", got, goldenWaveform)
	}
	active, period, err := pwm.DecodeWaveform(goldenWaveform)
	if err != nil {
		t.Fatalf("DecodeWaveform(golden): %v", err)
	}
	if active != 1500 || period != 20000 {
		t.Errorf("DecodeWaveform(golden) = (%d, %d), want (1500, 20000)", active, period)
	}
}

func TestGolden_EndToEndDispatch(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	cfg := pwm.Config{Enabled: true, Role: pwm.RoleOutput, DefaultActiveTicks: 500, DefaultPeriodTicks: 1000}
	if err := ep.Configure(root, cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	req := acf.Message{
		Kind:      acf.KindShort,
		ByteBusID: avtp.ByteBusID(1),
		Control:   acf.FlagWrite,
		Body:      goldenWaveform,
	}
	resp, err := ep.HandleRequest(root, req)
	if err != nil {
		t.Fatalf("HandleRequest(golden write): %v", err)
	}
	if !bytes.Equal(resp.Body, goldenWaveform) {
		t.Fatalf("HandleRequest(golden write) response body = % X, want % X", resp.Body, goldenWaveform)
	}
}
