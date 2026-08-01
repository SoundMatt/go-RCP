// Package mdio implements the MDIO endpoint type for the OPEN Alliance
// TC18 Remote Control Protocol (RCP), as described by the "OPEN Alliance
// TC18 Remote Control Protocol Specification v0.5.1_RC".
//
// This is one of five Phase 16 (v0.64.0) endpoint-type packages (see also
// lin, can, iseled, wakeup) built directly on top of the server package's
// register-map substrate (ROADMAP.md Milestones 45/46) and the request
// package's conditional-request/dispatch machinery (ROADMAP.md Milestone
// 49), exactly as the six Phase 14 endpoint types (gpio, spi, i2c, uart,
// adc, pwm) already are: an MDIO endpoint's functional configuration
// (Config) is read and written through
// server.Server.WriteFunctional/server.Server.ReadEndpoint, and
// Endpoint.HandleRequest implements the same request.Handler shape
// (avtp.StreamID, acf.Message) (acf.Message, error) every other endpoint
// type does, so it drops into request.Dispatcher unmodified.
//
// # Scope
//
// One MDIO endpoint is minimal pass-through management-interface access —
// matching regmap/types.go's two-signal MDC/MDIO list (clock plus the
// bidirectional data line an IEEE 802.3 Clause 22/45-style transaction
// drives). Per ROADMAP.md Milestone 51, this package does no PHY-specific
// interpretation of register content: it forwards an addressed
// register-read or register-write access through to whatever real MDIO
// interface is behind it (see Endpoint.SetTransport), the same "pass raw
// addressed access through, don't decode the PHY's own register semantics"
// posture i2c/doc.go's Scope section already establishes for its own
// address-opaque transfer body.
//
// Addressing follows the request's selected Mode — the wire format's
// 2-bit mdio_mode field: ModeMMDSingleWord and ModeMMDMultiByte each
// access a single Clause 45-style MMD (MDIO Manageable Device) register,
// as one word or as a multiple-byte access respectively, always 16 bits
// wide; ModeMMSSingleWord and ModeMMSMultiWord instead access an MMS
// (memory-mapped space) register, as one word or as a multiple
// (double-word) access respectively, 32 bits wide when the request's
// DevAddr selects MMS index 0 or 1 ("MMS0"/"MMS1") and 16 bits wide for
// every other MMS index (see Request.DataWidth). DevAddr selects which
// MMD or which MMS, depending on Mode. All four modes may be used against
// the same endpoint, so Mode is selected per request (see Request), not
// fixed at Configure time; which physical PHY a request reaches is a
// property of which endpoint (byte_bus_id) it addresses, not of a field
// within the request itself — see Request's doc comment in types.go.
//
// This endpoint type is useful even on a board with no physical MDIO pins
// wired at all: an integrated on-die PHY commonly exposes its registers
// over an internal MDIO-shaped interface with no external pins, and this
// package's Transport abstraction (see Endpoint.SetTransport) does not
// care whether the exchange it performs ever reaches a physical pin.
//
// # Explicit non-goal
//
// Per ROADMAP.md Milestone 48/51, this package's Endpoint.HandleRequest
// answers the plain, unconditional acf.Message request shape only.
// Compound/triggered/chained/timed request variants are provided by
// wrapping this package's Endpoint (a request.Handler) in a
// request.Dispatcher exactly as every other endpoint type is, rather than
// by this package decoding them itself.
//
// # A note on spec fidelity (Guiding Principle 10)
//
// The governing OPEN Alliance TC18 Remote Control Protocol Specification is
// available and normative. This package's Request/Mode addressing model —
// the four-value mdio_mode field (ModeMMDSingleWord, ModeMMDMultiByte,
// ModeMMSSingleWord, ModeMMSMultiWord) and the resulting 16-bit-vs-32-bit
// payload width selection (see Request.DataWidth) — is a verified
// transcription of that specification's MDIO addressing/access
// description, not a reasoned guess. So is the request header's wire
// layout request.go's encodeRequest/decodeRequest implement: a reserved
// byte, then a single byte packing mdio_mode and mdio_address together,
// with no further field after it (a write request's value, and a read
// response's value, occupy their own separate location — see
// EncodeWriteRequest/EncodeResponse — not a third field within the
// request header itself) — measured directly against the specification's
// own request format diagram, cross-checked against its accompanying
// prose, which describes the target MMD and the address within it as a
// single addressing concept the request carries, not two separate
// fields, rather than assumed to be one byte per logical field. An
// earlier revision of this package added a
// spurious extra 16-bit field between mdio_address and the payload,
// believing it was diagram-verified when it was not; that field is
// removed. DevAddr's meaning genuinely depends on Mode — for the two MMD
// modes it is the address of a register within an (implicitly selected)
// MMD, not a device selector; for the two MMS modes it selects which MMS,
// per the specification's own MMS0/MMS1-vs-other-MMS payload-width rule
// (see mmsWideIndexMax's doc comment in types.go). There is no separate
// PHY-address field because a target PHY is selected by which endpoint
// (byte_bus_id) a request addresses, not by a field within the request
// body — see Request's doc comment in types.go. Config's field
// order/width remains this implementation's own reasoned, self-consistent
// encoding rather than a verified transcription — the same open-item
// posture avtp/doc.go, server/doc.go, and i2c/doc.go document for their
// own packages, pending confirmation against a public interoperability
// reference.
package mdio

//fusa:req REQ-MDIO-001
//fusa:req REQ-MDIO-002
//fusa:req REQ-MDIO-003
//fusa:req REQ-MDIO-004
//fusa:req REQ-MDIO-005
//fusa:req REQ-MDIO-006
//fusa:req REQ-MDIO-007
//fusa:req REQ-MDIO-008
//fusa:req REQ-MDIO-009
