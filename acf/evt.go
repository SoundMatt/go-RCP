package acf

import "encoding/binary"

// ── TC18 §13.5 Table 30: endpoint-specific usage of the evt field ──────────
//
// The evt field of the request-descriptor header (Message.EVT) is four bits
// wide. Its top bit, evt[3], is the request-acknowledge flag ("evt[3]=1
// requests acknowledge", §13.5). Its bottom three bits, evt[2:0], are the
// write-semantic selector / config-vs-data discriminator: "event bits
// evt[2:0] are used to control the usage of the byte_msg_payload" (§13.5),
// with the exact per-endpoint-type meaning fixed by Table 30.
//
// Table 30 groups all thirteen endpoint types into exactly three rows, and
// this file models those three rows as EVTClass values. Every endpoint-type
// package declares which row governs it and calls Message.EVTDisposition,
// rather than each package re-deriving Table 30 for itself:
//
//	Table 30 row                                    EVTClass
//	───────────────────────────────────────────────────────────────────────
//	SPI                                             EVTClassChannelSelect
//	ADC, PWM_IN, I²C, LIN, CAN, UART, ISELED, MDIO  EVTClassConfigOnly
//	GPIO, PWM_OUT                                   EVTClassArithmetic
//
// The RC Server endpoint (EP0) and the wakeup-control endpoint do not appear
// in Table 30 at all and therefore have no EVTClass: their evt handling is
// not endpoint-payload-selector-shaped, so they are deliberately not forced
// through this mechanism.

const (
	// EVTAckRequestBit is evt[3], the request-acknowledge flag: "evt[3] is
	// used to request an acknowledge. I.e. evt[3]=1 requests acknowledge."
	// (§13.5). It is orthogonal to everything else in this file — every
	// function here masks it off before consulting Table 30.
	EVTAckRequestBit uint8 = 0x08

	// EVTSelectorMask isolates evt[2:0], the sub-field Table 30 governs.
	EVTSelectorMask uint8 = 0x07
)

// EVTSelector is the raw 3-bit evt[2:0] value, before Table 30's
// endpoint-type-specific interpretation is applied to it. It exists so
// callers and test tables can name a selector unambiguously (EVTSelector5)
// instead of passing a bare integer whose width and position are implicit.
type EVTSelector uint8

// The eight representable evt[2:0] values. Their meaning is entirely
// endpoint-type-specific — see EVTClass and ClassifyEVT.
const (
	EVTSelector0 EVTSelector = iota // 000b
	EVTSelector1                    // 001b
	EVTSelector2                    // 010b
	EVTSelector3                    // 011b
	EVTSelector4                    // 100b
	EVTSelector5                    // 101b
	EVTSelector6                    // 110b
	EVTSelector7                    // 111b

	evtSelectorCount // sentinel; keep last
)

// Valid reports whether s fits the 3-bit evt[2:0] wire field. A selector
// obtained from Message.EVTSelector is always valid by construction (it is
// masked); this exists for a caller that builds one from an arbitrary
// integer.
func (s EVTSelector) Valid() bool { return s < evtSelectorCount }

// EVTSelector returns evt[2:0] — the write-semantic selector /
// config-vs-data discriminator Table 30 governs — with the evt[3]
// request-acknowledge bit masked off.
func (m Message) EVTSelector() EVTSelector {
	return EVTSelector(m.EVT & EVTSelectorMask)
}

// EVTAckRequested reports whether evt[3] is set, i.e. whether this request
// asks the addressed endpoint to send an acknowledge (§13.5).
func (m Message) EVTAckRequested() bool {
	return m.EVT&EVTAckRequestBit != 0
}

// EVTClass names one of the three endpoint-type groupings TC18 §13.5 Table
// 30 defines. An endpoint-type package declares its own class once (e.g.
// gpio.EVTClass) and hands it to Message.EVTDisposition for every request it
// answers.
type EVTClass uint8

const (
	// EVTClassChannelSelect is Table 30's SPI row: evt[2:0] = 000b..101b
	// "selects channel 0 … 5 / the interface settings are to be applied
	// according to this selection / the CSN pin assigned to this selection
	// is to be asserted"; 110b is "reserved – request to be rejected with
	// error code = UNSUPPORTED_CMD"; 111b is the configuration-change
	// request.
	EVTClassChannelSelect EVTClass = iota

	// EVTClassConfigOnly is Table 30's ADC / PWM_IN / I²C / LIN / CAN /
	// UART / ISELED / MDIO row: the only evt[2:0] value with a defined
	// payload-routing meaning is 111b (configuration change). See
	// ClassifyEVT's doc comment for how this implementation resolves that
	// row's 000b entry, which the specification states inconsistently.
	EVTClassConfigOnly

	// EVTClassArithmetic is Table 30's GPIO / PWM_OUT row: evt[2:0] selects
	// how the byte_msg_payload combines with the current interface status
	// (set / OR / AND / XOR / saturating add / saturating subtract), with
	// 100b reserved and 111b the configuration-change request. See
	// EVTWriteOp.
	EVTClassArithmetic

	evtClassCount // sentinel; keep last
)

// Valid reports whether c is one of Table 30's three defined endpoint-type
// rows.
func (c EVTClass) Valid() bool { return c < evtClassCount }

// String renders c for logs and test failure messages.
func (c EVTClass) String() string {
	switch c {
	case EVTClassChannelSelect:
		return "channel-select (Table 30 SPI row)"
	case EVTClassConfigOnly:
		return "config-only (Table 30 ADC/PWM_IN/I2C/LIN/CAN/UART/ISELED/MDIO row)"
	case EVTClassArithmetic:
		return "arithmetic (Table 30 GPIO/PWM_OUT row)"
	default:
		return "?"
	}
}

// EVTAction is what Table 30 requires be done with a request's
// byte_msg_payload, once evt[2:0] has been interpreted under an EVTClass. A
// reserved selector is not an EVTAction: ClassifyEVT reports it as
// ErrEVTReserved instead, since the mandated response is to reject the
// request rather than to do anything with its payload.
type EVTAction uint8

const (
	// EVTActionInterface means the byte_msg_payload is destined for the
	// physical interface the endpoint drives — directly (Table 30's SPI and
	// GPIO/PWM_OUT 000b entries), or combined with the current interface
	// status per EVTDisposition.WriteOp (the GPIO/PWM_OUT arithmetic
	// entries).
	EVTActionInterface EVTAction = iota

	// EVTActionConfigure is evt[2:0] = 111b, identical in all three Table
	// 30 rows: "The byte_msg_payload is not presented to the interface but
	// used to change the configuration of the endpoint (see 12.7.1)." An
	// endpoint that receives this must NOT drive its bus/pins/interface
	// with the payload — see DecodeConfigRequestBody for the §12.7.1 body
	// shape it carries instead.
	EVTActionConfigure
)

// String renders a for logs and test failure messages.
func (a EVTAction) String() string {
	switch a {
	case EVTActionInterface:
		return "present payload at interface"
	case EVTActionConfigure:
		return "change endpoint configuration (TC18 §12.7.1)"
	default:
		return "?"
	}
}

// EVTWriteOp is the combining rule Table 30's GPIO/PWM_OUT row assigns to
// evt[2:0]. Each constant's numeric value is deliberately its own evt[2:0]
// encoding, not a locally invented sequence — hence the gap at 4 (100b),
// which that row reserves.
type EVTWriteOp uint8

const (
	// EVTWriteSet is 000b: "The byte_msg_payload is presented at the
	// interface."
	EVTWriteSet EVTWriteOp = 0

	// EVTWriteOr is 001b: "The 'byte_msg_payload' bitwise OR 'current
	// interface status' is written to the interface."
	EVTWriteOr EVTWriteOp = 1

	// EVTWriteAnd is 010b: "The 'byte_msg_payload' bitwise AND 'current
	// interface status' is written to the interface."
	EVTWriteAnd EVTWriteOp = 2

	// EVTWriteXor is 011b: "The 'byte_msg_payload' bitwise XOR 'current
	// interface status' is written to the interface."
	EVTWriteXor EVTWriteOp = 3

	// EVTWriteAddSaturating is 101b: "The 'byte_msg_payload' plus 'current
	// interface status' is written to the interface." Saturating, per the
	// note directly below Table 30: "While doing additions and subtractions
	// neither overflows nor wrap-arounds shall occur. The values are
	// saturated at 0x0000 on the low side and 0xFFFF at the high side."
	EVTWriteAddSaturating EVTWriteOp = 5

	// EVTWriteSubSaturating is 110b: "'byte_msg_payload' minus 'current
	// interface status' is written as is to interface." Saturating, per the
	// same note.
	//
	// Note the operand order: the specification's normative sentence
	// subtracts the CURRENT STATUS FROM THE PAYLOAD, not the other way
	// round. Table 30's parenthetical example for this row ("this can be
	// used to decrease the duty cycle of PWM_out") reads more naturally as
	// the opposite order, but the example is illustrative and the sentence
	// is normative — and its "as is" wording is emphatic about taking the
	// stated expression literally — so ApplyEVTWriteOp implements payload
	// minus current. See ApplyEVTWriteOp.
	EVTWriteSubSaturating EVTWriteOp = 6
)

// Valid reports whether op is one of the six combining rules Table 30's
// GPIO/PWM_OUT row defines (i.e. excludes the reserved 100b slot and the
// 111b configuration-change slot, which is an EVTAction rather than a
// combining rule).
func (op EVTWriteOp) Valid() bool {
	switch op {
	case EVTWriteSet, EVTWriteOr, EVTWriteAnd, EVTWriteXor,
		EVTWriteAddSaturating, EVTWriteSubSaturating:
		return true
	default:
		return false
	}
}

// String renders op for logs and test failure messages.
func (op EVTWriteOp) String() string {
	switch op {
	case EVTWriteSet:
		return "set"
	case EVTWriteOr:
		return "or"
	case EVTWriteAnd:
		return "and"
	case EVTWriteXor:
		return "xor"
	case EVTWriteAddSaturating:
		return "add (saturating)"
	case EVTWriteSubSaturating:
		return "subtract (saturating)"
	default:
		return "?"
	}
}

// EVTDisposition is one request's evt[2:0] value decoded against a single
// Table 30 row: what to do with the byte_msg_payload, plus whichever
// row-specific parameter that row encodes in the same three bits.
type EVTDisposition struct {
	// Action is what Table 30 says to do with the payload.
	Action EVTAction

	// Channel is the chip-select channel evt[2:0] selects, meaningful only
	// for EVTClassChannelSelect with Action == EVTActionInterface. It is
	// always in [0, 5] — Table 30's SPI row reserves 110b and assigns 111b
	// to configuration.
	Channel uint8

	// WriteOp is the combining rule evt[2:0] selects, meaningful only for
	// EVTClassArithmetic with Action == EVTActionInterface.
	WriteOp EVTWriteOp
}

// ClassifyEVT decodes evt (the full 4-bit field; evt[3] is masked off) under
// Table 30's row named by class.
//
// It returns ErrEVTReserved for a selector that row marks reserved, which
// every such Table 30 entry requires be answered with an error response
// carrying error code UNSUPPORTED_CMD, and ErrEVTUnknownClass for a class
// value that is not one of the three defined rows.
//
// # A documented deviation: Table 30's config-only row at 000b
//
// Table 30's ADC / PWM_IN / I²C / LIN / CAN / UART / ISELED / MDIO row reads
// "000b to 110b: reserved – request to be rejected with error code =
// UNSUPPORTED_CMD". Read literally that would reject evt[2:0] = 000b, which
// contradicts the rest of the same specification and would make eight of the
// thirteen endpoint types incapable of carrying any traffic at all:
//
//   - §13.7.9's Figure 33, "RC Client sends a standard read request", shows a
//     conformant ADC request whose evt bits are all zero.
//   - §13.7.7.3 states that for I²C "The byte msg payload is the I2C payload
//     including the address", i.e. an ordinary request's payload is
//     presented at the bus — which cannot happen if every such request is
//     rejected.
//   - §13.5's own framing is that "event bits evt[2:0] are used to control
//     the usage of the byte_msg_payload"; 000b is the absence of any special
//     control, and every other Table 30 row assigns 000b the plain
//     "presented at the interface" meaning.
//
// This implementation therefore treats 000b in that row as
// EVTActionInterface (the payload is presented at the interface as normal)
// and rejects 001b through 110b as reserved. That is the only reading under
// which the specification is self-consistent; it is nonetheless a deliberate
// departure from Table 30's literal text and is called out here, in
// EVTClassConfigOnly's doc comment, and in each affected endpoint package's
// doc.go rather than left implicit.
func ClassifyEVT(class EVTClass, evt uint8) (EVTDisposition, error) {
	sel := EVTSelector(evt & EVTSelectorMask)

	// evt[2:0] = 111b is the configuration-change request in all three
	// Table 30 rows, identically worded.
	if sel == EVTSelector7 {
		if !class.Valid() {
			return EVTDisposition{}, ErrEVTUnknownClass
		}
		return EVTDisposition{Action: EVTActionConfigure}, nil
	}

	switch class {
	case EVTClassChannelSelect:
		// "000b to 101b: selects channel 0 … 5"; "110b: reserved".
		if sel == EVTSelector6 {
			return EVTDisposition{}, ErrEVTReserved
		}
		return EVTDisposition{Action: EVTActionInterface, Channel: uint8(sel)}, nil

	case EVTClassConfigOnly:
		// See the deviation note above: 000b is the plain
		// present-at-the-interface case, 001b..110b are reserved.
		if sel != EVTSelector0 {
			return EVTDisposition{}, ErrEVTReserved
		}
		return EVTDisposition{Action: EVTActionInterface}, nil

	case EVTClassArithmetic:
		// "100b: reserved – request shall be ignored and an err-response
		// with error code = UNSUPPORTED_CMD shall be sent."
		if sel == EVTSelector4 {
			return EVTDisposition{}, ErrEVTReserved
		}
		op := EVTWriteOp(sel)
		if !op.Valid() { // unreachable: 000b-110b minus 100b are all valid
			return EVTDisposition{}, ErrEVTReserved
		}
		return EVTDisposition{Action: EVTActionInterface, WriteOp: op}, nil

	default:
		return EVTDisposition{}, ErrEVTUnknownClass
	}
}

// CheckEVTPayloadPresence implements TC18 §12.9.1's general, endpoint-type-
// independent rule, stated in the "Handling of requests" section rather than
// in any one endpoint's chapter: "If evt[2:0] ≠ 0 and no byte_msg_payload is
// present, then an error response shall be sent with the error code =
// UNSUPPORTED_CMD".
//
// It returns ErrEVTMissingPayload when that condition holds and nil
// otherwise. payloadLen is the length of the request's byte_msg_payload
// (Message.Body), excluding padding — Message.Pad is not part of the
// payload.
func CheckEVTPayloadPresence(evt uint8, payloadLen int) error {
	if evt&EVTSelectorMask != 0 && payloadLen == 0 {
		return ErrEVTMissingPayload
	}
	return nil
}

// EVTDisposition decodes this request's evt field under Table 30's row named
// by class, having first applied §12.9.1's general payload-presence rule. It
// is the single entry point every endpoint-type package calls; the
// underlying CheckEVTPayloadPresence and ClassifyEVT are exported separately
// so the transport layer can apply the general rule uniformly before
// dispatch (see udp.Router.Route) and so each rule can be tested in
// isolation.
func (m Message) EVTDisposition(class EVTClass) (EVTDisposition, error) {
	if err := CheckEVTPayloadPresence(m.EVT, len(m.Body)); err != nil {
		return EVTDisposition{}, err
	}
	return ClassifyEVT(class, m.EVT)
}

// ApplyEVTWriteOp combines payload with current under op, as Table 30's
// GPIO/PWM_OUT row defines, and returns the value to write to the interface.
//
// saturateAt is the high-side saturation bound for the arithmetic ops. Table
// 30's note ("The values are saturated at 0x0000 on the low side and 0xFFFF
// at the high side") states the bound for the 16-bit fields PWM_OUT's
// payload is made of; a caller whose interface word is a different width
// passes its own representable maximum instead (gpio passes its active-pin
// mask, pwm passes 0xFFFF per 16-bit field). The low-side bound is always
// zero.
//
// The bitwise ops are returned unmasked — restricting the result to the
// pins/bits an endpoint actually drives is the caller's business, since only
// the caller knows its own direction/active masks.
//
// EVTWriteSubSaturating computes payload - current, not current - payload;
// see its own doc comment for why, and for the specification wording that
// choice rests on.
func ApplyEVTWriteOp(op EVTWriteOp, payload, current, saturateAt uint32) (uint32, error) {
	switch op {
	case EVTWriteSet:
		return payload, nil
	case EVTWriteOr:
		return payload | current, nil
	case EVTWriteAnd:
		return payload & current, nil
	case EVTWriteXor:
		return payload ^ current, nil
	case EVTWriteAddSaturating:
		sum := uint64(payload) + uint64(current)
		if sum > uint64(saturateAt) {
			return saturateAt, nil
		}
		return uint32(sum), nil
	case EVTWriteSubSaturating:
		if payload <= current {
			return 0, nil
		}
		diff := payload - current
		if diff > saturateAt {
			return saturateAt, nil
		}
		return diff, nil
	default:
		return 0, ErrEVTReserved
	}
}

// EVTConfigStartAddrLen is the width of the "relative Register start address
// in EP_func" field that leads a configuration request's byte_msg_payload
// (TC18 §12.7.1 Figure 18). Register pointers elsewhere in the register map
// are 16 bits wide (see §12.7's svr_*_cfg_ptr rows), and Figure 18 shows the
// start address occupying the leading half of the payload's first quadlet.
const EVTConfigStartAddrLen = 2

// EncodeConfigRequestBody builds the byte_msg_payload of a §12.7.1
// configuration request (the evt[2:0] = 111b shape): the big-endian 16-bit
// register start address, relative to the addressed endpoint's EP_func
// block, followed by the configuration data to write there.
func EncodeConfigRequestBody(startAddr uint16, data []byte) []byte {
	buf := make([]byte, EVTConfigStartAddrLen+len(data))
	binary.BigEndian.PutUint16(buf[:EVTConfigStartAddrLen], startAddr)
	copy(buf[EVTConfigStartAddrLen:], data)
	return buf
}

// DecodeConfigRequestBody splits a §12.7.1 configuration request's
// byte_msg_payload into its register start address and configuration data.
// It never panics on malformed input, and returns ErrShortConfigRequest for
// a body too short to hold the start address at all. The returned slice
// aliases b — a caller that retains it beyond b's lifetime must copy it.
func DecodeConfigRequestBody(b []byte) (startAddr uint16, data []byte, err error) {
	if len(b) < EVTConfigStartAddrLen {
		return 0, nil, ErrShortConfigRequest
	}
	return binary.BigEndian.Uint16(b[:EVTConfigStartAddrLen]), b[EVTConfigStartAddrLen:], nil
}
