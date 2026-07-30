// Package udp implements an IEEE 1722-over-UDP/IP transport for the OPEN
// Alliance TC18 Remote Control Protocol (RCP), as described by the "OPEN
// Alliance TC18 Remote Control Protocol Specification v0.5.1_RC".
//
// This is ROADMAP.md Milestone 54 (v0.67.0)'s rebuild of the old `udp`
// package: through v0.66.1 this package (and the now-retired `wire`
// package it depended on) carried the pre-TC18 bespoke Command/Response/
// Status protocol over a 16-byte custom frame header. That header, and the
// package built around it, are REPLACE-flagged outright by Phase 17's
// disposition table — "the new wire format is IEEE 1722 AVTPDU/ACF (Phase
// 13), not a variant of this one" — so this rebuild carries over only the
// socket dial/listen/read-loop/pending-request-map scaffolding, not any of
// the old encoding.
//
// # Why `wire` was retired rather than rebuilt
//
// avtp/doc.go's own "Explicit non-goal" section already named 1722-over-
// UDP/IP as a real transport variant the specification permits, left for a
// later milestone; this package is that milestone. Once avtp.Header and
// acf.Message/acf.Frame (Phase 13) exist, they already are the wire format
// — a separate `wire` package sitting between them and this one would only
// re-export acf.EncodeFrame/DecodeFrame under new names for no behavioral
// benefit, the same "no successor package" reasoning Milestone 53 (v0.66.0)
// already applied when the old CRC-16 `e2e` package was retired outright in
// favor of the already-existing `crcsafe` (see ROADMAP.md). So `wire` is
// deleted, not rebuilt, and this package imports avtp/acf directly. Unlike
// TLS or a future stream-oriented transport, UDP also needs no additional
// length-prefixing on top of acf.Frame's own bytes: one UDP datagram
// already carries exactly one AVTPDU, so the datagram boundary alone
// delimits the frame.
//
// # Shape of this package
//
// Controller is the client side: it addresses one destination RC Server by
// its UDP address and presents its own avtp.StreamID identity on every
// AVTPDU it sends, correlating requests to responses by
// acf.Message.TransactionNum (in place of the old wire package's uint32
// Command.ID). Server is the listening side: it decodes each inbound
// datagram and hands the request to a Router, which special-cases
// byte_bus_id EP0 (configuration/discovery, answered against a
// *server.Server — see ep0.go) and otherwise looks up a caller-registered
// request.Handler by avtp.ByteBusID — the exact interface every Phase 14/16
// endpoint-type package's own Endpoint.HandleRequest method already
// satisfies, so registering gpio.Endpoint, spi.Endpoint, or a
// request.Dispatcher wrapping one, needs no adapter code. Router.Route also
// owns the one dispatch-wide decision every endpoint type shares: computing
// avtp.Header.Disposition against this server's own time-sync capability,
// and dropping (no reply at all) a timestamped AVTPDU a non-time-
// synchronized server cannot honor, rather than leaving every registered
// Handler to reimplement that rule itself.
//
// # Explicit non-goals
//
// This milestone originates only untimed (NTSCF) AVTPDUs from Controller —
// scheduling a request for presentation-timestamped (TSCF) delivery has no
// caller-facing API here yet, since no clock source/scheduler exists in
// this repo to target. Router does correctly receive and evaluate a TSCF
// header's Disposition (so a future timed sender already interoperates),
// but a DispositionScheduled request is executed immediately, identically
// to DispositionBestEffort — actual schedule-and-wait-for-the-timestamp
// behaviour is left as a follow-on, tracked here rather than silently
// guessed at, per this repo's Guiding Principle 10 posture. Multi-AVTPDU
// fragmentation (ROADMAP.md Milestone 52's fragment package) is not wired
// into this transport either; a caller that needs it composes
// fragment.Gateway in front of a Router-registered request.Dispatcher
// itself, the same "wrap, don't edit" posture Milestone 49 established.
//
// # Error-response wire shape
//
// The error-response wire shape this package introduces — Control's
// FlagError bit set, Body carrying a numeric ErrorCode as its leading byte
// (see errorCodeFor and EncodeErrorBody/DecodeErrorBody), with an optional
// UTF-8 diagnostic string trailing it — has no precedent elsewhere in this
// repo: every earlier milestone's Handler-shaped code returns a bare Go
// error up the call stack and leaves wire-level error framing to whichever
// transport calls it, which is exactly this package's own job. ErrorCode's
// eight defined values are a subset of the specification's own
// request-response error-code enumeration, and each one's numeric value is
// that enumeration's own verified assignment (measured directly against the
// specification's own error-code table) rather than a locally invented
// sequence — see errorcode.go's const block. The mapping from this repo's
// internal Go errors onto those codes (errorCodeFor) is this
// implementation's own reasoned choice where more than one code plausibly
// fits (see errorCodeFor's doc comment), not a verified transcription of
// the source specification's own error-condition-to-internal-error
// correspondence, since no such correspondence exists to transcribe in the
// first place.
package udp

//fusa:req REQ-UDP-001
//fusa:req REQ-UDP-002
//fusa:req REQ-UDP-003
//fusa:req REQ-UDP-004
//fusa:req REQ-UDP-005
//fusa:req REQ-UDP-006
//fusa:req REQ-UDP-007
//fusa:req REQ-UDP-008
//fusa:req REQ-UDP-009
//fusa:req REQ-UDP-010
//fusa:req REQ-UDP-011
//fusa:req REQ-UDP-012
//fusa:req REQ-UDP-013
//fusa:req REQ-UDP-014
