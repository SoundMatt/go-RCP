//fusa:test REQ-ADM-001
//fusa:test REQ-ADM-002
//fusa:test REQ-ADM-003
//fusa:test REQ-ADM-004
//fusa:test REQ-ADM-005
//fusa:test REQ-ADM-006
//fusa:test REQ-ADM-007
//fusa:test REQ-ADM-008

package admin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/admin"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/server"
	"github.com/SoundMatt/go-RCP/v9/udp"
)

const testEndpoint = avtp.ByteBusID(1)

type stubHandler struct{}

func (stubHandler) HandleRequest(_ avtp.StreamID, req acf.Message) (acf.Message, error) {
	return acf.Message{
		Kind:           req.Kind,
		ByteBusID:      req.ByteBusID,
		TransactionNum: req.TransactionNum,
		Control:        acf.FlagResponse | (req.Control & (acf.FlagRead | acf.FlagWrite)),
		Body:           req.Body,
	}, nil
}

func dialedController(t *testing.T) *udp.Controller {
	t.Helper()
	router := udp.NewRouter(udp.NewEP0Handler(server.NewServer()), false)
	if err := router.Register(testEndpoint, stubHandler{}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	srv, err := udp.NewServer(avtp.NewStreamID([6]byte{0x02, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE}, 1), "127.0.0.1:0", router)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	ctrl, err := udp.NewController(avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55}, 1), srv.Addr())
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	t.Cleanup(func() { _ = ctrl.Close() })
	return ctrl
}

func newServer(t *testing.T, bearer string) *admin.Server {
	t.Helper()
	srv := admin.New(admin.Config{BearerToken: bearer})
	if err := srv.Register("server-a", dialedController(t)); err != nil {
		t.Fatalf("Register: %v", err)
	}
	return srv
}

// TestGetServers_ReturnsAll lists all registered servers (REQ-ADM-001).
func TestGetServers_ReturnsAll(t *testing.T) {
	srv := newServer(t, "")
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/servers", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var infos []admin.ServerInfo
	if err := json.NewDecoder(w.Body).Decode(&infos); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(infos) != 1 || infos[0].Key != "server-a" {
		t.Errorf("infos = %+v, want one entry keyed server-a", infos)
	}
}

// TestGetServers_MethodNotAllowed rejects a non-GET on /servers (REQ-ADM-001).
func TestGetServers_MethodNotAllowed(t *testing.T) {
	srv := newServer(t, "")
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/servers", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

// TestGetServer_Known returns detail for a known server (REQ-ADM-002).
func TestGetServer_Known(t *testing.T) {
	srv := newServer(t, "")
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/servers/server-a", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var info admin.ServerInfo
	if err := json.NewDecoder(w.Body).Decode(&info); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if info.Key != "server-a" {
		t.Errorf("key = %q, want server-a", info.Key)
	}
	if info.StreamID == "" {
		t.Errorf("StreamID empty, want the dialed controller's identity")
	}
}

// TestGetServer_Unknown returns 404 for an unregistered key (REQ-ADM-002).
func TestGetServer_Unknown(t *testing.T) {
	srv := newServer(t, "")
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/servers/bogus", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// TestRequest_NoAuth dispatches a request via the registry when no bearer
// token is configured (REQ-ADM-003).
func TestRequest_NoAuth(t *testing.T) {
	srv := newServer(t, "")
	body := `{"endpoint":1,"write":false}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/servers/server-a/request", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — body: %s", w.Code, w.Body.String())
	}
	var resp acf.Message
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Control.Has(acf.FlagResponse) {
		t.Errorf("Control = %v, want FlagResponse set", resp.Control)
	}
}

// TestRequest_BearerRequired rejects a write endpoint without the
// configured bearer token (REQ-ADM-004).
func TestRequest_BearerRequired(t *testing.T) {
	srv := newServer(t, "secret")
	body := `{"endpoint":1}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/servers/server-a/request", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

// TestRequest_BearerAccepted accepts a matching bearer token (REQ-ADM-004).
func TestRequest_BearerAccepted(t *testing.T) {
	srv := newServer(t, "secret")
	body := `{"endpoint":1}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/servers/server-a/request", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// TestRequest_BadBody rejects an unparsable body (REQ-ADM-003).
func TestRequest_BadBody(t *testing.T) {
	srv := newServer(t, "")
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/servers/server-a/request", bytes.NewReader([]byte("not-json")))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// TestRequest_UnknownServer returns 404 for an unregistered key (REQ-ADM-003).
func TestRequest_UnknownServer(t *testing.T) {
	srv := newServer(t, "")
	body := `{"endpoint":1}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/servers/bogus/request", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// TestMetrics_ContainsServer emits per-server telemetry in Prometheus text
// format (REQ-ADM-006).
func TestMetrics_ContainsServer(t *testing.T) {
	srv := newServer(t, "")
	srv.RecordRequest("server-a", true, nil, false)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "server-a") {
		t.Errorf("metrics body missing server-a: %s", body)
	}
}

// TestMetrics_AfterRecordRequest covers the error/deadline-miss metric
// lines emitted once a server has recorded telemetry (REQ-ADM-006,
// REQ-ADM-007).
func TestMetrics_AfterRecordRequest(t *testing.T) {
	srv := newServer(t, "")
	srv.RecordRequest("server-a", true, errors.New("e"), true)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{"rcp_server_error_total", "rcp_server_deadline_miss_total"} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics missing %q\n%s", want, body)
		}
	}
}

// TestRecordRequest_ReflectedInServerInfo covers RecordRequest, getOrCreate
// (both the create and existing-entry paths), and the populated branch of
// serverInfoFor (REQ-ADM-007).
func TestRecordRequest_ReflectedInServerInfo(t *testing.T) {
	srv := newServer(t, "")
	srv.RecordRequest("server-a", false, errors.New("boom"), true)
	srv.RecordRequest("server-a", false, nil, false) // second call hits the existing-entry path

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/servers/server-a", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if body := w.Body.String(); !strings.Contains(body, `"healthy":false`) {
		t.Errorf("server info = %s, want healthy:false after RecordRequest", body)
	}
}

// TestEvents_SSE streams a published health event as SSE (REQ-ADM-005).
func TestEvents_SSE(t *testing.T) {
	srv := newServer(t, "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	lines := make(chan string, 8)
	rw := &sseResponseWriter{header: make(http.Header), lines: lines}
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/events", nil)

	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.Handler().ServeHTTP(rw, req)
	}()

	// Give handler time to register the subscriber.
	time.Sleep(10 * time.Millisecond)
	srv.RecordRequest("server-a", true, nil, false)

	select {
	case line := <-lines:
		if !strings.HasPrefix(line, "data:") {
			t.Errorf("SSE line = %q, want prefix 'data:'", line)
		}
	case <-time.After(2 * time.Second):
		t.Error("timed out waiting for SSE event")
	}

	cancel()
	<-done
}

// TestEvents_MethodNotAllowed rejects a non-GET on /events (REQ-ADM-005).
func TestEvents_MethodNotAllowed(t *testing.T) {
	srv := newServer(t, "")
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/events", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

// TestRegister_DuplicateKey returns udp.ErrAlreadyExists (REQ-ADM-008).
func TestRegister_DuplicateKey(t *testing.T) {
	srv := admin.New(admin.Config{})
	first := dialedController(t)
	second := dialedController(t)
	if err := srv.Register("server-a", first); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := srv.Register("server-a", second); !errors.Is(err, udp.ErrAlreadyExists) {
		t.Errorf("err = %v, want ErrAlreadyExists", err)
	}
}

// TestConcurrent_RecordRequestAndHandlers is safe for concurrent
// RecordRequest and HTTP handler invocations (REQ-ADM-008).
func TestConcurrent_RecordRequestAndHandlers(t *testing.T) {
	srv := newServer(t, "")

	var wg sync.WaitGroup
	const n = 30
	wg.Add(n * 2)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			srv.RecordRequest("server-a", true, nil, false)
		}()
		go func() {
			defer wg.Done()
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/servers", nil)
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, req)
		}()
	}
	wg.Wait()
}

// sseResponseWriter captures each Flush boundary as a separate string.
type sseResponseWriter struct {
	header http.Header
	mu     sync.Mutex
	buf    bytes.Buffer
	lines  chan string
}

func (s *sseResponseWriter) Header() http.Header { return s.header }
func (s *sseResponseWriter) WriteHeader(_ int)   {}
func (s *sseResponseWriter) Write(b []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(b)
}
func (s *sseResponseWriter) Flush() {
	s.mu.Lock()
	data := s.buf.String()
	s.buf.Reset()
	s.mu.Unlock()
	for _, line := range strings.Split(data, "\n") {
		if strings.HasPrefix(line, "data:") {
			select {
			case s.lines <- line:
			default:
			}
		}
	}
}
