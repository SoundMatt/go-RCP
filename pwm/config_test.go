//fusa:test REQ-PWM-001
//fusa:test REQ-PWM-002
//fusa:test REQ-PWM-003

package pwm_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/pwm"
	"github.com/SoundMatt/go-RCP/v9/server"
)

// rootStream and newDeclaredEndpoint are this package's own test helpers
// (pwm_test is an external test package).
func rootStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55}, 1)
}

// newDeclaredEndpoint returns a pwm.Endpoint declared (but not yet
// configured) on a fresh server.Server, with root as both the root client
// and the caller that will issue requests.
func newDeclaredEndpoint(t *testing.T) (*pwm.Endpoint, avtp.StreamID) {
	t.Helper()
	root := rootStream()
	s := server.NewServer()
	if err := s.ClaimRoot(root); err != nil {
		t.Fatalf("ClaimRoot: %v", err)
	}
	if err := s.AddEndpoint(root, avtp.ByteBusID(1), pwm.EndpointType); err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}
	return pwm.NewEndpoint(s, avtp.ByteBusID(1)), root
}

// TestRole_Valid checks Valid recognizes exactly the two defined roles
// (REQ-PWM-002).
func TestRole_Valid(t *testing.T) {
	for r := pwm.Role(0); r < 2; r++ {
		if !r.Valid() {
			t.Errorf("Role(%d).Valid() = false, want true", r)
		}
	}
	for _, r := range []pwm.Role{2, 3, 255} {
		if r.Valid() {
			t.Errorf("Role(%d).Valid() = true, want false", r)
		}
	}
}

// TestConfig_Validate checks a disabled endpoint always validates, an
// enabled one requires a recognized Role, and a RoleOutput endpoint's
// default waveform must not have DefaultActiveTicks exceed
// DefaultPeriodTicks (REQ-PWM-001).
func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     pwm.Config
		wantErr error
	}{
		{"disabled ok regardless of fields", pwm.Config{}, nil},
		{"invalid role", pwm.Config{Enabled: true, Role: 9}, pwm.ErrInvalidRole},
		{"output active exceeds period", pwm.Config{Enabled: true, Role: pwm.RoleOutput, DefaultActiveTicks: 200, DefaultPeriodTicks: 100}, pwm.ErrActiveExceedsPeriod},
		{"output valid", pwm.Config{Enabled: true, Role: pwm.RoleOutput, DefaultActiveTicks: 500, DefaultPeriodTicks: 1000}, nil},
		{"input ignores default fields", pwm.Config{Enabled: true, Role: pwm.RoleInput, DefaultActiveTicks: 1000, DefaultPeriodTicks: 1}, nil},
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
// a short or overlong buffer (REQ-PWM-003).
func TestConfigRoundTrip(t *testing.T) {
	cfg := pwm.Config{Enabled: true, Role: pwm.RoleOutput, DefaultActiveTicks: 1500, DefaultPeriodTicks: 20000}
	b := pwm.EncodeConfig(cfg)
	got, err := pwm.DecodeConfig(b)
	if err != nil {
		t.Fatalf("DecodeConfig: %v", err)
	}
	if got != cfg {
		t.Errorf("DecodeConfig round-trip = %+v, want %+v", got, cfg)
	}

	if _, err := pwm.DecodeConfig(b[:len(b)-1]); !errors.Is(err, pwm.ErrShortBuffer) {
		t.Errorf("DecodeConfig(short) err = %v, want ErrShortBuffer", err)
	}
	if _, err := pwm.DecodeConfig(append(b, 0x00)); !errors.Is(err, pwm.ErrTrailingBytes) {
		t.Errorf("DecodeConfig(overlong) err = %v, want ErrTrailingBytes", err)
	}
}

// TestWaveformRoundTrip checks EncodeWaveform/DecodeWaveform round-trip and
// reject a short or overlong buffer (REQ-PWM-003).
func TestWaveformRoundTrip(t *testing.T) {
	b := pwm.EncodeWaveform(1500, 20000)
	active, period, err := pwm.DecodeWaveform(b)
	if err != nil {
		t.Fatalf("DecodeWaveform: %v", err)
	}
	if active != 1500 || period != 20000 {
		t.Errorf("DecodeWaveform round-trip = (%d, %d), want (1500, 20000)", active, period)
	}

	if _, _, err := pwm.DecodeWaveform(b[:len(b)-1]); !errors.Is(err, pwm.ErrShortBuffer) {
		t.Errorf("DecodeWaveform(short) err = %v, want ErrShortBuffer", err)
	}
	if _, _, err := pwm.DecodeWaveform(append(b, 0x00)); !errors.Is(err, pwm.ErrTrailingBytes) {
		t.Errorf("DecodeWaveform(overlong) err = %v, want ErrTrailingBytes", err)
	}
}
