package iseled

import "github.com/SoundMatt/go-RCP/server"

// EndpointType re-exports server.EndpointTypeISELED so a caller that only
// imports this package doesn't also need to import server just to declare
// an ISELED endpoint's type with server.Server.AddEndpoint.
const EndpointType = server.EndpointTypeISELED

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
