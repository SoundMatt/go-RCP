package i2c

import (
	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/regmap"
)

// EndpointType re-exports regmap.EndpointTypeI2C so a caller that only
// imports this package doesn't also need to import server just to declare an
// I2C endpoint's type with server.Server.AddEndpoint.
const EndpointType = regmap.EndpointTypeI2C

// EVTClass is the row of TC18 §13.5 Table 30 that governs how this endpoint
// type interprets a request's evt[2:0] field. I²C sits in the row Table 30
// shares between ADC, PWM_IN, I²C, LIN, CAN, UART, ISELED and MDIO: the row
// that defines no interface-combining semantics at all, whose only special
// selector is the §12.7.1 configuration change at 111b.
// Endpoint.HandleRequest routes every request through
// acf.Message.EVTDisposition with this class rather than ignoring evt
// entirely — see acf/evt.go, including its documented reading of this row's
// 000b entry.
const EVTClass = acf.EVTClassConfigOnly

// BusSpeed selects one of this package's recognized I2C bus speed classes.
// See doc.go's spec-fidelity note for why this package assigns its own
// freestanding numbering rather than transcribing the source spec's
// bus-speed enumeration directly.
type BusSpeed uint8

const (
	// SpeedStandard is the 100 kbit/s standard-mode bus speed.
	SpeedStandard BusSpeed = iota

	// SpeedFast is the 400 kbit/s fast-mode bus speed.
	SpeedFast

	// SpeedFastPlus is the 1 Mbit/s fast-mode-plus bus speed.
	SpeedFastPlus

	// SpeedHigh is the 3.4 Mbit/s high-speed-mode bus speed.
	SpeedHigh

	// SpeedUltraFast is the 5 Mbit/s ultra-fast-mode bus speed (write-only
	// on real hardware; this package does not distinguish that restriction,
	// since it never parses the address/direction bits of a transfer body —
	// see doc.go's Scope section).
	SpeedUltraFast

	busSpeedCount // sentinel; keep last
)

// Valid reports whether s is one of this package's five recognized bus speed
// classes.
func (s BusSpeed) Valid() bool {
	return s < busSpeedCount
}
