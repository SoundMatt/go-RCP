package regmap

import "testing"

// TestSignalName_TableMatchesSpec pins the per-endpoint-type named-signal
// table (name, order, and count) against the governing OPEN Alliance TC18
// Remote Control Protocol Specification, as a regression guard for the
// go-RCP-06 fix: I2C's SCL/SDA order, CAN's RXD/TXD order, SPI's 9-signal
// PICO/POCI table, LIN's added NSLP, PWM_OUT's added inverted-phase
// PWM_OUTN, GPIO's 32 addressable pins, ISELED's differential ISP_P/ISP_N
// pair, and ADC's single ADC_IN signal.
func TestSignalName_TableMatchesSpec(t *testing.T) {
	tests := []struct {
		t     EndpointType
		names []string
	}{
		{EndpointTypeI2C, []string{"SCL", "SDA"}},
		{EndpointTypeCAN, []string{"RXD", "TXD"}},
		{EndpointTypeSPI, []string{"CLK", "PICO", "POCI", "CS0", "CS1", "CS2", "CS3", "CS4", "CS5"}},
		{EndpointTypeLIN, []string{"TXD", "RXD", "NSLP"}},
		{EndpointTypePWM, []string{"PWM_OUT", "PWM_OUTN"}},
		{EndpointTypeISELED, []string{"ISP_P", "ISP_N"}},
		{EndpointTypeADC, []string{"ADC_IN"}},
	}

	for _, tt := range tests {
		if got := SignalCount(tt.t); got != len(tt.names) {
			t.Errorf("SignalCount(%v) = %d, want %d", tt.t, got, len(tt.names))
		}
		for i, want := range tt.names {
			got, ok := SignalName(tt.t, uint8(i))
			if !ok || got != want {
				t.Errorf("SignalName(%v, %d) = (%q, %v), want (%q, true)", tt.t, i, got, ok, want)
			}
		}
		if _, ok := SignalName(tt.t, uint8(len(tt.names))); ok {
			t.Errorf("SignalName(%v, %d) = ok, want out of range", tt.t, len(tt.names))
		}
	}
}

// TestSignalName_GPIOAllThirtyTwoPins checks GPIO defines all 32
// individually addressable pin signals, "IO0".."IO31" in order, and rejects
// index 32.
func TestSignalName_GPIOAllThirtyTwoPins(t *testing.T) {
	if got := SignalCount(EndpointTypeGPIO); got != 32 {
		t.Fatalf("SignalCount(EndpointTypeGPIO) = %d, want 32", got)
	}
	for i := 0; i < 32; i++ {
		want := indexedSignalNames("IO", 32)[i]
		got, ok := SignalName(EndpointTypeGPIO, uint8(i))
		if !ok || got != want {
			t.Errorf("SignalName(GPIO, %d) = (%q, %v), want (%q, true)", i, got, ok, want)
		}
	}
	if _, ok := SignalName(EndpointTypeGPIO, 32); ok {
		t.Errorf("SignalName(GPIO, 32) = ok, want out of range")
	}
}
