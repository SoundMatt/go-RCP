// Package acf implements the RCP-over-ACF message layer of the OPEN
// Alliance TC18 Remote Control Protocol (RCP), as described by the "OPEN
// Alliance TC18 Remote Control Protocol Specification v0.5.1_RC": the
// ACF_ABB/ACF_GBB request-descriptor header, control flags, short/long
// message encoding, and the combined Frame that carries one such message
// inside an avtp.Header.
//
// # A note on this package's history (RELAY spec v1.14 §13.7.2)
//
// Through v0.66.0 this message layer lived directly in the avtp package
// (Message, EncodeMessage/DecodeMessage, ControlFlags, and Frame/
// EncodeFrame/DecodeFrame). RELAY spec v1.14's §13.7.2 cross-language
// module-name registry distinguishes this message-format concern (`acf`)
// from the avtp package's own AVTPDU header-framing concern (`avtp`) —
// rust-RCP and c-RCP already split them into two modules on that basis —
// so this package was split out to match. It imports avtp for Header (used
// by Frame) and for the ByteBusID/TransactionNum addressing types Message
// carries but does not itself define; avtp does not import acf (see
// avtp/doc.go for why, and for the framing model this package's messages
// are carried inside).
//
// # Message model
//
// Both message encodings — the short form (ACF_ABB, MessageKind KindShort)
// with no message_timestamp field, and the long form (ACF_GBB, KindLong)
// carrying an additional 8-byte message_timestamp slot inserted between the
// descriptor's two 32-bit words — share one request-descriptor header:
// acf_msg_type, acf_msg_length (in quadlets), pad, mtv, byte_bus_id, evt,
// hs, cs, transaction_num, the op/rsp/err/ms control bits (surfaced as
// ControlFlags — see its doc comment for how the exported flag names map
// onto those bits), and a dual-purpose field that is a requested read size
// for a plain read and a segment number once FlagMoreSegments is set.
// transaction_num correlates a request with its eventual response and is
// scoped to the enclosing AVTPDU's stream_id; this package carries that
// correlation field but does not itself track it — that belongs to the RC
// Server/client lifecycle layered on top in later milestones.
//
// Frame composes one avtp.Header with one Message into the single
// contiguous AVTPDU buffer EncodeFrame/DecodeFrame produce and consume.
//
// # v2.0 wire-format correction
//
// Earlier versions of this package (through v1.0.0) used a wholly bespoke
// 10-byte descriptor with no evt/hs/cs/mtv fields and control bits that did
// not correspond to any OPEN Alliance TC18 Remote Control Protocol
// Specification v0.5.1_RC field. This version replaces that layout with the
// specification's actual two-word descriptor (see EncodeMessage/
// DecodeMessage), adds the previously entirely-missing EVT/HS/CS/MTV
// fields, corrects acf_msg_length to be measured in quadlets rather than
// octets, and moves the message_timestamp slot to its documented position
// between the two descriptor words rather than after both. This is a
// breaking wire-format change; go-RCP v2.0.0 cannot interoperate with
// go-RCP v1.x on the wire.
//
// # The evt field (§13.5 Table 30)
//
// evt[2:0] is the write-semantic selector / config-vs-data discriminator
// every endpoint type's request handling turns on: it decides whether a
// request's byte_msg_payload reaches the physical interface at all, and if
// so how it combines with that interface's current status. TC18 §13.5
// Table 30 states those rules once, as three endpoint-type rows, so this
// package implements them once too — see evt.go's EVTClass, ClassifyEVT,
// Message.EVTDisposition and ApplyEVTWriteOp, which every endpoint-type
// package calls into rather than re-deriving Table 30 for itself. §12.9.1's
// general "evt[2:0] ≠ 0 with no byte_msg_payload" rule lives there too, as
// CheckEVTPayloadPresence.
//
// Through go-RCP v7.0.0 this was a genuine gap: EVT was carried at its
// correct wire position and was readable/settable by callers, but no
// endpoint package interpreted it, and gpio/spi invented their own in-band
// selector bytes instead. See CHANGELOG/ROADMAP for the breaking change
// that closed it.
package acf

//fusa:req REQ-AVTP-011
//fusa:req REQ-AVTP-012
//fusa:req REQ-AVTP-013
//fusa:req REQ-AVTP-014
//fusa:req REQ-AVTP-015
//fusa:req REQ-AVTP-016
//fusa:req REQ-EVT-001
//fusa:req REQ-EVT-002
//fusa:req REQ-EVT-003
//fusa:req REQ-EVT-004
//fusa:req REQ-EVT-005
//fusa:req REQ-EVT-006
//fusa:req REQ-EVT-007
