package spi

import "github.com/SoundMatt/go-RCP/regmap"

// MaxChannels is the largest number of independently pre-configured
// chip-select channels one SPI endpoint may declare, per ROADMAP.md
// Milestone 47.
const MaxChannels = 6

// EndpointType re-exports regmap.EndpointTypeSPI so a caller that only
// imports this package doesn't also need to import server just to declare a
// SPI endpoint's type with server.Server.AddEndpoint.
const EndpointType = regmap.EndpointTypeSPI

// Channel selects one of an endpoint's up to MaxChannels pre-configured
// chip-select channels. It is this package's request sub-opcode: the first
// byte of every transfer request/response body (see EncodeTransferRequest).
type Channel uint8

const (
	Channel0 Channel = iota
	Channel1
	Channel2
	Channel3
	Channel4
	Channel5

	channelCount // sentinel; keep last, must equal MaxChannels
)

// Valid reports whether c is one of the MaxChannels recognized channels.
func (c Channel) Valid() bool {
	return c < channelCount
}

// Mode selects a channel's clock polarity and phase, the conventional SPI
// mode numbering (CPOL, CPHA):
//
//	Mode0: CPOL=0, CPHA=0
//	Mode1: CPOL=0, CPHA=1
//	Mode2: CPOL=1, CPHA=0
//	Mode3: CPOL=1, CPHA=1
type Mode uint8

const (
	Mode0 Mode = iota
	Mode1
	Mode2
	Mode3

	modeCount // sentinel; keep last
)

// Valid reports whether m is one of the four recognized SPI modes.
func (m Mode) Valid() bool {
	return m < modeCount
}
