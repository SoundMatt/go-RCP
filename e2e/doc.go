// Package e2e implements the CRC32 safe-point mechanism, the safety-
// request ("MSB-set") readiness gate, and the server-side per-stream
// watchdog that automatically drives it, for the OPEN Alliance TC18 Remote
// Control Protocol (RCP), as described by the "OPEN Alliance TC18 Remote
// Control Protocol Specification v0.5.1_RC".
//
// This is the Phase 15 (v0.63.0) layer ROADMAP.md Milestone 50 describes:
// three related mechanisms that all trace back to the same idea — an
// endpoint can opt into stronger, safety-oriented guarantees on top of the
// request/dispatch machinery request/doc.go's Milestone 49 already shipped,
// without that machinery having to know any of this package exists. Every
// piece here is additive, wrapping (Guard around a request.Handler,
// SetSafeStateCheck/PurgeNonSafety as new request.Dispatcher methods) or
// purely new state (Supervisor) — no existing package's exported behavior
// changes for a caller that never touches e2e at all.
//
// # A note on this package's name
//
// This package was built at Milestone 50 (v0.63.0) under the name
// `crcsafe`, specifically to avoid colliding with the pre-TC18 bespoke
// Zone/Command protocol's own `e2e` package (a different, unrelated CRC-16/
// CCITT-FALSE mechanism) while that package still existed. Milestone 53
// (v0.66.0) retired the legacy `e2e` package outright with no successor,
// and RELAY spec v1.14 §13.7.2's cross-language module-naming registry
// separately settled on `e2e` as the canonical name every language's RCP
// implementation must use for this "end-to-end / E2E safety protection"
// concern (cpp-RCP and rust-RCP already used it). With the legacy package
// gone and the name no longer ambiguous, this package was renamed from
// `crcsafe` to `e2e` to converge with the other three implementations. The
// doc comments below still say "the old `e2e` package" or "the legacy `e2e`
// package" where they contrast against that retired, unrelated mechanism —
// never against an earlier version of this package itself.
//
// # CRC32 safe points
//
// Compute/Protect/Verify replace the legacy `e2e` package's ad hoc CRC-16/
// CCITT-FALSE scheme with a different, explicit per-endpoint opt-in mode —
// "different" in the three ways ROADMAP.md Milestone 50 calls out by name:
//
//   - a different polynomial: CRC32 (crc32.IEEE), not CRC-16/CCITT-FALSE;
//   - different coverage: the enclosing AVTPDU's stream addressing plus
//     every field of the RCP Message itself (addressing, timestamp, and
//     body combined), not just the old scheme's bespoke Command.Payload;
//   - different failure handling: Guard skips calling the wrapped Handler
//     outright and reports the dedicated, distinguishable ErrCRCMismatch
//     error, rather than the old package's separate sequence-counter replay
//     guard.
//
// Protect appends a trailing Len-byte CRC32 to a Message's Body; Verify
// strips and checks it. Guard composes the two into a request.Handler
// wrapper, the same "wrap, don't edit" pattern request.Dispatcher itself
// established for the six Phase 14 endpoint types.
//
// # Safety-request Kinds and the configured safe-state gate
//
// ROADMAP.md Milestone 50 also adds three safety-request ("MSB-set")
// variants directly to the request package's own Kind enum:
// request.KindCompoundSafety, request.KindCompoundWaitSafety, and
// request.KindTriggeredSafety (see request/kind.go's KindSafetyFlag,
// Kind.IsSafety, and Kind.Base — this is a tagged subclass of the existing
// taxonomy, not a fourth kind family). request.Dispatcher only lets one of
// these three tickets advance to StateExecuting once a configured
// request.SafeStateCheck reports the requester's addressed endpoint is
// actually in its configured safe state; Submit refuses to admit one at all
// with no SafeStateCheck configured. Supervisor.CheckFunc is this package's
// own answer to that gate — see "The watchdog" below — but a caller is free
// to back request.SafeStateCheck with any policy of its own instead;
// request.Dispatcher does not import e2e, only the other way around.
//
// # The watchdog and safety requests' purge-survival property
//
// Supervisor is the per-stream watchdog/sequence-monotonicity/overflow
// configuration ROADMAP.md Milestone 50 calls for, replacing — not
// adapting — the old `watchdog` package's client-push periodic-keepalive
// model entirely: nothing here is ever sent on the wire by this package,
// and nothing here runs a background goroutine. Supervisor.Observe records
// each request's arrival time and sequence number; Supervisor.InSafeState
// answers, purely by comparing that bookkeeping against the current clock
// reading on demand, whether a stream has gone quiet longer than its
// configured Timeout or previously violated its configured
// RequireMonotonicSequence rule (a sticky trip Reset explicitly clears).
//
// A caller wires the two request.Dispatcher hooks this milestone adds
// directly to a Supervisor:
//
//	sup := e2e.NewSupervisor(e2e.StreamConfig{Timeout: 50 * time.Millisecond})
//	dispatcher.SetSafeStateCheck(sup.CheckFunc())
//	// ... on whatever cadence a caller judges appropriate (e.g. once per
//	// Dispatcher.Pump call, or on its own timer):
//	if sup.InSafeState(stream) {
//	    dispatcher.PurgeNonSafety()
//	}
//
// request.Dispatcher.PurgeNonSafety is the "watchdog-driven purge of
// ordinary pending requests" ROADMAP.md Milestone 50's own text names: it
// finalizes every not-yet-finalized ticket that is not a safety-request
// Kind, specifically leaving every KindCompoundSafety/
// KindCompoundWaitSafety/KindTriggeredSafety ticket untouched — the
// materially new safety mechanism the roadmap flags as having no analogue
// anywhere in the old protocol. This package computes the watchdog verdict;
// request.Dispatcher performs the purge; neither package computes the
// other's half, matching the same one-directional-dependency shape as
// SafeStateCheck (e2e imports request; request never imports e2e).
//
// # Explicit non-goals
//
// This package does not edit or migrate the legacy `e2e` or `watchdog`
// packages — ROADMAP.md's own Phase 17 disposition table schedules that
// satellite-migration work for Milestone 53 (v0.66.0), not this one; both
// old packages are left fully intact and continue to compile and pass their
// own tests unchanged. It also does not implement multi-AVTPDU
// fragmentation (ROADMAP.md Milestone 52, v0.65.0): ComputeFragmented
// exists to pin down, ahead of that milestone, exactly how this package's
// CRC coverage rule is meant to interact with a reassembled multi-segment
// message once fragmentation itself lands — a fragmentation-aware endpoint
// built later is expected to call it instead of Compute for a message it
// reassembled from segments, not to reimplement the "final segment only"
// CRC-placement rule itself.
//
// # A note on spec fidelity (Guiding Principle 10)
//
// Every concrete choice this package makes — the CRC32 polynomial
// (crc32.IEEE), the exact covered-field byte layout and ordering in
// writeCovered, appending the CRC as a trailing field rather than a
// leading one, the KindSafetyFlag bit position (0x80) and which three base
// Kinds get a safety counterpart, the StreamConfig field set, and
// Supervisor's "never observed = already timed out" default — has not yet
// been independently re-verified against the governing OPEN Alliance TC18
// Remote Control Protocol Specification's own wire format and rules; see
// the ecosystem audit tracking issues for known gaps, the same open-item
// posture avtp/doc.go, server/doc.go, and request/doc.go already document
// for their own packages.
package e2e

//fusa:req REQ-CRC-001
//fusa:req REQ-CRC-002
//fusa:req REQ-CRC-003
//fusa:req REQ-CRC-004
//fusa:req REQ-CRC-005
//fusa:req REQ-CRC-006
//fusa:req REQ-CRC-007
//fusa:req REQ-CRC-008
//fusa:req REQ-CRC-009
