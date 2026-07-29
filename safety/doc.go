// Package safety provides latency and timing evidence for ASIL-B compliance.
// Run tests with RCP_LATENCY_DURATION=30s to produce COMMAND_LATENCY.md.
//
// # Milestone 58 (v0.71.0): re-pointed at the TC18 request/response path
//
// Through Milestone 57 (v0.70.0) TestCommandLatencyProfile measured the
// retired pre-TC18 protocol's rcp.Controller.Send against an in-process
// mock.Controller — a path this program no longer builds toward (see
// ROADMAP.md's Phase 18 cutover). This milestone re-points the same
// measurement methodology (P50/P99/P99.9/Max Send-latency percentiles,
// sustained ~64 MiB/s GC allocation pressure, a 100 Hz watchdog-rate
// workload, and the same GSN evidence writeup shape in
// COMMAND_LATENCY.md) at the TC18 path this program actually ships: a real
// loopback UDP socket, request.Dispatcher on the server side, and
// udp.Controller.Write on the client side — see
// command_latency_test.go's own doc comment for the setup.
package safety

//fusa:req REQ-SAFETY-001
