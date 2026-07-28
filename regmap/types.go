package regmap

import "github.com/SoundMatt/go-RCP/avtp"

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

// signalNames is this package's own named-signal-index scheme: for each
// endpoint type, the ordered list of named signals a pin-mapping entry may
// point its SignalIndex field at. A HW pin feeds exactly one named signal of
// exactly one endpoint.
//
// These names and orderings are this implementation's own reasoned,
// self-consistent scheme rather than a verified transcription of the TC18
// specification's own named-signal tables — the same open-item posture the
// avtp package documents for its subtype tags (see avtp/doc.go and its
// "note on spec fidelity"). Confirming these against a public
// interoperability reference is tracked as a follow-on, consistent with
// this repo's established practice of flagging spec ambiguity rather than
// silently guessing (see also the I²C bus-speed-enum note referenced at
// Milestone 48).
var signalNames = map[EndpointType][]string{
	EndpointTypeGPIO:   {"IO"},
	EndpointTypeSPI:    {"SCLK", "MOSI", "MISO", "CS0", "CS1", "CS2", "CS3"},
	EndpointTypeI2C:    {"SDA", "SCL"},
	EndpointTypeUART:   {"TX", "RX", "RTS", "CTS"},
	EndpointTypeADC:    {"AIN0", "AIN1", "AIN2", "AIN3", "AIN4", "AIN5", "AIN6", "AIN7"},
	EndpointTypePWM:    {"OUT"},
	EndpointTypeLIN:    {"TX", "RX"},
	EndpointTypeCAN:    {"TX", "RX"},
	EndpointTypeISELED: {"DATA"},
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
