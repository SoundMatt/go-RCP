//fusa:test REQ-WAKEUP-004

package wakeup_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/v9/wakeup"
)

// TestPowerStateRequestRoundTrip and TestWakeHandshakeRoundTrip check
// Encode/Decode round-trip and reject short/overlong buffers
// (REQ-WAKEUP-004).
func TestPowerStateRequestRoundTrip(t *testing.T) {
	b := wakeup.EncodePowerStateRequest(wakeup.PowerSleep)
	got, err := wakeup.DecodePowerStateRequest(b)
	if err != nil {
		t.Fatalf("DecodePowerStateRequest: %v", err)
	}
	if got != wakeup.PowerSleep {
		t.Errorf("DecodePowerStateRequest round-trip = %v, want PowerSleep", got)
	}
	if _, err := wakeup.DecodePowerStateRequest(nil); !errors.Is(err, wakeup.ErrShortBuffer) {
		t.Errorf("DecodePowerStateRequest(empty) err = %v, want ErrShortBuffer", err)
	}
	if _, err := wakeup.DecodePowerStateRequest(append(b, 0x00)); !errors.Is(err, wakeup.ErrTrailingBytes) {
		t.Errorf("DecodePowerStateRequest(overlong) err = %v, want ErrTrailingBytes", err)
	}
}

func TestWakeHandshakeRoundTrip(t *testing.T) {
	h := wakeup.WakeHandshake{Start: wakeup.StartCold, Sequence: 7}
	b := wakeup.EncodeWakeHandshake(h)
	got, err := wakeup.DecodeWakeHandshake(b)
	if err != nil {
		t.Fatalf("DecodeWakeHandshake: %v", err)
	}
	if got != h {
		t.Errorf("DecodeWakeHandshake round-trip = %+v, want %+v", got, h)
	}
	if _, err := wakeup.DecodeWakeHandshake(b[:len(b)-1]); !errors.Is(err, wakeup.ErrShortBuffer) {
		t.Errorf("DecodeWakeHandshake(short) err = %v, want ErrShortBuffer", err)
	}
	if _, err := wakeup.DecodeWakeHandshake(append(b, 0x00)); !errors.Is(err, wakeup.ErrTrailingBytes) {
		t.Errorf("DecodeWakeHandshake(overlong) err = %v, want ErrTrailingBytes", err)
	}
}
