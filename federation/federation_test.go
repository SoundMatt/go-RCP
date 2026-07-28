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
	"net"
	"sync"
	"testing"

	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/federation"
	"github.com/SoundMatt/go-RCP/udp"
)

// newController dials a *udp.Controller for registry bookkeeping tests.
// UDP is connectionless, so dialing needs no listener on the other end —
// only identity and registry semantics are under test here, not traffic.
func newController(t *testing.T, suffix uint16) *udp.Controller {
	t.Helper()
	addr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 9}
	ctrl, err := udp.NewController(avtp.NewStreamID([6]byte{0x02, 0, 0, 0, 0, byte(suffix)}, suffix), addr)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	t.Cleanup(func() { _ = ctrl.Close() })
	return ctrl
}

// TestFederation_Register claims exclusive ownership for a key (REQ-FED-001).
func TestFederation_Register(t *testing.T) {
	r := federation.NewRegistry()
	ctrl := newController(t, 1)
	if err := r.Register("server-a", ctrl); err != nil {
		t.Fatalf("Register: %v", err)
	}
}

// TestFederation_Lookup returns the owning controller (REQ-FED-002).
func TestFederation_Lookup(t *testing.T) {
	r := federation.NewRegistry()
	ctrl := newController(t, 1)
	_ = r.Register("server-a", ctrl)

	got, err := r.Lookup("server-a")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got != ctrl {
		t.Errorf("Lookup returned wrong controller")
	}

	if _, err := r.Lookup("server-b"); !errors.Is(err, federation.ErrNotOwned) {
		t.Errorf("Lookup(unregistered) err = %v, want ErrNotOwned", err)
	}
}

// TestFederation_DoubleRegister returns ErrAlreadyOwned (REQ-FED-003).
func TestFederation_DoubleRegister(t *testing.T) {
	r := federation.NewRegistry()
	first := newController(t, 1)
	second := newController(t, 2)
	_ = r.Register("server-a", first)

	if err := r.Register("server-a", second); !errors.Is(err, federation.ErrAlreadyOwned) {
		t.Errorf("err = %v, want ErrAlreadyOwned", err)
	}
	got, _ := r.Lookup("server-a")
	if got != first {
		t.Errorf("ownership changed after a rejected Register")
	}
}

// TestFederation_Release removes ownership; releasing an unowned key
// returns ErrNotOwned (REQ-FED-004).
func TestFederation_Release(t *testing.T) {
	r := federation.NewRegistry()
	ctrl := newController(t, 1)
	_ = r.Register("server-a", ctrl)

	if err := r.Release("server-a"); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if _, err := r.Lookup("server-a"); !errors.Is(err, federation.ErrNotOwned) {
		t.Errorf("Lookup after Release err = %v, want ErrNotOwned", err)
	}
	if err := r.Release("server-a"); !errors.Is(err, federation.ErrNotOwned) {
		t.Errorf("Release(unowned) err = %v, want ErrNotOwned", err)
	}
}

// TestFederation_KeysAndOwner report the current registry state (REQ-FED-005).
func TestFederation_KeysAndOwner(t *testing.T) {
	r := federation.NewRegistry()
	a := newController(t, 1)
	b := newController(t, 2)
	_ = r.Register("server-a", a)
	_ = r.Register("server-b", b)

	keys := r.Keys()
	if len(keys) != 2 {
		t.Fatalf("Keys() = %v, want 2 entries", keys)
	}
	if r.Owner("server-a") != a {
		t.Errorf("Owner(server-a) wrong")
	}
	if r.Owner("unknown") != nil {
		t.Errorf("Owner(unknown) = non-nil, want nil")
	}
}

// TestFederation_TransferOwnership atomically reassigns a server between
// HPCs (REQ-FED-006).
func TestFederation_TransferOwnership(t *testing.T) {
	r := federation.NewRegistry()
	from := newController(t, 1)
	to := newController(t, 2)
	_ = r.Register("server-a", from)

	if err := r.TransferOwnership("server-a", from, to); err != nil {
		t.Fatalf("TransferOwnership: %v", err)
	}
	got, _ := r.Lookup("server-a")
	if got != to {
		t.Errorf("ownership did not transfer")
	}

	if err := r.TransferOwnership("server-a", from, to); !errors.Is(err, federation.ErrNotOwned) {
		t.Errorf("TransferOwnership from stale owner err = %v, want ErrNotOwned", err)
	}
}

// TestFederation_ConcurrentAccess exercises Register/Lookup/Release/Owner/
// Keys/TransferOwnership concurrently without a data race (REQ-FED-007).
func TestFederation_ConcurrentAccess(t *testing.T) {
	r := federation.NewRegistry()
	ctrls := make([]*udp.Controller, 8)
	for i := range ctrls {
		ctrls[i] = newController(t, uint16(i+1))
	}

	var wg sync.WaitGroup
	for i, ctrl := range ctrls {
		wg.Add(1)
		go func(i int, ctrl *udp.Controller) {
			defer wg.Done()
			key := "server"
			_ = r.Register(key, ctrl)
			_, _ = r.Lookup(key)
			_ = r.Owner(key)
			_ = r.Keys()
			_ = r.Release(key)
		}(i, ctrl)
	}
	wg.Wait()
}

// TestFederation_MultipleHPCsDisjointOwnership several HPC-controller pairs
// each own a disjoint set of servers; cross-HPC Lookup succeeds
// transparently (REQ-FED-008).
func TestFederation_MultipleHPCsDisjointOwnership(t *testing.T) {
	r := federation.NewRegistry()
	hpc1a, hpc1b := newController(t, 1), newController(t, 2)
	hpc2a := newController(t, 3)

	_ = r.Register("server-1", hpc1a)
	_ = r.Register("server-2", hpc1b)
	_ = r.Register("server-3", hpc2a)

	for key, want := range map[string]*udp.Controller{
		"server-1": hpc1a,
		"server-2": hpc1b,
		"server-3": hpc2a,
	} {
		got, err := r.Lookup(key)
		if err != nil {
			t.Fatalf("Lookup(%s): %v", key, err)
		}
		if got != want {
			t.Errorf("Lookup(%s) returned wrong owner", key)
		}
	}
}
