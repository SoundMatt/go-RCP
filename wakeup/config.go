package wakeup

import "encoding/binary"

// Config is a Wakeup endpoint's functional (type-specific) configuration:
// whether it is enabled, and the pacing/count of the repeating
// wake-handshake message a wake from PowerSleep produces (see doc.go). It
// is stored as an endpoint's server.FunctionalBlock.Data (see
// Endpoint.Configure) — the generic/functional register-map split
// server/registermap.go establishes.
type Config struct {
	// Enabled reports whether this endpoint is configured for use. A
	// request against a disabled endpoint is rejected with
	// ErrNotConfigured.
	Enabled bool

	// WakeHandshakeIntervalMillis is the interval, in milliseconds, at
	// which a caller's own transport loop is expected to re-emit each
	// queued WakeHandshake (see doc.go's Scope section — this package has
	// no timer of its own and does not pace this itself).
	WakeHandshakeIntervalMillis uint32

	// WakeHandshakeRepeatCount is how many WakeHandshake TriggerEvent
	// values a Sleep→Normal transition queues up front.
	WakeHandshakeRepeatCount uint16
}

// configLen is Enabled(1) + WakeHandshakeIntervalMillis(4) +
// WakeHandshakeRepeatCount(2).
const configLen = 1 + 4 + 2

// Validate reports whether c is plausible: an Enabled endpoint must have a
// nonzero WakeHandshakeIntervalMillis and WakeHandshakeRepeatCount — a
// wake-handshake message that never repeats or repeats with no defined
// pacing is not a usable configuration.
func (c Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.WakeHandshakeIntervalMillis == 0 {
		return ErrInvalidHandshakeInterval
	}
	if c.WakeHandshakeRepeatCount == 0 {
		return ErrInvalidHandshakeRepeatCount
	}
	return nil
}

// EncodeConfig serializes c into its wire representation.
func EncodeConfig(c Config) []byte {
	buf := make([]byte, configLen)
	if c.Enabled {
		buf[0] = 1
	}
	binary.BigEndian.PutUint32(buf[1:5], c.WakeHandshakeIntervalMillis)
	binary.BigEndian.PutUint16(buf[5:7], c.WakeHandshakeRepeatCount)
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
		Enabled:                     b[0] != 0,
		WakeHandshakeIntervalMillis: binary.BigEndian.Uint32(b[1:5]),
		WakeHandshakeRepeatCount:    binary.BigEndian.Uint16(b[5:7]),
	}, nil
}
