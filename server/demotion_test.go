//fusa:test REQ-RCS-031

package server_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/lifecycle"
	"github.com/SoundMatt/go-RCP/regmap"
)

// TestDemoteToUnconfigured_RootSucceeds checks the root client can demote a
// StateHWLocked server back to StateUnconfigured, and that the pin-mapping
// table and endpoint declarations become writable again as a result
// (REQ-RCS-031).
func TestDemoteToUnconfigured_RootSucceeds(t *testing.T) {
	root := rootStream()
	s := newRootServer(t, root)
	advanceToHWLocked(t, s, root, 1)

	if err := s.DemoteToUnconfigured(root); err != nil {
		t.Fatalf("DemoteToUnconfigured: %v", err)
	}
	if s.State() != lifecycle.StateUnconfigured {
		t.Fatalf("State() = %v, want StateUnconfigured", s.State())
	}

	// The pin-mapping table and endpoint declarations must be writable
	// again, exactly as in a freshly-created server.
	if err := s.AddEndpoint(root, avtp.ByteBusID(2), regmap.EndpointTypeGPIO); err != nil {
		t.Fatalf("AddEndpoint after demotion: %v", err)
	}
	if err := s.SetPinAssignment(root, regmap.PinAssignment{Pin: 11, Endpoint: 2, SignalIndex: 0}); err != nil {
		t.Fatalf("SetPinAssignment after demotion: %v", err)
	}
}

// TestDemoteToUnconfigured_ConfigurationClaimantSucceeds checks the stream
// currently holding the Discovery-stream configuration claim — not just the
// root client — can also demote a StateHWLocked server, per the RC Server
// lifecycle chapter of the OPEN Alliance TC18 Remote Control Protocol
// Specification v0.5.1_RC (REQ-RCS-031).
func TestDemoteToUnconfigured_ConfigurationClaimantSucceeds(t *testing.T) {
	root := rootStream()
	s := newRootServer(t, root)
	advanceToHWLocked(t, s, root, 1)

	claimant := avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x88}, 4)
	if err := s.ClaimConfiguration(claimant); err != nil {
		t.Fatalf("ClaimConfiguration: %v", err)
	}

	if err := s.DemoteToUnconfigured(claimant); err != nil {
		t.Fatalf("DemoteToUnconfigured (claimant): %v", err)
	}
	if s.State() != lifecycle.StateUnconfigured {
		t.Fatalf("State() = %v, want StateUnconfigured", s.State())
	}
}

// TestDemoteToUnconfigured_RejectsUnauthorizedStream checks a stream that is
// neither the root client nor the active configuration claimant cannot
// demote the server (REQ-RCS-031).
func TestDemoteToUnconfigured_RejectsUnauthorizedStream(t *testing.T) {
	root := rootStream()
	s := newRootServer(t, root)
	advanceToHWLocked(t, s, root, 1)

	other := avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x99}, 5)
	err := s.DemoteToUnconfigured(other)
	if !errors.Is(err, lifecycle.ErrDemotionNotAuthorized) {
		t.Fatalf("DemoteToUnconfigured (unauthorized) = %v, want ErrDemotionNotAuthorized", err)
	}
	if s.State() != lifecycle.StateHWLocked {
		t.Errorf("State() = %v after rejected demotion, want StateHWLocked unchanged", s.State())
	}
}

// TestDemoteToUnconfigured_RejectsFromFullyConfigured checks there is no
// reverse transition out of StateFullyConfigured — demotion is only defined
// from StateHWLocked (REQ-RCS-031).
func TestDemoteToUnconfigured_RejectsFromFullyConfigured(t *testing.T) {
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

	err := s.DemoteToUnconfigured(root)
	if !errors.Is(err, lifecycle.ErrLifecycleOutOfOrder) {
		t.Fatalf("DemoteToUnconfigured (from FullyConfigured) = %v, want ErrLifecycleOutOfOrder", err)
	}
	if s.State() != lifecycle.StateFullyConfigured {
		t.Errorf("State() = %v after rejected demotion, want StateFullyConfigured unchanged", s.State())
	}
}

// TestDemoteToUnconfigured_RejectsFromUnconfigured checks demotion is a
// no-op-rejecting call, not an idempotent one, when the server is already in
// StateUnconfigured (REQ-RCS-031).
func TestDemoteToUnconfigured_RejectsFromUnconfigured(t *testing.T) {
	root := rootStream()
	s := newRootServer(t, root)

	err := s.DemoteToUnconfigured(root)
	if !errors.Is(err, lifecycle.ErrLifecycleOutOfOrder) {
		t.Fatalf("DemoteToUnconfigured (from Unconfigured) = %v, want ErrLifecycleOutOfOrder", err)
	}
}
