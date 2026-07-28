package pwm

import "encoding/binary"

// Config is a PWM endpoint's functional (type-specific) configuration: which
// Role it plays, and (for RoleOutput) the waveform it starts up driving
// before any write request arrives. It is stored as an endpoint's
// regmap.FunctionalBlock.Data (see Endpoint.Configure) — the
// generic/functional register-map split regmap/registermap.go establishes.
type Config struct {
	// Enabled reports whether this endpoint is configured for use. A request
	// against a disabled endpoint is rejected with ErrNotConfigured.
	Enabled bool

	// Role selects output vs input (see Role).
	Role Role

	// DefaultPeriodMicros and DefaultActiveMicros are the waveform a
	// RoleOutput endpoint applies immediately on Configure, before any write
	// request arrives. Ignored for RoleInput.
	DefaultPeriodMicros uint32
	DefaultActiveMicros uint32
}

// configLen is Enabled(1) + Role(1) + DefaultPeriodMicros(4) +
// DefaultActiveMicros(4).
const configLen = 1 + 1 + 4 + 4

// Validate reports whether c is plausible: an Enabled endpoint must have a
// recognized Role, and a RoleOutput endpoint's default waveform must satisfy
// the same ActiveMicros<=PeriodMicros invariant Endpoint.SetOutput enforces
// for a write request.
func (c Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if !c.Role.Valid() {
		return ErrInvalidRole
	}
	if c.Role == RoleOutput && c.DefaultActiveMicros > c.DefaultPeriodMicros {
		return ErrActiveExceedsPeriod
	}
	return nil
}

// EncodeConfig serializes c into its wire representation.
func EncodeConfig(c Config) []byte {
	buf := make([]byte, configLen)
	if c.Enabled {
		buf[0] = 1
	}
	buf[1] = byte(c.Role)
	binary.BigEndian.PutUint32(buf[2:6], c.DefaultPeriodMicros)
	binary.BigEndian.PutUint32(buf[6:10], c.DefaultActiveMicros)
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
		Enabled:             b[0] != 0,
		Role:                Role(b[1]),
		DefaultPeriodMicros: binary.BigEndian.Uint32(b[2:6]),
		DefaultActiveMicros: binary.BigEndian.Uint32(b[6:10]),
	}, nil
}
