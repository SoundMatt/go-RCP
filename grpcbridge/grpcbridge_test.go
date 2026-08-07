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

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/grpcbridge"
	"github.com/SoundMatt/go-RCP/v9/server"
	"github.com/SoundMatt/go-RCP/v9/udp"
	"google.golang.org/grpc"
)

func clientStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55}, 1)
}

func serverStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE}, 1)
}

const testAddr = avtp.ByteBusID(1)

type echoHandler struct{}

func (echoHandler) HandleRequest(_ avtp.StreamID, req acf.Message) (acf.Message, error) {
	return acf.Message{
		Kind:           req.Kind,
		ByteBusID:      req.ByteBusID,
		TransactionNum: req.TransactionNum,
		Control:        acf.FlagResponse | (req.Control & (acf.FlagRead | acf.FlagWrite)),
		Body:           req.Body,
	}, nil
}

// startServer wires a grpcbridge.Server end-to-end against a real upstream
// *udp.Controller reaching a udp.Server with echoHandler registered, and
// returns the gRPC listener address plus a *grpcbridge.Server the test can
// call PublishTelemetry on.
func startServer(t *testing.T) (addr string, bridgeSrv *grpcbridge.Server, cleanup func()) {
	t.Helper()
	router := udp.NewRouter(udp.NewEP0Handler(server.NewServer()), false)
	if err := router.Register(testAddr, echoHandler{}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	rcpSrv, err := udp.NewServer(serverStream(), "127.0.0.1:0", router)
	if err != nil {
		t.Fatalf("udp.NewServer: %v", err)
	}
	upstream, err := udp.NewController(clientStream(), rcpSrv.Addr())
	if err != nil {
		t.Fatalf("udp.NewController: %v", err)
	}

	bridgeSrv = grpcbridge.NewServer(upstream)
	gs := grpc.NewServer()
	grpcbridge.RegisterServer(gs, bridgeSrv)
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go func() { _ = gs.Serve(ln) }()

	return ln.Addr().String(), bridgeSrv, func() {
		gs.Stop()
		_ = upstream.Close()
		_ = rcpSrv.Close()
	}
}

// TestController_Connects Server accepts a gRPC connection from a
// Controller (REQ-GRPC-001).
func TestController_Connects(t *testing.T) {
	addr, _, cleanup := startServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	c, err := grpcbridge.NewController(ctx, addr)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	defer func() { _ = c.Close() }()
}

// TestController_Request_RoundTrip Controller.Request forwards through the
// gRPC bridge to the real upstream endpoint and returns its echoed response
// (REQ-GRPC-002).
func TestController_Request_RoundTrip(t *testing.T) {
	addr, _, cleanup := startServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, err := grpcbridge.NewController(ctx, addr)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	defer func() { _ = c.Close() }()

	resp, err := c.Write(ctx, testAddr, []byte("hello"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if string(resp.Body) != "hello" {
		t.Errorf("Body = %q, want %q", resp.Body, "hello")
	}
}

// TestController_Read_UnregisteredEndpoint an unregistered endpoint's
// wire-level error response is forwarded through the bridge as FlagError,
// not a transport-level error (REQ-GRPC-003).
func TestController_Read_UnregisteredEndpoint(t *testing.T) {
	addr, _, cleanup := startServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	c, err := grpcbridge.NewController(ctx, addr)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	defer func() { _ = c.Close() }()

	resp, err := c.Read(ctx, testAddr+1) // unregistered
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !resp.Control.Has(acf.FlagError) {
		t.Error("Control missing FlagError for an unregistered endpoint")
	}
}

// TestServer_Request_PayloadRoundTrip Server.Request forwards the request
// payload to the upstream controller and returns its response payload
// (REQ-GRPC-004).
func TestServer_Request_PayloadRoundTrip(t *testing.T) {
	addr, _, cleanup := startServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, err := grpcbridge.NewController(ctx, addr)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	defer func() { _ = c.Close() }()

	want := []byte("test-payload")
	resp, err := c.Write(ctx, testAddr, want)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if string(resp.Body) != string(want) {
		t.Errorf("response payload %q, want %q", resp.Body, want)
	}
}

// TestController_Subscribe Server.PublishTelemetry reaches a Controller's
// Subscribe stream (REQ-GRPC-005).
func TestController_Subscribe(t *testing.T) {
	addr, bridgeSrv, cleanup := startServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, err := grpcbridge.NewController(ctx, addr)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	defer func() { _ = c.Close() }()

	ch, err := c.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	go func() {
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			bridgeSrv.PublishTelemetry(&grpcbridge.TelemetryEvent{ByteBusID: testAddr, Body: []byte("hi")})
		}
	}()

	select {
	case ev := <-ch:
		if ev.ByteBusID != testAddr {
			t.Errorf("ByteBusID = %v, want %v", ev.ByteBusID, testAddr)
		}
	case <-time.After(3 * time.Second):
		t.Error("timeout waiting for telemetry event")
	}
}

// TestController_CloseIdempotent Close is safe to call twice (REQ-GRPC-006).
func TestController_CloseIdempotent(t *testing.T) {
	addr, _, cleanup := startServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	c, err := grpcbridge.NewController(ctx, addr)
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

// TestController_Request_AfterClose Request after Close returns ErrClosed
// (REQ-GRPC-007).
func TestController_Request_AfterClose(t *testing.T) {
	addr, _, cleanup := startServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	c, err := grpcbridge.NewController(ctx, addr)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	_ = c.Close()

	_, err = c.Read(ctx, testAddr)
	if !errors.Is(err, grpcbridge.ErrClosed) {
		t.Errorf("want ErrClosed, got %v", err)
	}
}

// TestServer_Request_InnerTimeout an upstream request that never completes
// (a request-context deadline shorter than the upstream needs) surfaces as
// a gRPC-level error from the Server's Request handler, not a hang
// (REQ-GRPC-008).
func TestServer_Request_InnerTimeout(t *testing.T) {
	addr, _, cleanup := startServer(t)
	defer cleanup()

	// A context that is already expired forces upstream.Request's own
	// ctx.Done() branch, exercising the Server's own error-propagation path
	// (Server.Request returns whatever error the upstream reports, verbatim).
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond)

	c, err := grpcbridge.NewController(context.Background(), addr)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	defer func() { _ = c.Close() }()

	if _, err := c.Read(ctx, testAddr); err == nil {
		t.Error("Read with an already-expired context unexpectedly succeeded")
	}
}
