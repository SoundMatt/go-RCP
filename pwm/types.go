package pwm

import "github.com/SoundMatt/go-RCP/server"

// EndpointType re-exports server.EndpointTypePWM so a caller that only
// imports this package doesn't also need to import server just to declare a
// PWM endpoint's type with server.Server.AddEndpoint.
const EndpointType = server.EndpointTypePWM

// Role selects whether one PWM endpoint generates an output waveform or
// measures an incoming one. Unlike gpio's per-pin Direction, Role is a
// whole-endpoint setting: server/types.go's PWM signal list names a single
// "OUT" signal, since PWM input observes the same physical pin in a
// different mode rather than adding a second signal.
type Role uint8

const (
	// RoleOutput means this endpoint generates a waveform: a write request
	// sets it, and a read request reads back what is currently applied.
	RoleOutput Role = iota

	// RoleInput means this endpoint measures an externally driven incoming
	// waveform: only read requests are accepted (see doc.go's Scope
	// section), and a read fails explicitly with ErrSignalLost rather than
	// returning stale data when no valid incoming edges have been observed.
	RoleInput

	roleCount // sentinel; keep last
)

// Valid reports whether r is one of this package's two recognized roles.
func (r Role) Valid() bool {
	return r < roleCount
}
