package iseled

import (
	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/regmap"
)

// EndpointType re-exports regmap.EndpointTypeISELED so a caller that only
// imports this package doesn't also need to import server just to declare
// an ISELED endpoint's type with server.Server.AddEndpoint.
const EndpointType = regmap.EndpointTypeISELED

// EVTClass is the row of TC18 §13.5 Table 30 that governs how this endpoint
// type interprets a request's evt[2:0] field. ISELED sits in the row Table 30
// shares between ADC, PWM_IN, I²C, LIN, CAN, UART, ISELED and MDIO: the row
// that defines no interface-combining semantics at all, whose only special
// selector is the §12.7.1 configuration change at 111b.
// Endpoint.HandleRequest routes every request through
// acf.Message.EVTDisposition with this class rather than ignoring evt
// entirely — see acf/evt.go, including its documented reading of this row's
// 000b entry.
const EVTClass = acf.EVTClassConfigOnly

// DeviceBroadcast is the reserved device address that targets every device
// on the chain at once, per doc.go's "Multi-device response aggregation"
// section. This package's own freestanding addressing scheme reserves the
// top of the 8-bit address space for it, leaving addresses
// 0..DeviceBroadcast-1 (255 individually addressable devices) for real
// chain positions.
const DeviceBroadcast uint8 = 0xFF

// Command is one controller-issued command sent down the chain: Address is
// either a specific device's chain position or DeviceBroadcast, and Data is
// the opaque command payload this layer does not interpret (see doc.go's
// Scope section).
type Command struct {
	Address uint8
	Data    []byte
}

// DeviceResponse is one device's answer to a Command: its own chain
// address (never DeviceBroadcast — a device always answers as itself) and
// opaque response payload.
type DeviceResponse struct {
	Address uint8
	Data    []byte
}

// AggregatedResponse is the ordered set of DeviceResponse values a
// broadcast (or, degenerately, a single targeted) Command produced. See
// EncodeAggregatedResponse/DecodeAggregatedResponse.
type AggregatedResponse []DeviceResponse
