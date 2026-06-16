//fusa:test REQ-FED-001
//fusa:test REQ-FED-002
//fusa:test REQ-FED-003
//fusa:test REQ-FED-004
//fusa:test REQ-FED-005
//fusa:test REQ-FED-006
//fusa:test REQ-FED-007
//fusa:test REQ-FED-008

package federation_test

import (
	"errors"
	"sync"
	"testing"

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/federation"
	"github.com/SoundMatt/go-RCP/mock"
)

func ctrl(z rcp.Zone) *mock.Controller { return mock.NewController(z, nil) }

// TestFederation_RegisterAndLookup registers a zone and retrieves it (REQ-FED-001).
func TestFederation_RegisterAndLookup(t *testing.T) {
	reg := federation.NewRegistry()
	c := ctrl(rcp.ZoneFrontLeft)

	if err := reg.Register(rcp.ZoneFrontLeft, c); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, err := reg.Lookup(rcp.ZoneFrontLeft)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got != c {
		t.Error("Lookup returned wrong controller")
	}
}

// TestFederation_LookupUnknown returns ErrNotOwned for unregistered zone (REQ-FED-002).
func TestFederation_LookupUnknown(t *testing.T) {
	reg := federation.NewRegistry()
	_, err := reg.Lookup(rcp.ZoneFrontLeft)
	if !errors.Is(err, federation.ErrNotOwned) {
		t.Errorf("err = %v, want ErrNotOwned", err)
	}
}

// TestFederation_RegisterDuplicate returns ErrAlreadyOwned (REQ-FED-003).
func TestFederation_RegisterDuplicate(t *testing.T) {
	reg := federation.NewRegistry()
	_ = reg.Register(rcp.ZoneFrontLeft, ctrl(rcp.ZoneFrontLeft))

	err := reg.Register(rcp.ZoneFrontLeft, ctrl(rcp.ZoneFrontLeft))
	if !errors.Is(err, federation.ErrAlreadyOwned) {
		t.Errorf("err = %v, want ErrAlreadyOwned", err)
	}
}

// TestFederation_Release removes ownership (REQ-FED-004).
func TestFederation_Release(t *testing.T) {
	reg := federation.NewRegistry()
	_ = reg.Register(rcp.ZoneFrontLeft, ctrl(rcp.ZoneFrontLeft))

	if err := reg.Release(rcp.ZoneFrontLeft); err != nil {
		t.Fatalf("Release: %v", err)
	}
	_, err := reg.Lookup(rcp.ZoneFrontLeft)
	if !errors.Is(err, federation.ErrNotOwned) {
		t.Errorf("after Release: err = %v, want ErrNotOwned", err)
	}
}

// TestFederation_ReleaseUnknown returns ErrNotOwned (REQ-FED-004).
func TestFederation_ReleaseUnknown(t *testing.T) {
	reg := federation.NewRegistry()
	err := reg.Release(rcp.ZoneFrontRight)
	if !errors.Is(err, federation.ErrNotOwned) {
		t.Errorf("err = %v, want ErrNotOwned", err)
	}
}

// TestFederation_Zones returns all registered zones (REQ-FED-005).
func TestFederation_Zones(t *testing.T) {
	reg := federation.NewRegistry()
	want := []rcp.Zone{rcp.ZoneFrontLeft, rcp.ZoneRearRight}
	for _, z := range want {
		_ = reg.Register(z, ctrl(z))
	}
	got := reg.Zones()
	if len(got) != len(want) {
		t.Errorf("Zones() len = %d, want %d", len(got), len(want))
	}
	set := make(map[rcp.Zone]bool)
	for _, z := range got {
		set[z] = true
	}
	for _, z := range want {
		if !set[z] {
			t.Errorf("zone %v missing from Zones()", z)
		}
	}
}

// TestFederation_Owner returns nil for unowned zone (REQ-FED-005).
func TestFederation_Owner(t *testing.T) {
	reg := federation.NewRegistry()
	if got := reg.Owner(rcp.ZoneFrontLeft); got != nil {
		t.Errorf("Owner() = %v, want nil", got)
	}
	c := ctrl(rcp.ZoneFrontLeft)
	_ = reg.Register(rcp.ZoneFrontLeft, c)
	if got := reg.Owner(rcp.ZoneFrontLeft); got != c {
		t.Error("Owner() returned wrong controller")
	}
}

// TestFederation_TransferOwnership moves a zone atomically (REQ-FED-006).
func TestFederation_TransferOwnership(t *testing.T) {
	reg := federation.NewRegistry()
	hpc1 := ctrl(rcp.ZoneFrontLeft)
	hpc2 := ctrl(rcp.ZoneFrontLeft)
	_ = reg.Register(rcp.ZoneFrontLeft, hpc1)

	if err := reg.TransferOwnership(rcp.ZoneFrontLeft, hpc1, hpc2); err != nil {
		t.Fatalf("TransferOwnership: %v", err)
	}
	got, _ := reg.Lookup(rcp.ZoneFrontLeft)
	if got != hpc2 {
		t.Error("after transfer, Lookup should return hpc2")
	}
}

// TestFederation_TransferOwnership_WrongOwner rejects transfer from non-owner (REQ-FED-006).
func TestFederation_TransferOwnership_WrongOwner(t *testing.T) {
	reg := federation.NewRegistry()
	hpc1 := ctrl(rcp.ZoneFrontLeft)
	hpc2 := ctrl(rcp.ZoneFrontLeft)
	wrongHPC := ctrl(rcp.ZoneFrontLeft)
	_ = reg.Register(rcp.ZoneFrontLeft, hpc1)

	err := reg.TransferOwnership(rcp.ZoneFrontLeft, wrongHPC, hpc2)
	if !errors.Is(err, federation.ErrNotOwned) {
		t.Errorf("err = %v, want ErrNotOwned", err)
	}
}

// TestFederation_Concurrent no race under concurrent register/lookup (REQ-FED-007).
func TestFederation_Concurrent(t *testing.T) {
	reg := federation.NewRegistry()
	zones := []rcp.Zone{rcp.ZoneFrontLeft, rcp.ZoneFrontRight, rcp.ZoneRearLeft, rcp.ZoneRearRight}
	for _, z := range zones {
		_ = reg.Register(z, ctrl(z))
	}

	const n = 40
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			z := zones[i%len(zones)]
			_, _ = reg.Lookup(z)
			_ = reg.Owner(z)
		}(i)
	}
	wg.Wait()
}

// TestFederation_MultiHPC multiple HPCs own disjoint zones (REQ-FED-008).
func TestFederation_MultiHPC(t *testing.T) {
	reg := federation.NewRegistry()

	hpcA_fl := ctrl(rcp.ZoneFrontLeft)
	hpcA_fr := ctrl(rcp.ZoneFrontRight)
	hpcB_rl := ctrl(rcp.ZoneRearLeft)
	hpcB_rr := ctrl(rcp.ZoneRearRight)

	for _, pair := range []struct {
		z rcp.Zone
		c rcp.Controller
	}{
		{rcp.ZoneFrontLeft, hpcA_fl},
		{rcp.ZoneFrontRight, hpcA_fr},
		{rcp.ZoneRearLeft, hpcB_rl},
		{rcp.ZoneRearRight, hpcB_rr},
	} {
		if err := reg.Register(pair.z, pair.c); err != nil {
			t.Fatalf("Register %v: %v", pair.z, err)
		}
	}

	// HPC-B can reach HPC-A's zones via the shared registry.
	got, err := reg.Lookup(rcp.ZoneFrontLeft)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got != hpcA_fl {
		t.Error("Lookup returned wrong controller for ZoneFrontLeft")
	}

	if len(reg.Zones()) != 4 {
		t.Errorf("Zones() len = %d, want 4", len(reg.Zones()))
	}
}
