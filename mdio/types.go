package mdio

import "github.com/SoundMatt/go-RCP/server"

// EndpointType re-exports server.EndpointTypeMDIO so a caller that only
// imports this package doesn't also need to import server just to declare
// an MDIO endpoint's type with server.Server.AddEndpoint.
const EndpointType = server.EndpointTypeMDIO

// AddressMode selects which IEEE 802.3 MDIO addressing shape a Request
// uses. See doc.go's Scope section.
type AddressMode uint8

const (
	// ModeClause22 is the original, simpler frame shape: a 5-bit PHY
	// address plus a 5-bit register address. Request.DevAddr is unused
	// (and must be zero — see Request.Validate).
	ModeClause22 AddressMode = iota

	// ModeClause45 is the extended frame shape: a 5-bit PHY address, a
	// 5-bit device (MMD) address, and a 16-bit register address.
	ModeClause45

	addressModeCount // sentinel; keep last
)

// Valid reports whether m is one of this package's two recognized address
// modes.
func (m AddressMode) Valid() bool {
	return m < addressModeCount
}

// phyAddrMax and devAddrMax bound the 5-bit PHY and device address fields
// both address modes share (Clause 22 leaves DevAddr unused, but the width
// is the same either way per the IEEE 802.3 frame format).
const (
	phyAddrMax    = 0x1F
	devAddrMax    = 0x1F
	regAddrMaxC22 = 0x1F
)

// Request is one addressed MDIO register access. Mode selects Clause 22 vs
// Clause 45 addressing (see AddressMode); DevAddr and the usable width of
// RegAddr depend on which mode is selected — see Validate.
type Request struct {
	Mode    AddressMode
	PhyAddr uint8
	DevAddr uint8
	RegAddr uint16
}

// Validate reports whether r is a plausible, encodable Request: Mode must
// be recognized, PhyAddr must fit its 5-bit width, and — mode-dependently —
// either DevAddr is zero and RegAddr fits 5 bits (ModeClause22), or DevAddr
// fits 5 bits with RegAddr free to use its full 16-bit width
// (ModeClause45).
func (r Request) Validate() error {
	if !r.Mode.Valid() {
		return ErrInvalidAddressMode
	}
	if r.PhyAddr > phyAddrMax {
		return ErrPhyAddrOutOfRange
	}
	switch r.Mode {
	case ModeClause22:
		if r.DevAddr != 0 {
			return ErrDevAddrNotSupported
		}
		if r.RegAddr > regAddrMaxC22 {
			return ErrRegAddrOutOfRange
		}
	case ModeClause45:
		if r.DevAddr > devAddrMax {
			return ErrDevAddrOutOfRange
		}
	}
	return nil
}
