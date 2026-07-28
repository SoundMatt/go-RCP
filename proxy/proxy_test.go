//fusa:test REQ-PX-001
//fusa:test REQ-PX-002
//fusa:test REQ-PX-003
//fusa:test REQ-PX-004
//fusa:test REQ-PX-005
//fusa:test REQ-PX-006
//fusa:test REQ-PX-007
//fusa:test REQ-PX-008

package proxy_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/proxy"
	"github.com/SoundMatt/go-RCP/request"
	"github.com/SoundMatt/go-RCP/server"
	"github.com/SoundMatt/go-RCP/udp"
)

// Compile-time assertion that Handler satisfies request.Handler, the
// interface udp.Router.Register expects (REQ-PX-008).
var _ request.Handler = (*proxy.Handler)(nil)

const (
	downstreamAddr = avtp.ByteBusID(1)
	upstreamAddr   = avtp.ByteBusID(5)
)

// recordingHandler answers with an echoed body and records the requester
// StreamID it was last invoked with, so a test can assert what identity
// actually reached the wire.
type recordingHandler struct {
	mu       sync.Mutex
	lastFrom avtp.StreamID
	calls    int
}

func (h *recordingHandler) HandleRequest(requester avtp.StreamID, req acf.Message) (acf.Message, error) {
	h.mu.Lock()
	h.lastFrom = requester
	h.calls++
	h.mu.Unlock()
	return acf.Message{
		Kind:           req.Kind,
		ByteBusID:      req.ByteBusID,
		TransactionNum: req.TransactionNum,
		Control:        acf.FlagResponse | (req.Control & (acf.FlagRead | acf.FlagWrite)),
		Body:           req.Body,
	}, nil
}

// snapshot reads lastFrom/calls under the lock — a plain field read here
// would race against HandleRequest's own locked write from the server's
// goroutine even after a synchronous request/response round trip, since a
// raw socket exchange establishes no happens-before edge the race detector
// can see.
func (h *recordingHandler) snapshot() (avtp.StreamID, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.lastFrom, h.calls
}

func clientStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55}, 1)
}

func proxyStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0}, 1)
}

func upstreamServerStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE}, 1)
}

func downstreamServerStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0x99, 0x99, 0x99, 0x99, 0x99}, 1)
}

// harness wires client -> downstream udp.Server -> proxy.Handler ->
// upstream udp.Server -> recordingHandler, and exposes both endpoints for
// inspection.
type harness struct {
	client     *udp.Controller
	upstream   *recordingHandler
	handler    *proxy.Handler
	downRouter *udp.Router
}

func newHarness(t *testing.T, transform proxy.TransformFunc) *harness {
	t.Helper()

	// Upstream side.
	upHandler := &recordingHandler{}
	upRouter := udp.NewRouter(udp.NewEP0Handler(server.NewServer()), false)
	if err := upRouter.Register(upstreamAddr, upHandler); err != nil {
		t.Fatalf("upstream Register: %v", err)
	}
	// Also registered at downstreamAddr, so a no-transform ("forward
	// unchanged") request — which reaches the upstream server still
	// addressed at the original downstream addr — has somewhere to land.
	if err := upRouter.Register(downstreamAddr, upHandler); err != nil {
		t.Fatalf("upstream Register (downstreamAddr passthrough): %v", err)
	}
	upSrv, err := udp.NewServer(upstreamServerStream(), "127.0.0.1:0", upRouter)
	if err != nil {
		t.Fatalf("upstream NewServer: %v", err)
	}
	t.Cleanup(func() { _ = upSrv.Close() })

	upCtrl, err := udp.NewController(proxyStream(), upSrv.Addr())
	if err != nil {
		t.Fatalf("dial upstream: %v", err)
	}

	// Downstream side, fronted by the proxy Handler.
	ph := proxy.NewHandler(upCtrl, transform, 0)
	t.Cleanup(func() { _ = ph.Close() })

	downRouter := udp.NewRouter(udp.NewEP0Handler(server.NewServer()), false)
	if regErr := downRouter.Register(downstreamAddr, ph); regErr != nil {
		t.Fatalf("downstream Register: %v", regErr)
	}
	downSrv, err := udp.NewServer(downstreamServerStream(), "127.0.0.1:0", downRouter)
	if err != nil {
		t.Fatalf("downstream NewServer: %v", err)
	}
	t.Cleanup(func() { _ = downSrv.Close() })

	client, err := udp.NewController(clientStream(), downSrv.Addr())
	if err != nil {
		t.Fatalf("dial downstream: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	return &harness{client: client, upstream: upHandler, handler: ph, downRouter: downRouter}
}

// TestProxy_ForwardsUnchanged with no transform, the request reaches the
// upstream endpoint under the same addr and body (REQ-PX-001).
func TestProxy_ForwardsUnchanged(t *testing.T) {
	h := newHarness(t, nil)

	resp, err := h.client.Write(context.Background(), downstreamAddr, []byte("hello"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if string(resp.Body) != "hello" {
		t.Errorf("resp.Body = %q, want %q", resp.Body, "hello")
	}
}

// TestProxy_TransformRemapsAddr TransformFunc rewrites the byte_bus_id
// before forwarding upstream (REQ-PX-002).
func TestProxy_TransformRemapsAddr(t *testing.T) {
	var mu sync.Mutex
	var gotAddr avtp.ByteBusID
	transform := func(_ avtp.StreamID, addr avtp.ByteBusID, _ acf.ControlFlags, body []byte) (avtp.ByteBusID, []byte, error) {
		mu.Lock()
		gotAddr = addr
		mu.Unlock()
		return upstreamAddr, body, nil
	}
	h := newHarness(t, transform)

	if _, err := h.client.Read(context.Background(), downstreamAddr); err != nil {
		t.Fatalf("Read: %v", err)
	}

	mu.Lock()
	got := gotAddr
	mu.Unlock()
	if got != downstreamAddr {
		t.Errorf("transform saw addr %d, want original downstream addr %d", got, downstreamAddr)
	}
	if _, calls := h.upstream.snapshot(); calls != 1 {
		t.Errorf("upstream calls = %d, want 1 (remapped request must reach upstreamAddr)", calls)
	}
}

// TestProxy_TransformErrorAborts a TransformFunc error aborts the forward
// without reaching the upstream handler (REQ-PX-003).
func TestProxy_TransformErrorAborts(t *testing.T) {
	boom := errors.New("boom")
	transform := func(avtp.StreamID, avtp.ByteBusID, acf.ControlFlags, []byte) (avtp.ByteBusID, []byte, error) {
		return 0, nil, boom
	}
	h := newHarness(t, transform)

	// HandleRequest returns an error which udp.Router reports as a
	// wire-level error response (see udp/router.go's errorResponse), not a
	// transport error to the caller.
	resp, err := h.client.Read(context.Background(), downstreamAddr)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !resp.Control.Has(acf.FlagError) {
		t.Errorf("Control = %v, want FlagError (transform aborted)", resp.Control)
	}
	if _, calls := h.upstream.snapshot(); calls != 0 {
		t.Errorf("upstream calls = %d, want 0 (transform error must not reach upstream)", calls)
	}
}

// TestProxy_PresentsOwnStreamIdentity the upstream endpoint sees the
// proxy's own StreamID as requester, never the original downstream
// caller's — the stream_id remapping half of a real RCP-level proxy
// (REQ-PX-004).
func TestProxy_PresentsOwnStreamIdentity(t *testing.T) {
	h := newHarness(t, nil)

	if _, err := h.client.Read(context.Background(), downstreamAddr); err != nil {
		t.Fatalf("Read: %v", err)
	}
	lastFrom, _ := h.upstream.snapshot()
	if lastFrom != proxyStream() {
		t.Errorf("upstream saw requester %v, want the proxy's own identity %v", lastFrom, proxyStream())
	}
	if lastFrom == clientStream() {
		t.Errorf("upstream saw the original downstream client's StreamID; it must never be forwarded")
	}
}

// TestProxy_ResponseCorrelatesWithOriginalRequest the response is
// repackaged against the original downstream request's ByteBusID/
// TransactionNum, not whatever addr was actually used upstream
// (REQ-PX-005).
func TestProxy_ResponseCorrelatesWithOriginalRequest(t *testing.T) {
	transform := func(_ avtp.StreamID, _ avtp.ByteBusID, _ acf.ControlFlags, body []byte) (avtp.ByteBusID, []byte, error) {
		return upstreamAddr, body, nil
	}
	h := newHarness(t, transform)

	resp, err := h.client.Read(context.Background(), downstreamAddr)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if resp.ByteBusID != downstreamAddr {
		t.Errorf("resp.ByteBusID = %d, want original downstream addr %d", resp.ByteBusID, downstreamAddr)
	}
}

// TestProxy_Close_Idempotent Close is safe to call multiple times, and a
// closed Handler reports ErrClosed directly, without attempting to reach
// upstream (REQ-PX-006).
func TestProxy_Close_Idempotent(t *testing.T) {
	h := newHarness(t, nil)

	if err := h.handler.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := h.handler.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}

	_, err := h.handler.HandleRequest(clientStream(), acf.Message{ByteBusID: downstreamAddr})
	if !errors.Is(err, proxy.ErrClosed) {
		t.Errorf("HandleRequest after Close err = %v, want ErrClosed", err)
	}
}

// TestProxy_Concurrent multiple goroutines may call HandleRequest
// simultaneously without a data race (REQ-PX-007).
func TestProxy_Concurrent(t *testing.T) {
	h := newHarness(t, nil)

	const n = 30
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_, _ = h.client.Read(context.Background(), downstreamAddr)
		}()
	}
	wg.Wait()
}

// TestProxy_ComposableAsRouterHandler a Handler implements request.Handler,
// so it composes into a udp.Router the same as any native endpoint's own
// Handler, including a second registration on an entirely different Router
// instance (REQ-PX-008).
func TestProxy_ComposableAsRouterHandler(t *testing.T) {
	h := newHarness(t, nil)

	secondRouter := udp.NewRouter(udp.NewEP0Handler(server.NewServer()), false)
	if err := secondRouter.Register(downstreamAddr, h.handler); err != nil {
		t.Fatalf("registering the same proxy.Handler into a second Router: %v", err)
	}
}
