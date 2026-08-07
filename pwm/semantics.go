package pwm

import "github.com/SoundMatt/go-RCP/v9/acf"

// waveformSaturateAt is the high-side saturation bound TC18 §13.5 Table 30's
// note states directly: "The values are saturated at 0x0000 on the low side
// and 0xFFFF at the high side." That bound is stated for exactly this
// endpoint type's payload shape — two 16-bit values (§13.7.5.3) — so pwm
// takes it literally, where gpio (whose interface word is PinCount bits
// wide) substitutes its own active-pin mask.
const waveformSaturateAt = 0xFFFF

// applyWaveformOp combines a request's waveform payload with the endpoint's
// currently applied waveform under op — TC18 §13.5 Table 30's GPIO/PWM_OUT
// row — and returns the waveform to drive.
//
// The two 16-bit fields are combined independently rather than as one packed
// 32-bit word. For the bitwise rules the two readings are identical; for the
// arithmetic rules they are not, and the per-field reading is the one Table
// 30's saturation note describes: a 0xFFFF bound is the ceiling of a single
// 16-bit field, and a packed-word reading would let a carry out of the
// active-time field corrupt the period field, which "neither overflows nor
// wrap-arounds shall occur" plainly forbids.
func applyWaveformOp(op acf.EVTWriteOp, payloadActive, payloadPeriod, currentActive, currentPeriod uint16) (active, period uint16, err error) {
	a, err := acf.ApplyEVTWriteOp(op, uint32(payloadActive), uint32(currentActive), waveformSaturateAt)
	if err != nil {
		return 0, 0, err
	}
	p, err := acf.ApplyEVTWriteOp(op, uint32(payloadPeriod), uint32(currentPeriod), waveformSaturateAt)
	if err != nil {
		return 0, 0, err
	}
	return uint16(a), uint16(p), nil
}
