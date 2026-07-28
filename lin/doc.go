// Package lin implements the LIN endpoint type for the OPEN Alliance TC18
// Remote Control Protocol (RCP), as described by the "OPEN Alliance TC18
// Remote Control Protocol Specification v0.5.1_RC".
//
// This is one of five Phase 16 (v0.64.0) endpoint-type packages (see also
// can, iseled, mdio, wakeup) built directly on top of the server package's
// register-map substrate (ROADMAP.md Milestones 45/46) and the request
// package's conditional-request/dispatch machinery (ROADMAP.md Milestone
// 49), exactly as the six Phase 14 endpoint types (gpio, spi, i2c, uart,
// adc, pwm) already are: a LIN endpoint's functional configuration (Config)
// is read and written through
// server.Server.WriteFunctional/server.Server.ReadEndpoint, and
// Endpoint.HandleRequest implements the same request.Handler shape
// (avtp.StreamID, acf.Message) (acf.Message, error) every other endpoint
// type does, so it drops into request.Dispatcher unmodified.
//
// # Scope
//
// One LIN endpoint is commander-only (the LIN "master" role) and models a
// single bus, the same one-bus-no-sub-channel shape i2c/doc.go's Scope
// section documents for I2C, matching regmap/types.go's LIN signal list
// (TX/RX, not a per-channel selector).
//
// Per ROADMAP.md Milestone 51's explicit instruction, this package is raw
// byte pass-through only: it has no awareness whatsoever of LIN frame
// structure — no protected identifier (PID) parity computation, no
// classic/enhanced checksum computation or verification, and no
// schedule-table scheduling. A request's entire body is opaque raw bytes to
// this layer, copied onto (and, for the response, read back from) the bus
// exactly as i2c's transfer request/response already does (see
// EncodeTransferRequest/DecodeTransferRequest). This is a deliberate,
// explicitly-flagged scope boundary, not an oversight: any future LIN
// client-side logic this repo builds (e.g. a rebuilt linbr per ROADMAP.md's
// Phase 17 disposition table) must own PID/checksum/schedule-table framing
// entirely itself, layered on top of this endpoint rather than inside it.
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
// The TC18 specification PDF is confidential to OPEN Alliance members. This
// package was built from a behavioral description of a LIN commander
// endpoint, not from the primary spec text. Its exact register/request byte
// layout (Config's field order/widths) is this implementation's own
// reasoned, self-consistent encoding rather than a verified transcription
// of the published byte assignments — the same open-item posture avtp/doc.go,
// server/doc.go, and i2c/doc.go document for their own packages, pending
// confirmation against a public interoperability reference.
package lin

//fusa:req REQ-LINEP-001
//fusa:req REQ-LINEP-002
//fusa:req REQ-LINEP-003
//fusa:req REQ-LINEP-004
//fusa:req REQ-LINEP-005
//fusa:req REQ-LINEP-006
//fusa:req REQ-LINEP-007
//fusa:req REQ-LINEP-008
