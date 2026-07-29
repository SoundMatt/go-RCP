// Package faultinject provides structured fault injection for validating
// the safety mechanisms this repo's TC18 rebuild introduced across Phases
// 15-17.
//
// A faultinject.Handler wraps any request.Handler and intercepts
// HandleRequest calls according to an ordered list of Rules. Rules may drop
// requests, add latency, or return one of this package's canned failure
// sentinels without forwarding to the wrapped Handler. Count-based rules
// auto-expire after a configured number of applications.
//
// # Fault catalogue rebuilt at Milestone 57 (ROADMAP.md, v0.70.0)
//
// Per Phase 17's disposition table, this package is ADAPT-flagged:
// structured fault injection as a harness pattern is reusable, but the
// fault-type catalogue moves from the retired watchdog/E2E/replay-guard
// specifics to this repo's actual TC18 safety mechanisms:
//
//   - FaultCRCFailure returns e2e.ErrCRCMismatch (Milestone 50), in place of
//     the retired legacy CRC-16 package's own mismatch error.
//   - FaultSafeStateEntry returns request.ErrPurgedByWatchdog (Milestone
//     50), simulating a Dispatcher.PurgeNonSafety sweep having already
//     cleared the request before this Handler was ever reached.
//   - FaultDiscoveryClaimTimeout returns
//     discovery.ErrNotConfigurationClaimant (Milestone 46), simulating a
//     Discovery-stream configuration claim having lapsed.
//   - FaultCancellation returns request.ErrTicketCancelled (Milestone 49),
//     simulating a cancellation request having retired this one first.
//   - FaultDrop and FaultSlow carry over unchanged: both are generic to any
//     request.Handler and were never specific to the retired watchdog/E2E
//     model in the first place.
//
// Every injected fault is a canned, immediate return — this package never
// actually drives the real e2e.Supervisor, discovery.Claim, or
// request.Dispatcher machinery to produce these outcomes organically. That
// is deliberate: faultinject exists to test a caller's handling of these
// outcomes in isolation, without needing to first orchestrate the real
// conditions (an expired watchdog, a lapsed claim, a live cancellation
// race) that would normally produce them.
//
// # Relationship to sim
//
// See sim/doc.go's own "Relationship to faultinject" note: sim models
// normal timing realism, this package injects abnormal behaviour; a
// SiL/HIL test typically composes both.
package faultinject

//fusa:req REQ-FI-001
//fusa:req REQ-FI-002
//fusa:req REQ-FI-003
//fusa:req REQ-FI-004
//fusa:req REQ-FI-005
//fusa:req REQ-FI-006
//fusa:req REQ-FI-007
//fusa:req REQ-FI-008
//fusa:req REQ-FI-009
