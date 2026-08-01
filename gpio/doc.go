// Package gpio implements the GPIO endpoint type for the OPEN Alliance TC18
// Remote Control Protocol (RCP), as described by the "OPEN Alliance TC18
// Remote Control Protocol Specification v0.5.1_RC".
//
// This is one of two Phase 14 (v0.60.0) endpoint-type packages (see spi for
// the other) built directly on top of the server package's register-map
// substrate (ROADMAP.md Milestones 45/46): a GPIO endpoint's functional
// configuration (Config) is read and written through
// server.Server.WriteFunctional/server.Server.ReadEndpoint exactly like any
// other endpoint's FunctionalBlock, and Endpoint.HandleRequest decodes and
// answers a plain acf.Message request the same request-descriptor header
// every endpoint type shares. GPIO and SPI were chosen to ship first because
// they have the simplest request/response payload shapes of the roughly
// dozen TC18 endpoint types, exercising the generic read/write/reconfigure
// request shape every later endpoint type reuses.
//
// # Scope
//
// One GPIO endpoint models up to MaxPins independently configured pins as a
// single bitmask: Config.Direction says which pins are output vs input,
// Config.TriggerEnable says which pins report a change/edge trigger signal.
// A read request returns the current value of every pin, regardless of
// direction. A write request carries a 4-byte operand bitmask (§13.7.4.1
// Figure 24) and nothing else; what is done with that bitmask is selected by
// the request's evt[2:0] field, not by anything in the body — see
// "# The evt field" below.
//
// # The evt field (§13.5 Table 30)
//
// GPIO shares one Table 30 row with PWM_OUT, and this package declares that
// row as EVTClass. Every request goes through acf.Message.EVTDisposition,
// the single shared implementation of Table 30 (acf/evt.go), which resolves
// evt[2:0] to:
//
//	000b  the operand is presented at the pins as is
//	001b  operand OR  current interface status
//	010b  operand AND current interface status
//	011b  operand XOR current interface status
//	100b  reserved — rejected with error code UNSUPPORTED_CMD
//	101b  operand plus  current interface status, saturating
//	110b  operand minus current interface status, saturating
//	111b  the operand is NOT presented at the pins; it is a §12.7.1
//	      configuration write into this endpoint's EP_func block
//
// Combining always affects output-direction pins only. §12.9.1's general
// rule — a non-zero evt[2:0] with no byte_msg_payload at all is answered
// with UNSUPPORTED_CMD — is applied by the same shared path.
//
// # Explicit non-goal
//
// Per ROADMAP.md Milestone 47, this package ships against the plain,
// unconditional acf.Message request kind only. Compound/triggered/chained/
// timed request variants are Phase 15's job (ROADMAP.md Milestone 49) and
// are retrofitted onto every endpoint type there, not decided here.
//
// # A note on spec fidelity (Guiding Principle 10)
//
// Two deliberate, documented interpretations remain in this package's
// arithmetic combining rules, both inherited from acf.ApplyEVTWriteOp:
//
//   - Table 30's saturation note fixes the high-side bound at 0xFFFF, the
//     width of the 16-bit fields PWM_OUT's payload is made of. A GPIO
//     endpoint's interface word is PinCount bits wide instead, so this
//     package saturates at its active-pin mask ((1<<PinCount)-1) — the
//     largest value its interface can represent — rather than at a bound
//     that would be meaningless for a 24-pin endpoint. See applyValue.
//   - Table 30's normative sentence for 110b subtracts the current interface
//     status FROM the payload, while its parenthetical example reads more
//     naturally as the opposite order. This package implements the
//     normative sentence. See acf.EVTWriteSubSaturating.
//
// Through v7.0.0 this package instead carried a one-byte write-semantic
// selector in the request body and never read evt[2:0] at all, with an
// invented eighth "AndNot" semantic occupying what Table 30 actually
// reserves. Both are gone; see CHANGELOG/ROADMAP for the breaking change.
package gpio

//fusa:req REQ-GPIO-001
//fusa:req REQ-GPIO-002
//fusa:req REQ-GPIO-003
//fusa:req REQ-GPIO-004
//fusa:req REQ-GPIO-005
//fusa:req REQ-GPIO-006
//fusa:req REQ-GPIO-007
//fusa:req REQ-GPIO-008
//fusa:req REQ-GPIO-009
//fusa:req REQ-GPIO-010
//fusa:req REQ-GPIO-011
//fusa:req REQ-GPIO-012
