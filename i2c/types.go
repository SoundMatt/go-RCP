package i2c

import "github.com/SoundMatt/go-RCP/server"

// EndpointType re-exports server.EndpointTypeI2C so a caller that only
// imports this package doesn't also need to import server just to declare an
// I2C endpoint's type with server.Server.AddEndpoint.
const EndpointType = server.EndpointTypeI2C

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
