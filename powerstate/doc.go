// Package powerstate paces the repeating wake-handshake retransmission
// wakeup.Endpoint queues but deliberately does not send itself, for the
// OPEN Alliance TC18 Remote Control Protocol (RCP), as described by the
// "OPEN Alliance TC18 Remote Control Protocol Specification v0.5.1_RC".
//
// This is Milestone 53 (v0.66.0)'s rebuild of the old `powerstate` package,
// per ROADMAP.md's Phase 17 disposition table: the old package tracked a
// three-state Active/Sleeping/BusOff model with no relationship at all to
// the specification's own Normal/StandBy/Sleep/Unpowered model, its cold-
// start/hot-start distinction, or its wake-handshake message sequence — all
// of which Milestone 51's wakeup package (wakeup.PowerState, wakeup.StartKind,
// wakeup.Endpoint) already implements in full. This package does not
// duplicate any of that: it exists purely to fill the one gap wakeup/doc.go
// explicitly leaves open — "a caller's own transport loop is expected to
// keep re-emitting [the wake-handshake message] ... until
// Endpoint.AcknowledgeWake is called" — since nothing in the wakeup package
// itself paces or transmits that repeating message.
//
// # What Driver does
//
// Driver.Pump drains a wakeup.Endpoint's trigger queue (see
// wakeup.Endpoint.DrainTriggers): every queued TriggerPowerStateChanged
// becomes an Event this call returns, and every queued TriggerWakeHandshake
// is queued internally and paced out one per Pump call through a
// caller-supplied Transmitter — so a caller that invokes Pump on its own
// ticker, at whatever cadence matches the endpoint's configured
// wakeup.Config.WakeHandshakeIntervalMillis, reproduces exactly the
// "repeating, caller-paced" retransmission wakeup/doc.go describes without
// this package owning a timer of its own (the same caller-driven posture
// crcsafe.Supervisor and request.Dispatcher.Pump already establish
// throughout this repo's safety-critical layers). Driver.Acknowledge
// forwards to wakeup.Endpoint.AcknowledgeWake and additionally discards any
// repeat this Driver had already pulled off that queue but not yet
// transmitted, so acknowledging a wake stops retries started either side of
// that boundary.
//
// # A note on spec fidelity (Guiding Principle 10)
//
// The TC18 specification PDF is confidential to OPEN Alliance members. This
// package adds no wire format or state-machine claim of its own beyond
// wakeup's — it is pacing/retry glue over an already-built endpoint type
// (see wakeup/doc.go's own spec-fidelity note for that package's open
// items).
package powerstate

//fusa:req REQ-PWR-001
//fusa:req REQ-PWR-002
//fusa:req REQ-PWR-003
//fusa:req REQ-PWR-004
//fusa:req REQ-PWR-005
