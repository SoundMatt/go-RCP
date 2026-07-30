// Package wakeup implements the Wakeup endpoint type for the OPEN Alliance
// TC18 Remote Control Protocol (RCP), as described by the "OPEN Alliance
// TC18 Remote Control Protocol Specification v0.5.1_RC".
//
// This is one of five Phase 16 (v0.64.0) endpoint-type packages (see also
// lin, can, iseled, mdio) built directly on top of the server package's
// register-map substrate (ROADMAP.md Milestones 45/46) and the request
// package's conditional-request/dispatch machinery (ROADMAP.md Milestone
// 49), exactly as the six Phase 14 endpoint types (gpio, spi, i2c, uart,
// adc, pwm) already are: a Wakeup endpoint's functional configuration
// (Config) is read and written through
// server.Server.WriteFunctional/server.Server.ReadEndpoint, and
// Endpoint.HandleRequest implements the same request.Handler shape
// (avtp.StreamID, acf.Message) (acf.Message, error) every other endpoint
// type does, so it drops into request.Dispatcher unmodified.
//
// # Scope
//
// Per ROADMAP.md Milestone 51, Wakeup is a dedicated power-management
// endpoint, not a generic device interface — matching regmap/types.go's
// single "WAKE" Wakeup signal, it exists purely to drive the server's own
// power dimension, not to expose any device's electrical signal the way
// gpio/spi/i2c/uart/adc/pwm's own signal lists do. A write request commands
// a PowerState transition (see Endpoint.HandleRequest); a read request
// reports the current one.
//
// PowerNormal/PowerStandBy/PowerSleep are the three states a write request
// may target (see Endpoint.transitionTo). PowerUnpowered exists in the
// PowerState enumeration purely to represent the state a caller might read
// about historically or externally — see this doc comment's spec-fidelity
// note below for why it is deliberately never this package's own current
// state or a requestable write target: a server object that is actually
// unpowered cannot itself be running code to answer the very request that
// would command it there or read its own state back.
//
// Waking from PowerSleep back to PowerNormal is the one transition this
// package treats specially, per Milestone 51's cold-start-vs-hot-start and
// wake-handshake requirements:
//
//   - StartKind records whether retained context survived the just-ended
//     sleep period (StartHot) or was lost (StartCold, see
//     Endpoint.SetRetentionLost) — Endpoint.LastStartKind reports the most
//     recent determination.
//   - The transition also queues Config.WakeHandshakeRepeatCount separate
//     TriggerWakeHandshake TriggerEvent values up front (see
//     WakeHandshake and Endpoint.DrainTriggers), representing the
//     "repeating" wake-handshake message a caller's own transport loop is
//     expected to keep re-emitting at Config.WakeHandshakeIntervalMillis
//     cadence until Endpoint.AcknowledgeWake is called (which discards any
//     not-yet-drained ones) or they run out. This package has no real
//     timer of its own — the same "no timer, caller drives the cadence"
//     posture uart/doc.go already documents for its own read-completion
//     polling — so it queues the full repeat count rather than pacing it
//     internally.
//
// # Relationship to lifecycle.LifecycleState (Phase 13)
//
// ROADMAP.md Milestone 51 notes Wakeup "drives [the] same state machine's
// power dimension" as lifecycle.LifecycleState (ROADMAP.md Milestone 44's
// Unconfigured→HWLocked→FullyConfigured configuration-readiness axis).
// This package reads that as two genuinely orthogonal axes on the same
// server.Server — configuration readiness versus power state — rather than
// as one LifecycleState value this package should extend with new states
// or otherwise require lifecycle/lifecycle.go to change for: PowerState is
// tracked entirely within this package's own Endpoint, the same
// "extension point already anticipated, no core-package change needed"
// posture the other four Phase 16 packages take toward regmap/types.go's
// endpoint-type enum. This is this implementation's own reasoned
// interpretation of a roadmap sentence that does not spell out a more
// specific mechanical coupling, flagged here per Guiding Principle 10
// rather than guessed silently.
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
// order/widths, WakeHandshake's field order), PowerUnpowered's
// non-requestable/non-reportable-as-current-state treatment, and the
// lifecycle.LifecycleState-orthogonality reading above have not yet been
// independently re-verified against the governing OPEN Alliance TC18
// Remote Control Protocol Specification's own published byte assignments
// or state-machine relationship — the same open-item posture avtp/doc.go,
// server/doc.go, and pwm/doc.go document for their own packages; see the
// ecosystem audit tracking issues for known gaps.
package wakeup

//fusa:req REQ-WAKEUP-001
//fusa:req REQ-WAKEUP-002
//fusa:req REQ-WAKEUP-003
//fusa:req REQ-WAKEUP-004
//fusa:req REQ-WAKEUP-005
//fusa:req REQ-WAKEUP-006
//fusa:req REQ-WAKEUP-007
//fusa:req REQ-WAKEUP-008
//fusa:req REQ-WAKEUP-009
