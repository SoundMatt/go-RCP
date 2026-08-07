//fusa:test REQ-ISELED-003
//fusa:test REQ-ISELED-004

package iseled_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/v9/iseled"
)

// TestCommandRoundTrip checks EncodeCommand/DecodeCommand round-trip, and
// that a corrupted trailing byte is caught by the CRC check
// (REQ-ISELED-003).
func TestCommandRoundTrip(t *testing.T) {
	cmd := iseled.Command{Address: 3, Data: []byte{0x10, 0x20, 0x30}}
	b := iseled.EncodeCommand(cmd)
	got, err := iseled.DecodeCommand(b)
	if err != nil {
		t.Fatalf("DecodeCommand: %v", err)
	}
	if got.Address != cmd.Address || !bytes.Equal(got.Data, cmd.Data) {
		t.Errorf("DecodeCommand round-trip = %+v, want %+v", got, cmd)
	}

	corrupted := append([]byte(nil), b...)
	corrupted[len(corrupted)-1] ^= 0xFF
	if _, err := iseled.DecodeCommand(corrupted); !errors.Is(err, iseled.ErrCRCMismatch) {
		t.Errorf("DecodeCommand(corrupted CRC) err = %v, want ErrCRCMismatch", err)
	}

	if _, err := iseled.DecodeCommand(b[:len(b)-2]); !errors.Is(err, iseled.ErrShortBuffer) {
		t.Errorf("DecodeCommand(short) err = %v, want ErrShortBuffer", err)
	}
	if _, err := iseled.DecodeCommand(append(b, 0x00)); !errors.Is(err, iseled.ErrTrailingBytes) {
		t.Errorf("DecodeCommand(overlong) err = %v, want ErrTrailingBytes", err)
	}
}

// TestAggregatedResponseRoundTrip checks EncodeAggregatedResponse/
// DecodeAggregatedResponse round-trip for zero, one, and several device
// responses (REQ-ISELED-004).
func TestAggregatedResponseRoundTrip(t *testing.T) {
	tests := []iseled.AggregatedResponse{
		nil,
		{{Address: 0, Data: []byte{0x01}}},
		{
			{Address: 0, Data: []byte{0x01, 0x02}},
			{Address: 1, Data: nil},
			{Address: 2, Data: []byte{0xAA, 0xBB, 0xCC}},
		},
	}
	for _, resp := range tests {
		b := iseled.EncodeAggregatedResponse(resp)
		got, err := iseled.DecodeAggregatedResponse(b)
		if err != nil {
			t.Fatalf("DecodeAggregatedResponse(%d entries): %v", len(resp), err)
		}
		if len(got) != len(resp) {
			t.Fatalf("DecodeAggregatedResponse round-trip len = %d, want %d", len(got), len(resp))
		}
		for i := range resp {
			if got[i].Address != resp[i].Address || !bytes.Equal(got[i].Data, resp[i].Data) {
				t.Errorf("entry %d = %+v, want %+v", i, got[i], resp[i])
			}
		}
	}

	if _, err := iseled.DecodeAggregatedResponse(nil); !errors.Is(err, iseled.ErrShortBuffer) {
		t.Errorf("DecodeAggregatedResponse(nil) err = %v, want ErrShortBuffer", err)
	}
	if _, err := iseled.DecodeAggregatedResponse([]byte{0x01}); !errors.Is(err, iseled.ErrShortBuffer) {
		t.Errorf("DecodeAggregatedResponse(truncated entry) err = %v, want ErrShortBuffer", err)
	}
}
