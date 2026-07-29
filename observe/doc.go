// Package observe wraps an RC client with OpenTelemetry tracing and a
// pluggable metrics hook for Prometheus-compatible instrumentation.
//
// # Re-pointed at the Controller-equivalent interface at Milestone 57
// (ROADMAP.md, v0.70.0)
//
// Per Phase 17's disposition table, this package is ADAPT-flagged:
// OpenTelemetry tracing/metrics decoration is a generic wrapper pattern,
// re-pointed here at *udp.Controller's own Request/Read/Write shape in
// place of the retired rcp.Controller's Send. The retired Subscribe
// instrumentation is dropped outright, not adapted — TC18 has no native
// server-push broadcast to subscribe to (Milestone 56's own framing), so
// there is nothing left to instrument there.
package observe

//fusa:req REQ-OB-001
//fusa:req REQ-OB-002
//fusa:req REQ-OB-003
//fusa:req REQ-OB-004
//fusa:req REQ-OB-005
//fusa:req REQ-OB-006
//fusa:req REQ-OB-007
//fusa:req REQ-OB-008
