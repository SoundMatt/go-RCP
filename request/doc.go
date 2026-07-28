// Package request implements the conditional-request taxonomy, sequencer
// primitive, cancellation requests, and request-lifecycle state machine for
// the OPEN Alliance TC18 Remote Control Protocol (RCP), as described by the
// "OPEN Alliance TC18 Remote Control Protocol Specification v0.5.1_RC".
//
// This is the Phase 15 (v0.62.0) layer ROADMAP.md Milestone 49 describes as
// the single largest new-territory item in go-RCP's TC18 protocol
// replacement program: the old bespoke Command/Response protocol had no
// equivalent request model of any kind, so this package is additive
// complexity sitting above the six Phase 14 endpoint-type packages (gpio,
// spi, i2c, uart, adc, pwm), not a refactor of anything those packages
// already shipped. Every one of their doc.go files explicitly deferred
// compound/triggered/chained/timed request handling to this milestone; this
// package is that deferred work, and it retrofits their already-shipped
// plain-request path onto the same lifecycle machinery by wrapping their
// Endpoint.HandleRequest method through the Handler interface rather than
// editing any of those six packages.
//
// # Wire layer: FlagExtended
//
// A conditional request is an acf.Message with acf.FlagExtended set on
// Control; Body then begins with this package's own envelope — a Kind byte
// followed by kind-specific fields (see Kind and the EncodeXxx/DecodeXxx
// functions in envelope.go and chained.go). A Message without FlagExtended
// set is the Phase 14 plain shape, synthesized here as KindPlain and handled
// identically to before. See avtp/doc.go's own "Milestone 49 addendum"
// section for why this milestone claims that control bit rather than
// prefixing every request (plain included) with an extra body byte.
//
// # The five conditional-request kinds
//
//   - KindCompound gates a write (or read) on a Sequencer register's
//     current value via a Conditional comparison, advancing the register
//     only when the condition holds; the inner request only reaches the
//     wrapped endpoint on a match.
//   - KindCompoundWait evaluates the same Conditional but never touches
//     endpoint output at all — it exists purely to report whether a gate
//     currently holds (and optionally advance the register), independent of
//     any write.
//   - KindTriggered defers an inner request until another endpoint's own
//     trigger signal fires at least once, via a caller-supplied TriggerPump
//     adapter around that endpoint's typed DrainTriggers method.
//   - KindChained forces strict, fail-fast sequential execution of multiple
//     sub-requests against one endpoint within a single envelope — see
//     ChainedResult's doc comment for why a failed segment aborts the rest
//     rather than skipping past it.
//   - KindTimed defers an inner request until a caller-supplied clock value
//     reaches a body-carried target, independent of the AVTPDU's own
//     timestamped (TSCF) header mechanism (avtp.Header.Disposition) —
//     "presentation-time execution without a timestamped header."
//
// # Sequencers
//
// Sequencer is the persistent per-register state store Compound/
// CompoundWait requests read and (conditionally) advance. It has no
// declared bit width or active-pin-style mask the way gpio.Config does —
// every register is a free-running uint32 counter that wraps rather than
// saturates; see Sequencer.Advance's doc comment for why that is this
// implementation's own reasoned choice given no declared bound exists to
// clamp against.
//
// # Cancellation requests
//
// Three Kind values implement ROADMAP.md Milestone 49's cancellation
// taxonomy: KindCancelAll (mandatory — clears every pending ticket a
// Dispatcher holds) and two optional, narrower variants this
// implementation chose as a natural partition orthogonal to "everything":
// KindCancelTransaction (one specific pending ticket, by TransactionNum)
// and KindCancelSequencer (every pending Compound/CompoundWait ticket gated
// on one sequencer register) — the latter a deliberate thematic pairing
// with the Sequencer primitive this same milestone introduces. Cancellation
// always executes first among any tickets that become eligible within the
// same Dispatcher.Pump call; see Kind.Priority's doc comment for the full
// fixed cross-type ordering and its rationale.
//
// # The request-lifecycle state machine
//
// Every ticket — regardless of Kind, including KindPlain — advances through
// exactly the same four states (State): StateQueued, StateStarted,
// StateExecuting, StateFinalized. Dispatcher.Submit performs admission
// (including the optional AccessCheck gate) and decoding; Dispatcher.Pump
// performs everything from there — readiness evaluation, fixed-priority
// ordering among whatever becomes ready in one call, the kind-specific
// StateExecuting action, and finalization. Dispatcher.Dispatch composes
// Submit+Pump+Response into one synchronous call for the kinds that always
// resolve immediately, mirroring the signature every Phase 14 endpoint's own
// HandleRequest already has.
//
// # Milestone 50 addendum: safety-request Kinds and the watchdog-driven purge
//
// ROADMAP.md Milestone 50 (v0.63.0) extends this package in place rather
// than adding a separate layer above it: KindCompoundSafety,
// KindCompoundWaitSafety, and KindTriggeredSafety are the safety-request
// ("MSB-set") variants of KindCompound/KindCompoundWait/KindTriggered (see
// KindSafetyFlag, Kind.IsSafety, Kind.Base) — a tag on the same envelope
// shape, not a fourth taxonomy. Dispatcher.Pump only ever lets one of these
// three advance to StateExecuting once the configured SafeStateCheck
// reports the requester's addressed endpoint is actually in its configured
// safe state (see SafeStateCheck and Dispatcher.SetSafeStateCheck); Submit
// refuses to admit one at all when no SafeStateCheck is configured
// (ErrSafeStateNotConfigured), the same "opt in explicitly or don't get the
// mechanism" posture the CRC safe-point mechanism takes (see the e2e
// package). The other new property this milestone adds — no analogue
// anywhere in the old protocol — is that these three Kinds specifically
// survive Dispatcher.PurgeNonSafety, the watchdog-driven bulk clear of every
// other pending ticket a caller invokes once a e2e.Supervisor judges a
// stream's request-arrival watchdog has tripped. This package does not
// itself compute either the safe-state verdict or the watchdog trip
// condition — both are the new e2e package's job; this package only
// consumes them through SafeStateCheck and PurgeNonSafety, mirroring how it
// already consumes an AccessCheck it does not itself define the policy for.
//
// # Explicit non-goal
//
// This package does not implement the CRC32 safe-point wire mechanism
// itself (envelope-independent — it protects the whole Message, plain
// requests included, not just conditional-request envelopes) — see the new
// e2e package for that, plus the per-stream watchdog/sequence-
// monotonicity/overflow configuration that decides when to call
// PurgeNonSafety. It also does not implement multi-AVTPDU fragmentation
// (ROADMAP.md Milestone 52); a KindChained envelope's segments are still
// delivered within one AVTPDU, same as every Phase 14 request today.
//
// # A note on spec fidelity (Guiding Principle 10)
//
// The TC18 specification PDF is confidential to OPEN Alliance members. This
// package was built from a behavioral description of the conditional-
// request model, not from the primary spec text. Every wire-level choice
// this package makes — the envelope byte layout in envelope.go/chained.go,
// claiming acf.FlagExtended for the envelope marker, the sequencer-
// wraparound arithmetic, the exact split between the mandatory and two
// optional cancellation variants, and the fixed cross-type priority
// ordering in Kind.Priority — is this implementation's own reasoned,
// self-consistent encoding rather than a verified transcription of the
// published wire format or the source specification's own priority table,
// the same open-item posture avtp/doc.go, server/doc.go, and every Phase 14
// endpoint package's doc.go already document for their own packages.
package request

//fusa:req REQ-REQ-001
//fusa:req REQ-REQ-002
//fusa:req REQ-REQ-003
//fusa:req REQ-REQ-004
//fusa:req REQ-REQ-005
//fusa:req REQ-REQ-006
//fusa:req REQ-REQ-007
//fusa:req REQ-REQ-008
//fusa:req REQ-REQ-009
//fusa:req REQ-REQ-010
//fusa:req REQ-REQ-011
//fusa:req REQ-REQ-012
//fusa:req REQ-REQ-013
//fusa:req REQ-REQ-014
//fusa:req REQ-REQ-015
//fusa:req REQ-REQ-016
//fusa:req REQ-REQ-017
//fusa:req REQ-REQ-018
//fusa:req REQ-REQ-019
//fusa:req REQ-REQ-020
//fusa:req REQ-REQ-021
//fusa:req REQ-REQ-022
//fusa:req REQ-REQ-023
//fusa:req REQ-REQ-024
//fusa:req REQ-REQ-025
