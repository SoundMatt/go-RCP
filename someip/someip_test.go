//fusa:test REQ-SIPC-001
//fusa:test REQ-SIPC-002
//fusa:test REQ-SIPC-003
//fusa:test REQ-SIPC-004
//fusa:test REQ-SIPC-005
//fusa:test REQ-SIPC-006
//fusa:test REQ-SIPC-007
//fusa:test REQ-SIPC-008

package someip_test

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/mock"
	"github.com/SoundMatt/go-RCP/someip"
)

func loopbackAddr(t *testing.T) *net.UDPAddr {
	t.Helper()
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return addr
}

// REQ-SIPC-001: encodeFrame produces a valid 16-byte SOME/IP header.
// REQ-SIPC-002: decodeFrame parses the header back correctly.
func TestHeaderRoundTrip(t *testing.T) {
	// We test encode/decode indirectly through Send/Server: if the server can
	// parse what the controller sent, both encode and decode are correct.
	inner := mock.NewController(rcp.ZoneFrontLeft, func(cmd *rcp.Command) *rcp.Response {
		return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusOK}
	})
	defer inner.Close()

	srv, err := someip.NewServer(inner, loopbackAddr(t), someip.DefaultServiceID)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.Close()

	c, err := someip.NewController(rcp.ZoneFrontLeft, srv.Addr(), someip.DefaultServiceID)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdSet, Priority: rcp.PriorityNormal}
	resp, err := c.Send(ctx, cmd)
	if err != nil {
		t.Fatalf("Send (header round-trip): %v", err)
	}
	if resp.Status != rcp.StatusOK {
		t.Errorf("status = %v, want StatusOK", resp.Status)
	}
}

// REQ-SIPC-003: Server accepts UDP requests and delegates to rcp.Controller.
func TestServer_Dispatch(t *testing.T) {
	dispatched := make(chan rcp.CommandType, 1)
	inner := mock.NewController(rcp.ZoneFrontLeft, func(cmd *rcp.Command) *rcp.Response {
		dispatched <- cmd.Type
		return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusOK}
	})
	defer inner.Close()

	srv, err := someip.NewServer(inner, loopbackAddr(t), someip.DefaultServiceID)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.Close()

	c, err := someip.NewController(rcp.ZoneFrontLeft, srv.Addr(), someip.DefaultServiceID)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	defer c.Close()

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
		t.Error("controller was not dispatched")
	}
}

// REQ-SIPC-004: Server replies with a SOME/IP RESPONSE datagram.
func TestServer_Response(t *testing.T) {
	inner := mock.NewController(rcp.ZoneFrontLeft, func(cmd *rcp.Command) *rcp.Response {
		return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusOK}
	})
	defer inner.Close()

	srv, err := someip.NewServer(inner, loopbackAddr(t), someip.DefaultServiceID)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.Close()

	c, err := someip.NewController(rcp.ZoneFrontLeft, srv.Addr(), someip.DefaultServiceID)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	defer c.Close()

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

// REQ-SIPC-005: Controller.Send encodes the command and decodes the SOME/IP response.
func TestController_Send(t *testing.T) {
	inner := mock.NewController(rcp.ZoneFrontLeft, func(cmd *rcp.Command) *rcp.Response {
		return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusOK}
	})
	defer inner.Close()

	srv, err := someip.NewServer(inner, loopbackAddr(t), someip.DefaultServiceID)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.Close()

	c, err := someip.NewController(rcp.ZoneFrontLeft, srv.Addr(), someip.DefaultServiceID)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, cmdType := range []rcp.CommandType{rcp.CmdSet, rcp.CmdGet, rcp.CmdReset} {
		cmd := &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: cmdType}
		resp, err := c.Send(ctx, cmd)
		if err != nil {
			t.Fatalf("Send(%v): %v", cmdType, err)
		}
		if resp.Status != rcp.StatusOK {
			t.Errorf("Send(%v) status = %v, want StatusOK", cmdType, resp.Status)
		}
	}
}

// REQ-SIPC-006: Controller.Send returns ErrZoneMismatch for wrong zone.
func TestController_Send_ZoneMismatch(t *testing.T) {
	inner := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer inner.Close()

	srv, err := someip.NewServer(inner, loopbackAddr(t), someip.DefaultServiceID)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.Close()

	c, err := someip.NewController(rcp.ZoneFrontLeft, srv.Addr(), someip.DefaultServiceID)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	defer c.Close()

	cmd := &rcp.Command{Zone: rcp.ZoneRearLeft, Type: rcp.CmdSet}
	_, err = c.Send(context.Background(), cmd)
	if !errors.Is(err, rcp.ErrZoneMismatch) {
		t.Errorf("want ErrZoneMismatch, got %v", err)
	}
}

// REQ-SIPC-007: Controller.Close is idempotent.
func TestController_CloseIdempotent(t *testing.T) {
	inner := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer inner.Close()

	srv, err := someip.NewServer(inner, loopbackAddr(t), someip.DefaultServiceID)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.Close()

	c, err := someip.NewController(rcp.ZoneFrontLeft, srv.Addr(), someip.DefaultServiceID)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// REQ-SIPC-008: Controller.Send returns ErrClosed after Close.
func TestController_Send_AfterClose(t *testing.T) {
	inner := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer inner.Close()

	srv, err := someip.NewServer(inner, loopbackAddr(t), someip.DefaultServiceID)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.Close()

	c, err := someip.NewController(rcp.ZoneFrontLeft, srv.Addr(), someip.DefaultServiceID)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	_ = c.Close()

	cmd := &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdSet}
	_, err = c.Send(context.Background(), cmd)
	if !errors.Is(err, rcp.ErrClosed) {
		t.Errorf("want ErrClosed, got %v", err)
	}
}
