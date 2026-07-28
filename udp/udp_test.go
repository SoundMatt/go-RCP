//fusa:test REQ-UDP-001
//fusa:test REQ-UDP-002
//fusa:test REQ-UDP-003
//fusa:test REQ-UDP-004
//fusa:test REQ-UDP-005
//fusa:test REQ-UDP-006

package udp_test

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/regmap"
	"github.com/SoundMatt/go-RCP/server"
	"github.com/SoundMatt/go-RCP/udp"
)

func clientStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55}, 1)
}

func serverStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE}, 1)
}

// stubHandler answers every request with a fixed response body, echoing the
// request's control flags' Read/Write bit and recording the requester and
// request it was last called with. Guarded by mu since the Server's serve
// goroutine calls HandleRequest concurrently with the test goroutine that
// inspects the recorded fields after Controller.Request returns.
type stubHandler struct {
	body []byte
	err  error

	mu        sync.Mutex
	lastReq   acf.Message
	lastFrom  avtp.StreamID
	callCount int
}

func (h *stubHandler) HandleRequest(requester avtp.StreamID, req acf.Message) (acf.Message, error) {
	h.mu.Lock()
	h.lastReq = req
	h.lastFrom = requester
	h.callCount++
	h.mu.Unlock()
	if h.err != nil {
		return acf.Message{}, h.err
	}
	return acf.Message{
		Kind:           req.Kind,
		ByteBusID:      req.ByteBusID,
		TransactionNum: req.TransactionNum,
		Control:        acf.FlagResponse | (req.Control & (acf.FlagRead | acf.FlagWrite)),
		Body:           h.body,
	}, nil
}

func (h *stubHandler) last() (acf.Message, avtp.StreamID) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.lastReq, h.lastFrom
}

// newTestServer starts a udp.Server backed by a root-claimed server.Server
// and returns it alongside a dialed Controller.
func newTestServer(t *testing.T) (*udp.Server, *server.Server, *udp.Router) {
	t.Helper()
	root := clientStream()
	srv := server.NewServer()
	if err := srv.ClaimRoot(root); err != nil {
		t.Fatalf("ClaimRoot: %v", err)
	}
	router := udp.NewRouter(udp.NewEP0Handler(srv), true)
	us, err := udp.NewServer(serverStream(), "127.0.0.1:0", router)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { _ = us.Close() })
	return us, srv, router
}

func dial(t *testing.T, us *udp.Server, stream avtp.StreamID) *udp.Controller {
	t.Helper()
	ctrl, err := udp.NewController(stream, us.Addr())
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	t.Cleanup(func() { _ = ctrl.Close() })
	return ctrl
}

// TestController_Discover_RoundTrips verifies a discovery read against a
// freshly declared endpoint round-trips through regmap.DecodeRegisterMap
// (REQ-UDP-001, REQ-UDP-003).
func TestController_Discover_RoundTrips(t *testing.T) {
	us, srv, _ := newTestServer(t)
	root := clientStream()
	if err := srv.AddEndpoint(root, 1, regmap.EndpointTypeGPIO); err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}

	ctrl := dial(t, us, root)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	buf, err := ctrl.Discover(ctx)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	m, err := regmap.DecodeRegisterMap(buf)
	if err != nil {
		t.Fatalf("DecodeRegisterMap: %v", err)
	}
	if _, ok := m.Endpoint(1); !ok {
		t.Errorf("decoded map missing endpoint 1")
	}
}

// TestController_Write_RoutesToHandler verifies a write request addressed
// to a registered endpoint reaches its Handler with the requester's
// avtp.StreamID and body intact (REQ-UDP-002).
func TestController_Write_RoutesToHandler(t *testing.T) {
	us, _, router := newTestServer(t)
	h := &stubHandler{body: []byte{0xAA}}
	if err := router.Register(1, h); err != nil {
		t.Fatalf("Register: %v", err)
	}

	client := clientStream()
	ctrl := dial(t, us, client)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	want := []byte{0x01, 0x02, 0x03}
	resp, err := ctrl.Write(ctx, 1, want)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !resp.Control.Has(acf.FlagResponse) {
		t.Errorf("response missing FlagResponse")
	}
	lastReq, lastFrom := h.last()
	if !bytes.Equal(lastReq.Body, want) {
		t.Errorf("handler body = % X, want % X", lastReq.Body, want)
	}
	if lastFrom != client {
		t.Errorf("handler requester = %v, want %v", lastFrom, client)
	}
}

// TestController_Read_UnknownEndpoint verifies an unregistered ByteBusID
// yields a wire-level error response, not a dropped/lost request
// (REQ-UDP-004).
func TestController_Read_UnknownEndpoint(t *testing.T) {
	us, _, _ := newTestServer(t)
	ctrl := dial(t, us, clientStream())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := ctrl.Read(ctx, 5)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !resp.Control.Has(acf.FlagError) {
		t.Errorf("response missing FlagError for unknown endpoint")
	}
}

// TestController_Request_ContextExpired verifies ErrTimeout is returned
// when the context is already cancelled (REQ-UDP-005).
func TestController_Request_ContextExpired(t *testing.T) {
	us, _, _ := newTestServer(t)
	ctrl := dial(t, us, clientStream())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := ctrl.Read(ctx, 1)
	if !errors.Is(err, udp.ErrTimeout) {
		t.Errorf("error = %v, want ErrTimeout", err)
	}
}

// TestController_Request_AfterClose verifies ErrClosed is returned once the
// Controller has been closed (REQ-UDP-006).
func TestController_Request_AfterClose(t *testing.T) {
	us, _, _ := newTestServer(t)
	ctrl := dial(t, us, clientStream())
	_ = ctrl.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := ctrl.Read(ctx, 1)
	if !errors.Is(err, udp.ErrClosed) {
		t.Errorf("error = %v, want ErrClosed", err)
	}
}
