package iseled

import "encoding/binary"

// Config is an ISELED endpoint's functional (type-specific) configuration:
// it models a single daisy chain (see doc.go's Scope section), the number
// of devices on it, and the timeout the controller waits for a device's
// response before giving up. It is stored as an endpoint's
// regmap.FunctionalBlock.Data (see Endpoint.Configure) — the
// generic/functional register-map split regmap/registermap.go establishes.
type Config struct {
	// Enabled reports whether this endpoint's chain is configured for use.
	// A command against a disabled chain is rejected with
	// ErrChainNotConfigured.
	Enabled bool

	// DeviceCount is the number of devices daisy-chained on this bus. An
	// enabled Config requires at least one.
	DeviceCount uint8

	// ResponseTimeoutMicros is how long, in microseconds, the controller
	// waits for a device's response before giving up on it.
	ResponseTimeoutMicros uint32
}

// configLen is Enabled(1) + DeviceCount(1) + ResponseTimeoutMicros(4).
const configLen = 1 + 1 + 4

// Validate reports whether c is plausible: an Enabled chain must have at
// least one device.
func (c Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.DeviceCount == 0 {
		return ErrInvalidDeviceCount
	}
	return nil
}

// EncodeConfig serializes c into its wire representation.
func EncodeConfig(c Config) []byte {
	buf := make([]byte, configLen)
	if c.Enabled {
		buf[0] = 1
	}
	buf[1] = c.DeviceCount
	binary.BigEndian.PutUint32(buf[2:6], c.ResponseTimeoutMicros)
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
		Enabled:               b[0] != 0,
		DeviceCount:           b[1],
		ResponseTimeoutMicros: binary.BigEndian.Uint32(b[2:6]),
	}, nil
}
