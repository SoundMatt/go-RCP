//fusa:test REQ-CANEP-001
//fusa:test REQ-CANEP-002

package can_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/can"
	"github.com/SoundMatt/go-RCP/server"
)

// rootStream and newDeclaredEndpoint are this package's own test helpers
// (can_test is an external test package).
func rootStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x04, 0x11, 0x22, 0x33, 0x44, 0x55}, 1)
}

// newDeclaredEndpoint returns a can.Endpoint declared (but not yet
// configured) on a fresh server.Server, with root as both the root client
// and the caller that will issue requests.
func newDeclaredEndpoint(t *testing.T) (*can.Endpoint, avtp.StreamID) {
	t.Helper()
	root := rootStream()
	s := server.NewServer()
	if err := s.ClaimRoot(root); err != nil {
		t.Fatalf("ClaimRoot: %v", err)
	}
	if err := s.AddEndpoint(root, avtp.ByteBusID(1), can.EndpointType); err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}
	return can.NewEndpoint(s, avtp.ByteBusID(1)), root
}

// TestConfig_Validate checks a disabled bus always validates, and an enabled
// bus requires a nonzero NominalBitrateKbps (REQ-CANEP-001).
func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     can.Config
		wantErr error
	}{
		{"disabled ok regardless of bitrate", can.Config{}, nil},
		{"enabled zero bitrate", can.Config{Enabled: true}, can.ErrInvalidBitrate},
		{"enabled valid bitrate", can.Config{Enabled: true, NominalBitrateKbps: 500}, nil},
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
// a short or overlong buffer (REQ-CANEP-002).
func TestConfigRoundTrip(t *testing.T) {
	cfg := can.Config{Enabled: true, NominalBitrateKbps: 500, DataBitrateKbps: 2000}
	b := can.EncodeConfig(cfg)
	got, err := can.DecodeConfig(b)
	if err != nil {
		t.Fatalf("DecodeConfig: %v", err)
	}
	if got != cfg {
		t.Errorf("DecodeConfig round-trip = %+v, want %+v", got, cfg)
	}

	if _, err := can.DecodeConfig(b[:len(b)-1]); !errors.Is(err, can.ErrShortBuffer) {
		t.Errorf("DecodeConfig(short) err = %v, want ErrShortBuffer", err)
	}
	if _, err := can.DecodeConfig(append(b, 0x00)); !errors.Is(err, can.ErrTrailingBytes) {
		t.Errorf("DecodeConfig(overlong) err = %v, want ErrTrailingBytes", err)
	}
}
