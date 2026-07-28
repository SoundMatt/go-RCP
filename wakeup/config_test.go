//fusa:test REQ-WAKEUP-001
//fusa:test REQ-WAKEUP-002

package wakeup_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/server"
	"github.com/SoundMatt/go-RCP/wakeup"
)

// rootStream and newDeclaredEndpoint are this package's own test helpers
// (wakeup_test is an external test package).
func rootStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x07, 0x11, 0x22, 0x33, 0x44, 0x55}, 1)
}

// newDeclaredEndpoint returns a wakeup.Endpoint declared (but not yet
// configured) on a fresh server.Server, with root as both the root client
// and the caller that will issue requests.
func newDeclaredEndpoint(t *testing.T) (*wakeup.Endpoint, avtp.StreamID) {
	t.Helper()
	root := rootStream()
	s := server.NewServer()
	if err := s.ClaimRoot(root); err != nil {
		t.Fatalf("ClaimRoot: %v", err)
	}
	if err := s.AddEndpoint(root, avtp.ByteBusID(1), wakeup.EndpointType); err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}
	return wakeup.NewEndpoint(s, avtp.ByteBusID(1)), root
}

// defaultConfig is a plausible enabled Config this package's other test
// files reuse.
func defaultConfig() wakeup.Config {
	return wakeup.Config{Enabled: true, WakeHandshakeIntervalMillis: 50, WakeHandshakeRepeatCount: 3}
}

// TestConfig_Validate checks a disabled endpoint always validates, and an
// enabled one requires a nonzero interval and repeat count (REQ-WAKEUP-001).
func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     wakeup.Config
		wantErr error
	}{
		{"disabled ok regardless of other fields", wakeup.Config{}, nil},
		{"enabled zero interval", wakeup.Config{Enabled: true, WakeHandshakeRepeatCount: 1}, wakeup.ErrInvalidHandshakeInterval},
		{"enabled zero repeat count", wakeup.Config{Enabled: true, WakeHandshakeIntervalMillis: 1}, wakeup.ErrInvalidHandshakeRepeatCount},
		{"enabled valid", defaultConfig(), nil},
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
// a short or overlong buffer (REQ-WAKEUP-002).
func TestConfigRoundTrip(t *testing.T) {
	cfg := defaultConfig()
	b := wakeup.EncodeConfig(cfg)
	got, err := wakeup.DecodeConfig(b)
	if err != nil {
		t.Fatalf("DecodeConfig: %v", err)
	}
	if got != cfg {
		t.Errorf("DecodeConfig round-trip = %+v, want %+v", got, cfg)
	}

	if _, err := wakeup.DecodeConfig(b[:len(b)-1]); !errors.Is(err, wakeup.ErrShortBuffer) {
		t.Errorf("DecodeConfig(short) err = %v, want ErrShortBuffer", err)
	}
	if _, err := wakeup.DecodeConfig(append(b, 0x00)); !errors.Is(err, wakeup.ErrTrailingBytes) {
		t.Errorf("DecodeConfig(overlong) err = %v, want ErrTrailingBytes", err)
	}
}
