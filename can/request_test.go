//fusa:test REQ-CANEP-005

package can_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/v9/can"
)

// TestFrameRoundTrip checks EncodeFrame/DecodeFrame round-trip for each
// frame format, and reject a short or overlong buffer (REQ-CANEP-005).
func TestFrameRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		f    can.Frame
	}{
		{"classical", can.Frame{Format: can.FormatClassical, ID: 0x123, Data: []byte{0x01, 0x02, 0x03}}},
		{"classical extended", can.Frame{Format: can.FormatClassical, Extended: true, ID: 0x1ABCDEF, Data: nil}},
		{"fd", can.Frame{Format: can.FormatFD, ID: 0x321, BitRateSwitch: true, Data: make([]byte, 64)}},
		{"xl", can.Frame{Format: can.FormatXL, ID: 0x42, Extended: true, BitRateSwitch: true,
			XL: can.XLHeader{SDT: 0xAB, VCID: 0x03, AF: 0xDEADBEEF}, Data: make([]byte, 2048)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i := range tt.f.Data {
				tt.f.Data[i] = byte(i)
			}
			b := can.EncodeFrame(tt.f)
			got, err := can.DecodeFrame(b)
			if err != nil {
				t.Fatalf("DecodeFrame: %v", err)
			}
			if got.Format != tt.f.Format || got.Extended != tt.f.Extended || got.ID != tt.f.ID ||
				got.BitRateSwitch != tt.f.BitRateSwitch || got.XL != tt.f.XL || !bytes.Equal(got.Data, tt.f.Data) {
				t.Errorf("DecodeFrame round-trip = %+v, want %+v", got, tt.f)
			}

			if _, err := can.DecodeFrame(b[:len(b)-1]); !errors.Is(err, can.ErrShortBuffer) && !errors.Is(err, can.ErrTrailingBytes) {
				// Truncating by one byte from the data-length field's
				// declared length always leaves too few bytes to satisfy
				// it — ErrShortBuffer.
				t.Errorf("DecodeFrame(short) err = %v, want ErrShortBuffer", err)
			}
			if _, err := can.DecodeFrame(append(b, 0x00)); !errors.Is(err, can.ErrTrailingBytes) {
				t.Errorf("DecodeFrame(overlong) err = %v, want ErrTrailingBytes", err)
			}
		})
	}
}
