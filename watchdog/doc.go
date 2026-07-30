// Package watchdog orchestrates e2e's server-side, request-arrival-timed
// safety mechanism (e2e.Supervisor) against one or more
// request.Dispatcher instances, for the OPEN Alliance TC18 Remote Control
// Protocol (RCP), as described by the "OPEN Alliance TC18 Remote Control
// Protocol Specification v0.5.1_RC".
//
// This is Milestone 53 (v0.66.0)'s rebuild of the old client-push `watchdog`
// package, per ROADMAP.md's Phase 17 disposition table, which flags this
// specific package as an architecture inversion rather than a refactor: the
// old package was an HPC-side Keeper that periodically pushed a CmdWatchdog
// keepalive command to each zone controller and tracked a client-observed
// Healthy/Degraded/Faulted health state machine. None of that survives —
// this package sends nothing on the wire and observes no "health state" at
// all. Milestone 50's e2e package already inverted the architecture: a
// stream's own inbound request arrivals are themselves the liveness signal,
// timed server-side by e2e.Supervisor, and a stream that goes quiet
// drives automatic safe-state entry (e2e.Supervisor.InSafeState) plus a
// watchdog-driven purge of every non-safety-request ticket
// (request.Dispatcher.PurgeNonSafety).
//
// e2e/doc.go documents that two-line wiring as a caller's own
// responsibility:
//
//	dispatcher.SetSafeStateCheck(sup.CheckFunc())
//	if sup.InSafeState(stream) {
//	    dispatcher.PurgeNonSafety()
//	}
//
// This package exists for the server exposing more than a handful of
// endpoints — each with its own Dispatcher, some sharing a requester stream
// with others, some not — where deciding *when* to check and *which*
// Dispatchers to purge for a given stream needs to be centralized rather
// than copy-pasted at every call site. Keeper is that centralization: it
// holds the (Supervisor, {stream -> Dispatchers}) association Watch builds
// up, and Keeper.Tick performs the check-and-purge sweep across all of it in
// one call.
//
// # Caller-driven, not a background goroutine
//
// Keeper.Tick is a synchronous, caller-driven sweep — the same posture
// e2e.Supervisor, request.Dispatcher.Pump, and fragment.Reassembler.Sweep
// already establish throughout this repo's safety-critical layers. Nothing
// in this package starts a goroutine or owns a timer of its own; a caller
// invokes Tick on whatever cadence it judges appropriate (e.g. once per
// server poll-loop iteration, or from its own ticker).
//
// # A note on spec fidelity (Guiding Principle 10)
//
// This package is pure orchestration glue over e2e.Supervisor and
// request.Dispatcher (see e2e/doc.go's own spec-fidelity note for their
// open items) — it makes no independent claim about wire format or
// spec-mandated behavior of its own.
package watchdog

//fusa:req REQ-WDG-001
//fusa:req REQ-WDG-002
//fusa:req REQ-WDG-003
//fusa:req REQ-WDG-004
//fusa:req REQ-WDG-005
