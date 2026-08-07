//fusa:test REQ-REST-001
//fusa:test REQ-REST-002
//fusa:test REQ-REST-003
//fusa:test REQ-REST-004
//fusa:test REQ-REST-005
//fusa:test REQ-REST-006
//fusa:test REQ-REST-007
//fusa:test REQ-REST-008

package restbridge_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/restbridge"
	"github.com/SoundMatt/go-RCP/v9/server"
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

// startTestServer wires a restbridge.Server end-to-end against a real
// upstream *udp.Controller reaching a udp.Server with echoHandler
// registered, and returns an httptest.Server plus the *restbridge.Server
// (for PublishTelemetry).
func startTestServer(t *testing.T) (*httptest.Server, *restbridge.Server) {
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

	bridgeSrv := restbridge.NewServer(upstream)
	ts := httptest.NewServer(bridgeSrv.Handler())
	t.Cleanup(func() {
		ts.Close()
		_ = upstream.Close()
		_ = rcpSrv.Close()
	})
	return ts, bridgeSrv
}

// TestServer_Request_Delivers POST /v1/endpoints/{addr}/request delivers a
// request to the real upstream endpoint (REQ-REST-001).
func TestServer_Request_Delivers(t *testing.T) {
	ts, _ := startTestServer(t)
	c := restbridge.NewController(ts.URL)
	defer func() { _ = c.Close() }()

	resp, err := c.Write(context.Background(), testAddr, []byte("hello"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if string(resp.Body) != "hello" {
		t.Errorf("Body = %q, want %q", resp.Body, "hello")
	}
}

// TestServer_Response_ControlField the JSON response includes the control
// field, distinguishing a normal response from a wire-level error
// (REQ-REST-002).
func TestServer_Response_ControlField(t *testing.T) {
	ts, _ := startTestServer(t)
	c := restbridge.NewController(ts.URL)
	defer func() { _ = c.Close() }()

	resp, err := c.Read(context.Background(), testAddr)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !resp.Control.Has(acf.FlagResponse) {
		t.Errorf("Control = %v, want FlagResponse set", resp.Control)
	}
}

// TestServer_Request_InvalidAddr an out-of-range endpoint address in the
// URL is rejected with 422 (REQ-REST-003).
func TestServer_Request_InvalidAddr(t *testing.T) {
	ts, _ := startTestServer(t)
	resp, err := http.Post(ts.URL+"/v1/endpoints/not-a-number/request", "application/json", nil) //nolint:noctx
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", resp.StatusCode)
	}
}

// TestServer_Telemetry_SSE GET /v1/telemetry returns an SSE stream fed by
// PublishTelemetry (REQ-REST-004).
func TestServer_Telemetry_SSE(t *testing.T) {
	ts, bridgeSrv := startTestServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c := restbridge.NewController(ts.URL)
	defer func() { _ = c.Close() }()

	ch, err := c.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	go func() {
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			bridgeSrv.PublishTelemetry(&restbridge.TelemetryEvent{ByteBusID: testAddr, Body: []byte("ping")})
		}
	}()

	select {
	case ev := <-ch:
		if ev.ByteBusID != testAddr {
			t.Errorf("ByteBusID = %v, want %v", ev.ByteBusID, testAddr)
		}
	case <-ctx.Done():
		t.Error("timeout waiting for SSE event")
	}
}

// TestController_Write_PayloadRoundTrip Controller.Write's response Body
// matches what the real upstream endpoint echoed (REQ-REST-005).
func TestController_Write_PayloadRoundTrip(t *testing.T) {
	ts, _ := startTestServer(t)
	c := restbridge.NewController(ts.URL)
	defer func() { _ = c.Close() }()

	want := []byte("roundtrip")
	resp, err := c.Write(context.Background(), testAddr, want)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if string(resp.Body) != string(want) {
		t.Errorf("payload = %q, want %q", resp.Body, want)
	}
}

// TestController_Read_UnregisteredEndpoint an unregistered endpoint's
// wire-level error response surfaces as FlagError, not a transport error
// (REQ-REST-006).
func TestController_Read_UnregisteredEndpoint(t *testing.T) {
	ts, _ := startTestServer(t)
	c := restbridge.NewController(ts.URL)
	defer func() { _ = c.Close() }()

	resp, err := c.Read(context.Background(), testAddr+1) // unregistered
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !resp.Control.Has(acf.FlagError) {
		t.Error("Control missing FlagError for an unregistered endpoint")
	}
}

// TestController_CloseIdempotent Close is safe to call twice (REQ-REST-007).
func TestController_CloseIdempotent(t *testing.T) {
	ts, _ := startTestServer(t)
	c := restbridge.NewController(ts.URL)
	if err := c.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// TestController_Request_AfterClose Request after Close returns ErrClosed
// (REQ-REST-008).
func TestController_Request_AfterClose(t *testing.T) {
	ts, _ := startTestServer(t)
	c := restbridge.NewController(ts.URL)
	_ = c.Close()

	_, err := c.Read(context.Background(), testAddr)
	if !errors.Is(err, restbridge.ErrClosed) {
		t.Errorf("want ErrClosed, got %v", err)
	}
}
