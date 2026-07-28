package server

import "github.com/SoundMatt/go-RCP/avtp"

// PinAssignment binds one physical hardware pin to a named signal of one
// declared endpoint. Pin numbering is board-specific and opaque to this
// package; SignalIndex is only meaningful relative to the declared
// EndpointType of Endpoint (see SignalName).
type PinAssignment struct {
	// Pin is the physical pin/pad number this assignment claims.
	Pin uint16

	// Endpoint is the byte_bus_id of the endpoint this pin feeds.
	Endpoint avtp.ByteBusID

	// SignalIndex selects which named signal of Endpoint's declared type
	// this pin carries.
	SignalIndex uint8
}

// PinMap is the HW pin-mapping table: the set of physical-pin-to-endpoint-
// signal bindings declared for this server. It is writable only while the
// server is StateUnconfigured (see Server.SetPinAssignment) and is validated
// for plausibility before the server may advance to StateHWLocked.
type PinMap struct {
	entries []PinAssignment
}

// Set adds or replaces the assignment for a's physical pin. Setting an
// assignment for a pin that already has one overwrites it; this lets a
// caller correct a mistake before the map is locked rather than requiring a
// separate remove-then-add.
func (m *PinMap) Set(a PinAssignment) {
	for i := range m.entries {
		if m.entries[i].Pin == a.Pin {
			m.entries[i] = a
			return
		}
	}
	m.entries = append(m.entries, a)
}

// Entries returns every assignment currently in the table, in the order
// they were first added (stable insertion order, not sorted by pin number).
func (m *PinMap) Entries() []PinAssignment {
	out := make([]PinAssignment, len(m.entries))
	copy(out, m.entries)
	return out
}

// endpointTypes is anything that can answer "what type is declared at this
// address", satisfied by *RegisterMap. Kept as a small interface so
// PinMap.Validate doesn't need to import the full register-map type.
type endpointTypes interface {
	endpointType(addr avtp.ByteBusID) (EndpointType, bool)
}

// Validate reports whether every assignment in the table is plausible given
// the endpoints declared in reg: no two assignments claim the same physical
// pin, every assignment's Endpoint was actually declared, and every
// assignment's SignalIndex is in range for that endpoint's declared type.
// This is the guard condition Server.AdvanceToHWLocked runs before locking
// the table; rejecting an inconsistent pin map here — rather than locking it
// in and letting a later phase discover the inconsistency — is the
// plausibility check the RC Server lifecycle requires before that
// transition.
func (m *PinMap) Validate(reg endpointTypes) error {
	seenPins := make(map[uint16]struct{}, len(m.entries))
	for _, a := range m.entries {
		if _, dup := seenPins[a.Pin]; dup {
			return ErrPinMapInvalid
		}
		seenPins[a.Pin] = struct{}{}

		t, ok := reg.endpointType(a.Endpoint)
		if !ok || !t.Valid() {
			return ErrPinMapInvalid
		}
		if _, ok := SignalName(t, a.SignalIndex); !ok {
			return ErrPinMapInvalid
		}
	}
	return nil
}

// Equal reports whether m and other hold exactly the same assignments,
// regardless of order. Used by whole-map writes to detect an attempt to
// change the pin-mapping table once it is no longer writable.
func (m *PinMap) Equal(other *PinMap) bool {
	if len(m.entries) != len(other.entries) {
		return false
	}
	byPin := make(map[uint16]PinAssignment, len(m.entries))
	for _, a := range m.entries {
		byPin[a.Pin] = a
	}
	for _, a := range other.entries {
		want, ok := byPin[a.Pin]
		if !ok || want != a {
			return false
		}
	}
	return true
}
