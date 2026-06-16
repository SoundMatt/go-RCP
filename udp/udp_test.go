//fusa:test REQ-UDP-001
//fusa:test REQ-UDP-002
//fusa:test REQ-UDP-003
//fusa:test REQ-UDP-004
//fusa:test REQ-UDP-005
//fusa:test REQ-UDP-006
//fusa:test REQ-UDP-007
//fusa:test REQ-UDP-008
//fusa:test REQ-UDP-009
//fusa:test REQ-UDP-010
//fusa:test REQ-UDP-011
//fusa:test REQ-UDP-012

package udp_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
	rcpudp "github.com/SoundMatt/go-RCP/udp"
)

func newServerController(t *testing.T, zone rcp.Zone) (*rcpudp.ZoneServer, *rcpudp.Controller) {
	t.Helper()
	srv, err := rcpudp.NewZoneServer(zone, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewZoneServer: %v", err)
	}
	ctrl, err := rcpudp.NewController(zone, srv.Addr())
	if err != nil {
		_ = srv.Close()
		t.Fatalf("NewController: %v", err)
	}
	t.Cleanup(func() {
		_ = ctrl.Close()
		_ = srv.Close()
	})
	return srv, ctrl
}

// TestUDP_Send_RoundTrip verifies command send + response receipt over loopback (REQ-UDP-001).
func TestUDP_Send_RoundTrip(t *testing.T) {
	_, ctrl := newServerController(t, rcp.ZoneFrontLeft)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdNoop, ID: 1}
	resp, err := ctrl.Send(ctx, cmd)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Status != rcp.StatusOK {
		t.Errorf("status = %v, want OK", resp.Status)
	}
}

// TestUDP_Send_CustomHandler verifies server-side handler is invoked (REQ-UDP-002).
func TestUDP_Send_CustomHandler(t *testing.T) {
	srv, ctrl := newServerController(t, rcp.ZoneFrontRight)
	srv.SetHandler(func(cmd *rcp.Command) *rcp.Response {
		return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusError}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontRight, Type: rcp.CmdSet})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Status != rcp.StatusError {
		t.Errorf("status = %v, want Error", resp.Status)
	}
}

// TestUDP_Send_PayloadRoundTrip verifies payload survives encoding/decoding (REQ-UDP-003).
func TestUDP_Send_PayloadRoundTrip(t *testing.T) {
	want := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	srv, ctrl := newServerController(t, rcp.ZoneRearLeft)
	srv.SetHandler(func(cmd *rcp.Command) *rcp.Response {
		return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusOK, Payload: cmd.Payload}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneRearLeft, Type: rcp.CmdSet, Payload: want})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !bytes.Equal(resp.Payload, want) {
		t.Errorf("payload = %v, want %v", resp.Payload, want)
	}
}

// TestUDP_Send_ZoneMismatch verifies ErrZoneMismatch is returned for wrong zone (REQ-UDP-004).
func TestUDP_Send_ZoneMismatch(t *testing.T) {
	_, ctrl := newServerController(t, rcp.ZoneRearRight)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdNoop})
	if !errors.Is(err, rcp.ErrZoneMismatch) {
		t.Errorf("error = %v, want ErrZoneMismatch", err)
	}
}

// TestUDP_Send_ContextCancelledBeforeSend verifies ErrTimeout on pre-cancelled context (REQ-UDP-005).
func TestUDP_Send_ContextCancelledBeforeSend(t *testing.T) {
	_, ctrl := newServerController(t, rcp.ZoneCentral)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneCentral, Type: rcp.CmdNoop})
	if !errors.Is(err, rcp.ErrTimeout) {
		t.Errorf("error = %v, want ErrTimeout", err)
	}
}

// TestUDP_Send_ContextTimeout verifies ErrTimeout when server is unreachable (REQ-UDP-005).
func TestUDP_Send_ContextTimeout(t *testing.T) {
	// Use a real server but set an immediate deadline so the response race is deterministic.
	_, ctrl := newServerController(t, rcp.ZoneFrontLeft)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(2 * time.Millisecond) // ensure deadline is already past

	_, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdNoop})
	if !errors.Is(err, rcp.ErrTimeout) {
		t.Errorf("error = %v, want ErrTimeout", err)
	}
}

// TestUDP_Send_AfterClose verifies ErrClosed is returned after controller is closed (REQ-UDP-006).
func TestUDP_Send_AfterClose(t *testing.T) {
	_, ctrl := newServerController(t, rcp.ZoneFrontLeft)
	_ = ctrl.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdNoop})
	if !errors.Is(err, rcp.ErrClosed) {
		t.Errorf("error = %v, want ErrClosed", err)
	}
}

// TestUDP_Subscribe_ReceivesStatus verifies Publish → Subscribe fan-out (REQ-UDP-007).
func TestUDP_Subscribe_ReceivesStatus(t *testing.T) {
	srv, ctrl := newServerController(t, rcp.ZoneFrontLeft)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, err := ctrl.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	time.Sleep(10 * time.Millisecond) // allow subscribe frame to reach server

	srv.Publish([]byte{0x01})
	select {
	case st := <-ch:
		if st.Zone != rcp.ZoneFrontLeft {
			t.Errorf("zone = %v, want FrontLeft", st.Zone)
		}
	case <-time.After(time.Second):
		t.Fatal("no Status received within 1s")
	}
}

// TestUDP_Subscribe_MultipleSubscribers verifies multiple subscribers all receive Status (REQ-UDP-008).
func TestUDP_Subscribe_MultipleSubscribers(t *testing.T) {
	srv, err := rcpudp.NewZoneServer(rcp.ZoneFrontRight, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()

	const n = 3
	ctrls := make([]*rcpudp.Controller, n)
	chs := make([]<-chan *rcp.Status, n)
	for i := range ctrls {
		ctrl, err := rcpudp.NewController(rcp.ZoneFrontRight, srv.Addr())
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = ctrl.Close() }()
		ctrls[i] = ctrl
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		ch, err := ctrl.Subscribe(ctx)
		if err != nil {
			t.Fatal(err)
		}
		chs[i] = ch
	}

	time.Sleep(20 * time.Millisecond) // allow subscribe frames to reach server
	srv.Publish(nil)

	for i, ch := range chs {
		select {
		case <-ch:
		case <-time.After(time.Second):
			t.Errorf("subscriber %d: no Status received", i)
		}
	}
}

// TestUDP_Subscribe_ClosedOnContextCancel verifies channel closes when ctx is cancelled (REQ-UDP-009).
func TestUDP_Subscribe_ClosedOnContextCancel(t *testing.T) {
	_, ctrl := newServerController(t, rcp.ZoneRearLeft)
	ctx, cancel := context.WithCancel(context.Background())

	ch, err := ctrl.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	cancel()

	select {
	case _, ok := <-ch:
		if ok {
			t.Error("channel should be closed after context cancel")
		}
	case <-time.After(time.Second):
		t.Fatal("channel not closed within 1s")
	}
}

// TestUDP_Registry_DialAndLookup verifies Registry.Dial + Lookup + Close (REQ-UDP-010).
func TestUDP_Registry_DialAndLookup(t *testing.T) {
	srv, err := rcpudp.NewZoneServer(rcp.ZoneRearRight, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()

	reg := rcpudp.NewRegistry()
	defer func() { _ = reg.Close() }()

	if dialErr := reg.Dial(rcp.ZoneRearRight, srv.Addr().String()); dialErr != nil {
		t.Fatalf("Dial: %v", dialErr)
	}

	ctrl, err := reg.Lookup(rcp.ZoneRearRight)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if ctrl.Zone() != rcp.ZoneRearRight {
		t.Errorf("zone = %v, want RearRight", ctrl.Zone())
	}
}

// TestUDP_Registry_DuplicateDial verifies ErrAlreadyExists on duplicate registration (REQ-UDP-011).
func TestUDP_Registry_DuplicateDial(t *testing.T) {
	srv, err := rcpudp.NewZoneServer(rcp.ZoneCentral, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()

	reg := rcpudp.NewRegistry()
	defer func() { _ = reg.Close() }()

	if err2 := reg.Dial(rcp.ZoneCentral, srv.Addr().String()); err2 != nil {
		t.Fatalf("first Dial: %v", err2)
	}
	if err2 := reg.Dial(rcp.ZoneCentral, srv.Addr().String()); !errors.Is(err2, rcp.ErrAlreadyExists) {
		t.Errorf("error = %v, want ErrAlreadyExists", err2)
	}
}

// TestUDP_Registry_LookupMissing verifies ErrNotFound for an unregistered zone (REQ-UDP-012).
func TestUDP_Registry_LookupMissing(t *testing.T) {
	reg := rcpudp.NewRegistry()
	defer func() { _ = reg.Close() }()

	_, err := reg.Lookup(rcp.ZoneFrontLeft)
	if !errors.Is(err, rcp.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}
