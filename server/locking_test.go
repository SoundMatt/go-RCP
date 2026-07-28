//fusa:test REQ-RCS-009
//fusa:test REQ-RCS-010
//fusa:test REQ-RCS-011

package server_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/server"
)

// TestSetPinAssignment_LockedAfterHWLock checks the pin-mapping table
// rejects writes once the server has left StateUnconfigured, even from the
// root client (REQ-RCS-009).
func TestSetPinAssignment_LockedAfterHWLock(t *testing.T) {
	root := rootStream()
	s := newRootServer(t, root)
	advanceToHWLocked(t, s, root, 1)

	err := s.SetPinAssignment(root, server.PinAssignment{Pin: 20, Endpoint: 1, SignalIndex: 0})
	if !errors.Is(err, server.ErrRegisterLocked) {
		t.Fatalf("SetPinAssignment after HW lock = %v, want ErrRegisterLocked", err)
	}
}

// TestWriteFunctional_LockedAfterFullyConfigured checks a functional block
// rejects writes once the server reaches StateFullyConfigured, for the root
// client and for a stream with an explicit grant alike (REQ-RCS-010).
func TestWriteFunctional_LockedAfterFullyConfigured(t *testing.T) {
	root := rootStream()
	s := newRootServer(t, root)
	advanceToHWLocked(t, s, root, 1)

	if err := s.WriteFunctional(root, 1, []byte{0x01}); err != nil {
		t.Fatalf("WriteFunctional (pre-lock): %v", err)
	}
	if err := s.SetQueueConfig(root, server.QueueConfig{FlushThreshold: 1}); err != nil {
		t.Fatalf("SetQueueConfig: %v", err)
	}
	if err := s.AdvanceToFullyConfigured(); err != nil {
		t.Fatalf("AdvanceToFullyConfigured: %v", err)
	}

	if err := s.WriteFunctional(root, 1, []byte{0x02}); !errors.Is(err, server.ErrRegisterLocked) {
		t.Fatalf("WriteFunctional (root, post-lock) = %v, want ErrRegisterLocked", err)
	}

	restricted := avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x66}, 2)
	s.Grant(restricted, 1)
	if err := s.WriteFunctional(restricted, 1, []byte{0x03}); !errors.Is(err, server.ErrRegisterLocked) {
		t.Fatalf("WriteFunctional (granted stream, post-lock) = %v, want ErrRegisterLocked", err)
	}
}

// TestWriteEP0_RejectsGeneralBlockChange checks a whole-map write that
// alters the general server register block's identification/capability
// fields is rejected, since that block is server-owned and never
// client-writable (REQ-RCS-011).
func TestWriteEP0_RejectsGeneralBlockChange(t *testing.T) {
	root := rootStream()
	s := newRootServer(t, root)

	buf, err := s.ReadEP0(root)
	if err != nil {
		t.Fatalf("ReadEP0: %v", err)
	}
	m, err := server.DecodeRegisterMap(buf)
	if err != nil {
		t.Fatalf("DecodeRegisterMap: %v", err)
	}
	m.General.VendorID = m.General.VendorID + 1 // tamper with an identity field

	err = s.WriteEP0(root, server.EncodeRegisterMap(m))
	if !errors.Is(err, server.ErrGeneralBlockReadOnly) {
		t.Fatalf("WriteEP0 with tampered general block = %v, want ErrGeneralBlockReadOnly", err)
	}
}
