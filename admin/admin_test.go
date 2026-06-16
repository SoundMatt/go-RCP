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
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/admin"
	"github.com/SoundMatt/go-RCP/mock"
)

func newServer(t *testing.T, bearer string) (*admin.Server, *mock.Registry) {
	t.Helper()
	reg := mock.NewRegistry()
	t.Cleanup(func() { reg.Close() })
	srv := admin.New(reg, admin.Config{BearerToken: bearer})
	return srv, reg
}

func TestGetZones_ReturnsAllZones(t *testing.T) {
	srv, _ := newServer(t, "")
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/zones", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var zones []admin.ZoneInfo
	if err := json.NewDecoder(w.Body).Decode(&zones); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(zones) == 0 {
		t.Error("expected non-empty zone list")
	}
}

func TestGetZones_MethodNotAllowed(t *testing.T) {
	srv, _ := newServer(t, "")
	req := httptest.NewRequest(http.MethodPost, "/zones", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestGetZone_KnownZone(t *testing.T) {
	srv, _ := newServer(t, "")
	req := httptest.NewRequest(http.MethodGet, "/zones/front-left", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var info admin.ZoneInfo
	if err := json.NewDecoder(w.Body).Decode(&info); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if info.Zone != "front-left" {
		t.Errorf("zone = %q, want front-left", info.Zone)
	}
}

func TestGetZone_UnknownZone(t *testing.T) {
	srv, _ := newServer(t, "")
	req := httptest.NewRequest(http.MethodGet, "/zones/bogus", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestSend_NoAuth(t *testing.T) {
	srv, _ := newServer(t, "")
	body := `{"type":1,"priority":0}`
	req := httptest.NewRequest(http.MethodPost, "/zones/front-left/send", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — body: %s", w.Code, w.Body.String())
	}
	var resp rcp.Response
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != rcp.StatusOK {
		t.Errorf("resp status = %v, want OK", resp.Status)
	}
}

func TestSend_BearerRequired(t *testing.T) {
	srv, _ := newServer(t, "secret")
	body := `{"type":1}`
	req := httptest.NewRequest(http.MethodPost, "/zones/front-left/send", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestSend_BearerAccepted(t *testing.T) {
	srv, _ := newServer(t, "secret")
	body := `{"type":1}`
	req := httptest.NewRequest(http.MethodPost, "/zones/front-left/send", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestSend_BadBody(t *testing.T) {
	srv, _ := newServer(t, "")
	req := httptest.NewRequest(http.MethodPost, "/zones/front-left/send", bytes.NewReader([]byte("not-json")))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestSend_UnknownZone(t *testing.T) {
	srv, _ := newServer(t, "")
	body := `{"type":1}`
	req := httptest.NewRequest(http.MethodPost, "/zones/bogus/send", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestMetrics_ContainsZone(t *testing.T) {
	srv, _ := newServer(t, "")
	srv.RecordSend(rcp.ZoneFrontLeft, true, nil, false)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "front-left") {
		t.Errorf("metrics body missing front-left zone: %s", body)
	}
}

func TestEvents_SSE(t *testing.T) {
	srv, _ := newServer(t, "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	lines := make(chan string, 8)
	rw := &sseResponseWriter{header: make(http.Header), lines: lines}
	req := httptest.NewRequest(http.MethodGet, "/events", nil).WithContext(ctx)

	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.Handler().ServeHTTP(rw, req)
	}()

	// Give handler time to register the subscriber.
	time.Sleep(10 * time.Millisecond)
	srv.RecordSend(rcp.ZoneCentral, true, nil, false)

	// Wait for the event line or timeout.
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
