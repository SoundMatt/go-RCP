//fusa:test REQ-ADC-001
//fusa:test REQ-ADC-002
//fusa:test REQ-ADC-003

package adc_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/v9/adc"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/server"
)

// rootStream and newDeclaredEndpoint are this package's own test helpers
// (adc_test is an external test package).
func rootStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55}, 1)
}

// newDeclaredEndpoint returns an adc.Endpoint declared (but not yet
// configured) on a fresh server.Server, with root as both the root client
// and the caller that will issue requests.
func newDeclaredEndpoint(t *testing.T) (*adc.Endpoint, avtp.StreamID) {
	t.Helper()
	root := rootStream()
	s := server.NewServer()
	if err := s.ClaimRoot(root); err != nil {
		t.Fatalf("ClaimRoot: %v", err)
	}
	if err := s.AddEndpoint(root, avtp.ByteBusID(1), adc.EndpointType); err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}
	return adc.NewEndpoint(s, avtp.ByteBusID(1)), root
}

// TestCombineModeAndTriggerMode_Valid checks Valid recognizes exactly the
// defined values for each enum (REQ-ADC-002).
func TestCombineModeAndTriggerMode_Valid(t *testing.T) {
	for m := adc.CombineMode(0); m < 2; m++ {
		if !m.Valid() {
			t.Errorf("CombineMode(%d).Valid() = false, want true", m)
		}
	}
	for _, m := range []adc.CombineMode{2, 3, 255} {
		if m.Valid() {
			t.Errorf("CombineMode(%d).Valid() = true, want false", m)
		}
	}

	for m := adc.TriggerMode(0); m < 3; m++ {
		if !m.Valid() {
			t.Errorf("TriggerMode(%d).Valid() = false, want true", m)
		}
	}
	for _, m := range []adc.TriggerMode{3, 4, 255} {
		if m.Valid() {
			t.Errorf("TriggerMode(%d).Valid() = true, want false", m)
		}
	}
}

// TestConfig_Validate checks a disabled channel always validates, and an
// enabled one enforces resolution, sample count, and enum bounds
// (REQ-ADC-001).
func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     adc.Config
		wantErr error
	}{
		{"disabled ok regardless of fields", adc.Config{}, nil},
		{"zero resolution", adc.Config{Enabled: true, ResolutionBits: 0, SampleCount: 1}, adc.ErrInvalidResolution},
		{"resolution too large", adc.Config{Enabled: true, ResolutionBits: adc.MaxResolutionBits + 1, SampleCount: 1}, adc.ErrInvalidResolution},
		{"zero sample count", adc.Config{Enabled: true, ResolutionBits: 12, SampleCount: 0}, adc.ErrInvalidSampleCount},
		{"invalid combine", adc.Config{Enabled: true, ResolutionBits: 12, SampleCount: 1, Combine: 9}, adc.ErrInvalidCombineMode},
		{"invalid trigger mode", adc.Config{Enabled: true, ResolutionBits: 12, SampleCount: 1, TriggerMode: 9}, adc.ErrInvalidTriggerMode},
		{"valid", adc.Config{Enabled: true, ResolutionBits: 12, SampleCount: 4, Combine: adc.CombineReplace, TriggerMode: adc.TriggerModeOnDemand}, nil},
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
// a short or overlong buffer (REQ-ADC-003).
func TestConfigRoundTrip(t *testing.T) {
	cfg := adc.Config{Enabled: true, ResolutionBits: 10, SampleCount: 8, Combine: adc.CombineRollingAverage, TriggerMode: adc.TriggerModeExternal}
	b := adc.EncodeConfig(cfg)
	got, err := adc.DecodeConfig(b)
	if err != nil {
		t.Fatalf("DecodeConfig: %v", err)
	}
	if got != cfg {
		t.Errorf("DecodeConfig round-trip = %+v, want %+v", got, cfg)
	}

	if _, err := adc.DecodeConfig(b[:len(b)-1]); !errors.Is(err, adc.ErrShortBuffer) {
		t.Errorf("DecodeConfig(short) err = %v, want ErrShortBuffer", err)
	}
	if _, err := adc.DecodeConfig(append(b, 0x00)); !errors.Is(err, adc.ErrTrailingBytes) {
		t.Errorf("DecodeConfig(overlong) err = %v, want ErrTrailingBytes", err)
	}
}
