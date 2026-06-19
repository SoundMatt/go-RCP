package admin_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
)

// TestParseZone_AllNames drives every named zone through handleZone so the
// parseZone switch is fully exercised (each returns 200 for a registered zone).
func TestParseZone_AllNames(t *testing.T) {
	srv, _ := newServer(t, "")
	for _, name := range []string{"front-left", "front-right", "rear-left", "rear-right", "central"} {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/zones/"+name, nil)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("GET /zones/%s status = %d, want 200", name, w.Code)
		}
	}
}

// TestRecordSend_ReflectedInZoneInfo covers RecordSend, getOrCreate (both the
// create and existing-entry paths), and the populated branch of zoneInfoFor.
func TestRecordSend_ReflectedInZoneInfo(t *testing.T) {
	srv, _ := newServer(t, "")
	srv.RecordSend(rcp.ZoneFrontLeft, false, errors.New("boom"), true)
	srv.RecordSend(rcp.ZoneFrontLeft, false, nil, false) // second call hits the existing-entry path

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/zones/front-left", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if body := w.Body.String(); !strings.Contains(body, `"healthy":false`) {
		t.Errorf("zone info = %s, want healthy:false after RecordSend", body)
	}
}

// TestMetrics_AfterRecordSend covers the per-zone metric lines emitted once a
// zone has recorded telemetry (error and deadline-miss counters).
func TestMetrics_AfterRecordSend(t *testing.T) {
	srv, _ := newServer(t, "")
	srv.RecordSend(rcp.ZoneCentral, true, errors.New("e"), true)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{"rcp_zone_error_total", "rcp_zone_deadline_miss_total"} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics missing %q\n%s", want, body)
		}
	}
}

// TestEvents_StreamsAndClosesOnContext covers handleEvents: it registers an SSE
// subscriber, streams a published event, and unwinds when the request context
// is cancelled.
func TestEvents_StreamsAndClosesOnContext(t *testing.T) {
	srv, _ := newServer(t, "")
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	// Publish an event shortly after the handler subscribes.
	go func() {
		time.Sleep(20 * time.Millisecond)
		srv.RecordSend(rcp.ZoneFrontLeft, true, nil, false)
	}()

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/events", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req) // returns when ctx expires

	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "event-stream") {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
}

// TestEvents_MethodNotAllowed covers the non-GET rejection branch of handleEvents.
func TestEvents_MethodNotAllowed(t *testing.T) {
	srv, _ := newServer(t, "")
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/events", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}
