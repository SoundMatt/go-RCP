//fusa:test REQ-GPIO-002
//fusa:test REQ-GPIO-003
//fusa:test REQ-GPIO-004
//fusa:test REQ-GPIO-005

package gpio_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/gpio"
	"github.com/SoundMatt/go-RCP/server"
)

// rootStream and newConfiguredEndpoint are this package's own test helpers
// (gpio_test is an external test package, so it cannot reuse server_test's
// unexported helpers of the same name).
func rootStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55}, 1)
}

// newConfiguredEndpoint returns a gpio.Endpoint declared and configured on a
// fresh server.Server, with root as both the root client and the caller
// that will issue requests.
func newConfiguredEndpoint(t *testing.T, cfg gpio.Config) (*gpio.Endpoint, avtp.StreamID) {
	t.Helper()
	root := rootStream()
	s := server.NewServer()
	if err := s.ClaimRoot(root); err != nil {
		t.Fatalf("ClaimRoot: %v", err)
	}
	if err := s.AddEndpoint(root, avtp.ByteBusID(1), gpio.EndpointType); err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}
	ep := gpio.NewEndpoint(s, avtp.ByteBusID(1))
	if err := ep.Configure(root, cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	return ep, root
}

// writeAndGetValue is a small test helper wrapping HandleRequest for a
// write request.
func writeAndGetValue(t *testing.T, ep *gpio.Endpoint, requester avtp.StreamID, sem gpio.WriteSemantic, operand uint32) uint32 {
	t.Helper()
	req := avtp.Message{
		Kind:      avtp.KindShort,
		ByteBusID: avtp.ByteBusID(1),
		Control:   avtp.FlagWrite,
		Body:      gpio.EncodeWriteRequest(sem, operand),
	}
	resp, err := ep.HandleRequest(requester, req)
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	v, err := gpio.DecodeValue(resp.Body)
	if err != nil {
		t.Fatalf("DecodeValue: %v", err)
	}
	return v
}

// TestWriteSemantic_Valid checks Valid recognizes exactly the eight defined
// semantics (REQ-GPIO-002).
func TestWriteSemantic_Valid(t *testing.T) {
	for s := gpio.WriteSemantic(0); s < 8; s++ {
		if !s.Valid() {
			t.Errorf("WriteSemantic(%d).Valid() = false, want true", s)
		}
	}
	for _, s := range []gpio.WriteSemantic{8, 9, 255} {
		if s.Valid() {
			t.Errorf("WriteSemantic(%d).Valid() = true, want false", s)
		}
	}
}

// TestApplyWrite_CombiningSemantics checks Replace/Or/And/AndNot/Xor only
// affect output-direction pins, leaving input pins untouched (REQ-GPIO-003).
func TestApplyWrite_CombiningSemantics(t *testing.T) {
	// 4 pins: pins 0-1 output, pins 2-3 input. Pre-seed an input value via
	// SetInputs so we can check it survives an output-only write untouched.
	cfg := gpio.Config{PinCount: 4, Direction: 0b0011}
	ep, root := newConfiguredEndpoint(t, cfg)
	ep.SetInputs(0b1100) // pins 2-3 (input) go high

	tests := []struct {
		name    string
		sem     gpio.WriteSemantic
		operand uint32
		want    uint32
	}{
		{"replace", gpio.SemanticReplace, 0b0001, 0b1101},
		{"or", gpio.SemanticOr, 0b0010, 0b1111},
		{"and", gpio.SemanticAnd, 0b0001, 0b1101},
		{"andnot", gpio.SemanticAndNot, 0b0001, 0b1100},
		{"xor", gpio.SemanticXor, 0b0011, 0b1111},
	}
	for _, tt := range tests {
		got := writeAndGetValue(t, ep, root, tt.sem, tt.operand)
		if got != tt.want {
			t.Errorf("%s: value = %04b, want %04b", tt.name, got, tt.want)
		}
	}
}

// TestApplyWrite_SaturatingClamps checks SaturatingAdd/SaturatingSubtract
// clamp at the active-pin mask / zero rather than wrapping (REQ-GPIO-004).
func TestApplyWrite_SaturatingClamps(t *testing.T) {
	cfg := gpio.Config{PinCount: 4, Direction: 0b1111}
	ep, root := newConfiguredEndpoint(t, cfg)

	got := writeAndGetValue(t, ep, root, gpio.SemanticSaturatingAdd, 0xFFFFFFFF)
	if got != 0b1111 {
		t.Errorf("saturating add clamp: value = %04b, want 1111", got)
	}

	got = writeAndGetValue(t, ep, root, gpio.SemanticSaturatingSubtract, 0xFFFFFFFF)
	if got != 0 {
		t.Errorf("saturating subtract clamp: value = %04b, want 0000", got)
	}
}

// TestApplyWrite_Reconfigure checks SemanticReconfigure replaces Direction
// (masked to active pins) instead of the pin value, and persists the change
// (REQ-GPIO-005).
func TestApplyWrite_Reconfigure(t *testing.T) {
	cfg := gpio.Config{PinCount: 4, Direction: 0b0011}
	ep, root := newConfiguredEndpoint(t, cfg)

	// Set an output value first so we can confirm reconfigure leaves it be.
	writeAndGetValue(t, ep, root, gpio.SemanticReplace, 0b0001)

	// Reconfigure to all-input, with a stray out-of-range bit that must be
	// masked away.
	got := writeAndGetValue(t, ep, root, gpio.SemanticReconfigure, 0xFFFFFFF0)
	if got != 0b0001 {
		t.Errorf("value after reconfigure = %04b, want unchanged 0001", got)
	}

	// A subsequent write to what is now an all-input endpoint must leave the
	// value alone (no output bits left to drive).
	got = writeAndGetValue(t, ep, root, gpio.SemanticReplace, 0b1111)
	if got != 0b0001 {
		t.Errorf("value after post-reconfigure write = %04b, want unchanged 0001", got)
	}
}

// TestApplyWrite_InvalidSemantic checks an out-of-range semantic byte is
// rejected (REQ-GPIO-002).
func TestApplyWrite_InvalidSemantic(t *testing.T) {
	cfg := gpio.Config{PinCount: 4, Direction: 0b1111}
	ep, root := newConfiguredEndpoint(t, cfg)

	req := avtp.Message{
		Kind:      avtp.KindShort,
		ByteBusID: avtp.ByteBusID(1),
		Control:   avtp.FlagWrite,
		Body:      gpio.EncodeWriteRequest(9, 0),
	}
	if _, err := ep.HandleRequest(root, req); !errors.Is(err, gpio.ErrInvalidSemantic) {
		t.Errorf("HandleRequest err = %v, want ErrInvalidSemantic", err)
	}
}
