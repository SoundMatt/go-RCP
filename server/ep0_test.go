//fusa:test REQ-RCS-012
//fusa:test REQ-RCS-013
//fusa:test REQ-RCS-014
//fusa:test REQ-RCS-015
//fusa:test REQ-RCS-016
//fusa:test REQ-RCS-017

package server_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/regmap"
)

// TestReadEP0_RoundTrips checks a whole-map read decodes back to a map with
// the same endpoints and pin-mapping table that were configured
// (REQ-RCS-012).
func TestReadEP0_RoundTrips(t *testing.T) {
	root := rootStream()
	s := newRootServer(t, root)

	if err := s.AddEndpoint(root, 1, regmap.EndpointTypeGPIO); err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}
	if err := s.SetPinAssignment(root, regmap.PinAssignment{Pin: 7, Endpoint: 1, SignalIndex: 0}); err != nil {
		t.Fatalf("SetPinAssignment: %v", err)
	}
	if err := s.WriteFunctional(root, 1, []byte{0xAA, 0xBB}); err != nil {
		t.Fatalf("WriteFunctional: %v", err)
	}

	buf, err := s.ReadEP0(root)
	if err != nil {
		t.Fatalf("ReadEP0: %v", err)
	}

	m, err := regmap.DecodeRegisterMap(buf)
	if err != nil {
		t.Fatalf("DecodeRegisterMap: %v", err)
	}
	ep, ok := m.Endpoint(1)
	if !ok {
		t.Fatalf("Endpoint(1) not found after round trip")
	}
	if ep.Generic.Type != regmap.EndpointTypeGPIO {
		t.Errorf("Endpoint(1).Generic.Type = %v, want EndpointTypeGPIO", ep.Generic.Type)
	}
	if !bytes.Equal(ep.Functional.Data, []byte{0xAA, 0xBB}) {
		t.Errorf("Endpoint(1).Functional.Data = % X, want AA BB", ep.Functional.Data)
	}
	pins := m.PinMap.Entries()
	if len(pins) != 1 || pins[0].Pin != 7 {
		t.Errorf("PinMap.Entries() = %+v, want one entry for pin 7", pins)
	}
}

// TestWriteEP0_RootCanUpdatePreLock checks the root client's whole-map
// write is applied when the server has not yet been fully configured
// (REQ-RCS-013).
func TestWriteEP0_RootCanUpdatePreLock(t *testing.T) {
	root := rootStream()
	s := newRootServer(t, root)
	if err := s.AddEndpoint(root, 1, regmap.EndpointTypeGPIO); err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}

	buf, err := s.ReadEP0(root)
	if err != nil {
		t.Fatalf("ReadEP0: %v", err)
	}
	m, err := regmap.DecodeRegisterMap(buf)
	if err != nil {
		t.Fatalf("DecodeRegisterMap: %v", err)
	}
	ep, _ := m.Endpoint(1)
	ep.Functional.Data = []byte{0x42}

	if writeErr := s.WriteEP0(root, regmap.EncodeRegisterMap(m)); writeErr != nil {
		t.Fatalf("WriteEP0: %v", writeErr)
	}

	got, err := s.ReadEndpoint(root, 1)
	if err != nil {
		t.Fatalf("ReadEndpoint: %v", err)
	}
	if !bytes.Contains(got, []byte{0x42}) {
		t.Errorf("ReadEndpoint(1) = % X, want it to contain the written functional byte 0x42", got)
	}
}

// TestWriteEP0_DeniedForNonRoot checks a stream that has not claimed the
// root-client role cannot perform a whole-map write (REQ-RCS-014).
func TestWriteEP0_DeniedForNonRoot(t *testing.T) {
	root := rootStream()
	s := newRootServer(t, root)
	other := avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x77}, 3)

	buf, err := s.ReadEP0(root)
	if err != nil {
		t.Fatalf("ReadEP0: %v", err)
	}

	// other has no grant for EP0 either, so ReadEP0 as other must also be
	// denied — exercised here alongside the write-side check.
	if _, err := s.ReadEP0(other); !errors.Is(err, regmap.ErrAccessDenied) {
		t.Fatalf("ReadEP0(other) = %v, want ErrAccessDenied", err)
	}

	if err := s.WriteEP0(other, buf); !errors.Is(err, regmap.ErrNotRootClient) {
		t.Fatalf("WriteEP0(other) = %v, want ErrNotRootClient", err)
	}
}

// TestRestrictedStream_GrantedEndpointAccessible checks a restricted
// (non-root) stream can read and write only an endpoint explicitly granted
// to it (REQ-RCS-015).
func TestRestrictedStream_GrantedEndpointAccessible(t *testing.T) {
	root := rootStream()
	s := newRootServer(t, root)
	if err := s.AddEndpoint(root, 1, regmap.EndpointTypeGPIO); err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}

	restricted := avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x88}, 4)
	s.Grant(restricted, 1)

	if err := s.WriteFunctional(restricted, 1, []byte{0x11}); err != nil {
		t.Fatalf("WriteFunctional(restricted, granted endpoint): %v", err)
	}
	got, err := s.ReadEndpoint(restricted, 1)
	if err != nil {
		t.Fatalf("ReadEndpoint(restricted, granted endpoint): %v", err)
	}
	if !bytes.Contains(got, []byte{0x11}) {
		t.Errorf("ReadEndpoint = % X, want it to contain 0x11", got)
	}
}

// TestRestrictedStream_UngrantedEndpointDenied checks a restricted stream is
// denied access to an endpoint it was never granted (REQ-RCS-016).
func TestRestrictedStream_UngrantedEndpointDenied(t *testing.T) {
	root := rootStream()
	s := newRootServer(t, root)
	if err := s.AddEndpoint(root, 1, regmap.EndpointTypeGPIO); err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}
	if err := s.AddEndpoint(root, 2, regmap.EndpointTypeGPIO); err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}

	restricted := avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x99}, 5)
	s.Grant(restricted, 1) // granted endpoint 1 only

	if _, err := s.ReadEndpoint(restricted, 2); !errors.Is(err, regmap.ErrAccessDenied) {
		t.Fatalf("ReadEndpoint(restricted, 2) = %v, want ErrAccessDenied", err)
	}
	if err := s.WriteFunctional(restricted, 2, []byte{0x01}); !errors.Is(err, regmap.ErrAccessDenied) {
		t.Fatalf("WriteFunctional(restricted, 2) = %v, want ErrAccessDenied", err)
	}
}

// TestClaimRoot_ExclusiveToOneStream checks the root-client role can only be
// held by one stream at a time, while re-claiming from the same stream that
// already holds it is idempotent (REQ-RCS-017).
func TestClaimRoot_ExclusiveToOneStream(t *testing.T) {
	root := rootStream()
	s := newRootServer(t, root)

	if err := s.ClaimRoot(root); err != nil {
		t.Fatalf("re-claiming root from the same stream: %v, want nil (idempotent)", err)
	}

	other := avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0xAA}, 6)
	if err := s.ClaimRoot(other); !errors.Is(err, regmap.ErrRootAlreadyClaimed) {
		t.Fatalf("ClaimRoot(other) = %v, want ErrRootAlreadyClaimed", err)
	}
}
