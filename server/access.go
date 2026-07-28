package server

import "github.com/SoundMatt/go-RCP/avtp"

// AccessController implements the EP0 root-client concept: exactly one
// stream may hold the root-client role, with full-register-map write
// access; every other stream is restricted to only the endpoints explicitly
// granted to it.
//
// This package (Milestone 45) treats EP0 itself as an address like any
// other for grant purposes: a restricted stream needs an explicit grant of
// EP0 to read the whole map through CanAccess/ReadEP0, the same as it would
// for any real endpoint. Milestone 46 (Discovery, see discovery.go) adds
// its own universal, grant-independent read of the register map starting at
// register 0 (Server.ReadDiscovery) — a deliberate, narrowly-scoped
// exception layered on top of this type rather than a change to CanAccess
// itself: CanAccess and Grant/Revoke below are unmodified by it, and every
// other address remains gated exactly as this milestone defined.
type AccessController struct {
	root    avtp.StreamID
	rootSet bool
	grants  map[avtp.StreamID]map[avtp.ByteBusID]struct{}
}

// NewAccessController returns an AccessController with no root claimed and
// no grants issued.
func NewAccessController() *AccessController {
	return &AccessController{grants: make(map[avtp.StreamID]map[avtp.ByteBusID]struct{})}
}

// ClaimRoot establishes stream as the root client. It succeeds if no stream
// has claimed the role yet, or if stream is already the current root
// client (an idempotent re-claim); it returns ErrRootAlreadyClaimed if a
// different stream holds the role.
func (a *AccessController) ClaimRoot(stream avtp.StreamID) error {
	if a.rootSet && a.root != stream {
		return ErrRootAlreadyClaimed
	}
	a.root = stream
	a.rootSet = true
	return nil
}

// IsRoot reports whether stream currently holds the root-client role.
func (a *AccessController) IsRoot(stream avtp.StreamID) bool {
	return a.rootSet && a.root == stream
}

// Grant gives a restricted (non-root) stream access to endpoint ep. It has
// no effect on the root client, which already has access to everything.
func (a *AccessController) Grant(stream avtp.StreamID, ep avtp.ByteBusID) {
	set, ok := a.grants[stream]
	if !ok {
		set = make(map[avtp.ByteBusID]struct{})
		a.grants[stream] = set
	}
	set[ep] = struct{}{}
}

// Revoke removes a previously granted endpoint from stream. It has no
// effect on the root client.
func (a *AccessController) Revoke(stream avtp.StreamID, ep avtp.ByteBusID) {
	if set, ok := a.grants[stream]; ok {
		delete(set, ep)
	}
}

// CanAccess reports whether stream may read or write ep: true
// unconditionally for the root client, and true for a restricted stream
// only if ep was explicitly granted to it.
func (a *AccessController) CanAccess(stream avtp.StreamID, ep avtp.ByteBusID) bool {
	if a.IsRoot(stream) {
		return true
	}
	_, ok := a.grants[stream][ep]
	return ok
}
