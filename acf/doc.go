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
// Both message encodings — the short form (ACF_ABB, MessageKind
// KindShort) with no timestamp field, and the long form (ACF_GBB,
// KindLong) carrying an additional 64-bit timestamp slot — share one
// request-descriptor header: message-kind tag, length, pad-byte count,
// byte_bus_id addressing, transaction_num correlation, the Ack/Read/Write/
// Response/Error/MoreSegments control bits, and a dual-purpose field that
// is a requested read size for a plain read and a segment number once
// MoreSegments is set. transaction_num correlates a request with its
// eventual response and is scoped to the enclosing AVTPDU's stream_id; this
// package carries that correlation field but does not itself track it —
// that belongs to the RC Server/client lifecycle layered on top in later
// milestones.
//
// Frame composes one avtp.Header with one Message into the single
// contiguous AVTPDU buffer EncodeFrame/DecodeFrame produce and consume.
//
// # Milestone 49 addendum: FlagExtended
//
// ROADMAP.md Milestone 49 (v0.62.0, the `request` package) claims one of
// this package's two originally-reserved control bits as FlagExtended: a
// marker that a Message's Body begins with the request package's own
// conditional-request envelope rather than being a bare, endpoint-specific
// payload. This was a small, additive, backward-compatible change — every
// message this package's own tests, and every Phase 14 endpoint type, ever
// constructed left both reserved bits at zero, so nothing that decoded
// successfully before decodes differently now. One control bit remains
// reserved (required zero) after this addendum; see FlagExtended's own doc
// comment for the reasoning and request/doc.go for how it's used.
//
// # A note on spec fidelity (Guiding Principle 10)
//
// The TC18 specification PDF is confidential to OPEN Alliance members. This
// package was built from a behavioral description of the wire format, not
// from the primary spec text, and its internal bit-field widths for the
// pad, control, and dual-purpose fields are this implementation's own
// reasoned, self-consistent encoding rather than a verified transcription
// of the published byte assignments. Structural behaviour — which fields
// exist, what they mean, and the validation/fallback rules around them — is
// what this milestone targets and tests; the precise numeric values are
// flagged here as an open item to confirm once a public interoperability
// reference becomes available, per this repo's established practice of
// surfacing spec ambiguity rather than silently guessing (see the I²C
// bus-speed-enum note at Milestone 48 for precedent).
package acf

//fusa:req REQ-AVTP-011
//fusa:req REQ-AVTP-012
//fusa:req REQ-AVTP-013
//fusa:req REQ-AVTP-014
//fusa:req REQ-AVTP-015
//fusa:req REQ-AVTP-016
