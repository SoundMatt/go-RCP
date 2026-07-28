package restbridge_test

import (
	"net/http"
	"strings"
	"testing"
)

// handleRequest must reject a malformed JSON body with 400.
func TestServer_Request_BadJSON(t *testing.T) {
	ts, _ := startTestServer(t)
	resp, err := http.Post(ts.URL+"/v1/endpoints/1/request", "application/json", strings.NewReader("{not json")) //nolint:noctx
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

// A negative or over-255 endpoint address is rejected with 422.
func TestServer_Request_AddrOutOfRange(t *testing.T) {
	ts, _ := startTestServer(t)
	resp, err := http.Post(ts.URL+"/v1/endpoints/999/request", "application/json", strings.NewReader(`{"control":64}`)) //nolint:noctx
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", resp.StatusCode)
	}
}
