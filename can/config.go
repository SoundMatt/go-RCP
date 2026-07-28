package can

import "encoding/binary"

// Config is a CAN endpoint's functional (type-specific) configuration: it
// models a single controller-only bus (see doc.go's Scope section), its
// arbitration-phase (nominal) bit rate, and its data-phase bit rate used by
// FD/XL frames with Frame.BitRateSwitch set. It is stored as an endpoint's
// server.FunctionalBlock.Data (see Endpoint.Configure) — the
// generic/functional register-map split server/registermap.go establishes.
type Config struct {
	// Enabled reports whether this endpoint's bus is configured for use. A
	// request against a disabled bus is rejected with ErrBusNotConfigured.
	Enabled bool

	// NominalBitrateKbps is the bus's arbitration-phase bit rate, in
	// kilobits per second.
	NominalBitrateKbps uint32

	// DataBitrateKbps is the bus's data-phase bit rate, in kilobits per
	// second, used by an FD/XL frame with BitRateSwitch set. Zero means the
	// bus does not support bit-rate switching (an enabled Config leaves it
	// zero for a Classical-only deployment).
	DataBitrateKbps uint32
}

// configLen is Enabled(1) + NominalBitrateKbps(4) + DataBitrateKbps(4).
const configLen = 1 + 4 + 4

// Validate reports whether c is plausible: an Enabled bus must have a
// nonzero NominalBitrateKbps.
func (c Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.NominalBitrateKbps == 0 {
		return ErrInvalidBitrate
	}
	return nil
}

// EncodeConfig serializes c into its wire representation.
func EncodeConfig(c Config) []byte {
	buf := make([]byte, configLen)
	if c.Enabled {
		buf[0] = 1
	}
	binary.BigEndian.PutUint32(buf[1:5], c.NominalBitrateKbps)
	binary.BigEndian.PutUint32(buf[5:9], c.DataBitrateKbps)
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
		NominalBitrateKbps: binary.BigEndian.Uint32(b[1:5]),
		DataBitrateKbps:    binary.BigEndian.Uint32(b[5:9]),
	}, nil
}
