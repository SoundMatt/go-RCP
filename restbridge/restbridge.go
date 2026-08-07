// Package restbridge provides an HTTP/JSON + SSE bridge for go-RCP, for the
// OPEN Alliance TC18 Remote Control Protocol (RCP), as described by the
// "OPEN Alliance TC18 Remote Control Protocol Specification v0.5.1_RC".
//
// This is ROADMAP.md Milestone 56 (v0.69.0)'s ADAPT-flagged rebuild: per
// Phase 17's disposition table, the reasoning is identical to grpcbridge's —
// browser/cloud HTTP access to an RC Server is orthogonal to TC18 RCP and
// stays genuinely necessary, re-pointed at the new Controller-equivalent
// interface, *udp.Controller (see grpcbridge's own package doc comment for
// the fuller rationale; this package's Server/Controller shape mirrors
// grpcbridge's exactly, one HTTP/SSE transport standing in for one gRPC
// connection):
//
//	POST /v1/endpoints/{addr}/request — JSON request delivery
//	GET  /v1/telemetry               — SSE telemetry stream
//
// Server exposes an upstream *udp.Controller over HTTP; Controller reaches
// a remote restbridge Server over HTTP, presenting the same
// Request/Read/Write surface a *udp.Controller does.
package restbridge

//fusa:req REQ-REST-001
//fusa:req REQ-REST-002
//fusa:req REQ-REST-003
//fusa:req REQ-REST-004
//fusa:req REQ-REST-005
//fusa:req REQ-REST-006
//fusa:req REQ-REST-007
//fusa:req REQ-REST-008

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/udp"
)

// ErrClosed is returned by Controller methods once Close has been called.
var ErrClosed = errors.New("rcp/restbridge: closed")

// ─── wire types ───────────────────────────────────────────────────────────────

// RequestBody is the JSON body for POST /v1/endpoints/{addr}/request.
type RequestBody struct {
	Control acf.ControlFlags `json:"control"`
	Body    []byte           `json:"body,omitempty"`
}

// ResponseBody is the JSON body returned by POST /v1/endpoints/{addr}/request.
type ResponseBody struct {
	ByteBusID      avtp.ByteBusID      `json:"byte_bus_id"`
	TransactionNum avtp.TransactionNum `json:"transaction_num"`
	Control        acf.ControlFlags    `json:"control"`
	Body           []byte              `json:"body,omitempty"`
}

// TelemetryEvent is streamed by GET /v1/telemetry (see Server.PublishTelemetry).
type TelemetryEvent struct {
	ByteBusID avtp.ByteBusID   `json:"byte_bus_id"`
	Control   acf.ControlFlags `json:"control"`
	Body      []byte           `json:"body,omitempty"`
}

// ─── Server ───────────────────────────────────────────────────────────────────

// Server bridges HTTP requests to an upstream *udp.Controller.
// Mount it on an http.ServeMux with Handler.
type Server struct {
	upstream *udp.Controller
	mux      *http.ServeMux

	mu   sync.Mutex
	subs map[chan *TelemetryEvent]struct{}
}

// NewServer returns a Server forwarding requests to upstream.
func NewServer(upstream *udp.Controller) *Server {
	s := &Server{
		upstream: upstream,
		mux:      http.NewServeMux(),
		subs:     make(map[chan *TelemetryEvent]struct{}),
	}
	s.mux.HandleFunc("POST /v1/endpoints/{addr}/request", s.handleRequest)
	s.mux.HandleFunc("GET /v1/telemetry", s.handleTelemetry)
	return s
}

// Handler returns the http.Handler for this server.
func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	addr, err := parseAddr(r.PathValue("addr"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	var req RequestBody
	if decErr := json.NewDecoder(r.Body).Decode(&req); decErr != nil {
		http.Error(w, "invalid JSON body: "+decErr.Error(), http.StatusBadRequest)
		return
	}

	resp, err := s.upstream.Request(r.Context(), addr, req.Control, req.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ResponseBody{
		ByteBusID:      resp.ByteBusID,
		TransactionNum: resp.TransactionNum,
		Control:        resp.Control,
		Body:           resp.Body,
	})
}

func (s *Server) handleTelemetry(w http.ResponseWriter, r *http.Request) {
	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	ch := make(chan *TelemetryEvent, 16)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.subs, ch)
		s.mu.Unlock()
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	f.Flush()

	for {
		select {
		case ev := <-ch:
			b, _ := json.Marshal(ev)
			fmt.Fprintf(w, "data: %s\n\n", b) //nolint:errcheck
			f.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// PublishTelemetry fans ev out to every currently connected SSE stream. A
// caller obtains ev however it likes, mirroring grpcbridge.Server's own
// caller-driven PublishTelemetry.
func (s *Server) PublishTelemetry(ev *TelemetryEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for ch := range s.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

// parseAddr converts a URL path segment to an avtp.ByteBusID.
func parseAddr(s string) (avtp.ByteBusID, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 || n > 0xFF {
		return 0, fmt.Errorf("rcp/restbridge: invalid endpoint address %q", s)
	}
	return avtp.ByteBusID(n), nil //nolint:gosec // bounds checked above
}

// ─── Client Controller ────────────────────────────────────────────────────────

// Controller reaches a remote restbridge Server over HTTP, presenting the
// same Request/Read/Write surface a *udp.Controller does.
type Controller struct {
	baseURL string
	client  *http.Client
	closed  atomic.Bool
}

// NewController returns a Controller that talks to serverURL (the base URL
// of a restbridge Server, e.g. "http://host:8080").
func NewController(serverURL string) *Controller {
	return &Controller{
		baseURL: strings.TrimRight(serverURL, "/"),
		client:  &http.Client{},
	}
}

// Request POSTs one request to addr and decodes the response.
func (c *Controller) Request(ctx context.Context, addr avtp.ByteBusID, control acf.ControlFlags, body []byte) (acf.Message, error) {
	if c.closed.Load() {
		return acf.Message{}, fmt.Errorf("rcp/restbridge: %w", ErrClosed)
	}

	reqBody, err := json.Marshal(RequestBody{Control: control, Body: body})
	if err != nil {
		return acf.Message{}, err
	}

	url := fmt.Sprintf("%s/v1/endpoints/%d/request", c.baseURL, addr)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return acf.Message{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return acf.Message{}, fmt.Errorf("rcp/restbridge: Request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return acf.Message{}, fmt.Errorf("rcp/restbridge: Request: status %d", resp.StatusCode)
	}

	var rb ResponseBody
	if err := json.NewDecoder(resp.Body).Decode(&rb); err != nil {
		return acf.Message{}, fmt.Errorf("rcp/restbridge: Request decode: %w", err)
	}
	return acf.Message{
		ByteBusID:      rb.ByteBusID,
		TransactionNum: rb.TransactionNum,
		Control:        rb.Control,
		Body:           rb.Body,
	}, nil
}

// Read is Request with acf.FlagRead set and no body.
func (c *Controller) Read(ctx context.Context, addr avtp.ByteBusID) (acf.Message, error) {
	return c.Request(ctx, addr, acf.FlagRead, nil)
}

// Write is Request with acf.FlagWrite set and the given body.
func (c *Controller) Write(ctx context.Context, addr avtp.ByteBusID, body []byte) (acf.Message, error) {
	return c.Request(ctx, addr, acf.FlagWrite, body)
}

// Subscribe opens an SSE stream against /v1/telemetry and parses events
// published by the remote Server's PublishTelemetry.
func (c *Controller) Subscribe(ctx context.Context) (<-chan *TelemetryEvent, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("rcp/restbridge: %w", ErrClosed)
	}

	url := c.baseURL + "/v1/telemetry"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.client.Do(req) //nolint:bodyclose // body transferred to SSE goroutine
	if err != nil {
		if resp != nil {
			_ = resp.Body.Close()
		}
		return nil, fmt.Errorf("rcp/restbridge: Subscribe: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("rcp/restbridge: Subscribe: status %d", resp.StatusCode)
	}

	ch := make(chan *TelemetryEvent, 16)
	go func() {
		defer close(ch)
		defer func() { _ = resp.Body.Close() }()
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var ev TelemetryEvent
			if err := json.Unmarshal([]byte(line[6:]), &ev); err != nil {
				continue
			}
			select {
			case ch <- &ev:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}

// Close marks the Controller closed. Idempotent.
func (c *Controller) Close() error {
	c.closed.CompareAndSwap(false, true)
	return nil
}
