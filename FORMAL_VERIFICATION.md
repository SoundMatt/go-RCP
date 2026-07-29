# Formal Verification Evidence

**Project:** go-RCP
**Package under evidence:** `formal`
**Standard:** ISO 26262:2018 (formal-methods verification of safety mechanisms)
**Document ID:** FV-002 (supersedes the Milestone 41 / v0.41.0 evidence for
the retired pre-TC18 Zone/Command protocol; see "Relationship to the
retired FV-001 evidence" below)
**Version:** 1.0
**Date:** see the PR that introduced this document

Source of truth: `formal/lifecycle.go`, `formal/power.go`,
`formal/safestate.go`, and their respective `_test.go` files. This document
summarizes what those files prove; it is not a substitute for reading them.

---

## Scope

This milestone (ROADMAP.md §58, v0.71.0) rebuilds this package's proofs
against the three state machines that replaced the pre-TC18 protocol's own
zone-health, client-push-watchdog, and anti-replay-window machines:

| Machine | Package | Proof file |
|---|---|---|
| Configuration-readiness lifecycle (Unconfigured → HWLocked → FullyConfigured) | `server`/`lifecycle` | `formal/lifecycle.go` |
| Power model (Normal / StandBy / Sleep) + wake-handshake retransmission pacing | `wakeup`/`powerstate` | `formal/power.go` |
| Automatic safe-state-entry watchdog (inter-arrival timeout, sequence monotonicity) | `e2e` | `formal/safestate.go` |

## Method

Unlike the retired Milestone 41 evidence (TLA+ specifications, checked
exhaustively over a declared state space by the TLC model checker), this
evidence is produced by this repository's own `formal` package: named
`Invariant` values (temporal properties expressed with `Always`,
`Eventually`, and hand-written trace predicates) checked by `formal.Checker`
against traces built by driving the *actual production types*
(`server.Server`, `wakeup.Endpoint`, `powerstate.Driver`, `e2e.Supervisor`)
through a specific, code-defined action sequence — not an abstract model of
them. This is closer to targeted property-based testing over real API calls
than to exhaustive model checking; see `formal/formal.go`'s package doc
comment ("A note on this package's proof method") for the full reasoning
and its explicitly acknowledged scope limits.

## Properties verified

### Lifecycle (`formal/lifecycle.go`)

- **No regression:** a trace's observed `LifecycleRank` never decreases,
  across both a fully plausible transition sequence and a rejected
  out-of-order attempt. (`TestLifecycleInvariants_ValidSequence`,
  `TestLifecycleInvariants_OutOfOrder_NeverReachesFullyConfigured`)
- **Reachability:** a plausible action sequence (declare an endpoint, set a
  pin assignment, lock hardware, write the functional block, set a valid
  queue configuration) eventually reaches `lifecycle.StateFullyConfigured`.
  (`TestLifecycleInvariants_ValidSequence`)
- **Rejection is a no-op, not a regression:** an out-of-order
  `AdvanceToFullyConfigured` attempt from `StateUnconfigured` leaves the
  rank at 0 for the whole trace — satisfying "never decreases" without ever
  satisfying "reaches fully-configured."
  (`TestLifecycleInvariants_OutOfOrder_NeverReachesFullyConfigured`)

### Power (`formal/power.go`)

- **Safety — never unpowered:** `wakeup.PowerUnpowered` is never observed
  as `wakeup.Endpoint`'s own current state across a full
  StandBy→Normal→Sleep→Normal wake cycle. (`TestPowerInvariants_WakeCycle`)
- **Liveness — retransmission drains:** `powerstate.Driver`'s pending
  wake-handshake queue eventually reaches zero once a Sleep→Normal wake
  queues `Config.WakeHandshakeRepeatCount` repeats.
  (`TestPowerInvariants_WakeCycle`, `TestPowerInvariants_FinalState`)
- **Determinism — start-kind resolves:** a Sleep→Normal wake eventually
  determines a cold/hot-start kind other than `wakeup.StartUnknown`, and
  every transmitted `WakeHandshake` in the cycle carries strictly
  increasing `Sequence` values starting at 0. (`TestPowerInvariants_WakeCycle`)

### Safe state (`formal/safestate.go`)

- **Automatic entry on silence:** `e2e.Supervisor.InSafeState` eventually
  reports `true` once a stream's configured `Timeout` elapses with no
  further `Observe` call, and clears again once activity resumes.
  (`TestTimeoutInvariants_TripsAndClears`)
- **Automatic entry on protocol violation, and its stickiness:** a
  sequence-monotonicity violation trips `InSafeState` immediately, the trip
  remains `true` through every subsequent arrival up to an explicit
  `Reset`, and `Reset` clears it. (`TestMonotonicityInvariants_TripsStickyThenResets`)
- **Fail-safe default:** a stream `Supervisor` has never observed at all is
  judged in-safe-state from the very first snapshot — there is no implicit
  grace period for "hasn't sent anything yet."
  (`TestSafeStateOf_NeverObserved_StartsTrue`)

## Result

All `formal` package tests pass (`go test ./formal/...`); every
`Checker.Check` call in the tests listed above returns `nil` (no
`ViolationError`) except where a test deliberately verifies that a
violation *is* correctly detected
(`TestLifecycleInvariants_OutOfOrder_NeverReachesFullyConfigured`).

## Relationship to the retired FV-001 evidence

The Milestone 41 evidence (informally, "FV-001") proved properties of the
pre-TC18 protocol's zone-health, client-push-watchdog, and anti-replay-
window state machines via TLA+/TLC. That protocol, and every state machine
it defined, was retired by Milestone 53 (v0.66.0) with no successor sharing
its shape — the properties it proved no longer describe anything this
program does. This document and the `formal` package's Milestone 58 content
are a from-scratch replacement scoped to the TC18 state machines that
actually exist today, not an update to the retired proofs.

## A note on spec fidelity (Guiding Principle 10)

The OPEN Alliance TC18 Remote Control Protocol Specification v0.5.1_RC PDF
is confidential to OPEN Alliance members. The three state machines this
document evidences were built, at earlier milestones, from a behavioral
description of the specification, not from its primary text — see
`server/doc.go`, `wakeup/doc.go`, and `e2e/doc.go`'s own spec-fidelity notes
for the open items in each. This document adds no new spec-fidelity claim
of its own: it verifies properties of this repository's own implementation
of those machines, against itself, not against the specification directly.
