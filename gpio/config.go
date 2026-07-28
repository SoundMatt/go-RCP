package gpio

import "encoding/binary"

// Config is a GPIO endpoint's functional (type-specific) configuration: how
// many of its up to MaxPins pins are in use, which of those are output vs
// input, and which report a change/edge trigger signal. It is stored as an
// endpoint's regmap.FunctionalBlock.Data (see Endpoint.Configure) — the
// generic/functional register-map split regmap/registermap.go establishes.
type Config struct {
	// PinCount is the number of pins in use, from 1 to MaxPins. Only the low
	// PinCount bits of Direction and TriggerEnable are meaningful; the rest
	// must be zero (see Validate).
	PinCount uint8

	// Direction is a per-pin bitmask: bit i set means pin i is an output pin
	// a write request may drive; bit i clear means pin i is an input pin
	// only Endpoint.SetInputs may drive.
	Direction uint32

	// TriggerEnable is a per-pin bitmask: bit i set means a change on pin i
	// is queued as a TriggerEvent (see Endpoint.DrainTriggers).
	TriggerEnable uint32
}

// configLen is PinCount(1) + Direction(4) + TriggerEnable(4).
const configLen = 1 + 4 + 4

// activeMask returns the bitmask covering exactly c's PinCount low-order
// bits (all 32 bits set if PinCount is MaxPins).
func (c Config) activeMask() uint32 {
	if c.PinCount >= MaxPins {
		return 0xFFFFFFFF
	}
	return (uint32(1) << c.PinCount) - 1
}

// Validate reports whether c is a plausible GPIO configuration: PinCount
// must be between 1 and MaxPins, and Direction/TriggerEnable must not set
// any bit outside c's active-pin mask.
func (c Config) Validate() error {
	if c.PinCount == 0 || c.PinCount > MaxPins {
		return ErrPinCountOutOfRange
	}
	active := c.activeMask()
	if c.Direction&^active != 0 || c.TriggerEnable&^active != 0 {
		return ErrPinCountOutOfRange
	}
	return nil
}

// EncodeConfig serializes c into its wire representation.
func EncodeConfig(c Config) []byte {
	buf := make([]byte, configLen)
	buf[0] = c.PinCount
	binary.BigEndian.PutUint32(buf[1:5], c.Direction)
	binary.BigEndian.PutUint32(buf[5:9], c.TriggerEnable)
	return buf
}

// DecodeConfig parses a Config from b. It never panics on malformed input,
// and rejects a buffer with any bytes left over once configLen is consumed,
// the same posture acf.DecodeFrame and regmap.DecodeRegisterMap take on a
// length mismatch.
func DecodeConfig(b []byte) (Config, error) {
	if len(b) < configLen {
		return Config{}, ErrShortBuffer
	}
	if len(b) > configLen {
		return Config{}, ErrTrailingBytes
	}
	return Config{
		PinCount:      b[0],
		Direction:     binary.BigEndian.Uint32(b[1:5]),
		TriggerEnable: binary.BigEndian.Uint32(b[5:9]),
	}, nil
}
