//fusa:test REQ-SHMEM-001
//fusa:test REQ-SHMEM-002

package shmem_test

import (
	"context"
	"errors"
	"testing"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/mock"
	"github.com/SoundMatt/go-RCP/shmem"
)

// ── Controller accessors and post-close behaviour ──────────────────────────────

func TestShmem_Controller_ZoneAndClose(t *testing.T) {
	reg := shmem.NewRegistry()
	defer reg.Close() //nolint:errcheck
	_, ctrl, err := reg.Open(rcp.ZoneCentral)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if ctrl.Zone() != rcp.ZoneCentral {
		t.Errorf("Zone() = %v, want ZoneCentral", ctrl.Zone())
	}
	if err := ctrl.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := ctrl.Send(context.Background(), &rcp.Command{Zone: rcp.ZoneCentral}); !errors.Is(err, rcp.ErrClosed) {
		t.Errorf("Send after Close = %v, want ErrClosed", err)
	}
	if _, err := ctrl.Subscribe(context.Background()); !errors.Is(err, rcp.ErrClosed) {
		t.Errorf("Subscribe after Close = %v, want ErrClosed", err)
	}
}

// ── SetHealthy reflected in published Status ───────────────────────────────────

func TestShmem_SetHealthy_ReflectedInStatus(t *testing.T) {
	reg := shmem.NewRegistry()
	defer reg.Close() //nolint:errcheck
	srv, ctrl, err := reg.Open(rcp.ZoneFrontLeft)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := ctrl.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	srv.SetHealthy(false)
	srv.Publish([]byte("tick"))
	select {
	case st := <-ch:
		if st.Healthy {
			t.Error("Status.Healthy = true, want false after SetHealthy(false)")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Status")
	}
}

// ── Registry interface ─────────────────────────────────────────────────────────

func TestShmem_Open_Duplicate(t *testing.T) {
	reg := shmem.NewRegistry()
	defer reg.Close() //nolint:errcheck
	if _, _, err := reg.Open(rcp.ZoneRearLeft); err != nil {
		t.Fatalf("first Open: %v", err)
	}
	if _, _, err := reg.Open(rcp.ZoneRearLeft); !errors.Is(err, rcp.ErrAlreadyExists) {
		t.Errorf("duplicate Open = %v, want ErrAlreadyExists", err)
	}
}

func TestShmem_Registry_LookupAndControllers(t *testing.T) {
	reg := shmem.NewRegistry()
	defer reg.Close() //nolint:errcheck
	_, ctrl, err := reg.Open(rcp.ZoneFrontRight)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got, err := reg.Lookup(rcp.ZoneFrontRight)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got != ctrl {
		t.Error("Lookup returned a different controller than Open")
	}
	if _, err := reg.Lookup(rcp.ZoneRearRight); !errors.Is(err, rcp.ErrNotFound) {
		t.Errorf("Lookup(unregistered) = %v, want ErrNotFound", err)
	}
	if n := len(reg.Controllers()); n != 1 {
		t.Errorf("Controllers() len = %d, want 1", n)
	}
}

func TestShmem_Registry_RegisterAndDeregister(t *testing.T) {
	reg := shmem.NewRegistry()
	defer reg.Close() //nolint:errcheck

	// Open then Deregister, leaving a *shmem.Controller we can re-Register.
	_, ctrl, err := reg.Open(rcp.ZoneCentral)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := reg.Deregister(rcp.ZoneCentral); err != nil {
		t.Fatalf("Deregister: %v", err)
	}
	if _, err := reg.Lookup(rcp.ZoneCentral); !errors.Is(err, rcp.ErrNotFound) {
		t.Errorf("Lookup after Deregister = %v, want ErrNotFound", err)
	}
	if err := reg.Deregister(rcp.ZoneCentral); !errors.Is(err, rcp.ErrNotFound) {
		t.Errorf("double Deregister = %v, want ErrNotFound", err)
	}

	// Re-register the orphaned controller: success path.
	if err := reg.Register(ctrl); err != nil {
		t.Fatalf("Register: %v", err)
	}
	// Registering again for the same zone is a duplicate.
	if err := reg.Register(ctrl); !errors.Is(err, rcp.ErrAlreadyExists) {
		t.Errorf("duplicate Register = %v, want ErrAlreadyExists", err)
	}
}

func TestShmem_Registry_Register_RejectsForeign(t *testing.T) {
	reg := shmem.NewRegistry()
	defer reg.Close() //nolint:errcheck
	foreign := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer foreign.Close() //nolint:errcheck
	if err := reg.Register(foreign); err == nil {
		t.Error("Register(non-shmem controller) = nil, want error")
	}
}

func TestShmem_Registry_ClosedErrors(t *testing.T) {
	reg := shmem.NewRegistry()
	if err := reg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := reg.Close(); err != nil {
		t.Errorf("second Close = %v, want nil", err)
	}
	if _, _, err := reg.Open(rcp.ZoneFrontLeft); !errors.Is(err, rcp.ErrClosed) {
		t.Errorf("Open after Close = %v, want ErrClosed", err)
	}
	foreign := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer foreign.Close() //nolint:errcheck
	if err := reg.Register(foreign); !errors.Is(err, rcp.ErrClosed) {
		t.Errorf("Register after Close = %v, want ErrClosed", err)
	}
	if _, err := reg.Lookup(rcp.ZoneFrontLeft); !errors.Is(err, rcp.ErrClosed) {
		t.Errorf("Lookup after Close = %v, want ErrClosed", err)
	}
}
