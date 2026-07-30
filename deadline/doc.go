// Package deadline monitors per-stream liveness from the two signal sources
// ROADMAP.md Milestone 53 identifies as the new model's nearest equivalents
// to the old periodic-Status-broadcast concept, for the OPEN Alliance TC18
// Remote Control Protocol (RCP), as described by the "OPEN Alliance TC18
// Remote Control Protocol Specification v0.5.1_RC".
//
// This is Milestone 53 (v0.66.0)'s rebuild of the old `deadline` package,
// per ROADMAP.md's Phase 17 disposition table: the old package reset a
// single per-zone timer on every incoming `Status` broadcast frame, a
// concept the new model has no equivalent of at all. The disposition table
// names two different signals with two different failure semantics as the
// closest available substitutes, and this package models both explicitly
// rather than collapsing them into one:
//
//   - per-endpoint triggers — the Trigger/DrainTriggers machinery gpio,
//     adc, pwm, uart, wakeup, and every other endpoint type already
//     implements (see e.g. gpio.Endpoint.DrainTriggers) — evidence that a
//     specific endpoint is actively producing events, i.e. genuine
//     liveness of whatever that endpoint is monitoring; and
//   - response-queue heartbeat flushes — regmap.QueueConfig's
//     HeartbeatIntervalMillis (regmap/queues.go, ROADMAP.md Milestone 45)
//     — evidence only that the link/queue itself is up, emitted
//     specifically "while the queue is otherwise idle" so a client can
//     "distinguish 'nothing to report' from 'the link is down'".
//
// Observing only the second without ever observing the first is a
// meaningfully different failure mode from observing neither — the link is
// up but nothing is happening upstream of it, versus the link itself being
// down — so Monitor exposes three LivenessState values (LivenessAlive,
// LivenessIdle, LivenessDead) rather than the old package's plain
// Alive/Dead boolean. A caller reports which kind of signal arrived via
// Monitor.ObserveTrigger or Monitor.ObserveHeartbeat; State (or the
// convenience Alive) reports the current verdict, computed lazily against
// an injectable clock — no goroutine, no timer, the same caller-driven
// posture e2e.Supervisor and fragment.Reassembler.Sweep already
// establish. DeadlineForQueue derives a plausible Config.Deadline directly
// from a regmap.QueueConfig's own HeartbeatIntervalMillis, so a caller does
// not have to duplicate that arithmetic (or worse, let the two configs
// silently drift apart) at the call site.
//
// # A note on spec fidelity (Guiding Principle 10)
//
// This package's three-state liveness model, and its choice to treat a
// queue heartbeat and an endpoint trigger as distinct signal classes
// rather than collapsing them, are this implementation's own reasoned,
// self-consistent design — the roadmap text this package implements says
// only that the old
// broadcast-liveness model has no direct equivalent and that the nearest
// substitutes "have different failure semantics and need to be modeled from
// scratch", without prescribing the exact state machine — the same
// open-item posture avtp/doc.go, server/doc.go, and wakeup/doc.go already
// document for their own packages.
package deadline

//fusa:req REQ-DL-001
//fusa:req REQ-DL-002
//fusa:req REQ-DL-003
//fusa:req REQ-DL-004
//fusa:req REQ-DL-005
//fusa:req REQ-DL-006
