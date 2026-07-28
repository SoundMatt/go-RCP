package adc

// Config is an ADC endpoint's functional (type-specific) configuration:
// resolution, the sample/average/combine model's tunables, and how this
// channel is expected to be kept sampling continuously. It is stored as an
// endpoint's regmap.FunctionalBlock.Data (see Endpoint.Configure) — the
// generic/functional register-map split regmap/registermap.go establishes.
type Config struct {
	// Enabled reports whether this channel is configured for use. A trigger
	// or request against a disabled channel is rejected with
	// ErrChannelNotConfigured.
	Enabled bool

	// ResolutionBits is the sample width, from 1 to MaxResolutionBits. Every
	// reported value is masked to its low ResolutionBits bits (see
	// Validate).
	ResolutionBits uint8

	// SampleCount is the number of raw samples the "sample" layer takes and
	// the "average" layer arithmetic-means into one averaged reading per
	// measurement. Must be at least 1 when Enabled (see Validate).
	SampleCount uint8

	// Combine selects the "combine" layer: how a measurement's averaged
	// sample combines with the endpoint's previous value.
	Combine CombineMode

	// TriggerMode selects how this channel is expected to be kept sampling
	// continuously (see TriggerMode).
	TriggerMode TriggerMode
}

// configLen is Enabled(1) + ResolutionBits(1) + SampleCount(1) + Combine(1)
// + TriggerMode(1).
const configLen = 1 + 1 + 1 + 1 + 1

// resolutionMask returns the bitmask covering exactly bits low-order bits.
func resolutionMask(bits uint8) uint16 {
	if bits >= MaxResolutionBits {
		return 0xFFFF
	}
	return uint16(1)<<bits - 1
}

// Validate reports whether c is plausible: an Enabled channel must have
// ResolutionBits between 1 and MaxResolutionBits, SampleCount at least 1, and
// recognized Combine/TriggerMode values.
func (c Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.ResolutionBits == 0 || c.ResolutionBits > MaxResolutionBits {
		return ErrInvalidResolution
	}
	if c.SampleCount == 0 {
		return ErrInvalidSampleCount
	}
	if !c.Combine.Valid() {
		return ErrInvalidCombineMode
	}
	if !c.TriggerMode.Valid() {
		return ErrInvalidTriggerMode
	}
	return nil
}

// EncodeConfig serializes c into its wire representation.
func EncodeConfig(c Config) []byte {
	buf := make([]byte, configLen)
	if c.Enabled {
		buf[0] = 1
	}
	buf[1] = c.ResolutionBits
	buf[2] = c.SampleCount
	buf[3] = byte(c.Combine)
	buf[4] = byte(c.TriggerMode)
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
		Enabled:        b[0] != 0,
		ResolutionBits: b[1],
		SampleCount:    b[2],
		Combine:        CombineMode(b[3]),
		TriggerMode:    TriggerMode(b[4]),
	}, nil
}
