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
// A write request combines its operand bitmask with the endpoint's current
// output state under one of eight WriteSemantic values — Replace, Or, And,
// AndNot, Xor, SaturatingAdd, SaturatingSubtract, and Reconfigure (an escape
// hatch that replaces Config.Direction itself rather than the pin value) —
// and a read request returns the current value of every pin, regardless of
// direction. See WriteSemantic and applyValue.
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
// This package's register/request byte layout for two specific design
// choices below has not yet been independently re-verified against the
// governing OPEN Alliance TC18 Remote Control Protocol Specification; see
// the ecosystem audit tracking issues for known gaps, the same open-item
// posture avtp/doc.go and server/doc.go document for their own packages:
//
//   - ROADMAP.md's own milestone text names seven combining rules (replace,
//     Or, And, Xor, saturating add, saturating subtract, and the
//     reconfiguration escape hatch) while stating the endpoint supports
//     eight write-semantics in total. This package resolves that count by
//     adding AndNot (a bitwise clear-mask, the natural complement to Or's
//     set-mask in a set/clear register pair) as the eighth. This is this
//     implementation's own reasoned choice to fill an ambiguous count, not a
//     confirmed reading of the source specification, and should be revisited
//     against a public interoperability reference before this encoding is
//     treated as final.
//   - Saturating add/subtract are defined over the endpoint's whole pin-value
//     word, clamped to the active-pin mask ((1<<PinCount)-1) rather than
//     wrapping on overflow/underflow — a reasoned interpretation of
//     "saturating" consistent with how every other repo package treats
//     saturating arithmetic (see e.g. ratelimit's token-bucket clamp), not a
//     verified transcription of the spec's own wording.
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
