package lin

import "encoding/binary"

// Config is a LIN endpoint's functional (type-specific) configuration: it
// models a single commander-only bus (see doc.go's Scope section), its baud
// rate, and the minimum time the commander waits after one frame's
// transfer completes before starting the next. It is stored as an
// endpoint's server.FunctionalBlock.Data (see Endpoint.Configure) — the
// generic/functional register-map split server/registermap.go establishes.
type Config struct {
	// Enabled reports whether this endpoint's bus is configured for use. A
	// transfer request against a disabled bus is rejected with
	// ErrBusNotConfigured.
	Enabled bool

	// BaudRate is the bus's configured baud rate, in bits per second.
	// Conventional LIN buses run in the 1000-20000 bps range, but this
	// package does not enforce that range itself — see Validate.
	BaudRate uint32

	// TrailingTimeMicros is the minimum delay, in microseconds, the
	// commander waits after a frame transfer on this bus completes before
	// starting another.
	TrailingTimeMicros uint16
}

// configLen is Enabled(1) + BaudRate(4) + TrailingTimeMicros(2).
const configLen = 1 + 4 + 2

// Validate reports whether c is plausible: an Enabled bus must have a
// nonzero BaudRate.
func (c Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.BaudRate == 0 {
		return ErrInvalidBaudRate
	}
	return nil
}

// EncodeConfig serializes c into its wire representation.
func EncodeConfig(c Config) []byte {
	buf := make([]byte, configLen)
	if c.Enabled {
		buf[0] = 1
	}
	binary.BigEndian.PutUint32(buf[1:5], c.BaudRate)
	binary.BigEndian.PutUint16(buf[5:7], c.TrailingTimeMicros)
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
		BaudRate:           binary.BigEndian.Uint32(b[1:5]),
		TrailingTimeMicros: binary.BigEndian.Uint16(b[5:7]),
	}, nil
}
