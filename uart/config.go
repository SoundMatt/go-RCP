package uart

import "encoding/binary"

// Config is a UART endpoint's functional (type-specific) configuration,
// shared by its independent TX and RX request handling — see doc.go's Scope
// section for why one block covers both directions. It is stored as an
// endpoint's server.FunctionalBlock.Data (see Endpoint.Configure) — the
// generic/functional register-map split server/registermap.go establishes.
type Config struct {
	// Enabled reports whether this endpoint is configured for use. A TX or
	// RX request against a disabled endpoint is rejected with
	// ErrUARTNotConfigured.
	Enabled bool

	// BaudRate is the line's configured symbol rate, in bits per second.
	// Must be nonzero when Enabled (see Validate).
	BaudRate uint32

	// DataBits is the number of data bits per frame, from 5 to 9.
	DataBits uint8

	// Parity is the frame's parity mode.
	Parity Parity

	// StopBits is the frame's stop-bit count.
	StopBits StopBits

	// FlowControl reports whether RTS/CTS hardware flow control is in use
	// (see server/types.go's UART signal list, which names RTS/CTS alongside
	// TX/RX).
	FlowControl bool

	// ReadTimeoutMicros is the maximum time an RX read request waits for its
	// requested byte count to become available before returning whatever
	// has actually arrived (see doc.go's note on this package's synchronous
	// stand-in for that timeout).
	ReadTimeoutMicros uint32
}

// configLen is Enabled(1) + BaudRate(4) + DataBits(1) + Parity(1) +
// StopBits(1) + FlowControl(1) + ReadTimeoutMicros(4).
const configLen = 1 + 4 + 1 + 1 + 1 + 1 + 4

// Validate reports whether c is plausible: an Enabled endpoint must have a
// nonzero BaudRate, DataBits between 5 and 9, and recognized Parity/StopBits.
func (c Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.BaudRate == 0 {
		return ErrZeroBaudRate
	}
	if c.DataBits < 5 || c.DataBits > 9 {
		return ErrInvalidDataBits
	}
	if !c.Parity.Valid() {
		return ErrInvalidParity
	}
	if !c.StopBits.Valid() {
		return ErrInvalidStopBits
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
	buf[5] = c.DataBits
	buf[6] = byte(c.Parity)
	buf[7] = byte(c.StopBits)
	if c.FlowControl {
		buf[8] = 1
	}
	binary.BigEndian.PutUint32(buf[9:13], c.ReadTimeoutMicros)
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
		Enabled:           b[0] != 0,
		BaudRate:          binary.BigEndian.Uint32(b[1:5]),
		DataBits:          b[5],
		Parity:            Parity(b[6]),
		StopBits:          StopBits(b[7]),
		FlowControl:       b[8] != 0,
		ReadTimeoutMicros: binary.BigEndian.Uint32(b[9:13]),
	}, nil
}
