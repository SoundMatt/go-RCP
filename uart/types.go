package uart

import (
	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/regmap"
)

// EndpointType re-exports regmap.EndpointTypeUART so a caller that only
// imports this package doesn't also need to import server just to declare a
// UART endpoint's type with server.Server.AddEndpoint.
const EndpointType = regmap.EndpointTypeUART

// EVTClass is the row of TC18 §13.5 Table 30 that governs how this endpoint
// type interprets a request's evt[2:0] field. UART sits in the row Table 30
// shares between ADC, PWM_IN, I²C, LIN, CAN, UART, ISELED and MDIO: the row
// that defines no interface-combining semantics at all, whose only special
// selector is the §12.7.1 configuration change at 111b.
// Endpoint.HandleRequest routes every request through
// acf.Message.EVTDisposition with this class rather than ignoring evt
// entirely — see acf/evt.go, including its documented reading of this row's
// 000b entry.
const EVTClass = acf.EVTClassConfigOnly

// Parity selects a UART endpoint's parity bit generation/checking mode.
type Parity uint8

const (
	ParityNone Parity = iota
	ParityOdd
	ParityEven
	ParityMark
	ParitySpace

	parityCount // sentinel; keep last
)

// Valid reports whether p is one of this package's five recognized parity
// modes.
func (p Parity) Valid() bool {
	return p < parityCount
}

// StopBits selects a UART endpoint's stop-bit count.
type StopBits uint8

const (
	StopBitsOne StopBits = iota
	StopBitsOneAndHalf
	StopBitsTwo

	stopBitsCount // sentinel; keep last
)

// Valid reports whether s is one of this package's three recognized stop-bit
// counts.
func (s StopBits) Valid() bool {
	return s < stopBitsCount
}
