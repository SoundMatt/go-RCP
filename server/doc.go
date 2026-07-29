// Package server implements the RC Server for the OPEN Alliance TC18 Remote
// Control Protocol (RCP), as described by the "OPEN Alliance TC18 Remote
// Control Protocol Specification v0.5.1_RC": the Server type that composes
// the lifecycle package's state machine, the regmap package's register-map
// model, and the discovery package's discovery mechanism into the single
// lifecycle-gated, access-controlled read/write surface a client interacts
// with.
//
// This package is the Phase 13 (v0.58.0) layer built directly on top of the
// avtp package's wire format (see ROADMAP.md, Part II, Milestone 45):
// avtp.ByteBusID is the addressing unit the register map and EP0 organize
// around, avtp.StreamID is what the root-client/restricted-stream access
// model keys off of, and acf.Message's Read/Write/Response control flags
// are the request/response shape register operations are expected to ride
// on once a later milestone wires this package to the wire-format layer.
// This package does not itself encode/decode acf.Message or acf.Frame —
// it stops at producing and consuming the register-map byte buffers those
// messages would carry as their Body.
//
// # A note on this package's shape (RELAY spec v1.14 §13.7.2)
//
// Through v0.66.0 this package directly implemented the lifecycle state
// machine, the register-map model, and the discovery mechanism all in one.
// RELAY spec v1.14's §13.7.2 cross-language module-name registry
// distinguishes those as three separate concerns — naming them
// `lifecycle`, `regmap`, and `discovery` — and cpp-RCP, rust-RCP, and
// c-RCP already split them into three modules on that basis, so this
// package's own data model and pure logic moved out into the three sibling
// packages of those names to match. What remains here — the Server type,
// its mutex, and every method below — is the composition and orchestration
// those three packages deliberately do not do themselves: Server is the
// only thing in this repo that holds a lifecycle.LifecycleState, a
// *regmap.RegisterMap, a *regmap.AccessController, and a discovery.Claim
// together and enforces the rules that span more than one of them (e.g.
// AdvanceToHWLocked's guard reads both the current LifecycleState and the
// RegisterMap's PinMap; WriteEP0 checks AccessController before touching
// the RegisterMap at all). This mirrors the composition shape
// ROADMAP.md's other satellite packages already use (e.g. watchdog
// orchestrating e2e.Supervisor without owning its data model).
//
// # Lifecycle
//
// A Server has exactly three configuration states (see
// lifecycle.LifecycleState): lifecycle.StateUnconfigured,
// lifecycle.StateHWLocked, and lifecycle.StateFullyConfigured. It advances
// forward one state at a time, only through a guard that rejects an
// implausible configuration rather than silently accepting it — see
// Server.AdvanceToHWLocked and Server.AdvanceToFullyConfigured — and can
// move backward exactly once, from StateHWLocked to StateUnconfigured, via
// Server.DemoteToUnconfigured, gated to the root client or whichever stream
// currently holds the Discovery-stream configuration claim (see
// ClaimConfiguration). Most register fields lock for the rest of the
// server's lifetime once their lock class closes for the current state,
// including for the root client (see regmap's register-locking errors);
// the one exception is each endpoint's own functional (type-specific)
// configuration block, which stays writable — via that endpoint's own
// registered stream(s), or via the root client through EP0 — even once
// lifecycle.StateFullyConfigured is reached (see WriteFunctional and
// WriteEP0). There is no reverse transition out of
// lifecycle.StateFullyConfigured.
//
// # Register-map structure
//
// The map (regmap.RegisterMap) splits, per endpoint, into two independent
// blocks: a GenericEndpointBlock the server itself owns (address, declared
// type, enabled flag), and that endpoint's own FunctionalBlock (opaque,
// type-specific configuration bytes this milestone doesn't interpret — see
// the "explicit non-goal" section below). Alongside the per-endpoint
// entries, the map carries one GeneralBlock (identification, protocol
// version, capability/capacity counters, and table pointers), one PinMap
// (the HW pin-mapping table), one StreamLimits table, and one QueueConfig
// table. regmap.EncodeRegisterMap/regmap.DecodeRegisterMap serialize the
// whole structure to and from a single contiguous buffer; GeneralBlock's
// own pointer fields are always recomputed at encode time rather than
// trusted from the caller, mirroring how acf.EncodeFrame recomputes
// Header.DataLength instead of accepting a value that could drift from the
// truth.
//
// # EP0 and the root client
//
// EP0 (byte_bus_id 0, see the regmap.EP0 constant) addresses the server
// itself as a pseudo-endpoint: Server.ReadEP0/Server.WriteEP0 operate on
// the whole register map rather than one endpoint's registers. Exactly one
// stream may hold the root-client role at a time
// (regmap.AccessController.ClaimRoot); it has full-register access, while
// every other stream is restricted to only the endpoints an operator has
// explicitly granted it (regmap.AccessController.Grant). This milestone
// (45) treats EP0 itself as subject to that same grant requirement for a
// restricted stream. Milestone 46 (Discovery, see discovery.go) layers its
// own universal, grant-independent, lifecycle-state-independent read of
// register 0 on top of this package — Server.ReadDiscovery and
// Server.HandleDiscoveryRequest — plus a timeout-releasable
// Discovery-stream configuration claim (Server.ClaimConfiguration) that
// coexists with, but is distinct from, the root-client claim described
// above; see regmap.AccessController's doc comment and discovery/doc.go's
// package-level notes.
//
// # Discovery (Milestone 46)
//
// discovery.go adds: a register-0 read (Server.ReadDiscovery) answerable in
// any LifecycleState and regardless of AccessController grants;
// Server.HandleDiscoveryRequest, which additionally enforces that a
// discovery request arrived on the untimed (NTSCF) AVTPDU header, dropping
// a timestamped (TSCF) one outright; Server.ClaimConfiguration and its
// companion Server.ConfigurationClaimant/Server.ReleaseConfigurationClaim,
// a configurable-timeout reservation of configuration rights scoped to one
// stream at a time, independent of AccessController.ClaimRoot. The
// discovery package itself additionally provides
// IsConformantServer/Topology/DiscoverTopology/WriteTopology/ReadTopology,
// client-side helpers for recognizing a conformant server from a discovery
// response and persisting its topology so re-discovery isn't mandatory
// every power cycle.
//
// # HW pin mapping and named signals
//
// regmap.PinMap binds physical pins to a (endpoint, named-signal-index)
// pair; it is writable only in lifecycle.StateUnconfigured and is the
// subject of AdvanceToHWLocked's plausibility check (no duplicate pin
// claims, no reference to an undeclared endpoint, no signal index out of
// range for that endpoint's declared EndpointType). See
// regmap.SignalName and its surrounding doc comment for this package's own
// named-signal scheme and the spec-fidelity caveat around it.
//
// # Explicit non-goal
//
// This milestone defines the register-map structure and its lifecycle
// rules, not the type-specific functional register layouts for any real
// endpoint type (GPIO, SPI, I²C, ...). FunctionalBlock carries opaque bytes
// for exactly that reason: interpreting them is Phase 14's (Milestones
// 47/48) and Phase 16's (Milestone 51) job, layered on top of the
// generic/functional split this package establishes.
//
// # A spec-fidelity note: endpoint-address ordering is a client obligation
//
// The specification's own review commentary calls out that the ordering
// client tooling uses when it walks a server's endpoint-address mapping
// table is a client-side obligation with no help from the wire format
// itself — nothing in the register map or its encoding enforces or even
// signals the "correct" order a client should assume. RegisterMap.Addresses
// returns this package's declared endpoints in ascending byte_bus_id order,
// and EncodeRegisterMap's endpoint table follows that same order, purely
// because ascending order is a reasonable, deterministic default for this
// implementation's own encode/decode round-trip — it is not, and cannot be,
// an enforcement of whatever ordering rule a real client is obligated to
// apply. go-RCP's own client-side configuration tooling is deferred to the
// Phase 17 control-plane migration (ROADMAP.md Milestone 55); that future
// tooling — not this package, and not the wire format — is where the
// client's ordering obligation must actually be enforced. This is an open
// design item carried forward, not a gap silently closed by this
// milestone's encode/decode order matching what a well-behaved client would
// expect today.
//
// # A note on spec fidelity (Guiding Principle 10)
//
// The TC18 specification PDF is confidential to OPEN Alliance members. This
// package was built from a behavioral description of the RC Server
// lifecycle and register map, not from the primary spec text. Its register
// byte layouts (GeneralBlock's field order and widths, the pin-mapping
// entry format, the named-signal-index scheme) are this implementation's
// own reasoned, self-consistent encoding rather than a verified
// transcription of the published register addresses — the same open-item
// posture avtp/doc.go documents for its subtype tags. Structural behaviour
// — the three-state lifecycle, the generic/functional split, EP0's
// root-client model, and the plausibility checks gating each transition —
// is what this milestone targets and tests; the precise wire byte
// assignments are flagged here as pending confirmation against a public
// interoperability reference, consistent with this repo's established
// practice of surfacing spec ambiguity rather than silently guessing (see
// also the I²C bus-speed-enum note referenced at Milestone 48). Two pieces
// of this package's lifecycle behaviour have since been checked directly
// against the RC Server lifecycle chapter and now match it: the
// StateHWLocked→StateUnconfigured reverse transition (DemoteToUnconfigured)
// exists because the specification says it does, not as a guess; and
// per-endpoint functional configuration staying writable in
// StateFullyConfigured (WriteFunctional, WriteEP0) reflects the
// specification's own statement that only server-wide/HW-pin configuration
// locks permanently at that state, not the per-endpoint functional blocks.
package server

//fusa:req REQ-RCS-001
//fusa:req REQ-RCS-002
//fusa:req REQ-RCS-003
//fusa:req REQ-RCS-004
//fusa:req REQ-RCS-005
//fusa:req REQ-RCS-006
//fusa:req REQ-RCS-007
//fusa:req REQ-RCS-008
//fusa:req REQ-RCS-009
//fusa:req REQ-RCS-010
//fusa:req REQ-RCS-011
//fusa:req REQ-RCS-012
//fusa:req REQ-RCS-013
//fusa:req REQ-RCS-014
//fusa:req REQ-RCS-015
//fusa:req REQ-RCS-016
//fusa:req REQ-RCS-017
//fusa:req REQ-RCS-018
//fusa:req REQ-RCS-019
//fusa:req REQ-RCS-020
//fusa:req REQ-RCS-021
//fusa:req REQ-RCS-022
//fusa:req REQ-RCS-023
//fusa:req REQ-RCS-024
//fusa:req REQ-RCS-025
//fusa:req REQ-RCS-026
//fusa:req REQ-RCS-027
//fusa:req REQ-RCS-028
//fusa:req REQ-RCS-029
//fusa:req REQ-RCS-030
//fusa:req REQ-RCS-020
//fusa:req REQ-RCS-031
