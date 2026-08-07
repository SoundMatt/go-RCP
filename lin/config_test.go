//fusa:test REQ-LINEP-001
//fusa:test REQ-LINEP-002

package lin_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/lin"
	"github.com/SoundMatt/go-RCP/v9/server"
)

// rootStream and newDeclaredEndpoint are this package's own test helpers
// (lin_test is an external test package).
func rootStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x03, 0x11, 0x22, 0x33, 0x44, 0x55}, 1)
}

// newDeclaredEndpoint returns a lin.Endpoint declared (but not yet
// configured) on a fresh server.Server, with root as both the root client
// and the caller that will issue requests.
func newDeclaredEndpoint(t *testing.T) (*lin.Endpoint, avtp.StreamID) {
	t.Helper()
	root := rootStream()
	s := server.NewServer()
	if err := s.ClaimRoot(root); err != nil {
		t.Fatalf("ClaimRoot: %v", err)
	}
	if err := s.AddEndpoint(root, avtp.ByteBusID(1), lin.EndpointType); err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}
	return lin.NewEndpoint(s, avtp.ByteBusID(1)), root
}

// TestConfig_Validate checks a disabled bus always validates, and an enabled
// bus requires a nonzero BaudRate (REQ-LINEP-001).
func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     lin.Config
		wantErr error
	}{
		{"disabled ok regardless of baud", lin.Config{}, nil},
		{"enabled zero baud", lin.Config{Enabled: true}, lin.ErrInvalidBaudRate},
		{"enabled valid baud", lin.Config{Enabled: true, BaudRate: 19200}, nil},
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
// a short or overlong buffer (REQ-LINEP-002).
func TestConfigRoundTrip(t *testing.T) {
	cfg := lin.Config{Enabled: true, BaudRate: 9600, TrailingTimeMicros: 42}
	b := lin.EncodeConfig(cfg)
	got, err := lin.DecodeConfig(b)
	if err != nil {
		t.Fatalf("DecodeConfig: %v", err)
	}
	if got != cfg {
		t.Errorf("DecodeConfig round-trip = %+v, want %+v", got, cfg)
	}

	if _, err := lin.DecodeConfig(b[:len(b)-1]); !errors.Is(err, lin.ErrShortBuffer) {
		t.Errorf("DecodeConfig(short) err = %v, want ErrShortBuffer", err)
	}
	if _, err := lin.DecodeConfig(append(b, 0x00)); !errors.Is(err, lin.ErrTrailingBytes) {
		t.Errorf("DecodeConfig(overlong) err = %v, want ErrTrailingBytes", err)
	}
}
