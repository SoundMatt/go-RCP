//fusa:test REQ-RCS-001
//fusa:test REQ-RCS-002
//fusa:test REQ-RCS-004
//fusa:test REQ-RCS-005
//fusa:test REQ-RCS-006
//fusa:test REQ-RCS-007
//fusa:test REQ-RCS-008

package server_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/lifecycle"
	"github.com/SoundMatt/go-RCP/v9/regmap"
	"github.com/SoundMatt/go-RCP/v9/server"
)

func rootStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55}, 1)
}

// newRootServer returns a fresh Server with stream as its claimed root
// client.
func newRootServer(t *testing.T, stream avtp.StreamID) *server.Server {
	t.Helper()
	s := server.NewServer()
	if err := s.ClaimRoot(stream); err != nil {
		t.Fatalf("ClaimRoot: %v", err)
	}
	return s
}

// TestNewServer_StartsUnconfigured checks a fresh server begins in the bare-
// defaults unconfigured state (REQ-RCS-001).
func TestNewServer_StartsUnconfigured(t *testing.T) {
	s := server.NewServer()
	if s.State() != lifecycle.StateUnconfigured {
		t.Errorf("State() = %v, want StateUnconfigured", s.State())
	}
}

// TestAdvanceToHWLocked_Succeeds checks a plausible pin map lets the server
// advance to StateHWLocked (REQ-RCS-002).
func TestAdvanceToHWLocked_Succeeds(t *testing.T) {
	root := rootStream()
	s := newRootServer(t, root)

	if err := s.AddEndpoint(root, avtp.ByteBusID(1), regmap.EndpointTypeGPIO); err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}
	if err := s.SetPinAssignment(root, regmap.PinAssignment{Pin: 10, Endpoint: 1, SignalIndex: 0}); err != nil {
		t.Fatalf("SetPinAssignment: %v", err)
	}

	if err := s.AdvanceToHWLocked(); err != nil {
		t.Fatalf("AdvanceToHWLocked: %v", err)
	}
	if s.State() != lifecycle.StateHWLocked {
		t.Errorf("State() = %v, want StateHWLocked", s.State())
	}
}

// TestAdvanceToHWLocked_RejectsUndeclaredEndpoint checks a pin map entry
// referencing an endpoint that was never declared fails the plausibility
// check (REQ-RCS-004).
func TestAdvanceToHWLocked_RejectsUndeclaredEndpoint(t *testing.T) {
	root := rootStream()
	s := newRootServer(t, root)

	if err := s.SetPinAssignment(root, regmap.PinAssignment{Pin: 10, Endpoint: 99, SignalIndex: 0}); err != nil {
		t.Fatalf("SetPinAssignment: %v", err)
	}

	err := s.AdvanceToHWLocked()
	if !errors.Is(err, regmap.ErrPinMapInvalid) {
		t.Fatalf("AdvanceToHWLocked err = %v, want ErrPinMapInvalid", err)
	}
	if s.State() != lifecycle.StateUnconfigured {
		t.Errorf("State() = %v after rejected transition, want StateUnconfigured unchanged", s.State())
	}
}

// advanceToHWLocked is a test helper that declares one GPIO endpoint with a
// plausible pin assignment and advances s to StateHWLocked.
func advanceToHWLocked(t *testing.T, s *server.Server, root avtp.StreamID, addr avtp.ByteBusID) {
	t.Helper()
	if err := s.AddEndpoint(root, addr, regmap.EndpointTypeGPIO); err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}
	if err := s.SetPinAssignment(root, regmap.PinAssignment{Pin: 10, Endpoint: addr, SignalIndex: 0}); err != nil {
		t.Fatalf("SetPinAssignment: %v", err)
	}
	if err := s.AdvanceToHWLocked(); err != nil {
		t.Fatalf("AdvanceToHWLocked: %v", err)
	}
}

// TestAdvanceToFullyConfigured_Succeeds checks a fully populated functional
// block plus a plausible queue configuration lets the server reach
// StateFullyConfigured (REQ-RCS-005).
func TestAdvanceToFullyConfigured_Succeeds(t *testing.T) {
	root := rootStream()
	s := newRootServer(t, root)
	advanceToHWLocked(t, s, root, 1)

	if err := s.WriteFunctional(root, 1, []byte{0x01}); err != nil {
		t.Fatalf("WriteFunctional: %v", err)
	}
	if err := s.SetQueueConfig(root, regmap.QueueConfig{FlushThreshold: 4}); err != nil {
		t.Fatalf("SetQueueConfig: %v", err)
	}

	if err := s.AdvanceToFullyConfigured(); err != nil {
		t.Fatalf("AdvanceToFullyConfigured: %v", err)
	}
	if s.State() != lifecycle.StateFullyConfigured {
		t.Errorf("State() = %v, want StateFullyConfigured", s.State())
	}
}

// TestAdvanceToFullyConfigured_RejectsIncompleteFunctional checks an
// endpoint left with an empty functional block blocks the transition
// (REQ-RCS-006).
func TestAdvanceToFullyConfigured_RejectsIncompleteFunctional(t *testing.T) {
	root := rootStream()
	s := newRootServer(t, root)
	advanceToHWLocked(t, s, root, 1)

	if err := s.SetQueueConfig(root, regmap.QueueConfig{FlushThreshold: 4}); err != nil {
		t.Fatalf("SetQueueConfig: %v", err)
	}

	err := s.AdvanceToFullyConfigured()
	if !errors.Is(err, lifecycle.ErrFunctionalBlockIncomplete) {
		t.Fatalf("AdvanceToFullyConfigured err = %v, want ErrFunctionalBlockIncomplete", err)
	}
	if s.State() != lifecycle.StateHWLocked {
		t.Errorf("State() = %v after rejected transition, want StateHWLocked unchanged", s.State())
	}
}

// TestAdvanceToFullyConfigured_RejectsUnflushableQueue checks a queue
// configuration with both thresholds at zero — which would never flush —
// blocks the transition (REQ-RCS-007).
func TestAdvanceToFullyConfigured_RejectsUnflushableQueue(t *testing.T) {
	root := rootStream()
	s := newRootServer(t, root)
	advanceToHWLocked(t, s, root, 1)

	if err := s.WriteFunctional(root, 1, []byte{0x01}); err != nil {
		t.Fatalf("WriteFunctional: %v", err)
	}
	// QueueConfig zero value: FlushThreshold=0, FlushTimeMillis=0.

	err := s.AdvanceToFullyConfigured()
	if !errors.Is(err, regmap.ErrQueueConfigInvalid) {
		t.Fatalf("AdvanceToFullyConfigured err = %v, want ErrQueueConfigInvalid", err)
	}
}

// TestLifecycle_CannotSkipState checks a server may not advance directly
// from StateUnconfigured to StateFullyConfigured (REQ-RCS-008).
func TestLifecycle_CannotSkipState(t *testing.T) {
	s := server.NewServer()
	err := s.AdvanceToFullyConfigured()
	if !errors.Is(err, lifecycle.ErrLifecycleOutOfOrder) {
		t.Fatalf("AdvanceToFullyConfigured from Unconfigured err = %v, want ErrLifecycleOutOfOrder", err)
	}
	if s.State() != lifecycle.StateUnconfigured {
		t.Errorf("State() = %v, want StateUnconfigured unchanged", s.State())
	}
}
