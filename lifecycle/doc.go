// Package lifecycle implements the RC Server's three-state configuration
// lifecycle state machine for the OPEN Alliance TC18 Remote Control
// Protocol (RCP), as described by the "OPEN Alliance TC18 Remote Control
// Protocol Specification v0.5.1_RC".
//
// # A note on this package's history (RELAY spec v1.14 §13.7.2)
//
// Through v0.66.0 this state machine lived directly in the server package
// alongside the register-map model (regmap) and the discovery mechanism
// (discovery). RELAY spec v1.14's §13.7.2 cross-language module-name
// registry distinguishes all three as separate concerns — naming them
// `lifecycle`, `regmap`, and `discovery` respectively — and cpp-RCP,
// rust-RCP, and c-RCP already split them into three modules on that basis;
// this package was split out to match. The server package remains: it now
// composes lifecycle.LifecycleState with a *regmap.RegisterMap and a
// discovery.Claim into the Server type, and owns every transition guard and
// access-control decision, mirroring the composition shape ROADMAP.md's
// other satellite packages already use.
//
// LifecycleState itself carries no behavior beyond String() — every
// transition guard (validating a pin-mapping table, checking every
// endpoint's functional block is populated, validating the queue
// configuration) needs data this package deliberately does not hold, so
// those guards stay server.Server methods; see server/server.go's
// AdvanceToHWLocked and AdvanceToFullyConfigured.
//
// The REQ-RCS-* //fusa:req requirement declarations for the RC Server as a
// whole (lifecycle transitions included) stay in server/doc.go rather than
// being redistributed across this package and its siblings — go-FuSa's
// traceability check (gofusa trace) matches requirement tags anywhere in
// the repository, not per package, and the transition guards those
// requirements describe are themselves server.Server methods (see above).
package lifecycle
