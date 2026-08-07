//fusa:req REQ-ADM-001
//fusa:req REQ-ADM-002
//fusa:req REQ-ADM-003
//fusa:req REQ-ADM-004
//fusa:req REQ-ADM-005
//fusa:req REQ-ADM-006
//fusa:req REQ-ADM-007
//fusa:req REQ-ADM-008

// Package admin provides an HTTP admin interface for runtime *udp.Controller
// inspection, for the OPEN Alliance TC18 Remote Control Protocol (RCP), as
// described by the "OPEN Alliance TC18 Remote Control Protocol
// Specification v0.5.1_RC".
//
// This is ROADMAP.md Milestone 55 (v0.68.0)'s ADAPT-flagged rebuild: per
// Phase 17's disposition table, "HTTP inspection surface is reusable; the
// data model moves from zones to servers/endpoints." A caller Registers
// each *udp.Controller it wants inspectable under a caller-chosen key —
// the same "re-key ownership by server/endpoint identity instead of Zone"
// pattern federation and udp.Registry already establish (see
// udp/registry.go) — in place of the retired rcp.Zone enum.
//
// Endpoints:
//
//	GET  /servers                 — list all registered servers
//	GET  /servers/{key}           — single-server detail
//	POST /servers/{key}/request   — issue a request (Bearer auth required)
//	GET  /events                  — SSE stream of health events
//	GET  /metrics                 — Prometheus text format metrics
package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/udp"
)

// ServerInfo is the JSON body returned by GET /servers/{key}.
type ServerInfo struct {
	Key               string    `json:"key"`
	StreamID          string    `json:"stream_id"`
	Healthy           bool      `json:"healthy"`
	LastSeen          time.Time `json:"last_seen"`
	RequestCount      int64     `json:"request_count"`
	ErrorCount        int64     `json:"error_count"`
	DeadlineMissCount int64     `json:"deadline_miss_count"`
}

// Event is a single server-sent event delivered on GET /events.
type Event struct {
	Type    string      `json:"type"`
	Key     string      `json:"key"`
	Payload interface{} `json:"payload,omitempty"`
}

// serverState is live per-server telemetry.
type serverState struct {
	mu       sync.RWMutex
	healthy  bool
	lastSeen time.Time
	reqCount atomic.Int64
	errCount atomic.Int64
	deadMiss atomic.Int64
}

// Server is the HTTP admin server.
type Server struct {
	bearer string // required token for write endpoints; empty = no auth

	mu     sync.RWMutex
	ctrls  map[string]*udp.Controller
	states map[string]*serverState
	subs   map[chan Event]struct{}
}

// Config configures the admin server.
type Config struct {
	// BearerToken, if non-empty, is required on POST /servers/{key}/request.
	BearerToken string
}

// New creates an empty admin Server. Callers Register each *udp.Controller
// they want inspectable.
func New(cfg Config) *Server {
	return &Server{
		bearer: cfg.BearerToken,
		ctrls:  make(map[string]*udp.Controller),
		states: make(map[string]*serverState),
		subs:   make(map[chan Event]struct{}),
	}
}

// Register makes ctrl inspectable and requestable under key. Returns
// udp.ErrAlreadyExists if key is already registered.
func (s *Server) Register(key string, ctrl *udp.Controller) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.ctrls[key]; ok {
		return fmt.Errorf("rcp/admin: server key %s: %w", key, udp.ErrAlreadyExists)
	}
	s.ctrls[key] = ctrl
	return nil
}

// Deregister removes key from this Server's inspectable set. It is a no-op
// for a key with nothing registered.
func (s *Server) Deregister(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.ctrls, key)
}

// Handler returns an http.Handler for mounting. Safe to call multiple times.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/servers", s.handleServerList)
	mux.HandleFunc("/servers/", s.handleServer)
	mux.HandleFunc("/events", s.handleEvents)
	mux.HandleFunc("/metrics", s.handleMetrics)
	return mux
}

// RecordRequest updates telemetry for a completed Request. Call this from
// your transport or observability layer after each Request returns.
func (s *Server) RecordRequest(key string, healthy bool, err error, deadlineMiss bool) {
	z := s.getOrCreate(key)
	z.reqCount.Add(1)
	if err != nil {
		z.errCount.Add(1)
	}
	if deadlineMiss {
		z.deadMiss.Add(1)
	}
	z.mu.Lock()
	z.healthy = healthy
	z.lastSeen = time.Now()
	z.mu.Unlock()

	s.publish(Event{Type: "health", Key: key, Payload: map[string]bool{"healthy": healthy}})
}

func (s *Server) getOrCreate(key string) *serverState {
	s.mu.Lock()
	defer s.mu.Unlock()
	if z, ok := s.states[key]; ok {
		return z
	}
	z := &serverState{lastSeen: time.Now()}
	s.states[key] = z
	return z
}

func (s *Server) publish(e Event) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for ch := range s.subs {
		select {
		case ch <- e:
		default:
		}
	}
}

// handleServerList serves GET /servers.
func (s *Server) handleServerList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.RLock()
	keys := make([]string, 0, len(s.ctrls))
	for k := range s.ctrls {
		keys = append(keys, k)
	}
	s.mu.RUnlock()

	infos := make([]ServerInfo, 0, len(keys))
	for _, k := range keys {
		infos = append(infos, s.serverInfoFor(k))
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(infos) //nolint:errcheck
}

// handleServer serves GET /servers/{key} and POST /servers/{key}/request.
func (s *Server) handleServer(w http.ResponseWriter, r *http.Request) {
	// path: /servers/{key} or /servers/{key}/request
	path := strings.TrimPrefix(r.URL.Path, "/servers/")
	parts := strings.SplitN(path, "/", 2)
	key := parts[0]

	s.mu.RLock()
	_, known := s.ctrls[key]
	s.mu.RUnlock()
	if !known {
		http.Error(w, "unknown server", http.StatusNotFound)
		return
	}

	if len(parts) == 2 && parts[1] == "request" {
		s.handleRequest(w, r, key)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	info := s.serverInfoFor(key)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info) //nolint:errcheck
}

// requestBody is the JSON body POST /servers/{key}/request accepts.
type requestBody struct {
	Endpoint avtp.ByteBusID `json:"endpoint"`
	Write    bool           `json:"write"`
	Body     []byte         `json:"body"` // base64 in JSON
}

// handleRequest serves POST /servers/{key}/request.
func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request, key string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.bearer != "" {
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if tok != s.bearer {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	var body requestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	ctrl := s.ctrls[key]
	s.mu.RUnlock()

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	control := acf.FlagRead
	if body.Write {
		control = acf.FlagWrite
	}
	resp, err := ctrl.Request(ctx, body.Endpoint, control, body.Body)
	s.RecordRequest(key, err == nil, err, false)
	if err != nil {
		http.Error(w, fmt.Sprintf("request error: %v", err), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}

// handleEvents serves GET /events as SSE.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	ch := make(chan Event, 64)
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

	for {
		select {
		case <-r.Context().Done():
			return
		case e := <-ch:
			data, _ := json.Marshal(e)
			fmt.Fprintf(w, "data: %s\n\n", data) //nolint:errcheck
			flusher.Flush()
		}
	}
}

// handleMetrics serves GET /metrics in Prometheus text format.
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	for key, z := range s.states {
		healthy := 0
		if z.healthy {
			healthy = 1
		}
		fmt.Fprintf(w, "rcp_server_healthy{key=%q} %d\n", key, healthy)                       //nolint:errcheck
		fmt.Fprintf(w, "rcp_server_request_total{key=%q} %d\n", key, z.reqCount.Load())       //nolint:errcheck
		fmt.Fprintf(w, "rcp_server_error_total{key=%q} %d\n", key, z.errCount.Load())         //nolint:errcheck
		fmt.Fprintf(w, "rcp_server_deadline_miss_total{key=%q} %d\n", key, z.deadMiss.Load()) //nolint:errcheck
	}
}

func (s *Server) serverInfoFor(key string) ServerInfo {
	s.mu.RLock()
	ctrl := s.ctrls[key]
	z, ok := s.states[key]
	s.mu.RUnlock()

	info := ServerInfo{Key: key, Healthy: true, LastSeen: time.Now()}
	if ctrl != nil {
		info.StreamID = ctrl.StreamID().String()
	}
	if ok {
		z.mu.RLock()
		info.Healthy = z.healthy
		info.LastSeen = z.lastSeen
		z.mu.RUnlock()
		info.RequestCount = z.reqCount.Load()
		info.ErrorCount = z.errCount.Load()
		info.DeadlineMissCount = z.deadMiss.Load()
	}
	return info
}
