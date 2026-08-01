package spi

import (
	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/regmap"
)

// MaxChannels is the largest number of independently pre-configured
// chip-select channels one SPI endpoint may declare, per ROADMAP.md
// Milestone 47. It is also exactly the number TC18 §13.5 Table 30's SPI row
// can address: "000b to 101b — selects channel 0 … 5".
const MaxChannels = 6

// EndpointType re-exports regmap.EndpointTypeSPI so a caller that only
// imports this package doesn't also need to import server just to declare a
// SPI endpoint's type with server.Server.AddEndpoint.
const EndpointType = regmap.EndpointTypeSPI

// EVTClass is the row of TC18 §13.5 Table 30 that governs how this endpoint
// type interprets a request's evt[2:0] field. SPI has that row to itself:
// evt[2:0] IS the channel selector. Endpoint.HandleRequest routes every
// request through acf.Message.EVTDisposition with this class — see
// acf/evt.go.
const EVTClass = acf.EVTClassChannelSelect

// Channel selects one of an endpoint's up to MaxChannels pre-configured
// chip-select channels. It is carried by the request's evt[2:0] field, per
// TC18 §13.5 Table 30's SPI row — "selects channel 0 … 5; the interface
// settings are to be applied according to this selection; the CSN pin
// assigned to this selection is to be asserted" — and not by any byte of the
// request body, which is the SPI payload in full from its first byte
// (§13.7.3, Figure 23).
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
