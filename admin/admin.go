//fusa:req REQ-ADM-001
//fusa:req REQ-ADM-002
//fusa:req REQ-ADM-003
//fusa:req REQ-ADM-004
//fusa:req REQ-ADM-005
//fusa:req REQ-ADM-006
//fusa:req REQ-ADM-007
//fusa:req REQ-ADM-008

// Package admin provides an HTTP admin interface for runtime registry inspection.
//
// Endpoints:
//
//	GET  /zones                  — list all registered zones
//	GET  /zones/{zone}           — single-zone detail
//	POST /zones/{zone}/send      — send a command (Bearer auth required)
//	GET  /events                 — SSE stream of health/power events
//	GET  /metrics                — Prometheus text format metrics
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

	rcp "github.com/SoundMatt/go-RCP"
)

// ZoneInfo is the JSON body returned by GET /zones/{zone}.
type ZoneInfo struct {
	Zone      string    `json:"zone"`
	Healthy   bool      `json:"healthy"`
	LastSeen  time.Time `json:"last_seen"`
	CmdRate   float64   `json:"cmd_rate_per_sec"`
	ErrCount  int64     `json:"error_count"`
	DeadMiss  int64     `json:"deadline_miss_count"`
}

// Event is a single server-sent event delivered on GET /events.
type Event struct {
	Type    string      `json:"type"`
	Zone    string      `json:"zone"`
	Payload interface{} `json:"payload,omitempty"`
}

// zoneState is live per-zone telemetry.
type zoneState struct {
	mu       sync.RWMutex
	healthy  bool
	lastSeen time.Time
	cmdCount atomic.Int64
	errCount atomic.Int64
	deadMiss atomic.Int64
}

// Server is the HTTP admin server.
type Server struct {
	registry rcp.Registry
	bearer   string // required token for write endpoints; empty = no auth

	mu    sync.RWMutex
	zones map[rcp.Zone]*zoneState
	subs  map[chan Event]struct{}
}

// Config configures the admin server.
type Config struct {
	// BearerToken, if non-empty, is required on POST /zones/{zone}/send.
	BearerToken string
}

// New creates an admin Server wrapping registry.
func New(registry rcp.Registry, cfg Config) *Server {
	return &Server{
		registry: registry,
		bearer:   cfg.BearerToken,
		zones:    make(map[rcp.Zone]*zoneState),
		subs:     make(map[chan Event]struct{}),
	}
}

// Handler returns an http.Handler for mounting. Safe to call multiple times.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/zones", s.handleZoneList)
	mux.HandleFunc("/zones/", s.handleZone)
	mux.HandleFunc("/events", s.handleEvents)
	mux.HandleFunc("/metrics", s.handleMetrics)
	return mux
}

// RecordSend updates telemetry for a completed Send. Call this from your
// transport or observability layer after each Send returns.
func (s *Server) RecordSend(zone rcp.Zone, healthy bool, err error, deadlineMiss bool) {
	z := s.getOrCreate(zone)
	z.cmdCount.Add(1)
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

	s.publish(Event{Type: "health", Zone: zone.String(), Payload: map[string]bool{"healthy": healthy}})
}

func (s *Server) getOrCreate(zone rcp.Zone) *zoneState {
	s.mu.Lock()
	defer s.mu.Unlock()
	if z, ok := s.zones[zone]; ok {
		return z
	}
	z := &zoneState{lastSeen: time.Now()}
	s.zones[zone] = z
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

// handleZoneList serves GET /zones.
func (s *Server) handleZoneList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctrls := s.registry.Controllers()
	infos := make([]ZoneInfo, 0, len(ctrls))
	for _, ctrl := range ctrls {
		infos = append(infos, s.zoneInfoFor(ctrl.Zone()))
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(infos) //nolint:errcheck
}

// handleZone serves GET /zones/{zone} and POST /zones/{zone}/send.
func (s *Server) handleZone(w http.ResponseWriter, r *http.Request) {
	// path: /zones/{zone} or /zones/{zone}/send
	path := strings.TrimPrefix(r.URL.Path, "/zones/")
	parts := strings.SplitN(path, "/", 2)
	zone, err := parseZone(parts[0])
	if err != nil {
		http.Error(w, "unknown zone", http.StatusNotFound)
		return
	}

	if len(parts) == 2 && parts[1] == "send" {
		s.handleSend(w, r, zone)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if _, err := s.registry.Lookup(zone); err != nil {
		http.Error(w, "zone not found", http.StatusNotFound)
		return
	}

	info := s.zoneInfoFor(zone)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info) //nolint:errcheck
}

// handleSend serves POST /zones/{zone}/send.
func (s *Server) handleSend(w http.ResponseWriter, r *http.Request, zone rcp.Zone) {
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

	var body struct {
		Type     rcp.CommandType `json:"type"`
		Priority rcp.Priority    `json:"priority"`
		Payload  []byte          `json:"payload"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	ctrl, err := s.registry.Lookup(zone)
	if err != nil {
		http.Error(w, "zone not found", http.StatusNotFound)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	resp, err := ctrl.Send(ctx, &rcp.Command{
		Zone:     zone,
		Type:     body.Type,
		Priority: body.Priority,
		Payload:  body.Payload,
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("send error: %v", err), http.StatusBadGateway)
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
	for zone, z := range s.zones {
		healthy := 0
		if z.healthy {
			healthy = 1
		}
		fmt.Fprintf(w, "rcp_zone_healthy{zone=%q} %d\n", zone.String(), healthy)          //nolint:errcheck
		fmt.Fprintf(w, "rcp_zone_cmd_total{zone=%q} %d\n", zone.String(), z.cmdCount.Load()) //nolint:errcheck
		fmt.Fprintf(w, "rcp_zone_error_total{zone=%q} %d\n", zone.String(), z.errCount.Load()) //nolint:errcheck
		fmt.Fprintf(w, "rcp_zone_deadline_miss_total{zone=%q} %d\n", zone.String(), z.deadMiss.Load()) //nolint:errcheck
	}
}

func (s *Server) zoneInfoFor(zone rcp.Zone) ZoneInfo {
	s.mu.RLock()
	z, ok := s.zones[zone]
	s.mu.RUnlock()

	info := ZoneInfo{Zone: zone.String(), Healthy: true, LastSeen: time.Now()}
	if ok {
		z.mu.RLock()
		info.Healthy = z.healthy
		info.LastSeen = z.lastSeen
		z.mu.RUnlock()
		info.ErrCount = z.errCount.Load()
		info.DeadMiss = z.deadMiss.Load()
	}
	return info
}

func parseZone(s string) (rcp.Zone, error) {
	switch s {
	case "front-left":
		return rcp.ZoneFrontLeft, nil
	case "front-right":
		return rcp.ZoneFrontRight, nil
	case "rear-left":
		return rcp.ZoneRearLeft, nil
	case "rear-right":
		return rcp.ZoneRearRight, nil
	case "central":
		return rcp.ZoneCentral, nil
	default:
		return rcp.ZoneUnknown, fmt.Errorf("unknown zone %q", s)
	}
}
