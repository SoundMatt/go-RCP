//fusa:test REQ-MCR-001
//fusa:test REQ-MCR-002
//fusa:test REQ-MCR-003
//fusa:test REQ-MCR-004
//fusa:test REQ-MCR-005

package mock_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/mock"
	"github.com/SoundMatt/go-RCP/udp"
)

func newTestClient() *mock.Client {
	router := udp.NewRouter(nil, true)
	return mock.NewClient(testStream(), router)
}

// TestClientRegistry_RegisterThenLookup verifies a registered Client is
// retrievable by its key (REQ-MCR-001).
func TestClientRegistry_RegisterThenLookup(t *testing.T) {
	reg := mock.NewClientRegistry()
	defer func() { _ = reg.Close() }()

	c := newTestClient()
	if err := reg.Register("root", c); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, err := reg.Lookup("root")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got != c {
		t.Error("Lookup returned a different Client")
	}
}

// TestClientRegistry_Register_Duplicate verifies a duplicate key is
// rejected with ErrAlreadyExists (REQ-MCR-002).
func TestClientRegistry_Register_Duplicate(t *testing.T) {
	reg := mock.NewClientRegistry()
	defer func() { _ = reg.Close() }()

	if err := reg.Register("root", newTestClient()); err != nil {
		t.Fatalf("Register: %v", err)
	}
	err := reg.Register("root", newTestClient())
	if !errors.Is(err, mock.ErrAlreadyExists) {
		t.Errorf("err = %v, want ErrAlreadyExists", err)
	}
}

// TestClientRegistry_Deregister verifies Deregister closes and removes the
// Client, and a repeat Deregister reports ErrNotFound (REQ-MCR-003).
func TestClientRegistry_Deregister(t *testing.T) {
	reg := mock.NewClientRegistry()
	defer func() { _ = reg.Close() }()

	if err := reg.Register("root", newTestClient()); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := reg.Deregister("root"); err != nil {
		t.Fatalf("Deregister: %v", err)
	}
	if _, err := reg.Lookup("root"); !errors.Is(err, mock.ErrNotFound) {
		t.Errorf("Lookup after deregister: err = %v, want ErrNotFound", err)
	}
	if err := reg.Deregister("root"); !errors.Is(err, mock.ErrNotFound) {
		t.Errorf("second Deregister: err = %v, want ErrNotFound", err)
	}
}

// TestClientRegistry_Clients verifies Clients reports every registered
// entry (REQ-MCR-004).
func TestClientRegistry_Clients(t *testing.T) {
	reg := mock.NewClientRegistry()
	defer func() { _ = reg.Close() }()

	if err := reg.Register("a", newTestClient()); err != nil {
		t.Fatalf("Register a: %v", err)
	}
	if err := reg.Register("b", newTestClient()); err != nil {
		t.Fatalf("Register b: %v", err)
	}
	if got := len(reg.Clients()); got != 2 {
		t.Errorf("Clients() len = %d, want 2", got)
	}
}

// TestClientRegistry_Close_ClosesAllAndRejectsFurtherUse verifies Close
// closes every registered Client, is idempotent, and rejects further
// Register/Lookup with ErrClosed (REQ-MCR-005).
func TestClientRegistry_Close_ClosesAllAndRejectsFurtherUse(t *testing.T) {
	reg := mock.NewClientRegistry()
	c := newTestClient()
	if err := reg.Register("root", c); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := reg.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := reg.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	if err := reg.Register("other", newTestClient()); !errors.Is(err, mock.ErrClosed) {
		t.Errorf("Register after close: err = %v, want ErrClosed", err)
	}
	if _, err := reg.Lookup("root"); !errors.Is(err, mock.ErrClosed) {
		t.Errorf("Lookup after close: err = %v, want ErrClosed", err)
	}
}
