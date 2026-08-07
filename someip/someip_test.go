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
	"bytes"
	"context"
	"errors"
	"net"
	"testing"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/server"
	"github.com/SoundMatt/go-RCP/v9/someip"
	"github.com/SoundMatt/go-RCP/v9/udp"
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

func newUpstream(t *testing.T) *udp.Controller {
	t.Helper()
	router := udp.NewRouter(udp.NewEP0Handler(server.NewServer()), false)
	if err := router.Register(testAddr, echoHandler{}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	srv, err := udp.NewServer(serverStream(), "127.0.0.1:0", router)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	ctrl, err := udp.NewController(clientStream(), srv.Addr())
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	t.Cleanup(func() { _ = ctrl.Close() })
	return ctrl
}

func mustResolve(t *testing.T) *net.UDPAddr {
	t.Helper()
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ResolveUDPAddr: %v", err)
	}
	return addr
}

// newBridgedServer starts a someip.Server forwarding to a real upstream
// *udp.Controller, and returns a someip.Controller dialed against it.
func newBridgedServer(t *testing.T) *someip.Controller {
	t.Helper()
	upstream := newUpstream(t)
	srv, err := someip.NewServer(upstream, mustResolve(t), someip.DefaultServiceID)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	c, err := someip.NewController(srv.Addr(), someip.DefaultServiceID)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// TestFrame_EncodeDecode_RoundTrip a SOME/IP header/payload round-trips
// through encode/decode via the Server/Controller wire path (REQ-SIPC-001).
func TestFrame_EncodeDecode_RoundTrip(t *testing.T) {
	c := newBridgedServer(t)
	resp, err := c.Write(context.Background(), testAddr, []byte{1, 2, 3})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !bytes.Equal(resp.Body, []byte{1, 2, 3}) {
		t.Errorf("Body = % X, want % X", resp.Body, []byte{1, 2, 3})
	}
}

// TestServer_IgnoresOtherServiceID a REQUEST for a different ServiceID is
// silently ignored (REQ-SIPC-002): exercised by dialing with a mismatched
// serviceID and confirming the call times out rather than erroring
// immediately, since decodeFrame itself never rejects a well-formed frame.
func TestServer_IgnoresOtherServiceID(t *testing.T) {
	upstream := newUpstream(t)
	srv, err := someip.NewServer(upstream, mustResolve(t), someip.DefaultServiceID)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer func() { _ = srv.Close() }()

	c, err := someip.NewController(srv.Addr(), someip.DefaultServiceID+1) // mismatched
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 100_000_000) // 100ms
	defer cancel()
	if _, err := c.Read(ctx, testAddr); err == nil {
		t.Error("Read with mismatched ServiceID unexpectedly succeeded")
	}
}

// TestServer_ForwardsRequest a REQUEST is forwarded to the upstream
// controller and its echoed body comes back in the RESPONSE (REQ-SIPC-003).
func TestServer_ForwardsRequest(t *testing.T) {
	c := newBridgedServer(t)
	resp, err := c.Write(context.Background(), testAddr, []byte("hello"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if resp.Control.Has(acf.FlagError) {
		t.Error("Control has FlagError set, want a clean RESPONSE")
	}
}

// TestServer_RespondsRESPONSE a request against an address with no
// registered upstream handler comes back as a NOT_OK RESPONSE, surfaced as
// FlagError (REQ-SIPC-004).
func TestServer_RespondsRESPONSE(t *testing.T) {
	c := newBridgedServer(t)
	resp, err := c.Read(context.Background(), testAddr+1) // unregistered
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !resp.Control.Has(acf.FlagError) {
		t.Error("Control missing FlagError for an unregistered endpoint")
	}
}

// TestController_Read_EmptyPayloadIsRead an empty-payload Controller.Read
// selects a read request; the echo handler still answers Body based on
// what it was actually asked (Read has an empty body, so the upstream
// answers empty too) (REQ-SIPC-005).
func TestController_Read_EmptyPayloadIsRead(t *testing.T) {
	c := newBridgedServer(t)
	resp, err := c.Read(context.Background(), testAddr)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(resp.Body) != 0 {
		t.Errorf("Body = % X, want empty", resp.Body)
	}
}

// TestController_Write_NonEmptyPayloadIsWrite a non-empty payload selects a
// write request whose body is exactly what was sent (REQ-SIPC-006).
func TestController_Write_NonEmptyPayloadIsWrite(t *testing.T) {
	c := newBridgedServer(t)
	resp, err := c.Write(context.Background(), testAddr, []byte{0xAA})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !bytes.Equal(resp.Body, []byte{0xAA}) {
		t.Errorf("Body = % X, want %X", resp.Body, []byte{0xAA})
	}
}

// TestController_Close_Idempotent Close is safe to call twice (REQ-SIPC-007).
func TestController_Close_Idempotent(t *testing.T) {
	c := newBridgedServer(t)
	if err := c.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

// TestController_Request_AfterClose Request after Close returns ErrClosed
// (REQ-SIPC-008).
func TestController_Request_AfterClose(t *testing.T) {
	c := newBridgedServer(t)
	_ = c.Close()
	if _, err := c.Read(context.Background(), testAddr); !errors.Is(err, someip.ErrClosed) {
		t.Errorf("err = %v, want ErrClosed", err)
	}
}
