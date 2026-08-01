//fusa:test REQ-EVT-001
//fusa:test REQ-EVT-002
//fusa:test REQ-EVT-003
//fusa:test REQ-EVT-004
//fusa:test REQ-EVT-005
//fusa:test REQ-EVT-006
//fusa:test REQ-EVT-007

package acf_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/acf"
)

// ── REQ-EVT-001..007: TC18 §13.5 Table 30 / §12.9.1 / §12.7.1 ─────────────
//
// Every expectation below is transcribed from the specification text quoted
// in the test that asserts it, not from any implementation's behaviour.

// TestEVTSelectorSplitsAckBit checks evt[3] and evt[2:0] are read as the two
// independent fields §13.5 defines: "evt[3] is used to request an
// acknowledge. I.e. evt[3]=1 requests acknowledge", while "event bits
// evt[2:0] are used to control the usage of the byte_msg_payload"
// (REQ-EVT-001).
func TestEVTSelectorSplitsAckBit(t *testing.T) {
	for evt := 0; evt < 16; evt++ {
		m := acf.Message{EVT: uint8(evt)}
		wantSel := acf.EVTSelector(evt & 0x07)
		wantAck := evt&0x08 != 0
		if got := m.EVTSelector(); got != wantSel {
			t.Errorf("EVT=%#x: EVTSelector() = %d, want %d", evt, got, wantSel)
		}
		if got := m.EVTAckRequested(); got != wantAck {
			t.Errorf("EVT=%#x: EVTAckRequested() = %v, want %v", evt, got, wantAck)
		}
		if !wantSel.Valid() {
			t.Errorf("EVT=%#x: selector %d reported invalid", evt, wantSel)
		}
	}
}

// TestClassifyEVT_ChannelSelectRow checks Table 30's SPI row: "000b to 101b
// — selects channel 0 … 5"; "110b — reserved – request to be rejected with
// error code = UNSUPPORTED_CMD"; "111b — The byte_msg_payload is not
// presented to the interface but used to change the configuration of the
// endpoint" (REQ-EVT-002).
func TestClassifyEVT_ChannelSelectRow(t *testing.T) {
	for sel := 0; sel <= 5; sel++ {
		got, err := acf.ClassifyEVT(acf.EVTClassChannelSelect, uint8(sel))
		if err != nil {
			t.Fatalf("evt[2:0]=%03b: unexpected error %v", sel, err)
		}
		if got.Action != acf.EVTActionInterface || int(got.Channel) != sel {
			t.Errorf("evt[2:0]=%03b: got %+v, want interface action on channel %d", sel, got, sel)
		}
	}

	if _, err := acf.ClassifyEVT(acf.EVTClassChannelSelect, 0b110); !errors.Is(err, acf.ErrEVTReserved) {
		t.Errorf("evt[2:0]=110b: err = %v, want ErrEVTReserved", err)
	}

	got, err := acf.ClassifyEVT(acf.EVTClassChannelSelect, 0b111)
	if err != nil || got.Action != acf.EVTActionConfigure {
		t.Errorf("evt[2:0]=111b: got (%+v, %v), want configure action", got, err)
	}
}

// TestClassifyEVT_ConfigOnlyRow checks Table 30's ADC / PWM_IN / I²C / LIN /
// CAN / UART / ISELED / MDIO row, including this implementation's documented
// reading of its 000b entry: 000b presents the payload at the interface as
// normal (§13.7.9 Figure 33 shows a conformant ADC request with evt=0000b,
// and §13.7.7.3 has the I²C payload reaching the bus), 001b through 110b are
// reserved, and 111b is the §12.7.1 configuration change (REQ-EVT-003).
func TestClassifyEVT_ConfigOnlyRow(t *testing.T) {
	got, err := acf.ClassifyEVT(acf.EVTClassConfigOnly, 0b000)
	if err != nil || got.Action != acf.EVTActionInterface {
		t.Errorf("evt[2:0]=000b: got (%+v, %v), want interface action", got, err)
	}

	for sel := 1; sel <= 6; sel++ {
		if _, resErr := acf.ClassifyEVT(acf.EVTClassConfigOnly, uint8(sel)); !errors.Is(resErr, acf.ErrEVTReserved) {
			t.Errorf("evt[2:0]=%03b: err = %v, want ErrEVTReserved", sel, resErr)
		}
	}

	got, err = acf.ClassifyEVT(acf.EVTClassConfigOnly, 0b111)
	if err != nil || got.Action != acf.EVTActionConfigure {
		t.Errorf("evt[2:0]=111b: got (%+v, %v), want configure action", got, err)
	}
}

// TestClassifyEVT_ArithmeticRow checks Table 30's GPIO / PWM_OUT row
// selector-by-selector, including the reserved 100b slot: "100b — reserved –
// request shall be ignored and an err-response with error code =
// UNSUPPORTED_CMD shall be sent" (REQ-EVT-004).
func TestClassifyEVT_ArithmeticRow(t *testing.T) {
	tests := []struct {
		sel  uint8
		want acf.EVTWriteOp
	}{
		{0b000, acf.EVTWriteSet},
		{0b001, acf.EVTWriteOr},
		{0b010, acf.EVTWriteAnd},
		{0b011, acf.EVTWriteXor},
		{0b101, acf.EVTWriteAddSaturating},
		{0b110, acf.EVTWriteSubSaturating},
	}
	for _, tt := range tests {
		got, err := acf.ClassifyEVT(acf.EVTClassArithmetic, tt.sel)
		if err != nil {
			t.Fatalf("evt[2:0]=%03b: unexpected error %v", tt.sel, err)
		}
		if got.Action != acf.EVTActionInterface || got.WriteOp != tt.want {
			t.Errorf("evt[2:0]=%03b: got %+v, want interface action with op %v", tt.sel, got, tt.want)
		}
		if !tt.want.Valid() {
			t.Errorf("op %v reported invalid", tt.want)
		}
	}

	if _, err := acf.ClassifyEVT(acf.EVTClassArithmetic, 0b100); !errors.Is(err, acf.ErrEVTReserved) {
		t.Errorf("evt[2:0]=100b: err = %v, want ErrEVTReserved", err)
	}

	got, err := acf.ClassifyEVT(acf.EVTClassArithmetic, 0b111)
	if err != nil || got.Action != acf.EVTActionConfigure {
		t.Errorf("evt[2:0]=111b: got (%+v, %v), want configure action", got, err)
	}
}

// TestClassifyEVT_IgnoresAckBitAndRejectsUnknownClass checks evt[3] never
// changes a classification (it is an orthogonal acknowledge request), and
// that an EVTClass outside Table 30's three rows is a caller error
// (REQ-EVT-001, REQ-EVT-002).
func TestClassifyEVT_IgnoresAckBitAndRejectsUnknownClass(t *testing.T) {
	for sel := 0; sel < 8; sel++ {
		plain, plainErr := acf.ClassifyEVT(acf.EVTClassArithmetic, uint8(sel))
		acked, ackedErr := acf.ClassifyEVT(acf.EVTClassArithmetic, uint8(sel)|acf.EVTAckRequestBit)
		if plain != acked {
			t.Errorf("evt[2:0]=%03b: ack bit changed the disposition: %+v vs %+v", sel, plain, acked)
		}
		if !errors.Is(ackedErr, plainErr) {
			t.Errorf("evt[2:0]=%03b: ack bit changed the error: %v vs %v", sel, plainErr, ackedErr)
		}
	}

	if _, err := acf.ClassifyEVT(acf.EVTClass(99), 0); !errors.Is(err, acf.ErrEVTUnknownClass) {
		t.Errorf("unknown class: err = %v, want ErrEVTUnknownClass", err)
	}
	if _, err := acf.ClassifyEVT(acf.EVTClass(99), 0b111); !errors.Is(err, acf.ErrEVTUnknownClass) {
		t.Errorf("unknown class at 111b: err = %v, want ErrEVTUnknownClass", err)
	}
}

// TestCheckEVTPayloadPresence checks §12.9.1's general rule: "If evt[2:0] ≠ 0
// and no byte_msg_payload is present, then an error response shall be sent
// with the error code = UNSUPPORTED_CMD" — and, by omission, that evt[2:0] =
// 0 with no payload is perfectly legal (§13.7.9's ADC read request is
// exactly that shape) (REQ-EVT-005).
func TestCheckEVTPayloadPresence(t *testing.T) {
	if err := acf.CheckEVTPayloadPresence(0, 0); err != nil {
		t.Errorf("evt[2:0]=0, no payload: err = %v, want nil", err)
	}
	if err := acf.CheckEVTPayloadPresence(acf.EVTAckRequestBit, 0); err != nil {
		t.Errorf("evt=1000b (ack only), no payload: err = %v, want nil", err)
	}
	for sel := 1; sel < 8; sel++ {
		if err := acf.CheckEVTPayloadPresence(uint8(sel), 0); !errors.Is(err, acf.ErrEVTMissingPayload) {
			t.Errorf("evt[2:0]=%03b, no payload: err = %v, want ErrEVTMissingPayload", sel, err)
		}
		if err := acf.CheckEVTPayloadPresence(uint8(sel), 1); err != nil {
			t.Errorf("evt[2:0]=%03b, 1-byte payload: err = %v, want nil", sel, err)
		}
	}

	// Message.EVTDisposition applies the same rule before Table 30.
	m := acf.Message{EVT: 0b001}
	if _, err := m.EVTDisposition(acf.EVTClassArithmetic); !errors.Is(err, acf.ErrEVTMissingPayload) {
		t.Errorf("EVTDisposition with empty body: err = %v, want ErrEVTMissingPayload", err)
	}
	m.Body = []byte{0, 0, 0, 1}
	if d, err := m.EVTDisposition(acf.EVTClassArithmetic); err != nil || d.WriteOp != acf.EVTWriteOr {
		t.Errorf("EVTDisposition with body = (%+v, %v), want OR op", d, err)
	}
}

// TestApplyEVTWriteOp checks each combining rule Table 30's GPIO/PWM_OUT row
// defines, the saturation note ("neither overflows nor wrap-arounds shall
// occur ... saturated at 0x0000 on the low side and 0xFFFF at the high
// side"), and the operand order of 110b, whose normative sentence is
// "'byte_msg_payload' minus 'current interface status' is written as is to
// interface" (REQ-EVT-006).
func TestApplyEVTWriteOp(t *testing.T) {
	const payload, current = uint32(0b1100), uint32(0b1010)

	tests := []struct {
		name string
		op   acf.EVTWriteOp
		want uint32
	}{
		{"set", acf.EVTWriteSet, 0b1100},
		{"or", acf.EVTWriteOr, 0b1110},
		{"and", acf.EVTWriteAnd, 0b1000},
		{"xor", acf.EVTWriteXor, 0b0110},
		{"add", acf.EVTWriteAddSaturating, 0b1100 + 0b1010},
		{"sub (payload minus current)", acf.EVTWriteSubSaturating, 0b1100 - 0b1010},
	}
	for _, tt := range tests {
		got, err := acf.ApplyEVTWriteOp(tt.op, payload, current, 0xFFFF)
		if err != nil {
			t.Fatalf("%s: unexpected error %v", tt.name, err)
		}
		if got != tt.want {
			t.Errorf("%s: got %#b, want %#b", tt.name, got, tt.want)
		}
	}

	// High-side saturation at the caller's bound, not a wrap-around.
	if got, _ := acf.ApplyEVTWriteOp(acf.EVTWriteAddSaturating, 0xFF00, 0xFF00, 0xFFFF); got != 0xFFFF {
		t.Errorf("add saturation: got %#x, want 0xFFFF", got)
	}
	// Low-side saturation at zero, not a wrap-around to a large value.
	if got, _ := acf.ApplyEVTWriteOp(acf.EVTWriteSubSaturating, 1, 5, 0xFFFF); got != 0 {
		t.Errorf("sub saturation: got %#x, want 0", got)
	}
	// The reserved slot is not a combining rule.
	if _, err := acf.ApplyEVTWriteOp(acf.EVTWriteOp(4), 0, 0, 0xFFFF); !errors.Is(err, acf.ErrEVTReserved) {
		t.Errorf("op 100b: err = %v, want ErrEVTReserved", err)
	}
}

// TestConfigRequestBodyCodec checks §12.7.1 Figure 18's configuration
// request payload shape — a relative EP_func register start address followed
// by the configuration data — round-trips, and that the decoder never panics
// on a body too short to hold the address (REQ-EVT-007).
func TestConfigRequestBodyCodec(t *testing.T) {
	data := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	body := acf.EncodeConfigRequestBody(0x0102, data)
	if want := []byte{0x01, 0x02, 0xDE, 0xAD, 0xBE, 0xEF}; !bytes.Equal(body, want) {
		t.Fatalf("EncodeConfigRequestBody = % X, want % X", body, want)
	}

	start, got, err := acf.DecodeConfigRequestBody(body)
	if err != nil {
		t.Fatalf("DecodeConfigRequestBody: %v", err)
	}
	if start != 0x0102 || !bytes.Equal(got, data) {
		t.Errorf("DecodeConfigRequestBody = (%#x, % X), want (0x0102, % X)", start, got, data)
	}

	for _, short := range [][]byte{nil, {}, {0x01}} {
		if _, _, err := acf.DecodeConfigRequestBody(short); !errors.Is(err, acf.ErrShortConfigRequest) {
			t.Errorf("DecodeConfigRequestBody(% X): err = %v, want ErrShortConfigRequest", short, err)
		}
	}

	// A zero-length configuration payload is a valid address-only body.
	if start, got, err := acf.DecodeConfigRequestBody([]byte{0x00, 0x04}); err != nil || start != 4 || len(got) != 0 {
		t.Errorf("address-only body = (%#x, % X, %v), want (0x4, empty, nil)", start, got, err)
	}
}

func FuzzDecodeConfigRequestBody(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0x00, 0x04, 0xAA})
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _, _ = acf.DecodeConfigRequestBody(b) // must not panic
	})
}
