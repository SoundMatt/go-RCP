//fusa:test REQ-ISELED-001
//fusa:test REQ-ISELED-002

package iseled_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/iseled"
	"github.com/SoundMatt/go-RCP/server"
)

// rootStream and newDeclaredEndpoint are this package's own test helpers
// (iseled_test is an external test package).
func rootStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x05, 0x11, 0x22, 0x33, 0x44, 0x55}, 1)
}

// newDeclaredEndpoint returns an iseled.Endpoint declared (but not yet
// configured) on a fresh server.Server, with root as both the root client
// and the caller that will issue requests.
func newDeclaredEndpoint(t *testing.T) (*iseled.Endpoint, avtp.StreamID) {
	t.Helper()
	root := rootStream()
	s := server.NewServer()
	if err := s.ClaimRoot(root); err != nil {
		t.Fatalf("ClaimRoot: %v", err)
	}
	if err := s.AddEndpoint(root, avtp.ByteBusID(1), iseled.EndpointType); err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}
	return iseled.NewEndpoint(s, avtp.ByteBusID(1)), root
}

// TestConfig_Validate checks a disabled chain always validates, and an
// enabled chain requires a nonzero DeviceCount (REQ-ISELED-001).
func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     iseled.Config
		wantErr error
	}{
		{"disabled ok regardless of device count", iseled.Config{}, nil},
		{"enabled zero devices", iseled.Config{Enabled: true}, iseled.ErrInvalidDeviceCount},
		{"enabled valid device count", iseled.Config{Enabled: true, DeviceCount: 4}, nil},
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
// a short or overlong buffer (REQ-ISELED-002).
func TestConfigRoundTrip(t *testing.T) {
	cfg := iseled.Config{Enabled: true, DeviceCount: 12, ResponseTimeoutMicros: 500}
	b := iseled.EncodeConfig(cfg)
	got, err := iseled.DecodeConfig(b)
	if err != nil {
		t.Fatalf("DecodeConfig: %v", err)
	}
	if got != cfg {
		t.Errorf("DecodeConfig round-trip = %+v, want %+v", got, cfg)
	}

	if _, err := iseled.DecodeConfig(b[:len(b)-1]); !errors.Is(err, iseled.ErrShortBuffer) {
		t.Errorf("DecodeConfig(short) err = %v, want ErrShortBuffer", err)
	}
	if _, err := iseled.DecodeConfig(append(b, 0x00)); !errors.Is(err, iseled.ErrTrailingBytes) {
		t.Errorf("DecodeConfig(overlong) err = %v, want ErrTrailingBytes", err)
	}
}
