// Package avtp implements the IEEE 1722 AVTPDU/ACF wire format used to carry
// the OPEN Alliance TC18 Remote Control Protocol (RCP), as described by the
// "OPEN Alliance TC18 Remote Control Protocol Specification v0.5.1_RC".
//
// This package is the Phase 13 (v0.57.0) foundation of go-RCP's full TC18
// protocol replacement program (see ROADMAP.md, Part II): it supersedes the
// old bespoke wire package's 16-byte frame header with real IEEE 1722
// framing. It is intentionally scoped to wire encoding/decoding only — no
// server lifecycle, register map, discovery, or endpoint behaviour lives
// here; those are later milestones layered on top of this one.
//
// # Framing model
//
// Two independent layers are encoded, one nested inside the other:
//
//  1. The AVTPDU layer (Header): the outer IEEE 1722 Ethernet frame, in
//     either its untimed "execute as soon as possible" form (NTSCF) or its
//     presentation-timestamped form (TSCF). This layer owns the per-stream
//     sequence-number and payload-length bookkeeping, plus — for the
//     timestamped variant only — a presentation timestamp and a validity
//     marker.
//  2. The RCP message layer (Message): the ACF payload carried inside the
//     AVTPDU. Both message encodings — the short form (ACF_ABB) with no
//     timestamp field, and the long form (ACF_GBB) carrying an additional
//     64-bit timestamp slot — share one request-descriptor header:
//     message-kind tag, length, pad-byte count, byte_bus_id addressing,
//     transaction_num correlation, the Ack/Read/Write/Response/Error/
//     MoreSegments control bits, and a dual-purpose field that is a
//     requested read size for a plain read and a segment number once
//     MoreSegments is set.
//
// stream_id addressing is a sender MAC address plus a suffix the sender
// assigns locally (see StreamID); byte_bus_id addresses an endpoint and is
// only meaningful relative to the stream_id of the AVTPDU that carried it
// (see ByteBusID); transaction_num correlates a request with its eventual
// response and is likewise scoped to the enclosing stream (see
// Message.TransactionNum). This package does not itself track that
// correlation or any per-stream state — that belongs to the RC Server/
// client lifecycle layered on top in later milestones.
//
// # Timestamp disposition
//
// A received AVTPDU's presentation timestamp does not always demand
// scheduled execution. Header.Disposition folds a missing, invalid, or
// uncertain timestamp marker down to best-effort ("run it now") execution,
// and — independently of the marker — instructs the caller to drop a
// timestamped (TSCF) AVTPDU outright when the receiving server has no
// time-synchronization support at all, since it has no clock to schedule
// against. See Disposition for the exact precedence between these rules.
//
// # Milestone 49 addendum: FlagExtended
//
// ROADMAP.md Milestone 49 (v0.62.0, the `request` package) claims one of
// this package's two originally-reserved control bits as FlagExtended: a
// marker that a Message's Body begins with the request package's own
// conditional-request envelope rather than being a bare, endpoint-specific
// payload. This is a small, additive, backward-compatible change — every
// message this package's own tests, and every Phase 14 endpoint type, ever
// constructed left both reserved bits at zero, so nothing that decoded
// successfully before decodes differently now. One control bit remains
// reserved (required zero) after this addendum; see FlagExtended's own doc
// comment for the reasoning and request/doc.go for how it's used.
//
// # Explicit non-goal
//
// This milestone targets Ethernet-carried AVTPDUs only. The specification
// separately allows CAN(FD/XL)-carried AVTPDUs and 1722-over-UDP/IP as
// alternative transports for the same wire format; both are real options
// the spec permits, not analogous to go-RCP's old ad hoc UDP/TLS wire
// format, and are deliberately left as a follow-on rather than blocking
// this milestone (see ROADMAP.md Milestone 44).
//
// # A note on spec fidelity (Guiding Principle 10)
//
// The TC18 specification PDF is confidential to OPEN Alliance members. This
// package was built from a behavioral description of the wire format, not
// from the primary spec text, and its exact numeric AVTPDU subtype tags
// (SubtypeNTSCF, SubtypeTSCF) and internal bit-field widths for the pad,
// control, and dual-purpose fields are this implementation's own reasoned,
// self-consistent encoding rather than a verified transcription of the
// published byte assignments. Structural behaviour — which fields exist,
// what they mean, and the validation/fallback rules around them — is what
// this milestone targets and tests; the precise numeric tag values are
// flagged here as an open item to confirm once a public interoperability
// reference becomes available, per this repo's established practice of
// surfacing spec ambiguity rather than silently guessing (see the I²C
// bus-speed-enum note at Milestone 48 for precedent).
package avtp

//fusa:req REQ-AVTP-001
//fusa:req REQ-AVTP-002
//fusa:req REQ-AVTP-003
//fusa:req REQ-AVTP-004
//fusa:req REQ-AVTP-005
//fusa:req REQ-AVTP-006
//fusa:req REQ-AVTP-007
//fusa:req REQ-AVTP-008
//fusa:req REQ-AVTP-009
//fusa:req REQ-AVTP-010
//fusa:req REQ-AVTP-011
//fusa:req REQ-AVTP-012
//fusa:req REQ-AVTP-013
//fusa:req REQ-AVTP-014
//fusa:req REQ-AVTP-015
//fusa:req REQ-AVTP-016
