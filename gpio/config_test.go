//fusa:test REQ-GPIO-001
//fusa:test REQ-GPIO-011

package gpio_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/gpio"
)

// TestConfig_Validate checks PinCount bounds and the active-pin masking rule
// (REQ-GPIO-001).
func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     gpio.Config
		wantErr error
	}{
		{"zero pin count", gpio.Config{PinCount: 0}, gpio.ErrPinCountOutOfRange},
		{"pin count too large", gpio.Config{PinCount: gpio.MaxPins + 1}, gpio.ErrPinCountOutOfRange},
		{"max pin count ok", gpio.Config{PinCount: gpio.MaxPins, Direction: 0xFFFFFFFF}, nil},
		{"direction bit out of range", gpio.Config{PinCount: 4, Direction: 0x10}, gpio.ErrPinCountOutOfRange},
		{"trigger bit out of range", gpio.Config{PinCount: 4, TriggerEnable: 0x10}, gpio.ErrPinCountOutOfRange},
		{"in range ok", gpio.Config{PinCount: 4, Direction: 0x0F, TriggerEnable: 0x05}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// TestConfigRoundTrip checks EncodeConfig/DecodeConfig round-trip and reject
// a short or overlong buffer (REQ-GPIO-011).
func TestConfigRoundTrip(t *testing.T) {
	cfg := gpio.Config{PinCount: 12, Direction: 0x00000FF0, TriggerEnable: 0x00000003}
	b := gpio.EncodeConfig(cfg)
	got, err := gpio.DecodeConfig(b)
	if err != nil {
		t.Fatalf("DecodeConfig: %v", err)
	}
	if got != cfg {
		t.Errorf("DecodeConfig round-trip = %+v, want %+v", got, cfg)
	}

	if _, err := gpio.DecodeConfig(b[:len(b)-1]); !errors.Is(err, gpio.ErrShortBuffer) {
		t.Errorf("DecodeConfig(short) err = %v, want ErrShortBuffer", err)
	}
	if _, err := gpio.DecodeConfig(append(b, 0x00)); !errors.Is(err, gpio.ErrTrailingBytes) {
		t.Errorf("DecodeConfig(overlong) err = %v, want ErrTrailingBytes", err)
	}
}
