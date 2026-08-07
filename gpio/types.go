package gpio

import (
	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/regmap"
)

// MaxPins is the largest number of independently configured pins one GPIO
// endpoint may declare, per ROADMAP.md Milestone 47.
const MaxPins = 32

// EndpointType re-exports regmap.EndpointTypeGPIO so a caller that only
// imports this package doesn't also need to import server just to declare a
// GPIO endpoint's type with server.Server.AddEndpoint.
const EndpointType = regmap.EndpointTypeGPIO

// EVTClass is the row of TC18 §13.5 Table 30 that governs how this endpoint
// type interprets a request's evt[2:0] field: GPIO shares the arithmetic row
// with PWM_OUT. Endpoint.HandleRequest routes every request through
// acf.Message.EVTDisposition with this class rather than decoding the
// write-semantic selector itself — see acf/evt.go.
const EVTClass = acf.EVTClassArithmetic
