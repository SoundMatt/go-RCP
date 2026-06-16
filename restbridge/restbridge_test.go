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

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/mock"
	"github.com/SoundMatt/go-RCP/restbridge"
)

// startTestServer spins up an httptest.Server backed by a mock controller.
func startTestServer(t *testing.T, zone rcp.Zone, handler func(*rcp.Command) *rcp.Response) (*httptest.Server, *mock.Controller) {
	t.Helper()
	inner := mock.NewController(zone, handler)
	srv := restbridge.NewServer(inner)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(func() {
		ts.Close()
		_ = inner.Close()
	})
	return ts, inner
}

// REQ-REST-001: POST /v1/zones/{zone}/send delivers a command.
func TestServer_Send(t *testing.T) {
	ts, _ := startTestServer(t, rcp.ZoneFrontLeft, func(cmd *rcp.Command) *rcp.Response {
		return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusOK}
	})

	c := restbridge.NewController(rcp.ZoneFrontLeft, ts.URL)
	defer func() { _ = c.Close() }()

	cmd := &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdSet, Priority: rcp.PriorityNormal}
	resp, err := c.Send(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Zone != rcp.ZoneFrontLeft {
		t.Errorf("resp.Zone = %v, want ZoneFrontLeft", resp.Zone)
	}
}

// REQ-REST-002: Response JSON includes the status field.
func TestServer_SendResponse_StatusField(t *testing.T) {
	ts, _ := startTestServer(t, rcp.ZoneFrontLeft, func(cmd *rcp.Command) *rcp.Response {
		return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusOK}
	})

	c := restbridge.NewController(rcp.ZoneFrontLeft, ts.URL)
	defer func() { _ = c.Close() }()

	cmd := &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdGet}
	resp, err := c.Send(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Status != rcp.StatusOK {
		t.Errorf("status = %v, want StatusOK", resp.Status)
	}
}

// REQ-REST-003: Server returns 422 for an unknown zone in the URL.
func TestServer_Send_UnknownZone(t *testing.T) {
	ts, _ := startTestServer(t, rcp.ZoneFrontLeft, nil)

	resp, err := http.Post(ts.URL+"/v1/zones/99/send", "application/json", nil) //nolint:noctx
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", resp.StatusCode)
	}
}

// REQ-REST-004: GET /v1/zones/{zone}/events returns an SSE stream.
func TestServer_Events_SSE(t *testing.T) {
	ts, inner := startTestServer(t, rcp.ZoneFrontLeft, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c := restbridge.NewController(rcp.ZoneFrontLeft, ts.URL)
	defer func() { _ = c.Close() }()

	ch, err := c.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	go func() {
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			inner.Publish([]byte("ping"))
		}
	}()

	select {
	case st := <-ch:
		if st.Zone != rcp.ZoneFrontLeft {
			t.Errorf("zone = %v, want ZoneFrontLeft", st.Zone)
		}
	case <-ctx.Done():
		t.Error("timeout waiting for SSE event")
	}
}

// REQ-REST-005: Controller.Send returns the Response from the server.
func TestController_Send_PayloadRoundTrip(t *testing.T) {
	want := []byte("roundtrip")
	ts, _ := startTestServer(t, rcp.ZoneFrontLeft, func(cmd *rcp.Command) *rcp.Response {
		return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusOK, Payload: cmd.Payload}
	})

	c := restbridge.NewController(rcp.ZoneFrontLeft, ts.URL)
	defer func() { _ = c.Close() }()

	cmd := &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdSet, Payload: want}
	resp, err := c.Send(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if string(resp.Payload) != string(want) {
		t.Errorf("payload = %q, want %q", resp.Payload, want)
	}
}

// REQ-REST-006: Controller.Send returns ErrZoneMismatch for wrong zone.
func TestController_Send_ZoneMismatch(t *testing.T) {
	ts, _ := startTestServer(t, rcp.ZoneFrontLeft, nil)

	c := restbridge.NewController(rcp.ZoneFrontLeft, ts.URL)
	defer func() { _ = c.Close() }()

	cmd := &rcp.Command{Zone: rcp.ZoneRearLeft, Type: rcp.CmdSet}
	_, err := c.Send(context.Background(), cmd)
	if !errors.Is(err, rcp.ErrZoneMismatch) {
		t.Errorf("want ErrZoneMismatch, got %v", err)
	}
}

// REQ-REST-007: Controller.Close is idempotent.
func TestController_CloseIdempotent(t *testing.T) {
	ts, _ := startTestServer(t, rcp.ZoneFrontLeft, nil)
	_ = ts
	c := restbridge.NewController(rcp.ZoneFrontLeft, ts.URL)
	if err := c.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// REQ-REST-008: Controller.Send returns ErrClosed after Close.
func TestController_Send_AfterClose(t *testing.T) {
	ts, _ := startTestServer(t, rcp.ZoneFrontLeft, nil)
	c := restbridge.NewController(rcp.ZoneFrontLeft, ts.URL)
	_ = c.Close()

	cmd := &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdSet}
	_, err := c.Send(context.Background(), cmd)
	if !errors.Is(err, rcp.ErrClosed) {
		t.Errorf("want ErrClosed, got %v", err)
	}
}
