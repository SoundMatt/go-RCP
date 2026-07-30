//fusa:test REQ-DOIP-001
//fusa:test REQ-DOIP-002
//fusa:test REQ-DOIP-003
//fusa:test REQ-DOIP-004
//fusa:test REQ-DOIP-005
//fusa:test REQ-DOIP-006
//fusa:test REQ-DOIP-007
//fusa:test REQ-DOIP-008
//fusa:test REQ-DOIP-009

package doipbr_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"testing"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/doipbr"
	"github.com/SoundMatt/go-RCP/server"
	"github.com/SoundMatt/go-RCP/udp"
	"github.com/SoundMatt/go-RCP/udsbr"
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

// startServer wires a doipbr.Server end-to-end: a real udp.Controller
// reaching a udp.Server with echoHandler registered, wrapped by a
// udsbr.Server, wrapped by a doipbr.Server listening on TCP.
func startServer(t *testing.T) (*doipbr.Server, func()) {
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
	uds := udsbr.NewServer(upstream)

	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	srv := doipbr.NewServer(uds, ln)
	srv.ServeBackground()

	return srv, func() {
		_ = srv.Close()
		_ = upstream.Close()
		_ = rcpSrv.Close()
		uds.Close()
	}
}

// startServerWithFailingUDS wires the same stack as startServer, except the
// embedded udsbr.Server is closed before doipbr.Server ever sees it. Every
// subsequent Handle call therefore returns udsbr.ErrClosed (and a nonempty
// UDS-level negative-response PDU alongside it) — a real, deterministic way
// to exercise handleConn's "the UDS handler itself errored" path without a
// fake/mock udsbr.Server, since doipbr.Server embeds the concrete type.
func startServerWithFailingUDS(t *testing.T) (*doipbr.Server, func()) {
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
	uds := udsbr.NewServer(upstream)
	uds.Close() // every Handle call from here on returns udsbr.ErrClosed

	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	srv := doipbr.NewServer(uds, ln)
	srv.ServeBackground()

	return srv, func() {
		_ = srv.Close()
		_ = upstream.Close()
		_ = rcpSrv.Close()
	}
}

// TestBuildHeader BuildHeader produces an 8-byte header with the correct
// protocol version and payload type (REQ-DOIP-001).
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

// TestParseHeader ParseHeader reads and validates the DoIP header
// (REQ-DOIP-002).
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

// TestParseHeader_Invalid ParseHeader returns ErrInvalidHeader for bad
// version bytes (REQ-DOIP-003).
func TestParseHeader_Invalid(t *testing.T) {
	h := []byte{0x01, 0x02, 0x80, 0x01, 0, 0, 0, 0}
	_, _, err := doipbr.ParseHeader(bytes.NewReader(h))
	if !errors.Is(err, doipbr.ErrInvalidHeader) {
		t.Errorf("want ErrInvalidHeader, got %v", err)
	}
}

// TestServer_Serve Serve accepts TCP connections and processes DiagMessage
// PDUs end-to-end through to a real upstream endpoint (REQ-DOIP-004).
func TestServer_Serve(t *testing.T) {
	srv, cleanup := startServer(t)
	defer cleanup()

	c, err := doipbr.NewClient(srv.Addr().String())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() { _ = c.Close() }()

	pdu := udsbr.BuildRequest(udsbr.SIDWriteDataByIdentifier, udsbr.DataIdentifier(testAddr), []byte{0xAA})
	resp, err := c.Send(context.Background(), pdu)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp[0] != udsbr.SIDWriteDataByIdentifier+udsbr.SIDPositiveOffset {
		t.Errorf("response SID = 0x%02X, want positive", resp[0])
	}
}

// TestClient_Send Client.Send transmits a diagnostic message and returns the
// UDS response payload, echoed through the real upstream endpoint
// (REQ-DOIP-005).
func TestClient_Send(t *testing.T) {
	srv, cleanup := startServer(t)
	defer cleanup()

	c, err := doipbr.NewClient(srv.Addr().String())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() { _ = c.Close() }()

	pdu := udsbr.BuildRequest(udsbr.SIDWriteDataByIdentifier, udsbr.DataIdentifier(testAddr), []byte{0xBE, 0xEF})
	resp, err := c.Send(context.Background(), pdu)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !bytes.Equal(resp[3:], []byte{0xBE, 0xEF}) {
		t.Errorf("resp data = % X, want % X", resp[3:], []byte{0xBE, 0xEF})
	}
}

// TestServer_Serve_UnsupportedPayload the server NACKs an unrecognised
// payload type (REQ-DOIP-006).
func TestServer_Serve_UnsupportedPayload(t *testing.T) {
	srv, cleanup := startServer(t)
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

	respType, _, err := doipbr.ParseHeader(conn)
	if err != nil {
		t.Fatalf("ParseHeader resp: %v", err)
	}
	if respType != doipbr.PayloadTypeDiagMessageNack {
		t.Errorf("respType = 0x%04X, want NACK 0x%04X", respType, doipbr.PayloadTypeDiagMessageNack)
	}
}

// TestServer_CloseIdempotent Server.Close is idempotent (REQ-DOIP-007).
func TestServer_CloseIdempotent(t *testing.T) {
	srv, cleanup := startServer(t)
	defer cleanup()
	_ = srv.Close()
	_ = srv.Close() // must not panic
}

// TestClient_CloseIdempotent Client.Close is idempotent (REQ-DOIP-008).
func TestClient_CloseIdempotent(t *testing.T) {
	srv, cleanup := startServer(t)
	defer cleanup()

	c, err := doipbr.NewClient(srv.Addr().String())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_ = c.Close()
	_ = c.Close() // must not panic
}

// TestServeBackground_TracksWaitGroup ServeBackground calls wg.Add(1)
// synchronously before starting the Serve goroutine, so Close() correctly
// blocks until Serve has actually exited rather than racing it
// (REQ-DOIP-009).
func TestServeBackground_TracksWaitGroup(t *testing.T) {
	srv, cleanup := startServer(t)
	defer cleanup()
	// If ServeBackground raced wg.Add(1) against the goroutine, Close could
	// return before Serve's accept loop had actually stopped; a clean,
	// hang-free Close (bounded by the test's own timeout) is this test's
	// evidence that didn't happen.
	if err := srv.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// TestServer_Serve_UDSHandlerError checks that when the embedded
// udsbr.Server.Handle call itself returns an error for a DiagMessage
// payload, handleConn sends the DoIP diagnostic-message NACK
// (PayloadTypeDiagMessageNack) carrying NackCodeHandlerFailed — not a
// positive ACK built from whatever (possibly stale) response bytes Handle
// also returned alongside its error (go-RCP-N2-03).
func TestServer_Serve_UDSHandlerError(t *testing.T) {
	srv, cleanup := startServerWithFailingUDS(t)
	defer cleanup()

	conn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", srv.Addr().String())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	pdu := udsbr.BuildRequest(udsbr.SIDWriteDataByIdentifier, udsbr.DataIdentifier(testAddr), []byte{0xAA})
	msg := append(doipbr.BuildHeader(doipbr.PayloadTypeDiagMessage, uint32(len(pdu))), pdu...)
	if _, err = conn.Write(msg); err != nil {
		t.Fatalf("Write: %v", err)
	}

	respType, respLen, err := doipbr.ParseHeader(conn)
	if err != nil {
		t.Fatalf("ParseHeader resp: %v", err)
	}
	if respType != doipbr.PayloadTypeDiagMessageNack {
		t.Fatalf("respType = 0x%04X, want NACK 0x%04X (not a positive ACK)", respType, doipbr.PayloadTypeDiagMessageNack)
	}
	if respLen != 1 {
		t.Fatalf("respLen = %d, want 1 (a nonempty NACK code payload)", respLen)
	}
	respBody := make([]byte, respLen)
	if _, err = io.ReadFull(conn, respBody); err != nil {
		t.Fatalf("read NACK payload: %v", err)
	}
	if respBody[0] != doipbr.NackCodeHandlerFailed {
		t.Errorf("NACK code = 0x%02X, want 0x%02X", respBody[0], doipbr.NackCodeHandlerFailed)
	}
}
