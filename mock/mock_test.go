package mock_test

//fusa:test REQ-CTRL-001
//fusa:test REQ-CTRL-002
//fusa:test REQ-CTRL-003
//fusa:test REQ-CTRL-004
//fusa:test REQ-CTRL-005
//fusa:test REQ-CTRL-006
//fusa:test REQ-CTRL-007
//fusa:test REQ-CTRL-008
//fusa:test REQ-CTRL-009
//fusa:test REQ-CTRL-010
//fusa:test REQ-REG-001
//fusa:test REQ-REG-002
//fusa:test REQ-REG-003
//fusa:test REQ-REG-004
//fusa:test REQ-REG-005
//fusa:test REQ-REG-006
//fusa:test REQ-REG-007
//fusa:test REQ-STATUS-001
//fusa:test REQ-ZONE-001

import (
	"context"
	"testing"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/mock"
)

func TestNewRegistry_AllZonesPrePopulated(t *testing.T) {
	reg := mock.NewRegistry()
	defer reg.Close()

	zones := []rcp.Zone{
		rcp.ZoneFrontLeft,
		rcp.ZoneFrontRight,
		rcp.ZoneRearLeft,
		rcp.ZoneRearRight,
		rcp.ZoneCentral,
	}
	for _, z := range zones {
		ctrl, err := reg.Lookup(z)
		if err != nil {
			t.Errorf("zone %s not found: %v", z, err)
		}
		if ctrl.Zone() != z {
			t.Errorf("zone mismatch: got %s want %s", ctrl.Zone(), z)
		}
	}
}

func TestRegistry_Register_Duplicate(t *testing.T) {
	reg := mock.NewRegistry()
	defer reg.Close()

	extra := mock.NewController(rcp.ZoneFrontLeft, nil)
	err := reg.Register(extra)
	if err == nil {
		t.Fatal("expected ErrAlreadyExists, got nil")
	}
}

func TestRegistry_Deregister(t *testing.T) {
	reg := mock.NewRegistry()
	defer reg.Close()

	if err := reg.Deregister(rcp.ZoneCentral); err != nil {
		t.Fatalf("deregister: %v", err)
	}
	_, err := reg.Lookup(rcp.ZoneCentral)
	if err == nil {
		t.Fatal("expected ErrNotFound after deregister")
	}
}

func TestRegistry_Lookup_NotFound(t *testing.T) {
	reg := mock.NewRegistry()
	defer reg.Close()

	_ = reg.Deregister(rcp.ZoneFrontLeft)
	_, err := reg.Lookup(rcp.ZoneFrontLeft)
	if err == nil {
		t.Fatal("expected error for missing zone")
	}
}

func TestRegistry_Close_Idempotent(t *testing.T) {
	reg := mock.NewRegistry()
	if err := reg.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := reg.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

func TestController_Send_DefaultOK(t *testing.T) {
	reg := mock.NewRegistry()
	defer reg.Close()

	ctrl, _ := reg.Lookup(rcp.ZoneFrontLeft)
	cmd := &rcp.Command{ID: 42, Zone: rcp.ZoneFrontLeft, Type: rcp.CmdSet}
	resp, err := ctrl.Send(context.Background(), cmd)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if resp.Status != rcp.StatusOK {
		t.Errorf("status: got %s want OK", resp.Status)
	}
	if resp.CommandID != 42 {
		t.Errorf("command ID: got %d want 42", resp.CommandID)
	}
}

func TestController_Send_CustomHandler(t *testing.T) {
	called := false
	handler := func(cmd *rcp.Command) *rcp.Response {
		called = true
		return &rcp.Response{
			CommandID: cmd.ID,
			Zone:      cmd.Zone,
			Status:    rcp.StatusError,
			Payload:   []byte("fault"),
		}
	}
	ctrl := mock.NewController(rcp.ZoneRearLeft, handler)
	defer ctrl.Close()

	resp, err := ctrl.Send(context.Background(), &rcp.Command{ID: 1, Zone: rcp.ZoneRearLeft, Type: rcp.CmdGet})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if !called {
		t.Fatal("handler not called")
	}
	if resp.Status != rcp.StatusError {
		t.Errorf("status: got %s want error", resp.Status)
	}
}

func TestController_Send_AfterClose(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontRight, nil)
	ctrl.Close()

	_, err := ctrl.Send(context.Background(), &rcp.Command{ID: 1})
	if err == nil {
		t.Fatal("expected error after close")
	}
}

func TestController_Send_ContextCancelled(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer ctrl.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := ctrl.Send(ctx, &rcp.Command{ID: 1})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestController_Close_Idempotent(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneCentral, nil)
	if err := ctrl.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := ctrl.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

func TestController_Subscribe_ReceivesPublish(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer ctrl.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, err := ctrl.Subscribe(ctx)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	ctrl.Publish([]byte(`{"speed":42}`))

	select {
	case s := <-ch:
		if s.Zone != rcp.ZoneFrontLeft {
			t.Errorf("zone: got %s want front-left", s.Zone)
		}
		if !s.Healthy {
			t.Error("expected healthy=true")
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for status")
	}
}

func TestController_Subscribe_ClosedOnControllerClose(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneRearRight, nil)

	ctx := context.Background()
	ch, err := ctrl.Subscribe(ctx)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	ctrl.Close()

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("channel should be closed")
		}
	case <-time.After(time.Second):
		t.Fatal("channel not closed after controller close")
	}
}

func TestController_Subscribe_AfterClose(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	ctrl.Close()

	_, err := ctrl.Subscribe(context.Background())
	if err == nil {
		t.Fatal("expected error subscribing to closed controller")
	}
}

func TestController_Zone(t *testing.T) {
	for _, z := range []rcp.Zone{rcp.ZoneFrontLeft, rcp.ZoneFrontRight, rcp.ZoneRearLeft, rcp.ZoneRearRight, rcp.ZoneCentral} {
		ctrl := mock.NewController(z, nil)
		if ctrl.Zone() != z {
			t.Errorf("Zone(): got %s want %s", ctrl.Zone(), z)
		}
	}
}

func TestController_Subscribe_SeqIncrementing(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneCentral, nil)
	defer ctrl.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, _ := ctrl.Subscribe(ctx)
	ctrl.Publish(nil)
	ctrl.Publish(nil)

	s1 := <-ch
	s2 := <-ch
	if s2.Seq <= s1.Seq {
		t.Errorf("seq not incrementing: %d then %d", s1.Seq, s2.Seq)
	}
}

func TestRegistry_Controllers(t *testing.T) {
	reg := mock.NewRegistry()
	defer reg.Close()

	ctrls := reg.Controllers()
	if len(ctrls) != 5 {
		t.Errorf("Controllers(): got %d want 5", len(ctrls))
	}
}

func TestZone_String(t *testing.T) {
	cases := []struct {
		zone rcp.Zone
		want string
	}{
		{rcp.ZoneFrontLeft, "front-left"},
		{rcp.ZoneFrontRight, "front-right"},
		{rcp.ZoneRearLeft, "rear-left"},
		{rcp.ZoneRearRight, "rear-right"},
		{rcp.ZoneCentral, "central"},
		{rcp.ZoneUnknown, "unknown"},
	}
	for _, tc := range cases {
		if got := tc.zone.String(); got != tc.want {
			t.Errorf("Zone(%d).String() = %q want %q", tc.zone, got, tc.want)
		}
	}
}

func TestResponseStatus_String(t *testing.T) {
	cases := []struct {
		s    rcp.ResponseStatus
		want string
	}{
		{rcp.StatusOK, "OK"},
		{rcp.StatusError, "error"},
		{rcp.StatusTimeout, "timeout"},
		{rcp.StatusBusy, "busy"},
		{rcp.ResponseStatus(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.s.String(); got != tc.want {
			t.Errorf("ResponseStatus(%d).String() = %q want %q", tc.s, got, tc.want)
		}
	}
}

func TestRegistry_Register_AfterClose(t *testing.T) {
	reg := mock.NewRegistry()
	reg.Close()

	ctrl := mock.NewController(rcp.ZoneUnknown, nil)
	err := reg.Register(ctrl)
	if err == nil {
		t.Fatal("expected error registering to closed registry")
	}
}

func FuzzController_Send(f *testing.F) {
	f.Add(uint32(1), uint8(1), uint16(1), uint8(0), []byte(`{"k":"v"}`))
	f.Fuzz(func(t *testing.T, id uint32, zone uint8, cmdType uint16, priority uint8, payload []byte) {
		ctrl := mock.NewController(rcp.Zone(zone), nil)
		defer ctrl.Close()
		cmd := &rcp.Command{
			ID:       id,
			Zone:     rcp.Zone(zone),
			Type:     rcp.CommandType(cmdType),
			Priority: rcp.Priority(priority),
			Payload:  payload,
		}
		resp, err := ctrl.Send(context.Background(), cmd)
		if err != nil {
			return
		}
		if resp == nil {
			t.Fatal("nil response without error")
		}
	})
}
