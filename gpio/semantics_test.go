//fusa:test REQ-GPIO-002
//fusa:test REQ-GPIO-003
//fusa:test REQ-GPIO-004
//fusa:test REQ-GPIO-005

package gpio_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/gpio"
	"github.com/SoundMatt/go-RCP/v9/server"
)

// rootStream and newConfiguredEndpoint are this package's own test helpers
// (gpio_test is an external test package, so it cannot reuse server_test's
// unexported helpers of the same name).
func rootStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55}, 1)
}

// newConfiguredEndpoint returns a gpio.Endpoint declared and configured on a
// fresh server.Server, with root as both the root client and the caller
// that will issue requests.
func newConfiguredEndpoint(t *testing.T, cfg gpio.Config) (*gpio.Endpoint, avtp.StreamID) {
	t.Helper()
	root := rootStream()
	s := server.NewServer()
	if err := s.ClaimRoot(root); err != nil {
		t.Fatalf("ClaimRoot: %v", err)
	}
	if err := s.AddEndpoint(root, avtp.ByteBusID(1), gpio.EndpointType); err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}
	ep := gpio.NewEndpoint(s, avtp.ByteBusID(1))
	if err := ep.Configure(root, cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	return ep, root
}

// writeMsg builds a GPIO write request carrying operand as its whole 4-byte
// body, with evt[2:0] = sel. TC18 §13.5 Table 30 puts the write-semantic
// selector in evt, and §13.7.4.1 fixes the body at exactly four bytes, so
// this is the complete shape of a GPIO write on the wire.
func writeMsg(sel acf.EVTSelector, operand uint32) acf.Message {
	return acf.Message{
		Kind:      acf.KindShort,
		ByteBusID: avtp.ByteBusID(1),
		EVT:       uint8(sel),
		Control:   acf.FlagWrite,
		Body:      gpio.EncodeWriteRequest(operand),
	}
}

// writeAndGetValue is a small test helper wrapping HandleRequest for a
// write request under the evt[2:0] selector sel.
func writeAndGetValue(t *testing.T, ep *gpio.Endpoint, requester avtp.StreamID, sel acf.EVTSelector, operand uint32) uint32 {
	t.Helper()
	resp, err := ep.HandleRequest(requester, writeMsg(sel, operand))
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	v, err := gpio.DecodeValue(resp.Body)
	if err != nil {
		t.Fatalf("DecodeValue: %v", err)
	}
	return v
}

// TestEVTClass_IsTable30ArithmeticRow pins which row of TC18 §13.5 Table 30
// governs this endpoint type. Table 30 lists "GPIO, PWM_OUT" as one row —
// the only row with combining semantics — so GPIO must declare the
// arithmetic class and nothing else (REQ-GPIO-002).
func TestEVTClass_IsTable30ArithmeticRow(t *testing.T) {
	if gpio.EVTClass != acf.EVTClassArithmetic {
		t.Errorf("gpio.EVTClass = %v, want %v", gpio.EVTClass, acf.EVTClassArithmetic)
	}
}

// TestWrite_EVTSelectsSemantic checks each of Table 30's GPIO/PWM_OUT
// combining rules is selected by evt[2:0] and affects output-direction pins
// only, leaving every input-direction pin untouched (REQ-GPIO-002,
// REQ-GPIO-003).
//
// Table 30, GPIO/PWM_OUT row:
//
//	000b  "The byte_msg_payload is presented at the interface"
//	001b  "The 'byte_msg_payload' bitwise OR 'current interface status'"
//	010b  "The 'byte_msg_payload' bitwise AND 'current interface status'"
//	011b  "The 'byte_msg_payload' bitwise XOR 'current interface status'"
func TestWrite_EVTSelectsSemantic(t *testing.T) {
	// 4 pins: pins 0-1 output, pins 2-3 input. Pre-seed an input value via
	// SetInputs so we can check it survives an output-only write untouched.
	cfg := gpio.Config{PinCount: 4, Direction: 0b0011}
	ep, root := newConfiguredEndpoint(t, cfg)
	ep.SetInputs(0b1100) // pins 2-3 (input) go high

	tests := []struct {
		name    string
		sel     acf.EVTSelector
		operand uint32
		want    uint32
	}{
		{"000b set", acf.EVTSelector0, 0b0001, 0b1101},
		{"001b or", acf.EVTSelector1, 0b0010, 0b1111},
		{"010b and", acf.EVTSelector2, 0b0001, 0b1101},
		{"011b xor", acf.EVTSelector3, 0b0011, 0b1110},
	}
	for _, tt := range tests {
		got := writeAndGetValue(t, ep, root, tt.sel, tt.operand)
		if got != tt.want {
			t.Errorf("%s: value = %04b, want %04b", tt.name, got, tt.want)
		}
	}
}

// TestWrite_EVTReservedRejected checks evt[2:0] = 100b is rejected. Table 30
// is explicit for the GPIO/PWM_OUT row: "100b: reserved – request shall be
// ignored and an err-response with error code = UNSUPPORTED_CMD shall be
// sent" (REQ-GPIO-002).
func TestWrite_EVTReservedRejected(t *testing.T) {
	ep, root := newConfiguredEndpoint(t, gpio.Config{PinCount: 4, Direction: 0b1111})

	before := writeAndGetValue(t, ep, root, acf.EVTSelector0, 0b0101)

	if _, err := ep.HandleRequest(root, writeMsg(acf.EVTSelector4, 0b1010)); !errors.Is(err, acf.ErrEVTReserved) {
		t.Fatalf("HandleRequest(evt=100b) err = %v, want acf.ErrEVTReserved", err)
	}

	// "request shall be ignored": the reserved request must not have moved
	// a single pin on its way to being rejected.
	after := writeAndGetValue(t, ep, root, acf.EVTSelector1, 0)
	if after != before {
		t.Errorf("value after rejected reserved request = %04b, want unchanged %04b", after, before)
	}
}

// TestWrite_SaturatingAddClamps checks evt[2:0] = 101b adds the payload to
// the current interface status and clamps instead of wrapping. Table 30:
// "101b: The 'byte_msg_payload' plus 'current interface status' is written
// to the interface", with the note "While doing additions and subtractions
// neither overflows nor wrap-arounds shall occur" (REQ-GPIO-004).
func TestWrite_SaturatingAddClamps(t *testing.T) {
	ep, root := newConfiguredEndpoint(t, gpio.Config{PinCount: 4, Direction: 0b1111})

	if got := writeAndGetValue(t, ep, root, acf.EVTSelector5, 0b0011); got != 0b0011 {
		t.Fatalf("add from zero: value = %04b, want 0011", got)
	}
	// 0b0011 + 0b0010 = 0b0101, no clamping needed.
	if got := writeAndGetValue(t, ep, root, acf.EVTSelector5, 0b0010); got != 0b0101 {
		t.Fatalf("add: value = %04b, want 0101", got)
	}
	// A payload that would overflow the endpoint's 4-bit interface word
	// clamps to the active-pin mask rather than wrapping to a small value.
	if got := writeAndGetValue(t, ep, root, acf.EVTSelector5, 0xFFFFFFFF); got != 0b1111 {
		t.Errorf("add clamp: value = %04b, want 1111", got)
	}
}

// TestWrite_SaturatingSubtractDirectionAndClamp checks evt[2:0] = 110b.
// Table 30's normative sentence is "'byte_msg_payload' minus 'current
// interface status' is written as is to interface" — payload minus current,
// not the other way round — clamped at zero on the low side by the same
// no-wrap-around note (REQ-GPIO-004).
func TestWrite_SaturatingSubtractDirectionAndClamp(t *testing.T) {
	ep, root := newConfiguredEndpoint(t, gpio.Config{PinCount: 4, Direction: 0b1111})

	// Seed the interface status at 0b0011.
	if got := writeAndGetValue(t, ep, root, acf.EVTSelector0, 0b0011); got != 0b0011 {
		t.Fatalf("seed: value = %04b, want 0011", got)
	}

	// payload 0b1010 (10) minus current 0b0011 (3) = 0b0111 (7). The
	// opposite operand order would clamp to zero instead, so this case
	// discriminates the two readings.
	if got := writeAndGetValue(t, ep, root, acf.EVTSelector6, 0b1010); got != 0b0111 {
		t.Errorf("subtract: value = %04b, want 0111 (payload minus current status)", got)
	}

	// payload 0b0001 (1) minus current 0b0111 (7) saturates at zero rather
	// than wrapping around to 0b1010.
	if got := writeAndGetValue(t, ep, root, acf.EVTSelector6, 0b0001); got != 0 {
		t.Errorf("subtract clamp: value = %04b, want 0000", got)
	}
}

// TestWrite_EVTConfigChangeIsNotPresentedAtPins checks evt[2:0] = 111b.
// Table 30: "The byte_msg_payload is not presented to the interface but used
// to change the configuration of the endpoint (see 12.7.1)" — so the payload
// must reach Config and must NOT reach the pins (REQ-GPIO-005).
func TestWrite_EVTConfigChangeIsNotPresentedAtPins(t *testing.T) {
	ep, root := newConfiguredEndpoint(t, gpio.Config{PinCount: 4, Direction: 0b0011})

	// Drive an output value first, so we can confirm the configuration
	// request leaves the pins exactly as they were.
	if got := writeAndGetValue(t, ep, root, acf.EVTSelector0, 0b0001); got != 0b0001 {
		t.Fatalf("seed: value = %04b, want 0001", got)
	}

	// gpio's EP_func block is PinCount(1) + Direction(4) + TriggerEnable(4);
	// patch Direction (offset 1) to all-input. Per §12.7.1 Figure 18 the
	// configuration request's payload leads with the relative EP_func start
	// address.
	cfgReq := acf.Message{
		Kind:      acf.KindShort,
		ByteBusID: avtp.ByteBusID(1),
		EVT:       uint8(acf.EVTSelector7),
		Control:   acf.FlagWrite,
		Body:      acf.EncodeConfigRequestBody(1, []byte{0x00, 0x00, 0x00, 0x00}),
	}
	if _, err := ep.HandleRequest(root, cfgReq); err != nil {
		t.Fatalf("HandleRequest(evt=111b): %v", err)
	}

	// The pin value is untouched by the configuration write itself...
	readResp, err := ep.HandleRequest(root, acf.Message{
		Kind: acf.KindShort, ByteBusID: avtp.ByteBusID(1), Control: acf.FlagRead,
	})
	if err != nil {
		t.Fatalf("HandleRequest(read): %v", err)
	}
	v, err := gpio.DecodeValue(readResp.Body)
	if err != nil {
		t.Fatalf("DecodeValue: %v", err)
	}
	if v != 0b0001 {
		t.Errorf("value after configuration request = %04b, want unchanged 0001", v)
	}

	// ...but the new Direction took effect: with every pin now an input, a
	// subsequent write has no output pin left to drive.
	if got := writeAndGetValue(t, ep, root, acf.EVTSelector0, 0b1111); got != 0b0001 {
		t.Errorf("value after post-reconfigure write = %04b, want unchanged 0001", got)
	}
}

// TestWrite_EVTNonZeroWithoutPayloadRejected checks TC18 §12.9.1's general
// rule reaches this endpoint type: "If evt[2:0] ≠ 0 and no byte_msg_payload
// is present, then an error response shall be sent with the error code =
// UNSUPPORTED_CMD" (REQ-GPIO-002).
func TestWrite_EVTNonZeroWithoutPayloadRejected(t *testing.T) {
	ep, root := newConfiguredEndpoint(t, gpio.Config{PinCount: 4, Direction: 0b1111})

	for _, sel := range []acf.EVTSelector{
		acf.EVTSelector1, acf.EVTSelector2, acf.EVTSelector3,
		acf.EVTSelector5, acf.EVTSelector6, acf.EVTSelector7,
	} {
		req := acf.Message{
			Kind: acf.KindShort, ByteBusID: avtp.ByteBusID(1),
			EVT: uint8(sel), Control: acf.FlagWrite,
		}
		if _, err := ep.HandleRequest(root, req); !errors.Is(err, acf.ErrEVTMissingPayload) {
			t.Errorf("evt[2:0]=%03b with empty body: err = %v, want acf.ErrEVTMissingPayload", sel, err)
		}
	}
}

// TestWrite_BodyMustBeExactlyFourBytes checks §13.7.4.1's explicit length
// rule: "A request not having exactly four bytes is rejected and an error
// response with error code = INVALID_PARAMETER will be sent." The five-byte
// case matters most — that was this package's own body shape through
// v7.0.0, when it carried an extra in-band selector byte (REQ-GPIO-002).
func TestWrite_BodyMustBeExactlyFourBytes(t *testing.T) {
	ep, root := newConfiguredEndpoint(t, gpio.Config{PinCount: 4, Direction: 0b1111})

	tests := []struct {
		name string
		body []byte
		want error
	}{
		{"empty", []byte{}, gpio.ErrShortBuffer},
		{"three bytes", []byte{0, 0, 0}, gpio.ErrShortBuffer},
		{"five bytes (the pre-v8 selector+operand shape)", []byte{1, 0, 0, 0, 1}, gpio.ErrTrailingBytes},
	}
	for _, tt := range tests {
		req := acf.Message{
			Kind: acf.KindShort, ByteBusID: avtp.ByteBusID(1),
			Control: acf.FlagWrite, Body: tt.body,
		}
		if _, err := ep.HandleRequest(root, req); !errors.Is(err, tt.want) {
			t.Errorf("%s: err = %v, want %v", tt.name, err, tt.want)
		}
	}
}
