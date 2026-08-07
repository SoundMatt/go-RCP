package regmap

import (
	"fmt"

	"github.com/SoundMatt/go-RCP/v9/avtp"
)

// EP0 is the reserved byte_bus_id that addresses the RC Server itself as a
// pseudo-endpoint, rather than any real hardware endpoint. See doc.go for
// what operations EP0 supports.
const EP0 avtp.ByteBusID = 0

// EndpointType identifies which functional endpoint kind a declared endpoint
// is. Only the identifiers are defined at this milestone — the type-specific
// functional register layouts for each of these are later Phase 14/16
// milestones (ROADMAP.md Milestones 47/48/51); this package only needs the
// tag to validate pin-mapping table entries against a named-signal-index
// range (see SignalName).
type EndpointType uint8

const (
	// EndpointTypeUnassigned is the zero value: no type has been declared
	// for this address yet.
	EndpointTypeUnassigned EndpointType = iota
	EndpointTypeGPIO
	EndpointTypeSPI
	EndpointTypeI2C
	EndpointTypeUART
	EndpointTypeADC
	EndpointTypePWM
	EndpointTypeLIN
	EndpointTypeCAN
	EndpointTypeISELED
	EndpointTypeMDIO
	EndpointTypeWakeup
	EndpointTypeDAC

	endpointTypeCount // sentinel; keep last
)

// indexedSignalNames returns count names of the form prefix+index (e.g.
// "IO0" .. "IO31" for indexedSignalNames("IO", 32)), for an endpoint type
// whose named signals are one-per-pin rather than individually named.
func indexedSignalNames(prefix string, count int) []string {
	names := make([]string, count)
	for i := 0; i < count; i++ {
		names[i] = fmt.Sprintf("%s%d", prefix, i)
	}
	return names
}

// signalNames is, for each endpoint type, the ordered list of named signals
// a pin-mapping entry may point its SignalIndex field at. A HW pin feeds
// exactly one named signal of exactly one endpoint.
//
// These names and orderings are a verified transcription of the governing
// OPEN Alliance TC18 Remote Control Protocol Specification's per-endpoint-
// type named-signal tables (PICO/POCI are that specification's own
// replacement terms for the older MOSI/MISO names). GPIO's 32 entries are
// generated (indexedSignalNames("IO", 32): "IO0".."IO31"), one per
// addressable pin, since the specification defines that table by index
// range rather than individual names the way the other endpoint types are.
var signalNames = map[EndpointType][]string{
	EndpointTypeGPIO:   indexedSignalNames("IO", 32),
	EndpointTypeSPI:    {"CLK", "PICO", "POCI", "CS0", "CS1", "CS2", "CS3", "CS4", "CS5"},
	EndpointTypeI2C:    {"SCL", "SDA"},
	EndpointTypeUART:   {"TX", "RX", "RTS", "CTS"},
	EndpointTypeADC:    {"ADC_IN"},
	EndpointTypePWM:    {"PWM_OUT", "PWM_OUTN"},
	EndpointTypeLIN:    {"TXD", "RXD", "NSLP"},
	EndpointTypeCAN:    {"RXD", "TXD"},
	EndpointTypeISELED: {"ISP_P", "ISP_N"},
	EndpointTypeMDIO:   {"MDC", "MDIO"},
	EndpointTypeWakeup: {"WAKE"},
	EndpointTypeDAC:    {"OUT"},
}

// Valid reports whether t is one of this package's recognized non-zero
// endpoint types.
func (t EndpointType) Valid() bool {
	return t > EndpointTypeUnassigned && t < endpointTypeCount
}

// SignalCount returns the number of named signals endpoint type t defines,
// or 0 for an unassigned or unrecognized type.
func SignalCount(t EndpointType) int {
	return len(signalNames[t])
}

// SignalName returns the named signal at idx for endpoint type t, and
// whether idx was in range for that type.
func SignalName(t EndpointType, idx uint8) (string, bool) {
	names := signalNames[t]
	if int(idx) >= len(names) {
		return "", false
	}
	return names[idx], true
}
