// Package avtp implements the IEEE 1722 AVTPDU header framing used to carry
// the OPEN Alliance TC18 Remote Control Protocol (RCP), as described by the
// "OPEN Alliance TC18 Remote Control Protocol Specification v0.5.1_RC".
//
// This package is the Phase 13 (v0.57.0) foundation of go-RCP's full TC18
// protocol replacement program (see ROADMAP.md, Part II): it supersedes the
// old bespoke wire package's 16-byte frame header with real IEEE 1722
// framing. It is intentionally scoped to AVTPDU header encoding/decoding and
// (stream_id, byte_bus_id) addressing only — no server lifecycle, register
// map, discovery, or endpoint behaviour lives here; those are later
// milestones layered on top of this one.
//
// # A note on this package's scope (RELAY spec v1.14 §13.7.2)
//
// Through v0.66.0 this package also carried the RCP-over-ACF message layer
// (ACF_ABB/ACF_GBB: the request-descriptor header, control flags, and
// short/long message encoding) directly — Message, EncodeMessage/
// DecodeMessage, ControlFlags, and Frame/EncodeFrame/DecodeFrame all lived
// here. RELAY spec v1.14's §13.7.2 cross-language module-name registry
// distinguishes that message-format concern from this package's own AVTPDU
// header-framing concern — naming them `acf` and `avtp` respectively — and
// rust-RCP and c-RCP already split them into two modules on that basis. This
// package was split to match: the message layer now lives in the sibling
// acf package (see acf/doc.go), which imports this package for Header and
// the ByteBusID/TransactionNum addressing types it carries but does not
// itself define. This package deliberately does not import acf, so that
// acf's Frame type (which needs both a Header and a Message) can depend on
// both without an import cycle.
//
// # Framing model
//
// Header is the outer IEEE 1722 Ethernet frame, in either its untimed
// "execute as soon as possible" form (NTSCF) or its presentation-
// timestamped form (TSCF). This layer owns the per-stream sequence-number
// and payload-length bookkeeping, plus — for the timestamped variant only —
// a presentation timestamp and a validity marker. It carries, but does not
// interpret, the acf package's message layer nested inside it (see
// acf.Frame).
//
// stream_id addressing is a sender MAC address plus a suffix the sender
// assigns locally (see StreamID); byte_bus_id addresses an endpoint and is
// only meaningful relative to the stream_id of the AVTPDU that carried it
// (see ByteBusID). This package does not itself track per-stream
// request/response correlation (acf.Message.TransactionNum) or any other
// per-stream state — that belongs to the RC Server/client lifecycle layered
// on top in later milestones.
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
// # Explicit non-goal (UPDATE: L2 and UDP/IP transports have since landed)
//
// This milestone (Phase 13, v0.57.0) targeted this header-framing format
// itself, not a specific transport to carry it over — at the time, that was
// deliberately left as a follow-on rather than blocking this milestone (see
// ROADMAP.md Milestone 44's original text). Two real transports carrying
// exactly this Header now exist: the sibling `udp` package (ROADMAP.md
// Milestone 54, v0.67.0) carries it over UDP/IP per IEEE 1722-2016 Annex J
// (as of the L2/Annex J transport pass documented in ROADMAP.md, this
// includes Annex J's own 4-byte encapsulation sequence number and standard
// port 17221 — udp/annexj.go was not conformant to either before that
// pass), and the new `l2` package carries it natively at layer 2 with
// EtherType 0x22F0, delivering on this milestone's own originally-stated
// primary target (see l2/doc.go). Both are permanent, equally-supported
// options; neither supersedes the other. The specification additionally
// allows CAN(FD/XL)-carried AVTPDUs, which remains a real, un-implemented
// option this package's Header does not currently target.
//
// # A note on spec fidelity (Guiding Principle 10)
//
// This package's exact numeric AVTPDU subtype tags (SubtypeNTSCF,
// SubtypeTSCF) and internal header bit-field widths have not yet been
// independently re-verified against the governing OPEN Alliance TC18
// Remote Control Protocol Specification; see the ecosystem audit tracking
// issues for known gaps. Structural behaviour — which fields exist, what
// they mean, and the validation/fallback rules around them — is what this
// milestone targets and tests; the precise numeric tag values are flagged
// here as an open item to confirm once a public interoperability reference
// becomes available, per this repo's established practice of surfacing
// spec ambiguity rather than silently guessing (see the I²C bus-speed-enum
// note at Milestone 48 for precedent).
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
