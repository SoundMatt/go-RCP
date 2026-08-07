//fusa:test REQ-RCS-009
//fusa:test REQ-RCS-010
//fusa:test REQ-RCS-011

package server_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/regmap"
	"github.com/SoundMatt/go-RCP/v9/server"
)

// TestSetPinAssignment_LockedAfterHWLock checks the pin-mapping table
// rejects writes once the server has left StateUnconfigured, even from the
// root client (REQ-RCS-009).
func TestSetPinAssignment_LockedAfterHWLock(t *testing.T) {
	root := rootStream()
	s := newRootServer(t, root)
	advanceToHWLocked(t, s, root, 1)

	err := s.SetPinAssignment(root, regmap.PinAssignment{Pin: 20, Endpoint: 1, SignalIndex: 0})
	if !errors.Is(err, regmap.ErrRegisterLocked) {
		t.Fatalf("SetPinAssignment after HW lock = %v, want ErrRegisterLocked", err)
	}
}

// TestWriteFunctional_StaysWritableAfterFullyConfigured checks a functional
// block remains writable once the server reaches StateFullyConfigured, both
// for the root client and for a stream with an explicit grant, matching the
// RC Server lifecycle chapter of the OPEN Alliance TC18 Remote Control
// Protocol Specification v0.5.1_RC: per-endpoint functional configuration
// stays open via that endpoint's own registered stream(s), or via the root
// client, even once every server-wide/HW-pin field has locked (REQ-RCS-010).
func TestWriteFunctional_StaysWritableAfterFullyConfigured(t *testing.T) {
	root := rootStream()
	s := newRootServer(t, root)
	advanceToHWLocked(t, s, root, 1)

	if err := s.WriteFunctional(root, 1, []byte{0x01}); err != nil {
		t.Fatalf("WriteFunctional (pre-lock): %v", err)
	}
	if err := s.SetQueueConfig(root, regmap.QueueConfig{FlushThreshold: 1}); err != nil {
		t.Fatalf("SetQueueConfig: %v", err)
	}
	if err := s.AdvanceToFullyConfigured(); err != nil {
		t.Fatalf("AdvanceToFullyConfigured: %v", err)
	}

	if err := s.WriteFunctional(root, 1, []byte{0x02}); err != nil {
		t.Fatalf("WriteFunctional (root, post-lock) = %v, want success", err)
	}

	restricted := avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x66}, 2)
	s.Grant(restricted, 1)
	if err := s.WriteFunctional(restricted, 1, []byte{0x03}); err != nil {
		t.Fatalf("WriteFunctional (granted stream, post-lock) = %v, want success", err)
	}

	// An ungranted, non-root stream still has no access at all, regardless
	// of lifecycle state.
	ungranted := avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x77}, 3)
	if err := s.WriteFunctional(ungranted, 1, []byte{0x04}); !errors.Is(err, regmap.ErrAccessDenied) {
		t.Fatalf("WriteFunctional (ungranted stream, post-lock) = %v, want ErrAccessDenied", err)
	}
}

// TestWriteEP0_LocksStreamsAndQueuesAfterFullyConfigured checks a whole-map
// write through EP0 that changes the request-stream or response/
// acknowledge-queue configuration tables is rejected once the server
// reaches StateFullyConfigured, even though the same write's per-endpoint
// functional-block change is independently permitted by
// TestWriteEP0_FunctionalBlockStaysWritableAfterFullyConfigured
// (REQ-RCS-010).
func TestWriteEP0_LocksStreamsAndQueuesAfterFullyConfigured(t *testing.T) {
	root := rootStream()
	s := newRootServer(t, root)
	advanceToHWLocked(t, s, root, 1)
	if err := s.WriteFunctional(root, 1, []byte{0x01}); err != nil {
		t.Fatalf("WriteFunctional: %v", err)
	}
	if err := s.SetQueueConfig(root, regmap.QueueConfig{FlushThreshold: 1}); err != nil {
		t.Fatalf("SetQueueConfig: %v", err)
	}
	if err := s.AdvanceToFullyConfigured(); err != nil {
		t.Fatalf("AdvanceToFullyConfigured: %v", err)
	}

	buf, err := s.ReadEP0(root)
	if err != nil {
		t.Fatalf("ReadEP0: %v", err)
	}
	m, err := regmap.DecodeRegisterMap(buf)
	if err != nil {
		t.Fatalf("DecodeRegisterMap: %v", err)
	}
	m.Queues.FlushThreshold = m.Queues.FlushThreshold + 1

	if err := s.WriteEP0(root, regmap.EncodeRegisterMap(m)); !errors.Is(err, regmap.ErrRegisterLocked) {
		t.Fatalf("WriteEP0 with tampered queue config = %v, want ErrRegisterLocked", err)
	}
}

// TestWriteEP0_FunctionalBlockStaysWritableAfterFullyConfigured checks a
// whole-map write through EP0 that touches only a declared endpoint's
// functional block succeeds for the root client once the server reaches
// StateFullyConfigured (REQ-RCS-010).
func TestWriteEP0_FunctionalBlockStaysWritableAfterFullyConfigured(t *testing.T) {
	root := rootStream()
	s := newRootServer(t, root)
	advanceToHWLocked(t, s, root, 1)
	if err := s.WriteFunctional(root, 1, []byte{0x01}); err != nil {
		t.Fatalf("WriteFunctional: %v", err)
	}
	if err := s.SetQueueConfig(root, regmap.QueueConfig{FlushThreshold: 1}); err != nil {
		t.Fatalf("SetQueueConfig: %v", err)
	}
	if err := s.AdvanceToFullyConfigured(); err != nil {
		t.Fatalf("AdvanceToFullyConfigured: %v", err)
	}

	if err := s.WriteEP0(root, mustEncodeWithFunctional(t, s, root, 1, []byte{0x02})); err != nil {
		t.Fatalf("WriteEP0 (functional-only change, post-lock) = %v, want success", err)
	}
	got, err := s.ReadEndpoint(root, 1)
	if err != nil {
		t.Fatalf("ReadEndpoint: %v", err)
	}
	if !bytes.Contains(got, []byte{0x02}) {
		t.Errorf("ReadEndpoint after WriteEP0 = %x, want it to carry the new functional byte 0x02", got)
	}
}

// mustEncodeWithFunctional reads s's current register map via EP0, replaces
// addr's functional block with data, and returns the re-encoded whole map,
// for tests exercising WriteEP0's functional-only-change allowance.
func mustEncodeWithFunctional(t *testing.T, s *server.Server, root avtp.StreamID, addr avtp.ByteBusID, data []byte) []byte {
	t.Helper()
	buf, err := s.ReadEP0(root)
	if err != nil {
		t.Fatalf("ReadEP0: %v", err)
	}
	m, err := regmap.DecodeRegisterMap(buf)
	if err != nil {
		t.Fatalf("DecodeRegisterMap: %v", err)
	}
	ep, ok := m.Endpoint(addr)
	if !ok {
		t.Fatalf("Endpoint(%v): not declared", addr)
	}
	ep.Functional.Data = data
	return regmap.EncodeRegisterMap(m)
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
	m, err := regmap.DecodeRegisterMap(buf)
	if err != nil {
		t.Fatalf("DecodeRegisterMap: %v", err)
	}
	m.General.VendorID = m.General.VendorID + 1 // tamper with an identity field

	err = s.WriteEP0(root, regmap.EncodeRegisterMap(m))
	if !errors.Is(err, regmap.ErrGeneralBlockReadOnly) {
		t.Fatalf("WriteEP0 with tampered general block = %v, want ErrGeneralBlockReadOnly", err)
	}
}
