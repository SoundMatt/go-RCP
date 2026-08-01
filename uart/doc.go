// Package uart implements the UART endpoint type for the OPEN Alliance TC18
// Remote Control Protocol (RCP), as described by the "OPEN Alliance TC18
// Remote Control Protocol Specification v0.5.1_RC".
//
// This is one of four Phase 14 (v0.61.0) endpoint-type packages (see also
// i2c, adc, pwm) built directly on top of the server package's register-map
// substrate (ROADMAP.md Milestones 45/46), the same foundation gpio and spi
// built on in Milestone 47 (v0.60.0): a UART endpoint's functional
// configuration (Config) is read and written through
// server.Server.WriteFunctional/server.Server.ReadEndpoint exactly like any
// other endpoint's FunctionalBlock, and Endpoint.HandleRequest decodes and
// answers a plain acf.Message request the same request-descriptor header
// every endpoint type shares.
//
// # Scope
//
// One UART endpoint models a single serial line's independent TX and RX
// directions, both governed by one shared Config block (baud rate, data
// bits, parity, stop bits, and optional RTS/CTS flow control — see
// regmap/types.go's UART signal list, which names TX/RX/RTS/CTS). A write
// request is TX: its Body is the raw bytes to transmit, handed to the
// endpoint's Transport (see Endpoint.SetTransport) or, with none configured,
// looped directly back into the endpoint's own RX FIFO. A read request is
// RX: it drains up to a caller-requested byte count from that RX FIFO — see
// Endpoint.Receive for how bytes not produced by TX loopback arrive there
// (a real driver feeding incoming line data in).
//
// A UART read request differs from every other Phase 14 endpoint's read
// request in one deliberate way: it must be entirely payload-less (see
// ErrReadRequestNotPayloadLess), carrying its requested byte count in the
// shared request-descriptor header's read-size field instead of the body
// (see acf.Message.ReadSizeOrSegment). This matters beyond this milestone:
// ROADMAP.md Milestone 49's compound-wait request kind gates execution on a
// comparison against an endpoint's current value, and for every other
// endpoint type that comparand is a fixed-width scalar register value
// (gpio's pin word, adc's sample, pwm's period/duration pair). For UART,
// there is no such scalar — RX read completion is inherently a variable-
// length byte-sequence comparison against accumulated RX FIFO content
// instead (see Endpoint.read's FIFO-drain-or-timeout completion below).
// Milestone 49 is not implemented by this package; this note exists so that
// asymmetry is visible in this package's own design now, before that later
// milestone has to retrofit around it.
//
// Read completion here is FIFO-drain-or-timeout: a read request returns as
// many bytes as are available up to the requested count. This package has no
// real timer of its own (no other Phase 14 endpoint type in this milestone
// uses one either), so it treats "timeout" as "whatever is available right
// now" and reports that in the response's leading completeness flag (see
// EncodeReadResponse) rather than blocking — fragmented delivery of partial
// data on a timeout, per ROADMAP.md Milestone 48. A caller that receives an
// incomplete response is expected to issue a follow-up read for the
// remainder, the same posture a real UART driver's poll loop would take.
//
// # Explicit non-goal
//
// Per ROADMAP.md Milestone 48, this package ships against the plain,
// unconditional acf.Message request kind only. Compound/triggered/chained/
// timed request variants — including the compound-wait accumulated-RX-data
// comparison flagged above — are Phase 15's job (ROADMAP.md Milestone 49)
// and are not decided here.
//
// # A note on spec fidelity (Guiding Principle 10)
//
// This package's exact register/request byte layouts (the Config field
// order/widths, the one-byte read-response completeness flag, and the
// two-byte TX write-response accepted-count) have not yet been
// independently re-verified against the governing OPEN Alliance TC18
// Remote Control Protocol Specification's own published byte assignments —
// the same open-item posture avtp/doc.go, server/doc.go, gpio/doc.go, and
// spi/doc.go document for their own packages; see the ecosystem audit
// tracking issues for known gaps.
package uart

//fusa:req REQ-UART-001
//fusa:req REQ-UART-002
//fusa:req REQ-UART-003
//fusa:req REQ-UART-004
//fusa:req REQ-UART-005
//fusa:req REQ-UART-006
//fusa:req REQ-UART-007
//fusa:req REQ-UART-008
//fusa:req REQ-UART-009
//fusa:req REQ-UART-010
//fusa:req REQ-UART-011
