//fusa:req REQ-REST-001
//fusa:req REQ-REST-002
//fusa:req REQ-REST-003
//fusa:req REQ-REST-004
//fusa:req REQ-REST-005
//fusa:req REQ-REST-006
//fusa:req REQ-REST-007
//fusa:req REQ-REST-008

// Package restbridge provides an HTTP/JSON + SSE bridge for go-RCP.
//
// Server exposes an rcp.Controller over HTTP so browser tooling and cloud
// services can send commands and stream status events without a gRPC client:
//
//	POST /v1/zones/{zone}/send  — JSON command delivery
//	GET  /v1/zones/{zone}/events — SSE status stream
//
// Controller implements rcp.Controller over HTTP, reaching a remote Bridge
// Server. Use NewController for the zone you want to control.
package restbridge

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	rcp "github.com/SoundMatt/go-RCP"
)

// ─── wire types ───────────────────────────────────────────────────────────────

// SendRequest is the JSON body for POST /v1/zones/{zone}/send.
type SendRequest struct {
	Type     rcp.CommandType `json:"type"`
	Priority rcp.Priority    `json:"priority"`
	Payload  []byte          `json:"payload,omitempty"`
}

// SendResponse is the JSON body returned by POST /v1/zones/{zone}/send.
type SendResponse struct {
	CommandID uint32             `json:"command_id"`
	Zone      rcp.Zone           `json:"zone"`
	Status    rcp.ResponseStatus `json:"status"`
	Payload   []byte             `json:"payload,omitempty"`
}

// ─── Server ───────────────────────────────────────────────────────────────────

// Server bridges HTTP requests to an rcp.Controller.
// Mount it on an http.ServeMux with Handler.
type Server struct {
	ctrl rcp.Controller
	mux  *http.ServeMux

	mu   sync.Mutex
	subs map[chan []byte]struct{}
}

// NewServer returns a Server wrapping ctrl.
func NewServer(ctrl rcp.Controller) *Server {
	s := &Server{
		ctrl: ctrl,
		mux:  http.NewServeMux(),
		subs: make(map[chan []byte]struct{}),
	}
	s.mux.HandleFunc("POST /v1/zones/{zone}/send", s.handleSend)
	s.mux.HandleFunc("GET /v1/zones/{zone}/events", s.handleEvents)
	return s
}

// Handler returns the http.Handler for this server.
func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	zoneStr := r.PathValue("zone")
	zone, err := parseZone(zoneStr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	var req SendRequest
	if decErr := json.NewDecoder(r.Body).Decode(&req); decErr != nil {
		http.Error(w, "invalid JSON body: "+decErr.Error(), http.StatusBadRequest)
		return
	}

	cmd := &rcp.Command{
		Zone:     zone,
		Type:     req.Type,
		Priority: req.Priority,
		Payload:  req.Payload,
	}
	resp, err := s.ctrl.Send(r.Context(), cmd)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(SendResponse{
		CommandID: resp.CommandID,
		Zone:      resp.Zone,
		Status:    resp.Status,
		Payload:   resp.Payload,
	})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	zoneStr := r.PathValue("zone")
	if _, err := parseZone(zoneStr); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	ch, err := s.ctrl.Subscribe(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	f.Flush()

	for {
		select {
		case st, ok := <-ch:
			if !ok {
				return
			}
			b, _ := json.Marshal(st)
			fmt.Fprintf(w, "data: %s\n\n", b)
			f.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// parseZone converts a zone string (numeric or name) to rcp.Zone.
func parseZone(s string) (rcp.Zone, error) {
	if n, err := strconv.Atoi(s); err == nil {
		z := rcp.Zone(n)
		if z < rcp.ZoneFrontLeft || z > rcp.ZoneCentral {
			return 0, fmt.Errorf("rcp/restbridge: zone %d out of range", n)
		}
		return z, nil
	}
	// try name
	for z := rcp.ZoneFrontLeft; z <= rcp.ZoneCentral; z++ {
		if z.String() == s {
			return z, nil
		}
	}
	return 0, fmt.Errorf("rcp/restbridge: unknown zone %q", s)
}

// ─── Client Controller ────────────────────────────────────────────────────────

// Controller implements rcp.Controller over HTTP, reaching a restbridge Server.
type Controller struct {
	zone    rcp.Zone
	baseURL string
	client  *http.Client
	nextID  atomic.Uint32
	closed  atomic.Bool
}

// NewController returns an rcp.Controller that talks to serverURL for zone.
// serverURL should be the base URL of the restbridge Server (e.g. "http://host:8080").
func NewController(zone rcp.Zone, serverURL string) *Controller {
	return &Controller{
		zone:    zone,
		baseURL: strings.TrimRight(serverURL, "/"),
		client:  &http.Client{},
	}
}

// Zone implements rcp.Controller.
func (c *Controller) Zone() rcp.Zone { return c.zone }

// Send implements rcp.Controller — POSTs the command and decodes the response.
func (c *Controller) Send(ctx context.Context, cmd *rcp.Command) (*rcp.Response, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("rcp/restbridge: zone %s: %w", c.zone, rcp.ErrClosed)
	}
	if cmd.Zone != c.zone {
		return nil, fmt.Errorf("rcp/restbridge: zone %s: %w", c.zone, rcp.ErrZoneMismatch)
	}

	body, err := json.Marshal(SendRequest{
		Type:     cmd.Type,
		Priority: cmd.Priority,
		Payload:  cmd.Payload,
	})
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/v1/zones/%d/send", c.baseURL, c.zone)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rcp/restbridge: Send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rcp/restbridge: Send: status %d", resp.StatusCode)
	}

	var sr SendResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("rcp/restbridge: Send decode: %w", err)
	}
	return &rcp.Response{
		CommandID: sr.CommandID,
		Zone:      sr.Zone,
		Status:    sr.Status,
		Payload:   sr.Payload,
	}, nil
}

// Subscribe implements rcp.Controller — opens an SSE stream and parses events.
func (c *Controller) Subscribe(ctx context.Context) (<-chan *rcp.Status, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("rcp/restbridge: zone %s: %w", c.zone, rcp.ErrClosed)
	}

	url := fmt.Sprintf("%s/v1/zones/%d/events", c.baseURL, c.zone)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.client.Do(req)
	if err != nil {
		if resp != nil {
			resp.Body.Close() //nolint:errcheck
		}
		return nil, fmt.Errorf("rcp/restbridge: Subscribe: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("rcp/restbridge: Subscribe: status %d", resp.StatusCode)
	}

	ch := make(chan *rcp.Status, 16)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var st rcp.Status
			if err := json.Unmarshal([]byte(line[6:]), &st); err != nil {
				continue
			}
			select {
			case ch <- &st:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}

// Close implements rcp.Controller — idempotent.
func (c *Controller) Close() error {
	c.closed.CompareAndSwap(false, true)
	return nil
}
