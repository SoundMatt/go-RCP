// Package iseled implements the ISELED endpoint type for the OPEN Alliance
// TC18 Remote Control Protocol (RCP), as described by the "OPEN Alliance
// TC18 Remote Control Protocol Specification v0.5.1_RC".
//
// This is one of five Phase 16 (v0.64.0) endpoint-type packages (see also
// lin, can, mdio, wakeup) built directly on top of the server package's
// register-map substrate (ROADMAP.md Milestones 45/46) and the request
// package's conditional-request/dispatch machinery (ROADMAP.md Milestone
// 49), exactly as the six Phase 14 endpoint types (gpio, spi, i2c, uart,
// adc, pwm) already are: an ISELED endpoint's functional configuration
// (Config) is read and written through
// server.Server.WriteFunctional/server.Server.ReadEndpoint, and
// Endpoint.HandleRequest implements the same request.Handler shape
// (avtp.StreamID, acf.Message) (acf.Message, error) every other endpoint
// type does, so it drops into request.Dispatcher unmodified.
//
// # Scope
//
// ISELED (Intelligent Solid-state Emitter LED) devices are daisy-chained on
// a single native 4b/5b-encoded data line — matching regmap/types.go's
// single "DATA" ISELED signal. One ISELED endpoint is controller-only and
// models one such chain: Config carries the number of devices on the chain
// (Config.DeviceCount) and the timeout the controller waits for a device's
// response before giving up (see doc.go's "Multi-device response
// aggregation" section below). This package treats 4b/5b line coding as a
// physical-layer transport concern entirely outside its own scope — see
// Endpoint.SetTransport — the same way gpio/spi/i2c/uart/adc/pwm never
// model the electrical signaling underneath their own Transport
// abstractions either; Command/Response here are the logical
// (already-decoded) byte payloads a Transport exchanges over the chain.
//
// # ISELED-native CRC (layered on top of, not instead of, general E2E)
//
// Every Command this package encodes carries its own trailing ISELED-native
// CRC (see EncodeCommand/DecodeCommand and ComputeCRC) — a per-message
// integrity check specific to the ISELED chain protocol itself, verified by
// the addressed device(s) at the physical/link layer independently of
// anything RCP-level. This is *layered on top of*, not a replacement for,
// the general end-to-end mechanism e2e (ROADMAP.md Milestone 50)
// already provides at the RCP-message level: a caller free to wrap this
// package's Endpoint in e2e.Guard gets both checks — e2e's CRC32
// safe point over the whole RCP Message, and this package's own CRC8 over
// just the ISELED command bytes inside that message's Body — exactly as
// ROADMAP.md Milestone 51 calls for. This package does not import e2e
// itself (nothing here depends on it to function), the same one-directional
// "wrap, don't require" relationship e2e/doc.go already established for
// every other endpoint type.
//
// # Multi-device response aggregation (optional)
//
// A Command addressed to DeviceBroadcast queries every device on the chain
// in one request; a Command addressed to a specific device address queries
// only that one. Endpoint.HandleRequest reports every device's individual
// Response it currently has recorded (see Endpoint.SetDeviceResponse) for a
// broadcast read, encoded as an AggregatedResponse (see
// EncodeAggregatedResponse), or a single Response for a targeted read. This
// aggregation is optional per ROADMAP.md Milestone 51 in the sense that a
// caller is never required to use DeviceBroadcast — addressing one device
// at a time works identically to every other controller-style endpoint type
// in this repo (i2c, lin).
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
// This package's exact register/request byte layouts (Config's field
// order/widths, Command/Response's field order) and the ISELED-native
// CRC's specific polynomial (ComputeCRC uses CRC-8/SAE J1850, a publicly
// documented standard 8-bit CRC, chosen as this implementation's own
// reasoned, self-consistent encoding) have not yet been independently
// re-verified against the governing OPEN Alliance TC18 Remote Control
// Protocol Specification's own published byte assignments and native-CRC
// algorithm — the same open-item posture avtp/doc.go, server/doc.go, and
// e2e/doc.go document for their own packages; see the ecosystem audit
// tracking issues for known gaps.
package iseled

//fusa:req REQ-ISELED-001
//fusa:req REQ-ISELED-002
//fusa:req REQ-ISELED-003
//fusa:req REQ-ISELED-004
//fusa:req REQ-ISELED-005
//fusa:req REQ-ISELED-006
//fusa:req REQ-ISELED-007
//fusa:req REQ-ISELED-008
//fusa:req REQ-ISELED-009
