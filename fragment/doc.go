// Package fragment implements multi-AVTPDU segmentation and reassembly for
// the OPEN Alliance TC18 Remote Control Protocol (RCP), as described by the
// "OPEN Alliance TC18 Remote Control Protocol Specification v0.5.1_RC".
//
// This is the Phase 16 (v0.65.0) layer ROADMAP.md Milestone 52 calls for: a
// segmentation/reassembly layer sitting on top of the already-shipped Phase
// 13 wire format (avtp), turning the FlagMoreSegments control bit and the
// dual-purpose ReadSizeOrSegment field avtp/message.go already defines (but
// leaves unexercised — see avtp/frame.go's own forward reference to this
// milestone) into a working multi-frame transfer mechanism. Fragmentation
// itself is explicitly optional in the source specification; ROADMAP.md
// Milestone 52 makes the binding call that this repo implements it anyway,
// as a v1.0.0 requirement, because three already-shipped pieces are
// materially incomplete without it: CAN XL's up-to-2048-byte payloads (can,
// Milestone 51) do not fit a single frame on realistic link MTUs, UART's
// FIFO-drain read-completion path (uart, Milestone 48) is designed around
// partial/fragmented delivery, and a large server register-map discovery
// read (server/discovery.go, Milestone 46) has no other way to exceed one
// frame's payload as a deployment's endpoint count grows.
//
// # Two independent halves
//
// Split (segment.go) is the send-side half: given one logical acf.Message
// whose Body exceeds a caller-chosen per-segment byte budget, it returns the
// ordered acf.Message segments to transmit, each a real, independently
// encodable AVTPDU-carried message via acf.EncodeMessage/acf.EncodeFrame
// — this package adds no wire format of its own, only a splitting/
// recombination policy layered on the one avtp already ships.
//
// Reassembler (reassembler.go) is the receive-side half: Add accumulates
// segments, keyed by Key (a message's ByteBusID/TransactionNum pair scoped
// to its enclosing AVTPDU's StreamID — the exact addressing/correlation
// scope avtp/doc.go already documents those two fields as having), until
// the terminal segment (FlagMoreSegments clear) arrives; Finish or
// FinishProtected then returns the reassembled logical Message. Reassembler
// treats an ordinary, never-fragmented Message as a trivial one-segment
// sequence that is immediately complete, so a caller never has to decide
// up front whether a given inbound Message is fragmented at all — every
// Message, fragmented or not, goes through the same Add/Finish call shape.
//
// # The E2E-CRC interaction rule
//
// e2e/crc.go's ComputeFragmented function — shipped ahead of this
// milestone specifically so this package could depend on it, per its own
// doc comment — already pins down the rule this package's
// Reassembler.FinishProtected wires in: only the final segment of a
// fragmented message carries a trailing CRC32 safe point, and that CRC is
// computed over the segments' combined Body, not the final segment alone.
// FinishProtected calls ComputeFragmented directly over this Reassembler's
// own per-segment Body slices, mirroring e2e.Verify's single-message
// contract (strip, recompute, compare, return the stripped message) at the
// segment-sequence level instead of after a caller has already had to
// concatenate everything itself.
//
// # Dispatcher integration
//
// Gateway (gateway.go) is this milestone's answer to request/doc.go's own
// explicit non-goal ("does not implement multi-AVTPDU fragmentation"): it
// wraps a request.Dispatcher (via the minimal Submitter interface, so
// neither package has to import the other's concrete type) so that a
// fragmented request is fully reassembled before Dispatcher.Submit ever
// sees it, and participates in the same StateQueued/StateStarted/
// StateExecuting/StateFinalized lifecycle every other Kind already does —
// Dispatcher itself needed no structural change to support this, exactly as
// ROADMAP.md's own guidance for this milestone anticipated. Gateway.Response
// is the symmetric send-side convenience for a resolved
// Dispatcher.Response whose Body is too large for one AVTPDU.
//
// # This package's own reasoned reassembly policy (Guiding Principle 10)
//
// Out-of-order segments, duplicate segments, and a stalled/abandoned
// sequence are not addressed by ROADMAP.md's own text or by the governing
// specification clearly enough to transcribe rather than decide; this
// package's exact handling has not yet been independently re-verified
// against the specification. Reassembler's own doc comment states this
// package's specific, reversible choices for each; see there rather than
// this file for the reasoning, per this repo's practice of keeping a single
// authoritative location for a given design decision rather than repeating
// it across files.
//
// # Explicit non-goals
//
// This package does not itself decide per-endpoint segment-size budgets,
// retransmission, or flow control — Split/Gateway.Response take a
// caller-supplied maxBody, and this package has no concept of a link's
// actual MTU or congestion state. It does not change avtp, request, or
// e2e's own exported surface at all: every integration point (Message's
// FlagMoreSegments/ReadSizeOrSegment fields, request.Dispatcher.Submit's
// signature, e2e.ComputeFragmented) already existed before this
// milestone landed, staged there specifically for this package to consume,
// per every one of those packages' own doc comments. It does not modify
// can, uart, or server/discovery.go — those packages' own request/response
// bodies simply become segmentable by a caller that chooses to route them
// through Split/Gateway, without any change to how those packages encode a
// CAN Frame, a UART read response, or a discovery register-map snapshot.
//
// # A note on spec fidelity (Guiding Principle 10)
//
// DefaultMaxSegmentBody, DefaultReassemblyTimeout, DefaultMaxSegments, and
// the strict-in-order reassembly policy Reassembler documents have not yet
// been independently re-verified against the governing OPEN Alliance TC18
// Remote Control Protocol Specification's own multi-AVTPDU fragmentation
// rules; see the ecosystem audit tracking issues for known gaps — the same
// open-item posture avtp/doc.go, server/doc.go, request/doc.go, and
// e2e/doc.go already document for their own packages.
package fragment

//fusa:req REQ-FRAG-001
//fusa:req REQ-FRAG-002
//fusa:req REQ-FRAG-003
//fusa:req REQ-FRAG-004
//fusa:req REQ-FRAG-005
//fusa:req REQ-FRAG-006
//fusa:req REQ-FRAG-007
//fusa:req REQ-FRAG-008
//fusa:req REQ-FRAG-009
//fusa:req REQ-FRAG-010
//fusa:req REQ-FRAG-011
//fusa:req REQ-FRAG-012
