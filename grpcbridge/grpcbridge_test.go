//fusa:test REQ-GRPC-001
//fusa:test REQ-GRPC-002
//fusa:test REQ-GRPC-003
//fusa:test REQ-GRPC-004
//fusa:test REQ-GRPC-005
//fusa:test REQ-GRPC-006
//fusa:test REQ-GRPC-007
//fusa:test REQ-GRPC-008

package grpcbridge_test

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/grpcbridge"
	"github.com/SoundMatt/go-RCP/mock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// startServer spins up a gRPC server backed by a mock controller and returns
// the listener address for client connections.
func startServer(t *testing.T, zone rcp.Zone, handler func(*rcp.Command) *rcp.Response) (addr string, cleanup func()) {
	t.Helper()
	inner := mock.NewController(zone, handler)
	gs := grpc.NewServer()
	grpcbridge.RegisterServer(gs, grpcbridge.NewServer(inner))
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go func() { _ = gs.Serve(ln) }()
	return ln.Addr().String(), func() {
		gs.Stop()
		_ = inner.Close()
	}
}

// REQ-GRPC-001: Server accepts a gRPC connection and delegates to rcp.Controller.
func TestServer_AcceptsConnection(t *testing.T) {
	addr, cleanup := startServer(t, rcp.ZoneFrontLeft, nil)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	c, err := grpcbridge.NewController(ctx, rcp.ZoneFrontLeft, addr)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	defer func() { _ = c.Close() }()
}

// REQ-GRPC-002: Controller.Send transmits a command and returns a response.
func TestController_Send(t *testing.T) {
	addr, cleanup := startServer(t, rcp.ZoneFrontLeft, func(cmd *rcp.Command) *rcp.Response {
		return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusOK}
	})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, err := grpcbridge.NewController(ctx, rcp.ZoneFrontLeft, addr)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	defer func() { _ = c.Close() }()

	cmd := &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdSet, Priority: rcp.PriorityNormal}
	resp, err := c.Send(ctx, cmd)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Status != rcp.StatusOK {
		t.Errorf("status = %v, want StatusOK", resp.Status)
	}
}

// REQ-GRPC-003: Controller.Send returns ErrZoneMismatch for wrong zone.
func TestController_Send_ZoneMismatch(t *testing.T) {
	addr, cleanup := startServer(t, rcp.ZoneFrontLeft, nil)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	c, err := grpcbridge.NewController(ctx, rcp.ZoneFrontLeft, addr)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	defer func() { _ = c.Close() }()

	cmd := &rcp.Command{Zone: rcp.ZoneRearLeft, Type: rcp.CmdSet}
	_, err = c.Send(ctx, cmd)
	if !errors.Is(err, rcp.ErrZoneMismatch) {
		t.Errorf("want ErrZoneMismatch, got %v", err)
	}
}

// REQ-GRPC-004: Server.Send propagates payload to inner controller.
func TestServer_Send_PayloadRoundTrip(t *testing.T) {
	want := []byte("test-payload")
	var got []byte
	addr, cleanup := startServer(t, rcp.ZoneFrontLeft, func(cmd *rcp.Command) *rcp.Response {
		got = cmd.Payload
		return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusOK, Payload: cmd.Payload}
	})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, err := grpcbridge.NewController(ctx, rcp.ZoneFrontLeft, addr)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	defer func() { _ = c.Close() }()

	cmd := &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdSet, Payload: want}
	resp, err := c.Send(ctx, cmd)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("server got payload %q, want %q", got, want)
	}
	if string(resp.Payload) != string(want) {
		t.Errorf("response payload %q, want %q", resp.Payload, want)
	}
}

// REQ-GRPC-005: Controller.Subscribe opens a streaming subscription.
func TestController_Subscribe(t *testing.T) {
	inner := mock.NewController(rcp.ZoneFrontLeft, nil)
	gs := grpc.NewServer()
	grpcbridge.RegisterServer(gs, grpcbridge.NewServer(inner))
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go func() { _ = gs.Serve(ln) }()
	defer gs.Stop()
	defer func() { _ = inner.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, err := grpcbridge.NewController(ctx, rcp.ZoneFrontLeft, ln.Addr().String())
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	defer func() { _ = c.Close() }()

	ch, err := c.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// Publish repeatedly until the server-side subscription is established
	// and the event arrives over the gRPC stream.
	go func() {
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			inner.Publish([]byte("hello"))
		}
	}()

	select {
	case st := <-ch:
		if st.Zone != rcp.ZoneFrontLeft {
			t.Errorf("zone = %v, want ZoneFrontLeft", st.Zone)
		}
	case <-time.After(3 * time.Second):
		t.Error("timeout waiting for status event")
	}
}

// REQ-GRPC-006: Controller.Close is idempotent.
func TestController_CloseIdempotent(t *testing.T) {
	addr, cleanup := startServer(t, rcp.ZoneFrontLeft, nil)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	c, err := grpcbridge.NewController(ctx, rcp.ZoneFrontLeft, addr)
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

// REQ-GRPC-007: Controller.Send returns ErrClosed after Close.
func TestController_Send_AfterClose(t *testing.T) {
	addr, cleanup := startServer(t, rcp.ZoneFrontLeft, nil)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	c, err := grpcbridge.NewController(ctx, rcp.ZoneFrontLeft, addr)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	_ = c.Close()

	cmd := &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdSet}
	_, err = c.Send(ctx, cmd)
	if !errors.Is(err, rcp.ErrClosed) {
		t.Errorf("want ErrClosed, got %v", err)
	}
}

// REQ-GRPC-008: Server.Send returns inner controller error to gRPC client.
func TestServer_Send_InnerError(t *testing.T) {
	addr, cleanup := startServer(t, rcp.ZoneFrontLeft, func(cmd *rcp.Command) *rcp.Response {
		return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusError}
	})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use a direct grpc client to call Send and inspect the response status.
	cc, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = cc.Close() }()

	c, err := grpcbridge.NewController(ctx, rcp.ZoneFrontLeft, addr)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	defer func() { _ = c.Close() }()

	cmd := &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdSet}
	resp, err := c.Send(ctx, cmd)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Status != rcp.StatusError {
		t.Errorf("status = %v, want StatusError", resp.Status)
	}
}
