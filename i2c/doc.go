// Package i2c implements the I2C endpoint type for the OPEN Alliance TC18
// Remote Control Protocol (RCP), as described by the "OPEN Alliance TC18
// Remote Control Protocol Specification v0.5.1_RC".
//
// This is one of four Phase 14 (v0.61.0) endpoint-type packages (see also
// uart, adc, pwm) built directly on top of the server package's register-map
// substrate (ROADMAP.md Milestones 45/46), the same foundation gpio and spi
// built on in Milestone 47 (v0.60.0): an I2C endpoint's functional
// configuration (Config) is read and written through
// server.Server.WriteFunctional/server.Server.ReadEndpoint exactly like any
// other endpoint's FunctionalBlock, and Endpoint.HandleRequest decodes and
// answers a plain acf.Message request the same request-descriptor header
// every endpoint type shares.
//
// # Scope
//
// One I2C endpoint is controller-only and models a single bus — unlike spi's
// up to six independently pre-configured chip-select channels, an I2C
// endpoint has no sub-opcode/channel selector at all, since a controller's
// I2C bus addresses its peripherals on the wire itself (via the transfer
// body's own leading address byte(s)) rather than through a protocol-level
// selector the way SPI's chip-select lines require. A transfer request's
// entire body — including the address byte(s) — is opaque raw bytes to this
// layer: this package does no protocol-level address parsing at all (see
// EncodeTransferRequest/DecodeTransferRequest), leaving interpretation of
// the address/read-write bit encoding to the caller. Config holds the bus's
// speed class and the minimum trailing time the controller waits after one
// transaction before starting the next. This package has no real I2C bus of
// its own to drive the exchange over, so Endpoint.SetTransport lets a caller
// supply the Transport that actually performs it (a simulated peripheral, a
// test double, or a real hardware driver); an endpoint with no Transport set
// defaults to a loopback echo. Every transfer queues a transaction-complete
// TriggerEvent (see Endpoint.DrainTriggers).
//
// # Explicit non-goal
//
// Per ROADMAP.md Milestone 48, this package ships against the plain,
// unconditional acf.Message request kind only. Compound/triggered/chained/
// timed request variants are Phase 15's job (ROADMAP.md Milestone 49) and
// are retrofitted onto every endpoint type there, not decided here.
//
// # A note on spec fidelity (Guiding Principle 10)
//
// This package's register/request byte layout has not yet been
// independently re-verified against the governing OPEN Alliance TC18
// Remote Control Protocol Specification; see the ecosystem audit tracking
// issues for known gaps, the same open-item posture avtp/doc.go,
// server/doc.go, gpio/doc.go, and spi/doc.go document for their own
// packages. One design choice here is flagged as an unresolved open item
// rather than a confirmed reading, per this milestone's own explicit
// instruction to surface it instead of guessing:
//
//   - The specification's bus-speed selector field has enumerated value
//     assignments that read as ambiguous/colliding in the source material
//     (more than one named speed class appears to share what would decode
//     to the same wire value) — ROADMAP.md Milestone 48 explicitly calls
//     this out as an item to flag rather than silently resolve, the same
//     posture this repo's other packages take toward their own open
//     spec-ambiguity items (Guiding Principle 10). Rather than guess which
//     collision arm the spec actually intends, this package
//     defines BusSpeed as its own freestanding, self-consistent one-based
//     enumeration covering the five conventional I2C speed classes
//     (Standard/Fast/Fast-Plus/High-Speed/Ultra-Fast-Mode). This is this
//     implementation's own reasoned encoding to fill an ambiguous field, not
//     a verified transcription of the published wire values, and should be
//     revisited against a public interoperability reference before being
//     treated as final.
package i2c

//fusa:req REQ-I2C-001
//fusa:req REQ-I2C-002
//fusa:req REQ-I2C-003
//fusa:req REQ-I2C-004
//fusa:req REQ-I2C-005
//fusa:req REQ-I2C-006
//fusa:req REQ-I2C-007
//fusa:req REQ-I2C-008
//fusa:req REQ-I2C-009
//fusa:req REQ-I2C-010
