// Package server implements the RC Server configuration lifecycle and
// register map for the OPEN Alliance TC18 Remote Control Protocol (RCP), as
// described by the "OPEN Alliance TC18 Remote Control Protocol
// Specification v0.5.1_RC".
//
// This package is the Phase 13 (v0.58.0) layer built directly on top of the
// avtp package's wire format (see ROADMAP.md, Part II, Milestone 45):
// avtp.ByteBusID is the addressing unit the register map and EP0 organize
// around, avtp.StreamID is what the root-client/restricted-stream access
// model keys off of, and avtp.Message's Read/Write/Response control flags
// are the request/response shape register operations are expected to ride
// on once a later milestone wires this package to the wire-format layer.
// This package does not itself encode/decode avtp.Message or avtp.Frame —
// it stops at producing and consuming the register-map byte buffers those
// messages would carry as their Body.
//
// # Lifecycle
//
// A Server has exactly three configuration states (see LifecycleState):
// StateUnconfigured, StateHWLocked, and StateFullyConfigured. It only ever
// advances forward, one state at a time, and only through a guard that
// rejects an implausible configuration rather than silently accepting it —
// see Server.AdvanceToHWLocked and Server.AdvanceToFullyConfigured. Once a
// register field's lock class closes for the current state, it stays
// closed for every requester for the rest of that server's lifetime,
// including the root client (see the register-locking errors in errors.go).
//
// # Register-map structure
//
// The map splits, per endpoint, into two independent blocks: a
// GenericEndpointBlock the server itself owns (address, declared type,
// enabled flag), and that endpoint's own FunctionalBlock (opaque,
// type-specific configuration bytes this milestone doesn't interpret — see
// the "explicit non-goal" section below). Alongside the per-endpoint
// entries, the map carries one GeneralBlock (identification, protocol
// version, capability/capacity counters, and table pointers), one PinMap
// (the HW pin-mapping table), one StreamLimits table, and one QueueConfig
// table. EncodeRegisterMap/DecodeRegisterMap serialize the whole structure
// to and from a single contiguous buffer; GeneralBlock's own pointer fields
// are always recomputed at encode time rather than trusted from the
// caller, mirroring how avtp.EncodeFrame recomputes Header.DataLength
// instead of accepting a value that could drift from the truth.
//
// # EP0 and the root client
//
// EP0 (byte_bus_id 0, see the EP0 constant) addresses the server itself as
// a pseudo-endpoint: Server.ReadEP0/Server.WriteEP0 operate on the whole
// register map rather than one endpoint's registers. Exactly one stream may
// hold the root-client role at a time (AccessController.ClaimRoot); it has
// full-register access, while every other stream is restricted to only the
// endpoints an operator has explicitly granted it (AccessController.Grant).
// This milestone treats EP0 itself as subject to that same grant
// requirement for a restricted stream — Discovery's universal,
// grant-independent read of register 0 (ROADMAP.md Milestone 46) is a
// deliberate exception layered on top of this package, not something this
// milestone pre-builds; see AccessController's doc comment.
//
// # HW pin mapping and named signals
//
// PinMap binds physical pins to a (endpoint, named-signal-index) pair; it
// is writable only in StateUnconfigured and is the subject of
// AdvanceToHWLocked's plausibility check (no duplicate pin claims, no
// reference to an undeclared endpoint, no signal index out of range for
// that endpoint's declared EndpointType). See SignalName and its
// surrounding doc comment in types.go for this package's own named-signal
// scheme and the spec-fidelity caveat around it.
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
// entry format, the named-signal-index scheme in types.go) are this
// implementation's own reasoned, self-consistent encoding rather than a
// verified transcription of the published register addresses — the same
// open-item posture avtp/doc.go documents for its subtype tags. Structural
// behaviour — the three-state lifecycle, the generic/functional split,
// EP0's root-client model, and the plausibility checks gating each
// transition — is what this milestone targets and tests; the precise wire
// byte assignments are flagged here as pending confirmation against a
// public interoperability reference, consistent with this repo's
// established practice of surfacing spec ambiguity rather than silently
// guessing (see also the I²C bus-speed-enum note referenced at Milestone
// 48).
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
