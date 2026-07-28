package spi

import "encoding/binary"

// ChannelConfig is one chip-select channel's functional configuration: its
// clock rate, clock mode, and the delay a controller inserts after a
// transfer completes before the channel may be reused.
type ChannelConfig struct {
	// Enabled reports whether this channel is configured for use. A
	// transfer request against a disabled channel is rejected with
	// ErrChannelNotConfigured.
	Enabled bool

	// ClockHz is the channel's clock rate in Hz. Must be nonzero when
	// Enabled (see Config.Validate).
	ClockHz uint32

	// Mode is the channel's clock polarity/phase.
	Mode Mode

	// InterTransferDelayMicros is the minimum delay, in microseconds, the
	// controller waits after a transfer on this channel completes before
	// starting another.
	InterTransferDelayMicros uint16
}

// channelConfigLen is Enabled(1) + ClockHz(4) + Mode(1) +
// InterTransferDelayMicros(2).
const channelConfigLen = 1 + 4 + 1 + 2

func encodeChannelConfig(c ChannelConfig) []byte {
	buf := make([]byte, channelConfigLen)
	if c.Enabled {
		buf[0] = 1
	}
	binary.BigEndian.PutUint32(buf[1:5], c.ClockHz)
	buf[5] = byte(c.Mode)
	binary.BigEndian.PutUint16(buf[6:8], c.InterTransferDelayMicros)
	return buf
}

func decodeChannelConfig(b []byte) (ChannelConfig, error) {
	if len(b) < channelConfigLen {
		return ChannelConfig{}, ErrShortBuffer
	}
	return ChannelConfig{
		Enabled:                  b[0] != 0,
		ClockHz:                  binary.BigEndian.Uint32(b[1:5]),
		Mode:                     Mode(b[5]),
		InterTransferDelayMicros: binary.BigEndian.Uint16(b[6:8]),
	}, nil
}

// Config is a SPI endpoint's functional (type-specific) configuration: one
// ChannelConfig per chip-select channel, always exactly MaxChannels slots
// (unused channels simply left Enabled=false). It is stored as an
// endpoint's server.FunctionalBlock.Data (see Endpoint.SetChannelConfig) —
// the generic/functional register-map split server/registermap.go
// establishes.
type Config struct {
	Channels [MaxChannels]ChannelConfig
}

// configLen is channelConfigLen * MaxChannels: a fixed-size table, no count
// prefix needed since every slot is always present.
const configLen = channelConfigLen * MaxChannels

// Validate reports whether c is plausible: every Enabled channel must have a
// nonzero ClockHz and a recognized Mode.
func (c Config) Validate() error {
	for _, ch := range c.Channels {
		if !ch.Enabled {
			continue
		}
		if ch.ClockHz == 0 {
			return ErrZeroClock
		}
		if !ch.Mode.Valid() {
			return ErrInvalidMode
		}
	}
	return nil
}

// EncodeConfig serializes c into its wire representation.
func EncodeConfig(c Config) []byte {
	buf := make([]byte, 0, configLen)
	for _, ch := range c.Channels {
		buf = append(buf, encodeChannelConfig(ch)...)
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
	var c Config
	for i := range c.Channels {
		ch, err := decodeChannelConfig(b[i*channelConfigLen : (i+1)*channelConfigLen])
		if err != nil {
			return Config{}, err
		}
		c.Channels[i] = ch
	}
	return c, nil
}
