// Package regmap implements the RC Server register-map model — the general
// server block, the generic/functional per-endpoint split, the HW
// pin-mapping table, the request-stream/queue configuration tables, and the
// EP0 root-client/grant access-control model that gates reads and writes
// against all of it — for the OPEN Alliance TC18 Remote Control Protocol
// (RCP), as described by the "OPEN Alliance TC18 Remote Control Protocol
// Specification v0.5.1_RC".
//
// # A note on this package's history (RELAY spec v1.14 §13.7.2)
//
// Through v0.66.0 this model lived directly in the server package alongside
// the lifecycle state machine (lifecycle) and the discovery mechanism
// (discovery). RELAY spec v1.14's §13.7.2 cross-language module-name
// registry distinguishes all three as separate concerns — cpp-RCP,
// rust-RCP, and c-RCP already split them into three modules on that basis
// — so this package was split out to match. The server package remains: it
// composes a *regmap.RegisterMap and a *regmap.AccessController with a
// lifecycle.LifecycleState into the Server type, and owns every lifecycle
// transition guard and the whole-map read/write surface (ReadEP0/WriteEP0)
// — see server/server.go.
//
// # Register-map structure
//
// The map splits, per endpoint, into two independent blocks: a
// GenericEndpointBlock the server itself owns (address, declared type,
// enabled flag), and that endpoint's own FunctionalBlock (opaque,
// type-specific configuration bytes). Alongside the per-endpoint entries,
// the map carries one GeneralBlock (identification, protocol version,
// capability/capacity counters, and table pointers), one PinMap (the HW
// pin-mapping table), one StreamLimits table, and one QueueConfig table.
// EncodeRegisterMap/DecodeRegisterMap serialize the whole structure to and
// from a single contiguous buffer; GeneralBlock's own pointer fields are
// always recomputed at encode time rather than trusted from the caller,
// mirroring how acf.EncodeFrame recomputes Header.DataLength instead of
// accepting a value that could drift from the truth.
//
// # EP0 and the root client
//
// EP0 (byte_bus_id 0, see the EP0 constant) addresses the server itself as
// a pseudo-endpoint. Exactly one stream may hold the root-client role at a
// time (AccessController.ClaimRoot); it has full-register access, while
// every other stream is restricted to only the endpoints an operator has
// explicitly granted it (AccessController.Grant). EP0 itself is subject to
// that same grant requirement for a restricted stream. The discovery
// package's universal, grant-independent, lifecycle-state-independent read
// of register 0 is a deliberate, narrowly-scoped exception layered on top
// of this package by server.Server.ReadDiscovery — AccessController.CanAccess
// is unmodified by it.
//
// # A note on spec fidelity (Guiding Principle 10)
//
// The TC18 specification PDF is confidential to OPEN Alliance members. This
// package was built from a behavioral description of the RC Server
// register map, not from the primary spec text. Its register byte layouts
// (GeneralBlock's field order and widths, the pin-mapping entry format,
// the named-signal-index scheme) are this implementation's own reasoned,
// self-consistent encoding rather than a verified transcription of the
// published register addresses — the same open-item posture avtp/doc.go
// documents for its subtype tags.
package regmap
