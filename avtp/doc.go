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
// # Spec fidelity: the header layout was wrong through v8.0.0 (RESOLVED)
//
// Through v8.0.0 this doc carried an open caveat: the package's numeric
// AVTPDU subtype tags and header bit-field widths "have not yet been
// independently re-verified against the governing OPEN Alliance TC18
// Remote Control Protocol Specification". That verification has now been
// done, against §11.1 p.22 Figure 6 (NTSCF-Header Version 0) and Figure 5
// (TSCF-Header Version 0), cross-checked against the worked CRC32 examples
// on p.79 (Figure 20 for NTSCF, Figure 19 for TSCF). Both header variants
// were wrong, and neither would ever have interoperated with a conformant
// peer:
//
//   - The NTSCF header was encoded as 13 octets — subtype(1), a flags
//     octet, sequence_num(1), a plain 16-bit data-length field, then
//     stream_id(8). Figure 6 defines exactly 12: a single quadlet packing
//     subtype(8) | sv(1) | version(3) | r(1) | ntscf_data_length(11) |
//     sequence_num(8), immediately followed by the 64-bit stream_id, with
//     no reserved gap. The length field is 11 bits straddling two octets
//     and precedes sequence_num, rather than following it as a whole
//     16-bit word.
//
//   - The TSCF header was encoded as that same wrong 13-octet layout plus a
//     bare 4-byte timestamp, 17 octets in all. Figure 5 defines 24: the
//     "subtype data" quadlet (subtype | sv | version | mr | rsv | tv |
//     sequence_num | reserved | tu), stream_id(64), avtp_timestamp(32), a
//     reserved "Format specific" quadlet, and a "Packet Info" quadlet
//     carrying stream_data_length(16) plus 16 reserved bits. Its
//     timestamp-validity marker is two separate single bits (tv and tu) in
//     two different octets, not the 2-bit field this package packed into
//     the flags octet.
//
//   - SubtypeTSCF was 0x83, apparently derived as "one past NTSCF's 0x82"
//     rather than read off the specification. Figure 5 labels the field
//     "subtype(0x05)". SubtypeNTSCF's 0x82 was correct.
//
// Fixing all three is a wire-format break with no compatibility shim, the
// same posture every prior TC18-conformance pass in ROADMAP.md took: a
// wrong wire format was never something a shim could paper over. See
// ROADMAP.md's v9.0.0 section.
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
//fusa:req REQ-AVTP-017
//fusa:req REQ-AVTP-018
//fusa:req REQ-AVTP-019
//fusa:req REQ-AVTP-020

// TC18 normative-surface coverage (see .fusa-reqs.json REQ-TC18-*): clauses
// this package already satisfies but which carried no requirement of their
// own until the TC18 coverage pass. Clauses this package does NOT satisfy are
// recorded in package tc18gap instead, not here.
//fusa:req REQ-TC18-117
