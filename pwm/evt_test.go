//fusa:test REQ-PWM-011

package pwm_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/pwm"
)

// ── REQ-PWM-011: TC18 §13.5 Table 30 / §12.9.1 evt handling ──
//
// PWM is the one endpoint-type package that spans two Table 30 rows:
// PWM_OUT shares the arithmetic row with GPIO, while PWM_IN sits in the
// row with ADC, I²C, LIN, CAN, UART, ISELED and MDIO. See EVTClassFor.

// evtWaveformReq builds a PWM write request carrying evt[2:0] = sel and a
// four-byte waveform payload (period, then active — §13.7.5.3).
func evtWaveformReq(sel acf.EVTSelector, activeTicks, periodTicks uint16) acf.Message {
	return acf.Message{
		Kind:      acf.KindShort,
		ByteBusID: avtp.ByteBusID(1),
		EVT:       uint8(sel),
		Control:   acf.FlagWrite,
		Body:      pwm.EncodeWaveform(activeTicks, periodTicks),
	}
}

func newOutputEndpoint(t *testing.T, active, period uint16) (*pwm.Endpoint, avtp.StreamID) {
	t.Helper()
	ep, root := newDeclaredEndpoint(t)
	cfg := pwm.Config{Enabled: true, Role: pwm.RoleOutput, DefaultActiveTicks: active, DefaultPeriodTicks: period}
	if err := ep.Configure(root, cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	return ep, root
}

// writeWaveform issues one write request and returns the resulting applied
// waveform from the response body.
func writeWaveform(t *testing.T, ep *pwm.Endpoint, root avtp.StreamID, sel acf.EVTSelector, active, period uint16) (uint16, uint16) {
	t.Helper()
	resp, err := ep.HandleRequest(root, evtWaveformReq(sel, active, period))
	if err != nil {
		t.Fatalf("HandleRequest(evt=%03b): %v", sel, err)
	}
	gotActive, gotPeriod, err := pwm.DecodeWaveform(resp.Body)
	if err != nil {
		t.Fatalf("DecodeWaveform: %v", err)
	}
	return gotActive, gotPeriod
}

// TestEVTClassFor_SplitsOutputAndInputRows checks PWM_OUT is placed in Table
// 30's arithmetic row (shared with GPIO) and PWM_IN in the row shared with
// ADC, I²C, LIN, CAN, UART, ISELED and MDIO (REQ-PWM-011).
func TestEVTClassFor_SplitsOutputAndInputRows(t *testing.T) {
	if got := pwm.EVTClassFor(pwm.RoleOutput); got != acf.EVTClassArithmetic {
		t.Errorf("EVTClassFor(RoleOutput) = %v, want %v", got, acf.EVTClassArithmetic)
	}
	if got := pwm.EVTClassFor(pwm.RoleInput); got != acf.EVTClassConfigOnly {
		t.Errorf("EVTClassFor(RoleInput) = %v, want %v", got, acf.EVTClassConfigOnly)
	}
	// An unrecognized Role must not be granted arithmetic write semantics.
	if got := pwm.EVTClassFor(pwm.Role(200)); got != acf.EVTClassConfigOnly {
		t.Errorf("EVTClassFor(Role(200)) = %v, want %v", got, acf.EVTClassConfigOnly)
	}
}

// TestOutput_EVTSelectsCombiningRule checks each of Table 30's GPIO/PWM_OUT
// combining rules applies to a PWM_OUT write, per-16-bit-field. Table 30's
// 101b example is explicitly this: "The 'byte_msg_payload' plus 'current
// interface status' is written to the interface (example: this can be used
// to increase the duty cycle of PWM_out)" (REQ-PWM-011).
func TestOutput_EVTSelectsCombiningRule(t *testing.T) {
	ep, root := newOutputEndpoint(t, 250, 1000)

	// 000b: presented at the interface as is.
	if a, p := writeWaveform(t, ep, root, acf.EVTSelector0, 300, 1000); a != 300 || p != 1000 {
		t.Fatalf("set: got (%d, %d), want (300, 1000)", a, p)
	}

	// 101b: payload plus current status, field by field.
	if a, p := writeWaveform(t, ep, root, acf.EVTSelector5, 100, 0); a != 400 || p != 1000 {
		t.Errorf("add: got (%d, %d), want (400, 1000)", a, p)
	}

	// 110b: payload minus current status ("written as is"), field by field.
	// Current is (400, 1000); payload (500, 1500) gives (100, 500). The
	// opposite operand order would saturate both fields at zero, so this
	// case discriminates the two readings.
	if a, p := writeWaveform(t, ep, root, acf.EVTSelector6, 500, 1500); a != 100 || p != 500 {
		t.Errorf("subtract: got (%d, %d), want (100, 500) [payload minus current status]", a, p)
	}

	// 011b: bitwise XOR against the current status, field by field.
	// Current is (100, 500).
	if a, p := writeWaveform(t, ep, root, acf.EVTSelector3, 0x00FF, 0x0F00); a != 100^0x00FF || p != 500^0x0F00 {
		t.Errorf("xor: got (%d, %d), want (%d, %d)", a, p, 100^0x00FF, 500^0x0F00)
	}
}

// TestOutput_SaturatesAtSixteenBitBounds checks Table 30's note applied to
// PWM_OUT's own payload shape: "While doing additions and subtractions
// neither overflows nor wrap-arounds shall occur. The values are saturated
// at 0x0000 on the low side and 0xFFFF at the high side" (REQ-PWM-011).
func TestOutput_SaturatesAtSixteenBitBounds(t *testing.T) {
	ep, root := newOutputEndpoint(t, 0x8000, 0xFFFF)

	// High side: 0x8000 + 0x8000 would wrap to 0; it must clamp instead.
	// (The period field is already at the ceiling.)
	if a, p := writeWaveform(t, ep, root, acf.EVTSelector5, 0x8000, 0x8000); a != 0xFFFF || p != 0xFFFF {
		t.Errorf("add clamp: got (%#x, %#x), want (0xFFFF, 0xFFFF)", a, p)
	}

	// Low side: payload below the current status clamps at zero rather than
	// wrapping to a large value.
	if a, p := writeWaveform(t, ep, root, acf.EVTSelector6, 1, 1); a != 0 || p != 0 {
		t.Errorf("subtract clamp: got (%#x, %#x), want (0, 0)", a, p)
	}
}

// TestOutput_EVTReservedRejected checks Table 30's reserved GPIO/PWM_OUT
// slot: "100b: reserved – request shall be ignored and an err-response with
// error code = UNSUPPORTED_CMD shall be sent" (REQ-PWM-011).
func TestOutput_EVTReservedRejected(t *testing.T) {
	ep, root := newOutputEndpoint(t, 250, 1000)

	if _, err := ep.HandleRequest(root, evtWaveformReq(acf.EVTSelector4, 900, 900)); !errors.Is(err, acf.ErrEVTReserved) {
		t.Fatalf("evt=100b: err = %v, want acf.ErrEVTReserved", err)
	}

	// "request shall be ignored": the applied waveform must be untouched.
	readResp, err := ep.HandleRequest(root, acf.Message{
		Kind: acf.KindShort, ByteBusID: avtp.ByteBusID(1), Control: acf.FlagRead,
	})
	if err != nil {
		t.Fatalf("HandleRequest(read): %v", err)
	}
	if a, p, err := pwm.DecodeWaveform(readResp.Body); err != nil || a != 250 || p != 1000 {
		t.Errorf("waveform after rejected reserved request = (%d, %d, %v), want (250, 1000, nil)", a, p, err)
	}
}

// TestInput_EVTReservedRejected checks a PWM_IN endpoint follows the
// config-only row: 001b through 110b are reserved (REQ-PWM-011).
func TestInput_EVTReservedRejected(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if err := ep.Configure(root, pwm.Config{Enabled: true, Role: pwm.RoleInput}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	ep.SetCapturedWaveform(120, 480)

	for sel := acf.EVTSelector(1); sel <= 6; sel++ {
		req := acf.Message{
			Kind: acf.KindShort, ByteBusID: avtp.ByteBusID(1),
			EVT: uint8(sel), Control: acf.FlagRead, Body: []byte{0x00},
		}
		if _, err := ep.HandleRequest(root, req); !errors.Is(err, acf.ErrEVTReserved) {
			t.Errorf("PWM_IN evt[2:0]=%03b: err = %v, want acf.ErrEVTReserved", sel, err)
		}
	}
}

// TestEVT_NonZeroWithoutPayloadRejected checks TC18 §12.9.1's general rule:
// "If evt[2:0] ≠ 0 and no byte_msg_payload is present, then an error
// response shall be sent with the error code = UNSUPPORTED_CMD"
// (REQ-PWM-011).
func TestEVT_NonZeroWithoutPayloadRejected(t *testing.T) {
	ep, root := newOutputEndpoint(t, 250, 1000)

	for sel := acf.EVTSelector(1); sel <= 7; sel++ {
		req := acf.Message{
			Kind: acf.KindShort, ByteBusID: avtp.ByteBusID(1),
			EVT: uint8(sel), Control: acf.FlagWrite,
		}
		if _, err := ep.HandleRequest(root, req); !errors.Is(err, acf.ErrEVTMissingPayload) {
			t.Errorf("evt[2:0]=%03b with empty body: err = %v, want acf.ErrEVTMissingPayload", sel, err)
		}
	}
}

// TestEVT_ConfigChangeIsNotPresentedAtInterface checks evt[2:0] = 111b routes
// the payload into the endpoint's §12.7.1 EP_func block instead of onto the
// waveform generator (REQ-PWM-011).
func TestEVT_ConfigChangeIsNotPresentedAtInterface(t *testing.T) {
	ep, root := newOutputEndpoint(t, 250, 1000)

	// Byte 0 of pwm's EP_func block is its Enabled flag; clearing it is a
	// §12.7.1 configuration write (Figure 18: the payload leads with the
	// relative EP_func start address).
	cfgReq := acf.Message{
		Kind: acf.KindShort, ByteBusID: avtp.ByteBusID(1),
		EVT: uint8(acf.EVTSelector7), Control: acf.FlagWrite,
		Body: acf.EncodeConfigRequestBody(0, []byte{0x00}),
	}
	if _, err := ep.HandleRequest(root, cfgReq); err != nil {
		t.Fatalf("HandleRequest(evt=111b): %v", err)
	}

	// No waveform update was queued by the configuration write itself: the
	// only pending trigger is Configure's own initial output.
	for _, ev := range ep.DrainTriggers() {
		if ev.Kind == pwm.TriggerOutputUpdated && (ev.ActiveTicks != 250 || ev.PeriodTicks != 1000) {
			t.Errorf("configuration payload reached the waveform generator: %+v", ev)
		}
	}

	// The configuration really was adopted.
	if _, err := ep.HandleRequest(root, evtWaveformReq(acf.EVTSelector0, 100, 1000)); !errors.Is(err, pwm.ErrNotConfigured) {
		t.Errorf("after configuration write: err = %v, want pwm.ErrNotConfigured", err)
	}
}
