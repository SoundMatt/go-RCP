//fusa:test REQ-RCS-018

package server_test

import (
	"testing"

	"github.com/SoundMatt/go-RCP/server"
)

// TestGeneralBlock_RoundTrips checks EncodeGeneralBlock followed by
// DecodeGeneralBlock reproduces an identical GeneralBlock: identification
// fields, protocol/register-map version, capability/capacity counters, and
// the four configuration-table pointers all survive byte-for-byte
// (REQ-RCS-018).
func TestGeneralBlock_RoundTrips(t *testing.T) {
	want := server.GeneralBlock{
		VendorID:                0x11223344,
		ProductID:               0x55667788,
		RegisterMapVersion:      server.RegisterMapVersion,
		MaxEndpoints:            16,
		MaxStreams:              4,
		MaxFunctionalBlockBytes: 256,
		PinMapPointer:           21,
		StreamConfigPointer:     40,
		QueueConfigPointer:      43,
		EndpointTablePointer:    53,
	}

	buf := server.EncodeGeneralBlock(want)
	got, rest, err := server.DecodeGeneralBlock(buf)
	if err != nil {
		t.Fatalf("DecodeGeneralBlock: %v", err)
	}
	if len(rest) != 0 {
		t.Errorf("DecodeGeneralBlock left %d trailing bytes, want 0", len(rest))
	}
	if got != want {
		t.Errorf("DecodeGeneralBlock = %+v, want %+v", got, want)
	}
}

// TestDecodeGeneralBlock_ShortBufferError checks a buffer shorter than the
// fixed general-block length is rejected rather than panicking.
func TestDecodeGeneralBlock_ShortBufferError(t *testing.T) {
	_, _, err := server.DecodeGeneralBlock(make([]byte, 5))
	if err == nil {
		t.Fatal("DecodeGeneralBlock on a short buffer: got nil error, want one")
	}
}
