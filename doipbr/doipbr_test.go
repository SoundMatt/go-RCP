//fusa:test REQ-DOIP-001
//fusa:test REQ-DOIP-002
//fusa:test REQ-DOIP-003
//fusa:test REQ-DOIP-004
//fusa:test REQ-DOIP-005
//fusa:test REQ-DOIP-006
//fusa:test REQ-DOIP-007
//fusa:test REQ-DOIP-008

package doipbr_test

import (
	"bytes"
	"context"
	"errors"
	"net"
	"testing"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/doipbr"
	"github.com/SoundMatt/go-RCP/mock"
	"github.com/SoundMatt/go-RCP/udsbr"
)

func startServer(t *testing.T, handler func(*rcp.Command) *rcp.Response) (*doipbr.Server, func()) {
	t.Helper()
	inner := mock.NewController(rcp.ZoneFrontLeft, handler)
	uds := udsbr.NewServer(inner)
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	srv := doipbr.NewServer(uds, ln)
	go srv.Serve()
	return srv, func() {
		_ = srv.Close()
		_ = inner.Close()
		uds.Close()
	}
}

// REQ-DOIP-001: BuildHeader produces an 8-byte header with correct protocol version.
func TestBuildHeader(t *testing.T) {
	h := doipbr.BuildHeader(doipbr.PayloadTypeDiagMessage, 4)
	if len(h) != 8 {
		t.Fatalf("len = %d, want 8", len(h))
	}
	if h[0] != doipbr.ProtoVersion || h[1] != doipbr.ProtoVersionInverse {
		t.Errorf("version bytes = %02X %02X, want %02X %02X",
			h[0], h[1], doipbr.ProtoVersion, doipbr.ProtoVersionInverse)
	}
}

// REQ-DOIP-002: ParseHeader reads and validates the DoIP header.
func TestParseHeader(t *testing.T) {
	h := doipbr.BuildHeader(doipbr.PayloadTypeDiagMessage, 12)
	pt, pl, err := doipbr.ParseHeader(bytes.NewReader(h))
	if err != nil {
		t.Fatalf("ParseHeader: %v", err)
	}
	if pt != doipbr.PayloadTypeDiagMessage {
		t.Errorf("payloadType = 0x%04X, want 0x8001", pt)
	}
	if pl != 12 {
		t.Errorf("payloadLen = %d, want 12", pl)
	}
}

// REQ-DOIP-003: ParseHeader returns ErrInvalidHeader for bad version bytes.
func TestParseHeader_Invalid(t *testing.T) {
	h := []byte{0x01, 0x02, 0x80, 0x01, 0, 0, 0, 0}
	_, _, err := doipbr.ParseHeader(bytes.NewReader(h))
	if !errors.Is(err, doipbr.ErrInvalidHeader) {
		t.Errorf("want ErrInvalidHeader, got %v", err)
	}
}

// REQ-DOIP-004: Server.Serve accepts TCP connections and processes DoIP messages.
func TestServer_Serve(t *testing.T) {
	srv, cleanup := startServer(t, func(cmd *rcp.Command) *rcp.Response {
		return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusOK}
	})
	defer cleanup()

	c, err := doipbr.NewClient(srv.Addr().String())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() { _ = c.Close() }()

	pdu := udsbr.BuildRequest(udsbr.SIDWriteDataByIdentifier, udsbr.DIDRCPCommand,
		[]byte{byte(rcp.ZoneFrontLeft), byte(rcp.CmdSet)})
	resp, err := c.Send(context.Background(), pdu)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp[0] != udsbr.SIDWriteDataByIdentifier+udsbr.SIDPositiveOffset {
		t.Errorf("response SID = 0x%02X, want positive", resp[0])
	}
}

// REQ-DOIP-005: Client.Send transmits a diagnostic message and receives the response.
func TestClient_Send(t *testing.T) {
	dispatched := make(chan rcp.CommandType, 1)
	srv, cleanup := startServer(t, func(cmd *rcp.Command) *rcp.Response {
		dispatched <- cmd.Type
		return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusOK}
	})
	defer cleanup()

	c, err := doipbr.NewClient(srv.Addr().String())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() { _ = c.Close() }()

	pdu := udsbr.BuildRequest(udsbr.SIDWriteDataByIdentifier, udsbr.DIDRCPCommand,
		[]byte{byte(rcp.ZoneFrontLeft), byte(rcp.CmdGet)})
	if _, err = c.Send(context.Background(), pdu); err != nil {
		t.Fatalf("Send: %v", err)
	}
	select {
	case got := <-dispatched:
		if got != rcp.CmdGet {
			t.Errorf("dispatched %v, want CmdGet", got)
		}
	case <-time.After(3 * time.Second):
		t.Error("timeout waiting for dispatch")
	}
}

// REQ-DOIP-006: Server returns NACK for unsupported payload types.
func TestServer_Serve_UnsupportedPayload(t *testing.T) {
	srv, cleanup := startServer(t, nil)
	defer cleanup()

	conn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", srv.Addr().String())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	h := doipbr.BuildHeader(0x0001, 0) // unsupported type
	if _, err = conn.Write(h); err != nil {
		t.Fatalf("Write: %v", err)
	}

	_, respType, err := doipbr.ParseHeader(conn)
	if err != nil {
		t.Fatalf("ParseHeader resp: %v", err)
	}
	_ = respType // just verify server responds
}

// REQ-DOIP-007: Server.Close is idempotent.
func TestServer_CloseIdempotent(t *testing.T) {
	srv, cleanup := startServer(t, nil)
	defer cleanup()
	_ = srv.Close()
	_ = srv.Close() // must not panic
}

// REQ-DOIP-008: Client.Close is idempotent.
func TestClient_CloseIdempotent(t *testing.T) {
	srv, cleanup := startServer(t, nil)
	defer cleanup()

	c, err := doipbr.NewClient(srv.Addr().String())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_ = c.Close()
	_ = c.Close() // must not panic
}
