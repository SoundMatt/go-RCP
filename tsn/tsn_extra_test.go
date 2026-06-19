//fusa:test REQ-TSN-003
//fusa:test REQ-TSN-006

package tsn_test

import (
	"context"
	"errors"
	"testing"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/mock"
	rcptsn "github.com/SoundMatt/go-RCP/tsn"
	rcpudp "github.com/SoundMatt/go-RCP/udp"
)

// ── Controller: Subscribe, Close, construction errors ──────────────────────────

func TestTSN_Subscribe(t *testing.T) {
	_, ctrl := newTSNPair(t, rcp.ZoneFrontLeft)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := ctrl.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if ch == nil {
		t.Fatal("Subscribe returned nil channel")
	}
}

func TestTSN_Send_AfterClose(t *testing.T) {
	_, ctrl := newTSNPair(t, rcp.ZoneFrontLeft)
	if err := ctrl.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdNoop})
	if !errors.Is(err, rcp.ErrClosed) {
		t.Errorf("Send after Close = %v, want ErrClosed", err)
	}
}

func TestTSN_NewController_BadAddr(t *testing.T) {
	if _, err := rcptsn.NewController(rcp.ZoneFrontLeft, "no-such-host:notaport", rcptsn.DefaultTSNConfig()); err == nil {
		t.Error("NewController(bad addr) = nil error, want resolve failure")
	}
}

// ── Registry ───────────────────────────────────────────────────────────────────

func newServerAddr(t *testing.T, zone rcp.Zone) string {
	t.Helper()
	srv, err := rcpudp.NewZoneServer(zone, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	return srv.Addr().String()
}

func TestTSN_Registry_DialLookupControllers(t *testing.T) {
	reg := rcptsn.NewRegistry()
	defer reg.Close() //nolint:errcheck
	addr := newServerAddr(t, rcp.ZoneFrontLeft)

	if err := reg.Dial(rcp.ZoneFrontLeft, addr, rcptsn.DefaultTSNConfig()); err != nil {
		t.Fatalf("Dial: %v", err)
	}
	if _, err := reg.Lookup(rcp.ZoneFrontLeft); err != nil {
		t.Errorf("Lookup: %v", err)
	}
	if _, err := reg.Lookup(rcp.ZoneCentral); !errors.Is(err, rcp.ErrNotFound) {
		t.Errorf("Lookup(unregistered) = %v, want ErrNotFound", err)
	}
	if n := len(reg.Controllers()); n != 1 {
		t.Errorf("Controllers() len = %d, want 1", n)
	}
	// Dialing the same zone again is a duplicate.
	if err := reg.Dial(rcp.ZoneFrontLeft, addr, rcptsn.DefaultTSNConfig()); !errors.Is(err, rcp.ErrAlreadyExists) {
		t.Errorf("duplicate Dial = %v, want ErrAlreadyExists", err)
	}
}

func TestTSN_Registry_Deregister(t *testing.T) {
	reg := rcptsn.NewRegistry()
	defer reg.Close() //nolint:errcheck
	addr := newServerAddr(t, rcp.ZoneRearLeft)
	if err := reg.Dial(rcp.ZoneRearLeft, addr, rcptsn.DefaultTSNConfig()); err != nil {
		t.Fatalf("Dial: %v", err)
	}
	if err := reg.Deregister(rcp.ZoneRearLeft); err != nil {
		t.Fatalf("Deregister: %v", err)
	}
	if _, err := reg.Lookup(rcp.ZoneRearLeft); !errors.Is(err, rcp.ErrNotFound) {
		t.Errorf("Lookup after Deregister = %v, want ErrNotFound", err)
	}
	if err := reg.Deregister(rcp.ZoneRearLeft); !errors.Is(err, rcp.ErrNotFound) {
		t.Errorf("double Deregister = %v, want ErrNotFound", err)
	}
}

func TestTSN_Registry_Register_RejectsForeign(t *testing.T) {
	reg := rcptsn.NewRegistry()
	defer reg.Close() //nolint:errcheck
	foreign := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer foreign.Close() //nolint:errcheck
	if err := reg.Register(foreign); err == nil {
		t.Error("Register(non-tsn controller) = nil, want error")
	}
}

func TestTSN_Registry_Dial_BadAddr(t *testing.T) {
	reg := rcptsn.NewRegistry()
	defer reg.Close() //nolint:errcheck
	if err := reg.Dial(rcp.ZoneFrontLeft, "no-such-host:notaport", rcptsn.DefaultTSNConfig()); err == nil {
		t.Error("Dial(bad addr) = nil, want resolve failure")
	}
}

func TestTSN_Registry_ClosedErrors(t *testing.T) {
	reg := rcptsn.NewRegistry()
	if err := reg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := reg.Close(); err != nil {
		t.Errorf("second Close = %v, want nil", err)
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
