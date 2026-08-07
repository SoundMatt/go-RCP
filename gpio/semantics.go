package gpio

import "github.com/SoundMatt/go-RCP/v9/acf"

// applyValue combines the request's operand (the byte_msg_payload) with the
// endpoint's current pin-value word under op — TC18 §13.5 Table 30's
// GPIO/PWM_OUT row — and masks the result to active, the endpoint's
// active-pin mask.
//
// The combining rules themselves live in acf.ApplyEVTWriteOp, shared with
// pwm (Table 30 gives GPIO and PWM_OUT one row, not two); this function only
// supplies GPIO's own saturation bound and active-pin masking.
//
// Saturation bound: Table 30's note fixes the high side at 0xFFFF, which is
// the width of the 16-bit fields PWM_OUT's payload is made of. A GPIO
// endpoint's interface word is instead PinCount bits wide, so its high-side
// bound is the active-pin mask — the largest value the interface can
// actually represent — and masking the bitwise results to the same mask
// keeps every combining rule confined to declared pins.
func applyValue(current, operand uint32, op acf.EVTWriteOp, active uint32) (uint32, error) {
	v, err := acf.ApplyEVTWriteOp(op, operand, current, active)
	if err != nil {
		return 0, err
	}
	return v & active, nil
}
