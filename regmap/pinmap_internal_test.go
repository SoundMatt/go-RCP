//fusa:test REQ-RCS-003
//fusa:test REQ-RCS-019

package regmap

import (
	"errors"
	"testing"
)

// These are internal (white-box) tests: they build a genuinely duplicated
// pin table by appending directly to PinMap.entries, bypassing Set's own
// overwrite-by-pin de-duplication (a legitimate convenience for callers
// fixing their own mistake, but one that would otherwise make a duplicate
// pin unreachable to construct from outside this package). A malformed
// wire buffer decoded by decodePinMap can produce the same shape, since
// decodePinMap also builds entries by appending, not by using Set.

// TestPinMap_ValidateRejectsDuplicatePin checks two assignments claiming
// the same physical pin fail Validate (REQ-RCS-003).
func TestPinMap_ValidateRejectsDuplicatePin(t *testing.T) {
	reg := NewRegisterMap()
	reg.endpoints[1] = &EndpointRegisters{Generic: GenericEndpointBlock{Address: 1, Type: EndpointTypeGPIO, Enabled: true}}
	reg.endpoints[2] = &EndpointRegisters{Generic: GenericEndpointBlock{Address: 2, Type: EndpointTypeGPIO, Enabled: true}}

	pm := &PinMap{entries: []PinAssignment{
		{Pin: 10, Endpoint: 1, SignalIndex: 0},
		{Pin: 10, Endpoint: 2, SignalIndex: 0}, // same physical pin as above
	}}

	if err := pm.Validate(reg); !errors.Is(err, ErrPinMapInvalid) {
		t.Fatalf("Validate = %v, want ErrPinMapInvalid", err)
	}
}

// TestPinMap_ValidateRejectsSignalIndexOutOfRange checks a signal index
// beyond what the endpoint's declared type defines fails Validate
// (REQ-RCS-019).
func TestPinMap_ValidateRejectsSignalIndexOutOfRange(t *testing.T) {
	reg := NewRegisterMap()
	// EndpointTypeADC defines exactly one named signal (index 0).
	reg.endpoints[1] = &EndpointRegisters{Generic: GenericEndpointBlock{Address: 1, Type: EndpointTypeADC, Enabled: true}}

	pm := &PinMap{entries: []PinAssignment{
		{Pin: 10, Endpoint: 1, SignalIndex: 5}, // out of range for ADC
	}}

	if err := pm.Validate(reg); !errors.Is(err, ErrPinMapInvalid) {
		t.Fatalf("Validate = %v, want ErrPinMapInvalid", err)
	}
}

// TestPinMap_ValidateAcceptsInRangeSignal is the positive counterpart to
// the above two: a plausible, non-duplicated, in-range table passes.
func TestPinMap_ValidateAcceptsInRangeSignal(t *testing.T) {
	reg := NewRegisterMap()
	reg.endpoints[1] = &EndpointRegisters{Generic: GenericEndpointBlock{Address: 1, Type: EndpointTypeSPI, Enabled: true}}

	pm := &PinMap{entries: []PinAssignment{
		{Pin: 10, Endpoint: 1, SignalIndex: uint8(SignalCount(EndpointTypeSPI) - 1)},
	}}

	if err := pm.Validate(reg); err != nil {
		t.Fatalf("Validate = %v, want nil", err)
	}
}
