package uart

import "github.com/SoundMatt/go-RCP/server"

// EndpointType re-exports server.EndpointTypeUART so a caller that only
// imports this package doesn't also need to import server just to declare a
// UART endpoint's type with server.Server.AddEndpoint.
const EndpointType = server.EndpointTypeUART

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
