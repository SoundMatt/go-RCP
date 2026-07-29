// Package codegen generates typed Go request.Handler endpoint stubs and
// go-FuSa requirement skeletons from a manifest YAML/JSON file.
//
// # Manifest schema moved at Milestone 57 (ROADMAP.md, v0.70.0)
//
// Per Phase 17's disposition table, this package is ADAPT-flagged:
// manifest-to-stub code generation is reusable, but the manifest schema
// itself moves from zone declarations to server/endpoint declarations. A
// manifest now declares one or more servers, each with its own
// avtp.StreamID identity (16 hex characters, the same "stream_id" field
// name and encoding config.ServerEntry already established at Milestone
// 55, kept consistent here rather than inventing a second convention), and
// each server declares the endpoints it exposes (a caller-chosen name, an
// avtp.ByteBusID address, a free-text Type label, and an ASIL). The
// generator emits, per declared endpoint:
//
//   - A Go source file with a stub type implementing request.Handler,
//     directly registrable into a *udp.Router (Router.Register) — the new
//     model's equivalent of the retired generator's per-zone
//     rcp.Controller stub.
//   - A matching _test.go skeleton with //fusa:test annotations.
//   - JSON entries for .fusa-reqs.json ready for go-FuSa compliance.
//
// # A note on the Type field (Guiding Principle 10 / spec fidelity)
//
// EndpointSpec.Type is a free-text label (e.g. "gpio", "adc"), not
// validated against regmap.EndpointType's own closed enum: this package is
// reused across every future endpoint-type milestone, and new endpoint
// types keep landing on their own schedule independent of this generator's
// release cadence — a closed-enum validation here would need a matching
// release of this package every time regmap.EndpointType gains a value,
// which defeats the point of a manifest-driven generator. A caller wanting
// requestedType safety maps EndpointSpec.Type to a regmap.EndpointType at
// the call site that consumes GeneratedFile output, not inside this
// package.
package codegen

//fusa:req REQ-CG-001
//fusa:req REQ-CG-002
//fusa:req REQ-CG-003
//fusa:req REQ-CG-004
//fusa:req REQ-CG-005
//fusa:req REQ-CG-006
//fusa:req REQ-CG-007
//fusa:req REQ-CG-008
//fusa:req REQ-CG-009
