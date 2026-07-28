package can

import "github.com/SoundMatt/go-RCP/server"

// EndpointType re-exports server.EndpointTypeCAN so a caller that only
// imports this package doesn't also need to import server just to declare a
// CAN endpoint's type with server.Server.AddEndpoint.
const EndpointType = server.EndpointTypeCAN

// Format selects which CAN frame format a Frame uses. See doc.go's Scope
// section for each format's payload cap and extra fields.
type Format uint8

const (
	// FormatClassical is the original CAN 2.0 frame format: up to
	// ClassicalMaxPayload data bytes, no bit-rate switching, no XL header.
	FormatClassical Format = iota

	// FormatFD is CAN FD (Flexible Data-rate): up to FDMaxPayload data
	// bytes, with an optional higher-rate data-phase bit-rate switch (see
	// Frame.BitRateSwitch).
	FormatFD

	// FormatXL is CAN XL: up to XLMaxPayload data bytes, with the extra
	// header fields XLHeader carries, plus its own bit-rate switch.
	FormatXL

	formatCount // sentinel; keep last
)

// Valid reports whether f is one of this package's three recognized frame
// formats.
func (f Format) Valid() bool {
	return f < formatCount
}

// Payload caps, by frame format (see doc.go's Scope section).
const (
	// ClassicalMaxPayload is the maximum Data length for FormatClassical.
	ClassicalMaxPayload = 8

	// FDMaxPayload is the maximum Data length for FormatFD.
	FDMaxPayload = 64

	// XLMaxPayload is the maximum Data length for FormatXL.
	XLMaxPayload = 2048
)

// MaxPayload returns the maximum Data length f allows, or 0 for an
// unrecognized format.
func (f Format) MaxPayload() int {
	switch f {
	case FormatClassical:
		return ClassicalMaxPayload
	case FormatFD:
		return FDMaxPayload
	case FormatXL:
		return XLMaxPayload
	default:
		return 0
	}
}

// XLHeader carries CAN XL's extra per-frame header fields (see doc.go's
// spec-fidelity note for why these three fields specifically). It is only
// meaningful when Frame.Format is FormatXL.
type XLHeader struct {
	// SDT is the service-data-unit type: an 8-bit tag identifying the
	// higher-layer content Data carries.
	SDT uint8

	// VCID is the virtual CAN network ID this frame belongs to.
	VCID uint8

	// AF is the 32-bit acceptance field, used for XL-specific frame
	// filtering ahead of the classical 11/29-bit identifier match.
	AF uint32
}

// Frame is one CAN data frame — Classical, FD, or XL, per Format. It has no
// remote-transmission-request field: doc.go's Scope section explains why
// this package supports data frames only.
type Frame struct {
	// Format selects Classical/FD/XL framing.
	Format Format

	// Extended reports whether ID is a 29-bit extended identifier (true) or
	// an 11-bit standard identifier (false).
	Extended bool

	// ID is the frame's arbitration identifier: 11 bits when !Extended, 29
	// bits when Extended. A caller-supplied ID outside that range's
	// representable width is rejected by Validate.
	ID uint32

	// BitRateSwitch selects the higher-rate data phase for FormatFD and
	// FormatXL frames. Ignored (and required false by Validate) for
	// FormatClassical, which has no data-phase bit-rate switch to select.
	BitRateSwitch bool

	// XL carries CAN XL's extra header fields. Ignored for every format
	// other than FormatXL.
	XL XLHeader

	// Data is the frame payload. Its length must not exceed Format's
	// MaxPayload().
	Data []byte
}

// standardIDMax and extendedIDMax bound Frame.ID: 11 bits (0x7FF) for a
// standard identifier, 29 bits (0x1FFFFFFF) for an extended one.
const (
	standardIDMax = 0x7FF
	extendedIDMax = 0x1FFFFFFF
)

// Validate reports whether f is a plausible, encodable Frame: Format must be
// recognized, ID must fit its Extended/standard width, Data must not exceed
// Format's payload cap, and BitRateSwitch/XL must not be set for a format
// that doesn't define them.
func (f Frame) Validate() error {
	if !f.Format.Valid() {
		return ErrInvalidFormat
	}
	max := uint32(standardIDMax)
	if f.Extended {
		max = extendedIDMax
	}
	if f.ID > max {
		return ErrIDOutOfRange
	}
	if len(f.Data) > f.Format.MaxPayload() {
		return ErrPayloadTooLarge
	}
	if f.Format == FormatClassical {
		if f.BitRateSwitch {
			return ErrBitRateSwitchNotSupported
		}
		if f.XL != (XLHeader{}) {
			return ErrXLHeaderNotSupported
		}
	}
	if f.Format == FormatFD && f.XL != (XLHeader{}) {
		return ErrXLHeaderNotSupported
	}
	return nil
}
