package restbridge_test

import (
	"net/http"
	"strings"
	"testing"

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/restbridge"
)

// handleSend must reject a malformed JSON body with 400.
func TestServer_Send_BadJSON(t *testing.T) {
	ts, _ := startTestServer(t, rcp.ZoneFrontLeft, nil)
	resp, err := http.Post(ts.URL+"/v1/zones/FrontLeft/send", "application/json", strings.NewReader("{not json")) //nolint:noctx
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

// A zone-name URL exercises the name-matching branch of parseZone, and routing
// the command to a zone the controller does not own surfaces the controller
// error path as 500.
func TestServer_Send_ControllerError(t *testing.T) {
	ts, _ := startTestServer(t, rcp.ZoneFrontLeft, nil)
	// Controller owns FrontLeft; a FrontRight command triggers ErrZoneMismatch.
	resp, err := http.Post(ts.URL+"/v1/zones/FrontRight/send", "application/json", strings.NewReader(`{"type":2}`)) //nolint:noctx
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
}

// GET /events with an out-of-range numeric zone is rejected with 422.
func TestServer_Events_UnknownZone(t *testing.T) {
	ts, _ := startTestServer(t, rcp.ZoneFrontLeft, nil)
	resp, err := http.Get(ts.URL + "/v1/zones/99/events") //nolint:noctx
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", resp.StatusCode)
	}
}

// Client Controller reports its configured zone.
func TestController_Zone(t *testing.T) {
	c := restbridge.NewController(rcp.ZoneRearRight, "http://127.0.0.1:0")
	defer func() { _ = c.Close() }()
	if c.Zone() != rcp.ZoneRearRight {
		t.Errorf("Zone() = %v, want ZoneRearRight", c.Zone())
	}
}
