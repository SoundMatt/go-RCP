// Package pwm implements the PWM endpoint type for the OPEN Alliance TC18
// Remote Control Protocol (RCP), as described by the "OPEN Alliance TC18
// Remote Control Protocol Specification v0.5.1_RC".
//
// This is one of four Phase 14 (v0.61.0) endpoint-type packages (see also
// i2c, uart, adc) built directly on top of the server package's register-map
// substrate (ROADMAP.md Milestones 45/46), the same foundation gpio and spi
// built on in Milestone 47 (v0.60.0): a PWM endpoint's functional
// configuration (Config) is read and written through
// server.Server.WriteFunctional/server.Server.ReadEndpoint exactly like any
// other endpoint's FunctionalBlock, and Endpoint.HandleRequest decodes and
// answers a plain acf.Message request the same request-descriptor header
// every endpoint type shares.
//
// # Scope
//
// One PWM endpoint plays exactly one of two roles (Config.Role): RoleOutput
// generates a waveform, RoleInput measures an externally driven incoming
// one. Both roles share the same symmetric two-field payload shape — a
// period and an active (high) duration, both in microseconds (see
// EncodeWaveform/DecodeWaveform) — ROADMAP.md Milestone 48 specifies for PWM
// output and PWM input alike.
//
// A RoleOutput endpoint accepts both write requests (apply a new waveform)
// and read requests (read back the currently applied one); Configure also
// immediately applies Config's default waveform, so an output endpoint is
// always driving something once configured rather than sitting at an
// undefined value until the first write. This package has no real PWM
// hardware of its own to drive, so Endpoint.SetOutputTransport lets a caller
// supply the OutputTransport that actually generates the waveform; an
// endpoint with none set defaults to simply storing the applied waveform for
// readback.
//
// A RoleInput endpoint is response-only: per ROADMAP.md Milestone 48, a
// write request against it is rejected with ErrWriteNotSupportedForInput,
// since there is nothing for a write to command on a measurement-only role.
// A read request returns the most recently captured waveform through the
// configured InputTransport (see Endpoint.SetInputTransport) — or, with none
// set, whatever was last fed through Endpoint.SetCapturedWaveform — and
// fails explicitly with ErrSignalLost when no valid waveform has been
// captured (or SetSignalLost was called more recently), rather than
// returning stale data or silently hanging, per this milestone's explicit
// instruction.
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
// The TC18 specification PDF is confidential to OPEN Alliance members. This
// package was built from a behavioral description of a PWM endpoint, not
// from the primary spec text. Its exact register/request byte layouts
// (Config's field order/widths and the period-then-active waveform field
// order) are this implementation's own reasoned, self-consistent encoding
// rather than a verified transcription of the published byte assignments —
// the same open-item posture avtp/doc.go, server/doc.go, gpio/doc.go, and
// spi/doc.go document for their own packages, pending confirmation against a
// public interoperability reference. Modeling RoleOutput/RoleInput as a
// single endpoint with a Role switch, rather than two separate endpoint
// types, is likewise this implementation's own reasoned reading of
// regmap/types.go's single "OUT" PWM signal name, not a confirmed one.
package pwm

//fusa:req REQ-PWM-001
//fusa:req REQ-PWM-002
//fusa:req REQ-PWM-003
//fusa:req REQ-PWM-004
//fusa:req REQ-PWM-005
//fusa:req REQ-PWM-006
//fusa:req REQ-PWM-007
//fusa:req REQ-PWM-008
//fusa:req REQ-PWM-009
//fusa:req REQ-PWM-010
