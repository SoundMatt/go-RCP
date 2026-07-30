// Package spi implements the SPI endpoint type for the OPEN Alliance TC18
// Remote Control Protocol (RCP), as described by the "OPEN Alliance TC18
// Remote Control Protocol Specification v0.5.1_RC".
//
// This is one of two Phase 14 (v0.60.0) endpoint-type packages (see gpio for
// the other) built directly on top of the server package's register-map
// substrate (ROADMAP.md Milestones 45/46): a SPI endpoint's functional
// configuration (Config) is read and written through
// server.Server.WriteFunctional/server.Server.ReadEndpoint exactly like any
// other endpoint's FunctionalBlock, and Endpoint.HandleRequest decodes and
// answers a plain acf.Message request the same request-descriptor header
// every endpoint type shares.
//
// # Scope
//
// One SPI endpoint is controller-only and models up to MaxChannels
// independently pre-configured chip-select channels: Config holds one
// ChannelConfig (clock rate, mode, inter-transfer delay) per channel. A
// transfer request's body leads with a one-byte Channel sub-opcode selecting
// which pre-configured channel the raw payload that follows is transferred
// over (see EncodeTransferRequest); the response echoes the channel and
// carries the bytes the controller received back, since a real SPI transfer
// is inherently full-duplex. This package has no real SPI bus of its own to
// drive the exchange over, so Endpoint.SetTransport lets a caller supply the
// Transport that actually performs one channel's byte exchange (a simulated
// peripheral, a test double, or a real hardware driver); a channel with no
// Transport set defaults to a loopback echo. Every transfer queues a
// transfer-complete TriggerEvent bracketed by a chip-select-assert/
// chip-select-deassert edge pair (see TriggerEvent and Endpoint.DrainTriggers).
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
// This package's exact register/request byte layouts (ChannelConfig's
// field order and widths, the Channel sub-opcode convention, and the
// four-value clock-polarity/phase Mode enumeration) have not yet been
// independently re-verified against the governing OPEN Alliance TC18
// Remote Control Protocol Specification's own published byte assignments —
// the same open-item posture avtp/doc.go, server/doc.go, and gpio/doc.go
// document for their own packages; see the ecosystem audit tracking
// issues for known gaps. Structural behaviour — six independently configured
// channels, sub-opcode channel selection, full-duplex raw transfer, and the
// transfer-complete/chip-select-edge trigger model — is what this milestone
// targets and tests; the precise wire byte assignments are flagged here as
// pending confirmation against a public interoperability reference.
package spi

//fusa:req REQ-SPI-001
//fusa:req REQ-SPI-002
//fusa:req REQ-SPI-003
//fusa:req REQ-SPI-004
//fusa:req REQ-SPI-005
//fusa:req REQ-SPI-006
//fusa:req REQ-SPI-007
//fusa:req REQ-SPI-008
//fusa:req REQ-SPI-009
//fusa:req REQ-SPI-010
//fusa:req REQ-SPI-011
