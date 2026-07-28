package mdio

import "errors"

// Encoding errors.
var (
	// ErrShortBuffer is returned when a buffer is too short to hold the
	// structure a decoder was asked to parse.
	ErrShortBuffer = errors.New("rcp/mdio: buffer too short")

	// ErrTrailingBytes is returned when a decoder is handed more bytes than
	// its fixed-length structure declares.
	ErrTrailingBytes = errors.New("rcp/mdio: buffer longer than declared structure length")
)

// Request validation errors.
var (
	// ErrInvalidAddressMode is returned when a Request's Mode is not one of
	// this package's two recognized address modes.
	ErrInvalidAddressMode = errors.New("rcp/mdio: unrecognized address mode")

	// ErrPhyAddrOutOfRange is returned when a Request's PhyAddr exceeds its
	// 5-bit width.
	ErrPhyAddrOutOfRange = errors.New("rcp/mdio: PHY address out of range for its 5-bit width")

	// ErrDevAddrNotSupported is returned when a ModeClause22 Request sets a
	// nonzero DevAddr, which Clause 22 addressing has no field for.
	ErrDevAddrNotSupported = errors.New("rcp/mdio: device address is only defined for ModeClause45")

	// ErrDevAddrOutOfRange is returned when a ModeClause45 Request's
	// DevAddr exceeds its 5-bit width.
	ErrDevAddrOutOfRange = errors.New("rcp/mdio: device address out of range for its 5-bit width")

	// ErrRegAddrOutOfRange is returned when a ModeClause22 Request's
	// RegAddr exceeds its 5-bit width (ModeClause45's 16-bit register
	// address has no narrower cap to enforce).
	ErrRegAddrOutOfRange = errors.New("rcp/mdio: register address out of range for ModeClause22's 5-bit width")
)

// Request-handling errors.
var (
	// ErrWrongEndpoint is returned when a request's ByteBusID does not match
	// the Endpoint it was handed to.
	ErrWrongEndpoint = errors.New("rcp/mdio: request addressed to a different endpoint")

	// ErrNotConfigured is returned when a request is handled against an
	// endpoint whose Config does not currently mark Enabled.
	ErrNotConfigured = errors.New("rcp/mdio: endpoint is not configured/enabled")

	// ErrRequestMustReadOrWrite is returned when a request sets neither the
	// Read nor the Write control flag, so there is nothing for an MDIO
	// endpoint to do with it.
	ErrRequestMustReadOrWrite = errors.New("rcp/mdio: request must set the Read or Write control flag")
)
