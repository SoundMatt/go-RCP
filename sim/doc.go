// Package sim provides timing-realistic pacing and simulated request
// latency for Software-in-the-Loop (SiL) / Hardware-in-the-Loop (HIL)
// testing against this repo's real endpoint-type packages, without physical
// ECUs.
//
// # Scope narrowed at Milestone 57 (ROADMAP.md, v0.70.0)
//
// The pre-TC18 sim.Controller (retired by this milestone) bundled three
// concerns into one type: simulated response latency, periodic Status
// publishing, and client-push watchdog-miss detection. Two of those three
// now have dedicated homes elsewhere in this repo's rebuilt satellite
// packages — watchdog/liveness detection is e2e.Supervisor's job
// (server-side, request-arrival-timed, Milestone 50), and there is no
// server-push Status broadcast left to simulate at all (Milestone 56's own
// "TC18 has no native server-push broadcast to subscribe to" framing) — so
// this package now does exactly one thing: model the passage of time a
// physical endpoint's own request handling and continuous-sampling
// behaviour would take, which nothing else in this repo's model owns.
//
// Two timing models this package provides:
//
//   - Pacer paces a caller-supplied action (e.g. an ADC channel's
//     Endpoint.Trigger, or a synthesized PWM waveform update) at a
//     configured interval, measured against an injectable Clock rather than
//     a real-time goroutine timer — filling exactly the gap ROADMAP.md's own
//     milestone text names ("ADC sample intervals, PWM cycle timing, and so
//     on") that this repo's endpoint-type packages deliberately leave to a
//     caller (see e.g. adc/doc.go's and pwm/doc.go's own "no internal
//     timer/goroutine" posture).
//   - LatencyHandler wraps any request.Handler with a configurable simulated
//     response latency (constant or jittered), the one part of the retired
//     Controller's three concerns that is still genuinely about timing
//     realism rather than fault injection (see the "Relationship to
//     faultinject" note below) or liveness detection.
//
// # Relationship to faultinject (Milestone 57)
//
// sim and faultinject both wrap request.Handler and both can add latency,
// but for different reasons: sim models what a real endpoint's timing
// *should* look like under normal operation, while faultinject
// (faultinject/faultinject.go) deliberately injects abnormal behaviour
// (dropped requests, corrupted safe points, simulated safe-state entry) to
// validate a caller's fault handling. A SiL/HIL test typically composes
// both — sim.LatencyHandler wrapping the real endpoint, and
// faultinject.Handler wrapping that — the same "faultinject is composable
// with the timing simulator" relationship the pre-TC18 sim/faultinject pair
// already documented, carried forward unchanged in spirit.
package sim

//fusa:req REQ-SIM-001
//fusa:req REQ-SIM-002
//fusa:req REQ-SIM-003
//fusa:req REQ-SIM-004
//fusa:req REQ-SIM-005
//fusa:req REQ-SIM-006
//fusa:req REQ-SIM-007
//fusa:req REQ-SIM-008
//fusa:req REQ-SIM-009
//fusa:req REQ-SIM-010
