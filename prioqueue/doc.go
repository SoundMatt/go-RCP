// Package prioqueue provides a client-side dispatch queue ordered by the
// OPEN Alliance TC18 Remote Control Protocol (RCP)'s own fixed cross-type
// request-execution priority, as described by the "OPEN Alliance TC18
// Remote Control Protocol Specification v0.5.1_RC".
//
// This is Milestone 53 (v0.66.0)'s rebuild of the old `prioqueue` package,
// per ROADMAP.md's Phase 17 disposition table: the old package ordered a
// container/heap-based queue by a client-assigned
// PriorityCritical/High/Normal enum carried on every command. The new
// protocol has no equivalent of a client-assigned priority at all —
// Milestone 49's request.Kind fixes a total ordering by request *kind*
// instead (cancellation, chained, triggered, timed, compound-wait,
// compound, plain — see request.Kind.Priority and its own doc comment for
// the full rationale), applied uniformly by every request.Dispatcher on the
// server side regardless of what a client claims. This package's Queue
// mirrors that same ordering client-side, for a caller that wants to choose
// which of several already-built, not-yet-submitted requests to release
// next: instead of tagging an arbitrary priority value the server would
// ignore anyway, a caller picks the request Kind that already matches what
// it is trying to do (a cancellation, a triggered follow-up, a timed
// action, ...) and this package orders purely by that Kind's fixed rank.
//
// Queue itself is deliberately unaware of how an Item's Message body was
// encoded — encoding a Kind's specific envelope is request.EncodeXxx's job,
// not this package's; Queue only ever needs a request.Kind value to rank an
// Item by request.Kind.Priority.
//
// # A note on spec fidelity (Guiding Principle 10)
//
// This package adds no priority-ordering claim of its own beyond
// request.Kind's already-documented one (see request/kind.go's own
// spec-fidelity note on priorityRank) — it purely reuses that ordering
// client-side.
package prioqueue

//fusa:req REQ-PQ-001
//fusa:req REQ-PQ-002
//fusa:req REQ-PQ-003
//fusa:req REQ-PQ-004
//fusa:req REQ-PQ-005
//fusa:req REQ-PQ-006
