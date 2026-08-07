//fusa:test REQ-I2C-001
//fusa:test REQ-I2C-002
//fusa:test REQ-I2C-003

package i2c_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/i2c"
	"github.com/SoundMatt/go-RCP/v9/server"
)

// rootStream and newDeclaredEndpoint are this package's own test helpers
// (i2c_test is an external test package).
func rootStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55}, 1)
}

// newDeclaredEndpoint returns an i2c.Endpoint declared (but not yet
// configured) on a fresh server.Server, with root as both the root client
// and the caller that will issue requests.
func newDeclaredEndpoint(t *testing.T) (*i2c.Endpoint, avtp.StreamID) {
	t.Helper()
	root := rootStream()
	s := server.NewServer()
	if err := s.ClaimRoot(root); err != nil {
		t.Fatalf("ClaimRoot: %v", err)
	}
	if err := s.AddEndpoint(root, avtp.ByteBusID(1), i2c.EndpointType); err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}
	return i2c.NewEndpoint(s, avtp.ByteBusID(1)), root
}

// TestBusSpeed_Valid checks Valid recognizes exactly the five defined speed
// classes (REQ-I2C-002).
func TestBusSpeed_Valid(t *testing.T) {
	for s := i2c.BusSpeed(0); s < 5; s++ {
		if !s.Valid() {
			t.Errorf("BusSpeed(%d).Valid() = false, want true", s)
		}
	}
	for _, s := range []i2c.BusSpeed{5, 6, 255} {
		if s.Valid() {
			t.Errorf("BusSpeed(%d).Valid() = true, want false", s)
		}
	}
}

// TestConfig_Validate checks a disabled bus always validates, and an enabled
// bus requires a recognized Speed (REQ-I2C-001).
func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     i2c.Config
		wantErr error
	}{
		{"disabled ok regardless of speed", i2c.Config{Speed: 9}, nil},
		{"enabled invalid speed", i2c.Config{Enabled: true, Speed: 9}, i2c.ErrInvalidSpeed},
		{"enabled valid speed", i2c.Config{Enabled: true, Speed: i2c.SpeedFast}, nil},
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
// a short or overlong buffer (REQ-I2C-003).
func TestConfigRoundTrip(t *testing.T) {
	cfg := i2c.Config{Enabled: true, Speed: i2c.SpeedFastPlus, TrailingTimeMicros: 42}
	b := i2c.EncodeConfig(cfg)
	got, err := i2c.DecodeConfig(b)
	if err != nil {
		t.Fatalf("DecodeConfig: %v", err)
	}
	if got != cfg {
		t.Errorf("DecodeConfig round-trip = %+v, want %+v", got, cfg)
	}

	if _, err := i2c.DecodeConfig(b[:len(b)-1]); !errors.Is(err, i2c.ErrShortBuffer) {
		t.Errorf("DecodeConfig(short) err = %v, want ErrShortBuffer", err)
	}
	if _, err := i2c.DecodeConfig(append(b, 0x00)); !errors.Is(err, i2c.ErrTrailingBytes) {
		t.Errorf("DecodeConfig(overlong) err = %v, want ErrTrailingBytes", err)
	}
}
