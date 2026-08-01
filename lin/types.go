package lin

import (
	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/regmap"
)

// EndpointType re-exports regmap.EndpointTypeLIN so a caller that only
// imports this package doesn't also need to import server just to declare a
// LIN endpoint's type with server.Server.AddEndpoint.
const EndpointType = regmap.EndpointTypeLIN

// EVTClass is the row of TC18 §13.5 Table 30 that governs how this endpoint
// type interprets a request's evt[2:0] field. LIN sits in the row Table 30
// shares between ADC, PWM_IN, I²C, LIN, CAN, UART, ISELED and MDIO: the row
// that defines no interface-combining semantics at all, whose only special
// selector is the §12.7.1 configuration change at 111b.
// Endpoint.HandleRequest routes every request through
// acf.Message.EVTDisposition with this class rather than ignoring evt
// entirely — see acf/evt.go, including its documented reading of this row's
// 000b entry.
const EVTClass = acf.EVTClassConfigOnly
