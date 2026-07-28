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
// (avtp.StreamID, avtp.Message) (avtp.Message, error) every other endpoint
// type does, so it drops into request.Dispatcher unmodified.
//
// # Scope
//
// One MDIO endpoint is minimal pass-through management-interface access —
// matching server/types.go's two-signal MDC/MDIO list (clock plus the
// bidirectional data line an IEEE 802.3 Clause 22/45-style transaction
// drives). Per ROADMAP.md Milestone 51, this package does no PHY-specific
// interpretation of register content: it forwards an addressed
// register-read or register-write access through to whatever real MDIO
// interface is behind it (see Endpoint.SetTransport), the same "pass raw
// addressed access through, don't decode the PHY's own register semantics"
// posture i2c/doc.go's Scope section already establishes for its own
// address-opaque transfer body.
//
// Addressing follows the request's selected AddressMode: ModeClause22
// carries a 5-bit PHY address and a 5-bit register address (the original,
// simpler IEEE 802.3 Clause 22 frame shape); ModeClause45 additionally
// carries a 5-bit device (MMD) address and widens the register address to
// 16 bits, per Clause 45's extended addressing. Both modes may coexist on
// one physical MDIO bus targeting different PHYs, so AddressMode is
// selected per request (see Request), not fixed at Configure time.
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
// answers the plain, unconditional avtp.Message request shape only.
// Compound/triggered/chained/timed request variants are provided by
// wrapping this package's Endpoint (a request.Handler) in a
// request.Dispatcher exactly as every other endpoint type is, rather than
// by this package decoding them itself.
//
// # A note on spec fidelity (Guiding Principle 10)
//
// The TC18 specification PDF is confidential to OPEN Alliance members. This
// package was built from a behavioral description of an MDIO pass-through
// endpoint, not from the primary spec text. Its exact register/request byte
// layouts (Config's field order/widths, Request's field order and bit
// widths) are this implementation's own reasoned, self-consistent encoding
// rather than a verified transcription of the published byte assignments —
// the same open-item posture avtp/doc.go, server/doc.go, and i2c/doc.go
// document for their own packages, pending confirmation against a public
// interoperability reference. The Clause 22/45 addressing field widths
// themselves (5-bit PHY/device address, 5-bit or 16-bit register address)
// reflect the publicly documented IEEE 802.3 MDIO frame formats, not any
// TC18-specific text.
package mdio

//fusa:req REQ-MDIO-001
//fusa:req REQ-MDIO-002
//fusa:req REQ-MDIO-003
//fusa:req REQ-MDIO-004
//fusa:req REQ-MDIO-005
//fusa:req REQ-MDIO-006
//fusa:req REQ-MDIO-007
//fusa:req REQ-MDIO-008
