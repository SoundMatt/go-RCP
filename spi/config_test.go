//fusa:test REQ-SPI-001
//fusa:test REQ-SPI-002
//fusa:test REQ-SPI-003
//fusa:test REQ-SPI-009

package spi_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/server"
	"github.com/SoundMatt/go-RCP/spi"
)

// rootStream and newDeclaredEndpoint are this package's own test helpers
// (spi_test is an external test package).
func rootStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55}, 1)
}

// newDeclaredEndpoint returns a spi.Endpoint declared (but not yet
// channel-configured) on a fresh server.Server, with root as both the root
// client and the caller that will issue requests.
func newDeclaredEndpoint(t *testing.T) (*spi.Endpoint, avtp.StreamID) {
	t.Helper()
	_, ep, root := newDeclaredEndpointWithServer(t)
	return ep, root
}

// newDeclaredEndpointWithServer is like newDeclaredEndpoint but also returns
// the backing server.Server, for tests that need to inspect the persisted
// register map directly.
func newDeclaredEndpointWithServer(t *testing.T) (*server.Server, *spi.Endpoint, avtp.StreamID) {
	t.Helper()
	root := rootStream()
	s := server.NewServer()
	if err := s.ClaimRoot(root); err != nil {
		t.Fatalf("ClaimRoot: %v", err)
	}
	if err := s.AddEndpoint(root, avtp.ByteBusID(1), spi.EndpointType); err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}
	return s, spi.NewEndpoint(s, avtp.ByteBusID(1)), root
}

// TestChannel_Valid checks Valid recognizes exactly the six defined channels
// (REQ-SPI-001).
func TestChannel_Valid(t *testing.T) {
	for c := spi.Channel(0); c < spi.MaxChannels; c++ {
		if !c.Valid() {
			t.Errorf("Channel(%d).Valid() = false, want true", c)
		}
	}
	for _, c := range []spi.Channel{spi.MaxChannels, spi.MaxChannels + 1, 255} {
		if c.Valid() {
			t.Errorf("Channel(%d).Valid() = true, want false", c)
		}
	}
}

// TestMode_Valid checks Valid recognizes exactly the four clock modes
// (REQ-SPI-003).
func TestMode_Valid(t *testing.T) {
	for m := spi.Mode(0); m < 4; m++ {
		if !m.Valid() {
			t.Errorf("Mode(%d).Valid() = false, want true", m)
		}
	}
	for _, m := range []spi.Mode{4, 5, 255} {
		if m.Valid() {
			t.Errorf("Mode(%d).Valid() = true, want false", m)
		}
	}
}

// TestSetChannelConfig_LeavesOtherChannelsUntouched checks configuring one
// channel persists exactly that channel's ChannelConfig without disturbing
// the other five (REQ-SPI-002).
func TestSetChannelConfig_LeavesOtherChannelsUntouched(t *testing.T) {
	s, ep, root := newDeclaredEndpointWithServer(t)

	cc0 := spi.ChannelConfig{Enabled: true, ClockHz: 1_000_000, Mode: spi.Mode0, InterTransferDelayMicros: 5}
	if err := ep.SetChannelConfig(root, spi.Channel0, cc0); err != nil {
		t.Fatalf("SetChannelConfig(0): %v", err)
	}
	cc2 := spi.ChannelConfig{Enabled: true, ClockHz: 4_000_000, Mode: spi.Mode3, InterTransferDelayMicros: 1}
	if err := ep.SetChannelConfig(root, spi.Channel2, cc2); err != nil {
		t.Fatalf("SetChannelConfig(2): %v", err)
	}

	// Read back the persisted functional block through the whole-map read
	// path to confirm both channels' configuration survived independently,
	// and every other channel is still the zero value.
	raw, err := s.ReadEP0(root)
	if err != nil {
		t.Fatalf("ReadEP0: %v", err)
	}
	m, err := server.DecodeRegisterMap(raw)
	if err != nil {
		t.Fatalf("DecodeRegisterMap: %v", err)
	}
	regs, ok := m.Endpoint(1)
	if !ok {
		t.Fatalf("Endpoint(1) not found in decoded map")
	}
	got, err := spi.DecodeConfig(regs.Functional.Data)
	if err != nil {
		t.Fatalf("DecodeConfig: %v", err)
	}
	if got.Channels[0] != cc0 {
		t.Errorf("Channels[0] = %+v, want %+v", got.Channels[0], cc0)
	}
	if got.Channels[2] != cc2 {
		t.Errorf("Channels[2] = %+v, want %+v", got.Channels[2], cc2)
	}
	for _, i := range []int{1, 3, 4, 5} {
		if got.Channels[i] != (spi.ChannelConfig{}) {
			t.Errorf("Channels[%d] = %+v, want zero value untouched", i, got.Channels[i])
		}
	}
}

// TestConfig_Validate checks an enabled channel with a zero clock rate or an
// invalid mode fails validation (REQ-SPI-009).
func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     spi.Config
		wantErr error
	}{
		{"all disabled ok", spi.Config{}, nil},
		{"zero clock", spi.Config{Channels: [spi.MaxChannels]spi.ChannelConfig{{Enabled: true, Mode: spi.Mode0}}}, spi.ErrZeroClock},
		{"invalid mode", spi.Config{Channels: [spi.MaxChannels]spi.ChannelConfig{{Enabled: true, ClockHz: 1000, Mode: 9}}}, spi.ErrInvalidMode},
		{"valid", spi.Config{Channels: [spi.MaxChannels]spi.ChannelConfig{{Enabled: true, ClockHz: 1000, Mode: spi.Mode1}}}, nil},
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

// TestConfigRoundTrip checks EncodeConfig/DecodeConfig round-trip all six
// channel slots, including disabled ones (REQ-SPI-009).
func TestConfigRoundTrip(t *testing.T) {
	var cfg spi.Config
	cfg.Channels[0] = spi.ChannelConfig{Enabled: true, ClockHz: 1_000_000, Mode: spi.Mode0, InterTransferDelayMicros: 5}
	cfg.Channels[5] = spi.ChannelConfig{Enabled: true, ClockHz: 500_000, Mode: spi.Mode2, InterTransferDelayMicros: 100}
	// Channels 1-4 stay disabled/zero.

	b := spi.EncodeConfig(cfg)
	got, err := spi.DecodeConfig(b)
	if err != nil {
		t.Fatalf("DecodeConfig: %v", err)
	}
	if got != cfg {
		t.Errorf("DecodeConfig round-trip = %+v, want %+v", got, cfg)
	}

	if _, err := spi.DecodeConfig(b[:len(b)-1]); !errors.Is(err, spi.ErrShortBuffer) {
		t.Errorf("DecodeConfig(short) err = %v, want ErrShortBuffer", err)
	}
	if _, err := spi.DecodeConfig(append(b, 0x00)); !errors.Is(err, spi.ErrTrailingBytes) {
		t.Errorf("DecodeConfig(overlong) err = %v, want ErrTrailingBytes", err)
	}
}
