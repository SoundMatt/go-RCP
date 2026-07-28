package i2c

import "encoding/binary"

// Config is an I2C endpoint's functional (type-specific) configuration: it
// models a single controller-only bus (unlike spi's up to six chip-select
// channels, this package's Scope section explains why one bus is enough), its
// speed class, and the minimum time the controller waits after one
// transaction completes before starting the next. It is stored as an
// endpoint's server.FunctionalBlock.Data (see Endpoint.Configure) — the
// generic/functional register-map split server/registermap.go establishes.
type Config struct {
	// Enabled reports whether this endpoint's bus is configured for use. A
	// transfer request against a disabled bus is rejected with
	// ErrBusNotConfigured.
	Enabled bool

	// Speed is the bus's configured speed class.
	Speed BusSpeed

	// TrailingTimeMicros is the minimum delay, in microseconds, the
	// controller waits after a transaction on this bus completes before
	// starting another.
	TrailingTimeMicros uint16
}

// configLen is Enabled(1) + Speed(1) + TrailingTimeMicros(2).
const configLen = 1 + 1 + 2

// Validate reports whether c is plausible: an Enabled bus must have a
// recognized Speed.
func (c Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if !c.Speed.Valid() {
		return ErrInvalidSpeed
	}
	return nil
}

// EncodeConfig serializes c into its wire representation.
func EncodeConfig(c Config) []byte {
	buf := make([]byte, configLen)
	if c.Enabled {
		buf[0] = 1
	}
	buf[1] = byte(c.Speed)
	binary.BigEndian.PutUint16(buf[2:4], c.TrailingTimeMicros)
	return buf
}

// DecodeConfig parses a Config from b. It never panics on malformed input,
// and rejects a buffer whose length isn't exactly configLen, the same
// "don't silently ignore extra or missing input" posture the rest of this
// repo's decoders take.
func DecodeConfig(b []byte) (Config, error) {
	if len(b) < configLen {
		return Config{}, ErrShortBuffer
	}
	if len(b) > configLen {
		return Config{}, ErrTrailingBytes
	}
	return Config{
		Enabled:            b[0] != 0,
		Speed:              BusSpeed(b[1]),
		TrailingTimeMicros: binary.BigEndian.Uint16(b[2:4]),
	}, nil
}
