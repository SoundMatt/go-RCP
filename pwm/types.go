package pwm

import (
	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/regmap"
)

// EndpointType re-exports regmap.EndpointTypePWM so a caller that only
// imports this package doesn't also need to import server just to declare a
// PWM endpoint's type with server.Server.AddEndpoint.
const EndpointType = regmap.EndpointTypePWM

// EVTClassFor returns the row of TC18 §13.5 Table 30 that governs how a PWM
// endpoint in role r interprets a request's evt[2:0] field.
//
// Unlike every other endpoint-type package, this one cannot declare a single
// constant class: Table 30 puts PWM_OUT and PWM_IN in *different* rows.
// PWM_OUT shares the arithmetic row with GPIO (its payload can be combined
// with the currently applied waveform), while PWM_IN sits in the row with
// ADC, I²C, LIN, CAN, UART, ISELED and MDIO, where the only defined
// selector is the §12.7.1 configuration change — which is consistent with
// PWM_IN being read-only (see Role and doc.go's Scope section). Since Role
// is a whole-endpoint configuration setting, the class is fixed for any one
// configured endpoint and simply follows it.
//
// An unrecognized Role maps to the more restrictive config-only row rather
// than silently granting arithmetic write semantics; Config.Validate rejects
// such a Role before an endpoint can ever be enabled with it.
func EVTClassFor(r Role) acf.EVTClass {
	if r == RoleOutput {
		return acf.EVTClassArithmetic
	}
	return acf.EVTClassConfigOnly
}

// Role selects whether one PWM endpoint generates an output waveform or
// measures an incoming one. Unlike gpio's per-pin Direction, Role is a
// whole-endpoint setting: regmap/types.go's PWM signal list names the
// normal- and inverted-phase output pins (PWM_OUT/PWM_OUTN), since PWM
// input observes the same physical pins in a different mode rather than
// adding separate input signals.
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
