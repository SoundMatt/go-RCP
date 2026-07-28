// Package can implements the CAN endpoint type for the OPEN Alliance TC18
// Remote Control Protocol (RCP), as described by the "OPEN Alliance TC18
// Remote Control Protocol Specification v0.5.1_RC".
//
// This is one of five Phase 16 (v0.64.0) endpoint-type packages (see also
// lin, iseled, mdio, wakeup) built directly on top of the server package's
// register-map substrate (ROADMAP.md Milestones 45/46) and the request
// package's conditional-request/dispatch machinery (ROADMAP.md Milestone
// 49), exactly as the six Phase 14 endpoint types (gpio, spi, i2c, uart,
// adc, pwm) already are: a CAN endpoint's functional configuration (Config)
// is read and written through
// server.Server.WriteFunctional/server.Server.ReadEndpoint, and
// Endpoint.HandleRequest implements the same request.Handler shape
// (avtp.StreamID, acf.Message) (acf.Message, error) every other endpoint
// type does, so it drops into request.Dispatcher unmodified.
//
// # Scope
//
// One CAN endpoint is controller-only and models a single bus, matching
// regmap/types.go's CAN signal list (TX/RX, not a per-channel selector).
// Per ROADMAP.md Milestone 51, the frame format — Classical CAN, CAN FD, or
// CAN XL — is selected per request via Frame.Format (see Frame and
// Format.Valid), not fixed at Configure time, since a controller's
// arbitration-phase bit rate is shared across formats and a single bus may
// legitimately carry a mix of frame formats over time. Every Frame is a
// data frame: this package's Frame type has no remote-transmission-request
// field at all, so there is structurally nothing for a caller to set to
// request a remote frame — data frames only, per Milestone 51's explicit
// instruction, rather than a runtime-rejected RTR bit.
//
// FormatClassical caps Data at 8 bytes (ClassicalMaxPayload). FormatFD caps
// Data at 64 bytes (FDMaxPayload) and additionally carries a bit-rate-switch
// flag (Frame.BitRateSwitch) selecting the higher-rate data phase FD framing
// defines. FormatXL caps Data at 2048 bytes (XLMaxPayload) and additionally
// carries CAN XL's extra header fields (Frame.XL: a service-data-unit type,
// a virtual CAN network ID, and a 32-bit acceptance field) alongside its own
// bit-rate-switch flag — see XLHeader. A write request transmits one Frame;
// a read request returns the most recently received one (see
// Endpoint.SetReceivedFrame) rather than looping the just-transmitted frame
// back, since a real CAN bus's RX path is independent of, and generally not
// a copy of, whatever this controller itself last transmitted.
//
// # Open gap: no trigger-signal table (Guiding Principle 10)
//
// Every other Phase 14/16 endpoint type in this repo exposes a
// DrainTriggers method queuing endpoint-specific TriggerEvent values, for
// request.KindTriggered to gate on (see request/doc.go). This package
// deliberately does not: the behavioral description ROADMAP.md Milestone 51
// was built from names no trigger-signal table for CAN in the source
// specification at all — unlike, say, gpio's edge-trigger table or i2c's
// transaction-complete signal, there is no defined named event a CAN
// endpoint reports as a request-layer trigger source. Per Guiding Principle
// 10, this package documents that as an open gap rather than inventing a
// trigger scheme with no specification basis to anchor it to. A future
// endpoint-external convenience (e.g. "trigger on any received frame") could
// still be layered on top of Endpoint.SetReceivedFrame by a caller, but that
// is explicitly not the same thing as a specification-defined trigger-table
// signal, and this package does not claim it is.
//
// # A note on spec fidelity (Guiding Principle 10)
//
// The TC18 specification PDF is confidential to OPEN Alliance members. This
// package was built from a behavioral description of a CAN controller
// endpoint, not from the primary spec text. Its exact register/request byte
// layouts (Config's field order/widths, Frame's field order, and XLHeader's
// SDT/VCID/AF field shapes) are this implementation's own reasoned,
// self-consistent encoding rather than a verified transcription of the
// published byte assignments — the same open-item posture avtp/doc.go,
// server/doc.go, and i2c/doc.go document for their own packages, pending
// confirmation against a public interoperability reference. XLHeader's
// three named fields (SDT, VCID, AF) reflect the publicly documented CAN XL
// frame format (ISO 11898-1), not any TC18-specific text; how the TC18
// register map actually surfaces them (if at all beyond "CAN XL's extra
// header fields") is exactly the kind of detail this note flags as
// unconfirmed.
package can

//fusa:req REQ-CANEP-001
//fusa:req REQ-CANEP-002
//fusa:req REQ-CANEP-003
//fusa:req REQ-CANEP-004
//fusa:req REQ-CANEP-005
//fusa:req REQ-CANEP-006
//fusa:req REQ-CANEP-007
//fusa:req REQ-CANEP-008
//fusa:req REQ-CANEP-009
//fusa:req REQ-CANEP-010
