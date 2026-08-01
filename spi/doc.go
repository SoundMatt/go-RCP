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
// transfer request's body is the raw SPI payload from its very first byte;
// which pre-configured channel it is transferred over is selected by the
// request's evt[2:0] field, not by anything in the body (see "# The evt
// field" below). The response carries the bytes the controller received
// back, since a real SPI transfer is inherently full-duplex. This package
// has no real SPI bus of its own to
// drive the exchange over, so Endpoint.SetTransport lets a caller supply the
// Transport that actually performs one channel's byte exchange (a simulated
// peripheral, a test double, or a real hardware driver); a channel with no
// Transport set defaults to a loopback echo. Every transfer queues a
// transfer-complete TriggerEvent bracketed by a chip-select-assert/
// chip-select-deassert edge pair (see TriggerEvent and Endpoint.DrainTriggers).
//
// # The evt field (§13.5 Table 30)
//
// SPI has a Table 30 row to itself, declared by this package as EVTClass.
// Every request goes through acf.Message.EVTDisposition, the single shared
// implementation of Table 30 (acf/evt.go), which resolves evt[2:0] to:
//
//	000b-101b  selects chip-select channel 0-5; that channel's interface
//	           settings are applied and its CSN pin asserted
//	110b       reserved — rejected with error code UNSUPPORTED_CMD
//	111b       the payload is NOT presented on the bus; it is a §12.7.1
//	           configuration write into this endpoint's EP_func block
//
// §12.9.1's general rule — a non-zero evt[2:0] with no byte_msg_payload at
// all is answered with UNSUPPORTED_CMD — is applied by the same shared path.
// Note that this makes a zero-length transfer expressible only on channel 0;
// that is the specification's own consequence, not this package's choice.
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
// This package's exact register byte layouts (ChannelConfig's field order
// and widths, and the four-value clock-polarity/phase Mode enumeration) have
// not yet been independently re-verified against the governing OPEN Alliance
// TC18 Remote Control Protocol Specification's own published byte
// assignments — the same open-item posture avtp/doc.go and server/doc.go
// document for their own packages; see the ecosystem audit tracking issues
// for known gaps. The channel-selection mechanism is no longer among those
// open items: through v7.0.0 this package carried a one-byte Channel
// sub-opcode at the head of the request body and never read evt[2:0] at all,
// which §13.7.3's Figure 23 and Table 30's SPI row both contradict. It now
// reads evt[2:0], and the body is the payload in full.
//
// One inconsistency in the specification is worth recording: Figure 23's
// caption describes its example as targeting "SPI channel 3" while the
// figure itself shows evt = 0101b, i.e. channel 5 under Table 30's own
// mapping. Table 30 is the normative statement and is unambiguous
// ("selects channel 0 … 5"), so this package maps channel = evt[2:0].
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
//fusa:req REQ-SPI-012
