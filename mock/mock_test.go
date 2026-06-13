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
//fusa:test REQ-CTRL-011
//fusa:test REQ-CTRL-012
//fusa:test REQ-CTRL-013
//fusa:test REQ-CTRL-014
//fusa:test REQ-CTRL-015
//fusa:test REQ-CTRL-016
//fusa:test REQ-CTRL-017
//fusa:test REQ-CTRL-018
//fusa:test REQ-CTRL-019
//fusa:test REQ-CTRL-020
//fusa:test REQ-CTRL-021
//fusa:test REQ-CTRL-022
//fusa:test REQ-CTRL-023
//fusa:test REQ-CTRL-024
//fusa:test REQ-REG-001
//fusa:test REQ-REG-002
//fusa:test REQ-REG-003
//fusa:test REQ-REG-004
//fusa:test REQ-REG-005
//fusa:test REQ-REG-006
//fusa:test REQ-REG-007
//fusa:test REQ-REG-008
//fusa:test REQ-REG-009
//fusa:test REQ-REG-010
//fusa:test REQ-REG-011
//fusa:test REQ-REG-012
//fusa:test REQ-RESP-001
//fusa:test REQ-RESP-002
//fusa:test REQ-STAT-001
//fusa:test REQ-STAT-002
//fusa:test REQ-STAT-003
//fusa:test REQ-STAT-004

import (
	"bytes"
	"context"
	"errors"
	"sync"
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
	_ = ctrl.Close()

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

	_ = ctrl.Close()

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
	_ = ctrl.Close()

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
	_ = reg.Close()

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

// ── REQ-RESP-001: Response.CommandID echoes Command.ID ────────────────────────

func TestController_Response_EchoesCommandID(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer ctrl.Close()

	resp, err := ctrl.Send(context.Background(), &rcp.Command{ID: 0xDEAD_BEEF, Zone: rcp.ZoneFrontLeft})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if resp.CommandID != 0xDEAD_BEEF {
		t.Errorf("Response.CommandID = %d, want 0xDEADBEEF", resp.CommandID)
	}
}

// ── REQ-RESP-002: Response.Zone echoes the controller zone ───────────────────

func TestController_Response_EchoesZone(t *testing.T) {
	for _, z := range []rcp.Zone{rcp.ZoneFrontLeft, rcp.ZoneRearRight, rcp.ZoneCentral} {
		ctrl := mock.NewController(z, nil)
		resp, err := ctrl.Send(context.Background(), &rcp.Command{ID: 1, Zone: z})
		if err != nil {
			_ = ctrl.Close()
			t.Fatalf("zone %s send: %v", z, err)
		}
		if resp.Zone != z {
			t.Errorf("zone %s: Response.Zone = %s, want %s", z, resp.Zone, z)
		}
		_ = ctrl.Close()
	}
}

// ── REQ-CTRL-011: Subscribe channel closed on context cancel ─────────────────

func TestController_Subscribe_ClosedOnContextCancel(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer ctrl.Close()

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := ctrl.Subscribe(ctx)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	cancel()

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("channel should be closed after context cancel")
		}
	case <-time.After(time.Second):
		t.Fatal("channel not closed after context cancel")
	}
}

// ── REQ-CTRL-011: Cancelling one subscriber leaves others intact ─────────────

func TestController_Subscribe_CancelOnePreservesOthers(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer ctrl.Close()

	ctx1, cancel1 := context.WithCancel(context.Background())
	ctx2 := context.Background()

	ch1, _ := ctrl.Subscribe(ctx1)
	ch2, err := ctrl.Subscribe(ctx2)
	if err != nil {
		t.Fatalf("subscribe ch2: %v", err)
	}

	cancel1()
	time.Sleep(50 * time.Millisecond) // allow goroutine to remove ch1

	ctrl.Publish([]byte("ping"))

	select {
	case s, ok := <-ch2:
		if !ok {
			t.Fatal("ch2 unexpectedly closed")
		}
		if string(s.Payload) != "ping" {
			t.Errorf("ch2 payload = %q, want %q", s.Payload, "ping")
		}
	case <-time.After(time.Second):
		t.Fatal("ch2 did not receive publish after ch1 cancelled")
	}
	_ = ch1
}

// ── REQ-CTRL-012: Multiple subscribers each receive published Status ──────────

func TestController_MultipleSubscribers_EachReceive(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneCentral, nil)
	defer ctrl.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	const n = 4
	channels := make([]<-chan *rcp.Status, n)
	for i := range channels {
		ch, err := ctrl.Subscribe(ctx)
		if err != nil {
			t.Fatalf("subscribe %d: %v", i, err)
		}
		channels[i] = ch
	}

	ctrl.Publish([]byte("broadcast"))

	for i, ch := range channels {
		select {
		case s := <-ch:
			if string(s.Payload) != "broadcast" {
				t.Errorf("subscriber %d: payload = %q, want broadcast", i, s.Payload)
			}
		case <-ctx.Done():
			t.Fatalf("subscriber %d timed out", i)
		}
	}
}

// ── REQ-CTRL-013: CmdNoop accepted without error ─────────────────────────────

func TestController_Send_CmdNoop(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer ctrl.Close()

	resp, err := ctrl.Send(context.Background(), &rcp.Command{ID: 1, Type: rcp.CmdNoop})
	if err != nil {
		t.Fatalf("CmdNoop send: %v", err)
	}
	if resp.Status != rcp.StatusOK {
		t.Errorf("CmdNoop status = %s, want OK", resp.Status)
	}
}

// ── REQ-CTRL-014: CmdWatchdog accepted without error ─────────────────────────

func TestController_Send_CmdWatchdog(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer ctrl.Close()

	resp, err := ctrl.Send(context.Background(), &rcp.Command{
		ID:       1,
		Type:     rcp.CmdWatchdog,
		Priority: rcp.PriorityCritical,
	})
	if err != nil {
		t.Fatalf("CmdWatchdog send: %v", err)
	}
	if resp.Status != rcp.StatusOK {
		t.Errorf("CmdWatchdog status = %s, want OK", resp.Status)
	}
}

// ── REQ-CTRL-015: CmdReset accepted without error ────────────────────────────

func TestController_Send_CmdReset(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer ctrl.Close()

	resp, err := ctrl.Send(context.Background(), &rcp.Command{ID: 1, Type: rcp.CmdReset})
	if err != nil {
		t.Fatalf("CmdReset send: %v", err)
	}
	if resp.Status != rcp.StatusOK {
		t.Errorf("CmdReset status = %s, want OK", resp.Status)
	}
}

// ── REQ-CTRL-016: Handler Response returned verbatim ─────────────────────────

func TestController_Handler_ResponseReturnedVerbatim(t *testing.T) {
	want := &rcp.Response{
		CommandID: 77,
		Zone:      rcp.ZoneRearLeft,
		Status:    rcp.StatusBusy,
		Payload:   []byte("verbatim"),
	}
	ctrl := mock.NewController(rcp.ZoneRearLeft, func(_ *rcp.Command) *rcp.Response { return want })
	defer ctrl.Close()

	got, err := ctrl.Send(context.Background(), &rcp.Command{ID: 77})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if got != want {
		t.Errorf("Response pointer differs: got %p, want %p", got, want)
	}
}

// ── REQ-CTRL-017: Publish on closed controller does not panic ────────────────

func TestController_Publish_AfterClose_NoPanic(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontRight, nil)
	_ = ctrl.Close()
	// Must not panic.
	ctrl.Publish([]byte("should be dropped"))
}

// ── REQ-CTRL-018: Concurrent Send is data-race free ─────────────────────────

func TestController_Send_Concurrent(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneCentral, nil)
	defer ctrl.Close()

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(id uint32) {
			defer wg.Done()
			_, _ = ctrl.Send(context.Background(), &rcp.Command{ID: id})
		}(uint32(i))
	}
	wg.Wait()
}

// ── REQ-CTRL-019: Concurrent Publish and Subscribe are data-race free ────────

func TestController_Publish_Subscribe_Concurrent(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneCentral, nil)
	defer ctrl.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch, err := ctrl.Subscribe(ctx)
			if err != nil {
				return
			}
			for range ch {
			}
		}()
	}
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctrl.Publish([]byte("x"))
		}()
	}
	wg.Wait()
}

// ── REQ-CTRL-020: Subscribe Status carries correct Zone ──────────────────────

func TestController_Subscribe_StatusZone(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneRearLeft, nil)
	defer ctrl.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, _ := ctrl.Subscribe(ctx)
	ctrl.Publish(nil)

	select {
	case s := <-ch:
		if s.Zone != rcp.ZoneRearLeft {
			t.Errorf("Status.Zone = %s, want rear-left", s.Zone)
		}
	case <-ctx.Done():
		t.Fatal("timeout")
	}
}

// ── REQ-CTRL-021: Subscribe Status carries correct Payload ───────────────────

func TestController_Subscribe_StatusPayload(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer ctrl.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, _ := ctrl.Subscribe(ctx)
	want := []byte(`{"sensor":"temp","value":22.5}`)
	ctrl.Publish(want)

	select {
	case s := <-ch:
		if !bytes.Equal(s.Payload, want) {
			t.Errorf("Status.Payload = %q, want %q", s.Payload, want)
		}
	case <-ctx.Done():
		t.Fatal("timeout")
	}
}

// ── REQ-CTRL-022: Subscribe Status Healthy=true while open ───────────────────

func TestController_Subscribe_StatusHealthyWhileOpen(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer ctrl.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, _ := ctrl.Subscribe(ctx)
	ctrl.Publish(nil)

	select {
	case s := <-ch:
		if !s.Healthy {
			t.Error("Status.Healthy = false, want true for open controller")
		}
	case <-ctx.Done():
		t.Fatal("timeout")
	}
}

// ── REQ-CTRL-023: Cancelled context does not invoke handler ─────────────────

func TestController_Send_CancelledContext_HandlerNotInvoked(t *testing.T) {
	invoked := false
	ctrl := mock.NewController(rcp.ZoneFrontLeft, func(_ *rcp.Command) *rcp.Response {
		invoked = true
		return &rcp.Response{Status: rcp.StatusOK}
	})
	defer ctrl.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := ctrl.Send(ctx, &rcp.Command{ID: 1})
	if err == nil {
		t.Fatal("expected error for pre-cancelled context")
	}
	if invoked {
		t.Error("handler was invoked despite cancelled context")
	}
}

// ── REQ-CTRL-024: Send with nil Payload does not panic ───────────────────────

func TestController_Send_NilPayload_NoPanic(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer ctrl.Close()

	resp, err := ctrl.Send(context.Background(), &rcp.Command{ID: 1, Payload: nil})
	if err != nil {
		t.Fatalf("send nil payload: %v", err)
	}
	if resp == nil {
		t.Fatal("nil response")
	}
}

// ── REQ-STAT-001, REQ-STAT-002 already covered by TestController_Subscribe_SeqIncrementing
// and TestController_Subscribe_StatusZone above.

// ── REQ-STAT-003: Status.Healthy false is not observable on open controller ──
// (Healthy=true is tested by REQ-CTRL-022; Healthy=false after close would
// require a pre-subscribed channel draining after Close, which is covered by
// TestController_Subscribe_ClosedOnControllerClose — channel is closed, no
// additional Status is delivered.)

// ── REQ-STAT-004: Publish nil Payload produces nil Status.Payload ────────────

func TestController_Publish_NilPayload(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneCentral, nil)
	defer ctrl.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, _ := ctrl.Subscribe(ctx)
	ctrl.Publish(nil)

	select {
	case s := <-ch:
		if s.Payload != nil {
			t.Errorf("Status.Payload = %v, want nil", s.Payload)
		}
	case <-ctx.Done():
		t.Fatal("timeout")
	}
}

// ── REQ-REG-008: Deregister ErrNotFound for never-registered zone ────────────

func TestRegistry_Deregister_NeverRegistered(t *testing.T) {
	reg := mock.NewRegistry()
	defer reg.Close()

	_ = reg.Deregister(rcp.ZoneFrontLeft) // pre-populated, remove it
	err := reg.Deregister(rcp.ZoneFrontLeft)
	if !errors.Is(err, rcp.ErrNotFound) {
		t.Errorf("second Deregister: got %v, want ErrNotFound", err)
	}
}

// ── REQ-REG-009: Registered controller retrievable via Lookup ────────────────

func TestRegistry_Register_ThenLookup(t *testing.T) {
	reg := mock.NewRegistry()
	defer reg.Close()

	_ = reg.Deregister(rcp.ZoneFrontLeft)
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	if err := reg.Register(ctrl); err != nil {
		t.Fatalf("register: %v", err)
	}
	got, err := reg.Lookup(rcp.ZoneFrontLeft)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if got.Zone() != rcp.ZoneFrontLeft {
		t.Errorf("lookup zone = %s, want front-left", got.Zone())
	}
}

// ── REQ-REG-010: Registry Close calls Close on all registered controllers ────

func TestRegistry_Close_ClosesAllControllers(t *testing.T) {
	reg := mock.NewRegistry()

	// Subscribe to one controller to observe its close.
	ctrl, err := reg.Lookup(rcp.ZoneFrontLeft)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	ch, err := ctrl.Subscribe(context.Background())
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	_ = reg.Close()

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("subscriber channel still open after registry close")
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber channel not closed after registry close")
	}
}

// ── REQ-REG-011: Lookup on closed Registry returns error ─────────────────────

func TestRegistry_Lookup_AfterClose(t *testing.T) {
	reg := mock.NewRegistry()
	_ = reg.Close()

	_, err := reg.Lookup(rcp.ZoneFrontLeft)
	if err == nil {
		t.Fatal("expected error from Lookup on closed registry")
	}
}

// ── REQ-REG-012: Deregister twice returns ErrNotFound ────────────────────────

func TestRegistry_Deregister_Twice_ErrNotFound(t *testing.T) {
	reg := mock.NewRegistry()
	defer reg.Close()

	if err := reg.Deregister(rcp.ZoneCentral); err != nil {
		t.Fatalf("first deregister: %v", err)
	}
	err := reg.Deregister(rcp.ZoneCentral)
	if !errors.Is(err, rcp.ErrNotFound) {
		t.Errorf("second deregister: got %v, want ErrNotFound", err)
	}
}

// ── REQ-ERR-007, REQ-ERR-008, REQ-ERR-009, REQ-ERR-010 (via mock returns) ──

func TestMock_ErrorWrapping_ErrClosed(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	_ = ctrl.Close()

	_, err := ctrl.Send(context.Background(), &rcp.Command{})
	if !errors.Is(err, rcp.ErrClosed) {
		t.Errorf("Send after close: errors.Is(err, ErrClosed) = false, got %v", err)
	}
}

func TestMock_ErrorWrapping_ErrNotFound(t *testing.T) {
	reg := mock.NewRegistry()
	defer reg.Close()

	_ = reg.Deregister(rcp.ZoneFrontLeft)
	_, err := reg.Lookup(rcp.ZoneFrontLeft)
	if !errors.Is(err, rcp.ErrNotFound) {
		t.Errorf("Lookup missing zone: errors.Is(err, ErrNotFound) = false, got %v", err)
	}
}

func TestMock_ErrorWrapping_ErrAlreadyExists(t *testing.T) {
	reg := mock.NewRegistry()
	defer reg.Close()

	extra := mock.NewController(rcp.ZoneFrontLeft, nil)
	err := reg.Register(extra)
	if !errors.Is(err, rcp.ErrAlreadyExists) {
		t.Errorf("duplicate register: errors.Is(err, ErrAlreadyExists) = false, got %v", err)
	}
}

func TestMock_ErrorWrapping_ErrTimeout(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer ctrl.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ctrl.Send(ctx, &rcp.Command{})
	if !errors.Is(err, rcp.ErrTimeout) {
		t.Errorf("cancelled context Send: errors.Is(err, ErrTimeout) = false, got %v", err)
	}
}
