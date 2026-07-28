//fusa:test REQ-UART-001
//fusa:test REQ-UART-002
//fusa:test REQ-UART-003

package uart_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/server"
	"github.com/SoundMatt/go-RCP/uart"
)

// rootStream and newDeclaredEndpoint are this package's own test helpers
// (uart_test is an external test package).
func rootStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55}, 1)
}

// newDeclaredEndpoint returns a uart.Endpoint declared (but not yet
// configured) on a fresh server.Server, with root as both the root client and
// the caller that will issue requests.
func newDeclaredEndpoint(t *testing.T) (*uart.Endpoint, avtp.StreamID) {
	t.Helper()
	root := rootStream()
	s := server.NewServer()
	if err := s.ClaimRoot(root); err != nil {
		t.Fatalf("ClaimRoot: %v", err)
	}
	if err := s.AddEndpoint(root, avtp.ByteBusID(1), uart.EndpointType); err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}
	return uart.NewEndpoint(s, avtp.ByteBusID(1)), root
}

// TestParityAndStopBits_Valid checks Valid recognizes exactly the defined
// values for each enum (REQ-UART-002).
func TestParityAndStopBits_Valid(t *testing.T) {
	for p := uart.Parity(0); p < 5; p++ {
		if !p.Valid() {
			t.Errorf("Parity(%d).Valid() = false, want true", p)
		}
	}
	for _, p := range []uart.Parity{5, 6, 255} {
		if p.Valid() {
			t.Errorf("Parity(%d).Valid() = true, want false", p)
		}
	}

	for s := uart.StopBits(0); s < 3; s++ {
		if !s.Valid() {
			t.Errorf("StopBits(%d).Valid() = false, want true", s)
		}
	}
	for _, s := range []uart.StopBits{3, 4, 255} {
		if s.Valid() {
			t.Errorf("StopBits(%d).Valid() = true, want false", s)
		}
	}
}

// TestConfig_Validate checks a disabled endpoint always validates, and an
// enabled one enforces baud rate, data-bits range, parity, and stop bits
// (REQ-UART-001).
func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     uart.Config
		wantErr error
	}{
		{"disabled ok regardless of fields", uart.Config{}, nil},
		{"zero baud", uart.Config{Enabled: true, BaudRate: 0, DataBits: 8}, uart.ErrZeroBaudRate},
		{"data bits too low", uart.Config{Enabled: true, BaudRate: 9600, DataBits: 4}, uart.ErrInvalidDataBits},
		{"data bits too high", uart.Config{Enabled: true, BaudRate: 9600, DataBits: 10}, uart.ErrInvalidDataBits},
		{"invalid parity", uart.Config{Enabled: true, BaudRate: 9600, DataBits: 8, Parity: 9}, uart.ErrInvalidParity},
		{"invalid stop bits", uart.Config{Enabled: true, BaudRate: 9600, DataBits: 8, StopBits: 9}, uart.ErrInvalidStopBits},
		{"valid", uart.Config{Enabled: true, BaudRate: 115200, DataBits: 8, Parity: uart.ParityNone, StopBits: uart.StopBitsOne}, nil},
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
// a short or overlong buffer (REQ-UART-003).
func TestConfigRoundTrip(t *testing.T) {
	cfg := uart.Config{
		Enabled:           true,
		BaudRate:          115200,
		DataBits:          8,
		Parity:            uart.ParityEven,
		StopBits:          uart.StopBitsTwo,
		FlowControl:       true,
		ReadTimeoutMicros: 50000,
	}
	b := uart.EncodeConfig(cfg)
	got, err := uart.DecodeConfig(b)
	if err != nil {
		t.Fatalf("DecodeConfig: %v", err)
	}
	if got != cfg {
		t.Errorf("DecodeConfig round-trip = %+v, want %+v", got, cfg)
	}

	if _, err := uart.DecodeConfig(b[:len(b)-1]); !errors.Is(err, uart.ErrShortBuffer) {
		t.Errorf("DecodeConfig(short) err = %v, want ErrShortBuffer", err)
	}
	if _, err := uart.DecodeConfig(append(b, 0x00)); !errors.Is(err, uart.ErrTrailingBytes) {
		t.Errorf("DecodeConfig(overlong) err = %v, want ErrTrailingBytes", err)
	}
}
