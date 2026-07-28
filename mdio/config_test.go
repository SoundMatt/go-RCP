//fusa:test REQ-MDIO-001

package mdio_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/mdio"
	"github.com/SoundMatt/go-RCP/server"
)

// rootStream and newDeclaredEndpoint are this package's own test helpers
// (mdio_test is an external test package).
func rootStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x06, 0x11, 0x22, 0x33, 0x44, 0x55}, 1)
}

// newDeclaredEndpoint returns an mdio.Endpoint declared (but not yet
// configured) on a fresh server.Server, with root as both the root client
// and the caller that will issue requests.
func newDeclaredEndpoint(t *testing.T) (*mdio.Endpoint, avtp.StreamID) {
	t.Helper()
	root := rootStream()
	s := server.NewServer()
	if err := s.ClaimRoot(root); err != nil {
		t.Fatalf("ClaimRoot: %v", err)
	}
	if err := s.AddEndpoint(root, avtp.ByteBusID(1), mdio.EndpointType); err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}
	return mdio.NewEndpoint(s, avtp.ByteBusID(1)), root
}

// TestConfigRoundTrip checks EncodeConfig/DecodeConfig round-trip and reject
// a short or overlong buffer (REQ-MDIO-001).
func TestConfigRoundTrip(t *testing.T) {
	cfg := mdio.Config{Enabled: true}
	b := mdio.EncodeConfig(cfg)
	got, err := mdio.DecodeConfig(b)
	if err != nil {
		t.Fatalf("DecodeConfig: %v", err)
	}
	if got != cfg {
		t.Errorf("DecodeConfig round-trip = %+v, want %+v", got, cfg)
	}

	if _, err := mdio.DecodeConfig(b[:len(b)-1]); !errors.Is(err, mdio.ErrShortBuffer) {
		t.Errorf("DecodeConfig(short) err = %v, want ErrShortBuffer", err)
	}
	if _, err := mdio.DecodeConfig(append(b, 0x00)); !errors.Is(err, mdio.ErrTrailingBytes) {
		t.Errorf("DecodeConfig(overlong) err = %v, want ErrTrailingBytes", err)
	}
}
