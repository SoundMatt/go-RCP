//fusa:req REQ-SPEC-001
//fusa:req REQ-ERR-001
//fusa:req REQ-ERR-002
//fusa:req REQ-ERR-003
//fusa:req REQ-ERR-004
//fusa:req REQ-ERR-005
//fusa:req REQ-ERR-006
//fusa:req REQ-ERR-007
//fusa:req REQ-ERR-008
//fusa:req REQ-ERR-009
//fusa:req REQ-ERR-010
//fusa:req REQ-ERR-011

// Package rcp provides the root types for go-RCP's implementation of the
// OPEN Alliance TC18 Remote Control Protocol Specification v0.5.1_RC: the
// RELAY specification version this module tracks (SpecVersion), this
// module's own RELAY-mandatory error sentinels (spec §5.1), and the
// zero-copy Loan buffer type the loan package's Controller recycles.
//
// The protocol's own wire types (avtp.StreamID/ByteBusID, acf.Message),
// server lifecycle (server.Server), and request dispatch
// (request.Dispatcher) live in their own packages — see ROADMAP.md Part II
// for the full package map. Adapt (adapt.go) is this package's bridge to
// relay.Caller.
//
// Through ROADMAP.md Milestone 58 (v0.71.0) this package also defined a
// self-consistent but bespoke Zone/Command/Response/Status/Controller/
// Registry API that predated this repo's TC18 replacement — see the
// document's "Full Protocol Replacement" note at the top. Milestone 59
// (v1.0.0, Phase 18 "Cutover") removed that surface once every satellite
// package that depended on it had migrated to the Endpoint/register-map
// model (Phases 53-58); there is no compatibility shim, per the roadmap's
// own explicit rejection of one.
package rcp

import (
	relay "github.com/SoundMatt/RELAY"
)

// SpecVersion is the RELAY specification version this package implements.
// Sourcing it from relay.SpecVersion (rather than hardcoding it) also
// confirms this module builds against a RELAY dependency that publishes the
// §15 canonical-type JSON schemas reachable via relay.Schema().
//
//fusa:req REQ-SPEC-001
//fusa:req REQ-CONF-002
//fusa:req REQ-CONF-003
const SpecVersion = relay.SpecVersion

// wrapErr holds a clean error message while maintaining an Unwrap chain
// so errors.Is traversal reaches RELAY sentinels.
type wrapErr struct {
	msg    string
	parent error
}

func (e *wrapErr) Error() string { return e.msg }
func (e *wrapErr) Unwrap() error { return e.parent }

// Mandatory RELAY sentinels (spec §5.1). Each wraps the corresponding
// relay package sentinel so errors.Is(err, relay.ErrXxx) returns true.
// These are protocol-agnostic — nothing about the TC18 replacement changes
// them.
//
//fusa:req REQ-ERR-001
//fusa:req REQ-ERR-004
var (
	ErrClosed          = &wrapErr{"rcp: closed", relay.ErrClosed}
	ErrNotConnected    = &wrapErr{"rcp: not connected", relay.ErrNotConnected}
	ErrTimeout         = &wrapErr{"rcp: request timeout", relay.ErrTimeout}
	ErrPayloadTooLarge = &wrapErr{"rcp: payload too large", relay.ErrPayloadTooLarge}
)

// ErrNotFound is this module's one RELAY spec §5.4-style protocol-specific
// sentinel: RequestFromMessage/ParseEndpointID (adapt.go) return it when a
// relay.Message.ID does not parse as a valid avtp.ByteBusID. Spec §5.4
// originally named this sentinel for the retired bespoke protocol's own
// "zone not in registry" condition; the closest analogous condition in the
// new addressing model — "this message's ID does not resolve to an
// endpoint address" — reuses the same name and relay.ErrNotConnected parent
// rather than inventing a new one, since §5.4 sentinels are "if exposed,
// MUST use these exact names," not a fixed catalogue every implementation
// must reproduce verbatim.
//
//fusa:req REQ-ERR-005
var ErrNotFound = &wrapErr{"rcp: endpoint id not found", ErrNotConnected}

// Loan is a payload buffer borrowed from a zero-copy loaning pool. The
// caller MUST either pass its Payload to whatever send-loaned call the pool
// owner exposes (transferring ownership) or call Return to release it back
// to the pool without sending. This type is wire-agnostic and outlived the
// bespoke Zone/Command-era LoaningController interface it originally
// shipped alongside: the loan package's own Controller (built against
// *udp.Controller) is this repo's current such pool owner.
type Loan struct {
	Payload []byte
	release func()
}

// Return releases the Loan back to the pool without sending.
// Must not be called after the Loan's Payload has been sent.
func (l *Loan) Return() {
	if l.release != nil {
		l.release()
	}
}

// NewLoan creates a Loan with the given payload and release function.
// Intended for use by loaning-pool implementations in external packages —
// the loan package's Controller is this repo's own such implementation.
func NewLoan(payload []byte, release func()) *Loan {
	return &Loan{Payload: payload, release: release}
}
