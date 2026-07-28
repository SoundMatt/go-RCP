// Package adc implements the ADC endpoint type for the OPEN Alliance TC18
// Remote Control Protocol (RCP), as described by the "OPEN Alliance TC18
// Remote Control Protocol Specification v0.5.1_RC".
//
// This is one of four Phase 14 (v0.61.0) endpoint-type packages (see also
// i2c, uart, pwm) built directly on top of the server package's register-map
// substrate (ROADMAP.md Milestones 45/46), the same foundation gpio and spi
// built on in Milestone 47 (v0.60.0): an ADC endpoint's functional
// configuration (Config) is read and written through
// server.Server.WriteFunctional/server.Server.ReadEndpoint exactly like any
// other endpoint's FunctionalBlock, and Endpoint.HandleRequest decodes and
// answers a plain acf.Message request the same request-descriptor header
// every endpoint type shares.
//
// # Scope
//
// One ADC endpoint models a single channel of up to MaxResolutionBits (16)
// bits, converting through a three-layer sample/average/combine model:
// Config.SampleCount raw readings are taken from the endpoint's Transport
// (the "sample" layer, see Endpoint.SetTransport), arithmetic-averaged into
// one reading (the "average" layer), then combined with the endpoint's
// previous value under Config.Combine (the "combine" layer — either replace
// outright or a simple rolling average, see CombineMode) to produce the
// value a request reports.
//
// An ADC endpoint never samples on its own — nothing about this package
// runs a background timer or goroutine, matching every other Phase 14
// package's purely synchronous request/response posture. Endpoint.Trigger is
// this package's single entry point for performing one measurement, and
// there are exactly two ways ROADMAP.md Milestone 48 describes for a caller
// to keep a channel sampling continuously, both driven externally by
// invoking Trigger repeatedly rather than by this package looping
// internally:
//
//   - Triggered off another endpoint's own trigger signal: a caller
//     observes another endpoint's DrainTriggers queue (the same
//     TriggerEvent/DrainTriggers mechanism spi introduced in v0.60.0) and
//     invokes Trigger once per drained event.
//   - Self-triggered off its own measurement-done event: a caller observes
//     this endpoint's own DrainTriggers queue (which reports
//     TriggerMeasurementDone) and invokes Trigger again for each one
//     drained, chaining continuous sampling off the endpoint's own
//     completion signal.
//
// Config.TriggerMode records which of these (if either) a given channel is
// configured for; see TriggerMode's doc comment for exactly how that shapes
// a plain read request's behavior.
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
// package was built from a behavioral description of an ADC endpoint, not
// from the primary spec text, and two design choices are flagged here as
// this implementation's own reasoned encoding rather than a verified
// transcription of the published register/request byte layouts, the same
// open-item posture avtp/doc.go, server/doc.go, gpio/doc.go, and spi/doc.go
// document for their own packages:
//
//   - The "combine" layer of the three-layer sample/average/combine model
//     ROADMAP.md Milestone 48 names is not itself spelled out beyond that
//     label. This package interprets it as a combine against the endpoint's
//     accumulated history (replace-outright or a simple rolling average),
//     rather than e.g. combining multiple channels or averaging windows —
//     this is this implementation's own reasoned reading of an
//     under-specified term, not a confirmed one.
//   - How continuous sampling is actually driven (an external caller
//     invoking Trigger repeatedly, rather than an internal timer/goroutine
//     this package runs itself) is this implementation's own design choice
//     to keep this package's request handling synchronous like every other
//     Phase 14 endpoint type, not a transcription of any specific spec
//     mechanism.
package adc

//fusa:req REQ-ADC-001
//fusa:req REQ-ADC-002
//fusa:req REQ-ADC-003
//fusa:req REQ-ADC-004
//fusa:req REQ-ADC-005
//fusa:req REQ-ADC-006
//fusa:req REQ-ADC-007
//fusa:req REQ-ADC-008
//fusa:req REQ-ADC-009
//fusa:req REQ-ADC-010
