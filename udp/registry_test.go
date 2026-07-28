//fusa:test REQ-UDP-011
//fusa:test REQ-UDP-012
//fusa:test REQ-UDP-013

package udp_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/server"
	"github.com/SoundMatt/go-RCP/udp"
)

// TestRegistry_DialAndLookup verifies Registry.Dial + Lookup round-trip a
// Controller under its registered key (REQ-UDP-011).
func TestRegistry_DialAndLookup(t *testing.T) {
	router := udp.NewRouter(udp.NewEP0Handler(server.NewServer()), true)
	us, err := udp.NewServer(serverStream(), "127.0.0.1:0", router)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = us.Close() }()

	reg := udp.NewRegistry()
	defer func() { _ = reg.Close() }()

	ctrl, err := reg.Dial("primary", clientStream(), us.Addr().String())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}

	got, err := reg.Lookup("primary")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got != ctrl {
		t.Errorf("Lookup returned a different *Controller than Dial")
	}
}

// TestRegistry_DuplicateDial verifies ErrAlreadyExists on a second Dial
// under the same key (REQ-UDP-012).
func TestRegistry_DuplicateDial(t *testing.T) {
	router := udp.NewRouter(udp.NewEP0Handler(server.NewServer()), true)
	us, err := udp.NewServer(serverStream(), "127.0.0.1:0", router)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = us.Close() }()

	reg := udp.NewRegistry()
	defer func() { _ = reg.Close() }()

	if _, err := reg.Dial("primary", clientStream(), us.Addr().String()); err != nil {
		t.Fatalf("first Dial: %v", err)
	}
	if _, err := reg.Dial("primary", clientStream(), us.Addr().String()); !errors.Is(err, udp.ErrAlreadyExists) {
		t.Errorf("error = %v, want ErrAlreadyExists", err)
	}
}

// TestRegistry_LookupMissing verifies ErrNotFound for an unregistered key
// (REQ-UDP-013).
func TestRegistry_LookupMissing(t *testing.T) {
	reg := udp.NewRegistry()
	defer func() { _ = reg.Close() }()

	_, err := reg.Lookup("nope")
	if !errors.Is(err, udp.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}
