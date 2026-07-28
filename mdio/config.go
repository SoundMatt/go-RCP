package mdio

// Config is an MDIO endpoint's functional (type-specific) configuration.
// Per doc.go's Scope section, addressing (PHY/device/register) is selected
// per request rather than fixed here, so this is deliberately minimal — the
// same "nothing to configure beyond enablement" shape as this package's
// pass-through posture implies. It is stored as an endpoint's
// server.FunctionalBlock.Data (see Endpoint.Configure) — the
// generic/functional register-map split server/registermap.go establishes.
type Config struct {
	// Enabled reports whether this endpoint is configured for use. A
	// request against a disabled endpoint is rejected with
	// ErrNotConfigured.
	Enabled bool
}

// configLen is Enabled(1).
const configLen = 1

// Validate reports whether c is plausible. Every Config value is plausible
// today (Enabled has no invariant to violate), but this method exists for
// symmetry with every sibling endpoint package's Config.Validate.
func (c Config) Validate() error {
	return nil
}

// EncodeConfig serializes c into its wire representation.
func EncodeConfig(c Config) []byte {
	buf := make([]byte, configLen)
	if c.Enabled {
		buf[0] = 1
	}
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
	return Config{Enabled: b[0] != 0}, nil
}
