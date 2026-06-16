//fusa:test REQ-CAN-001
//fusa:test REQ-CAN-002
//fusa:test REQ-CAN-003
//fusa:test REQ-CAN-004
//fusa:test REQ-CAN-005
//fusa:test REQ-CAN-006
//fusa:test REQ-CAN-007
//fusa:test REQ-CAN-008

package canbr_test

import (
	"context"
	"errors"
	"testing"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/canbr"
	"github.com/SoundMatt/go-RCP/mock"
)

// REQ-CAN-001: Frame.Encode produces wire bytes from a CAN frame.
// REQ-CAN-002: Decode parses wire bytes back into the original Frame.
func TestFrameRoundTrip(t *testing.T) {
	want := canbr.Frame{
		ID:   0x1A2,
		DLC:  4,
		Data: [8]byte{0x01, 0x02, 0x03, 0x04},
	}
	b := want.Encode()
	if len(b) != 13 {
		t.Fatalf("Encode len = %d, want 13", len(b))
	}
	got, err := canbr.Decode(b)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("ID = %X, want %X", got.ID, want.ID)
	}
	if got.DLC != want.DLC {
		t.Errorf("DLC = %d, want %d", got.DLC, want.DLC)
	}
	if got.Data != want.Data {
		t.Errorf("Data mismatch")
	}
}

func TestDecode_ShortFrame(t *testing.T) {
	_, err := canbr.Decode([]byte{0x00, 0x01})
	if !errors.Is(err, canbr.ErrMalformedFrame) {
		t.Errorf("want ErrMalformedFrame, got %v", err)
	}
}

// REQ-CAN-003: Bus.Send delivers a frame to all subscribers.
func TestBus_Broadcast(t *testing.T) {
	bus := canbr.NewBus()
	ch1 := bus.Subscribe()
	ch2 := bus.Subscribe()
	defer bus.Unsubscribe(ch1)
	defer bus.Unsubscribe(ch2)

	f := canbr.Frame{ID: 0x100, DLC: 1, Data: [8]byte{0xFF}}
	bus.Send(f)

	for _, ch := range []chan canbr.Frame{ch1, ch2} {
		select {
		case got := <-ch:
			if got.ID != 0x100 {
				t.Errorf("got ID %X, want 0x100", got.ID)
			}
		case <-time.After(time.Second):
			t.Error("timeout waiting for frame")
		}
	}
}

// REQ-CAN-004: Server dispatches incoming CAN frames to rcp.Controller.
func TestServer_Dispatch(t *testing.T) {
	dispatched := make(chan rcp.CommandType, 1)
	inner := mock.NewController(rcp.ZoneFrontLeft, func(cmd *rcp.Command) *rcp.Response {
		dispatched <- cmd.Type
		return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusOK}
	})
	defer inner.Close()

	bus := canbr.NewBus()
	srv := canbr.NewServer(inner, bus)
	defer srv.Close()

	c := canbr.NewController(rcp.ZoneFrontLeft, bus)
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdGet}
	if _, err := c.Send(ctx, cmd); err != nil {
		t.Fatalf("Send: %v", err)
	}
	select {
	case got := <-dispatched:
		if got != rcp.CmdGet {
			t.Errorf("dispatched type = %v, want CmdGet", got)
		}
	default:
		t.Error("dispatch not called")
	}
}

// REQ-CAN-005: Server sends a CAN RESPONSE frame after inner controller responds.
func TestServer_Response(t *testing.T) {
	inner := mock.NewController(rcp.ZoneFrontLeft, func(cmd *rcp.Command) *rcp.Response {
		return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusOK}
	})
	defer inner.Close()

	bus := canbr.NewBus()
	srv := canbr.NewServer(inner, bus)
	defer srv.Close()

	c := canbr.NewController(rcp.ZoneFrontLeft, bus)
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := c.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdNoop})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Status != rcp.StatusOK {
		t.Errorf("status = %v, want StatusOK", resp.Status)
	}
}

// REQ-CAN-006: Controller.Send encodes and decodes a CAN request/response.
func TestController_Send(t *testing.T) {
	inner := mock.NewController(rcp.ZoneFrontLeft, func(cmd *rcp.Command) *rcp.Response {
		return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusOK}
	})
	defer inner.Close()

	bus := canbr.NewBus()
	srv := canbr.NewServer(inner, bus)
	defer srv.Close()

	c := canbr.NewController(rcp.ZoneFrontLeft, bus)
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdSet, Payload: []byte{0xAB, 0xCD}}
	resp, err := c.Send(ctx, cmd)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Status != rcp.StatusOK {
		t.Errorf("status = %v, want StatusOK", resp.Status)
	}
}

// REQ-CAN-007: Controller.Close is idempotent.
func TestController_CloseIdempotent(t *testing.T) {
	bus := canbr.NewBus()
	c := canbr.NewController(rcp.ZoneFrontLeft, bus)
	if err := c.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// REQ-CAN-008: Controller.Send returns ErrClosed after Close.
func TestController_Send_AfterClose(t *testing.T) {
	bus := canbr.NewBus()
	c := canbr.NewController(rcp.ZoneFrontLeft, bus)
	_ = c.Close()

	_, err := c.Send(context.Background(), &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdSet})
	if !errors.Is(err, rcp.ErrClosed) {
		t.Errorf("want ErrClosed, got %v", err)
	}
}
