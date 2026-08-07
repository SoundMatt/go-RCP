// Package proxy provides a transparent RCP-level proxy for multi-hop
// topologies, for the OPEN Alliance TC18 Remote Control Protocol (RCP), as
// described by the "OPEN Alliance TC18 Remote Control Protocol
// Specification v0.5.1_RC".
//
// This is ROADMAP.md Milestone 55 (v0.68.0)'s ADAPT-flagged rebuild: per
// Phase 17's disposition table, "the intercept/transform/forward pattern is
// reusable, but a real RCP-level proxy must handle stream_id/byte_bus_id
// remapping — an area the specification itself flags as a client-side
// responsibility with no server-side safety net (Phase 13). Rebuild
// carefully, not mechanically."
//
// The retired package wrapped one upstream rcp.Controller and presented the
// same interface downstream, forwarding a *rcp.Command verbatim (its Zone
// field carried the only addressing information a caller could rewrite).
// That shape does not survive: a downstream caller in the new model is not
// another Controller wrapper chained in front of this one — it is a
// requester stream sending a request.Handler-shaped call into whatever
// routes to this proxy (typically a udp.Router.Register slot). So Handler,
// this package's replacement for the old Controller, implements
// request.Handler itself:
//
//   - byte_bus_id remapping: TransformFunc, if set, may rewrite the
//     inbound request's addr (and/or its body) before Handler forwards it
//     to the upstream *udp.Controller.
//   - stream_id remapping: Handler forwards through its own upstream
//     *udp.Controller, which presents that Controller's own avtp.StreamID
//     identity on the wire — never the original downstream requester's.
//     The downstream requester's identity is available to TransformFunc
//     (for logging/policy decisions) but is never itself put on the wire
//     upstream; this is the deliberate "gateway," not "relay," posture the
//     disposition table's own "no server-side safety net" warning is about
//     getting right.
//
// The response Handler returns is repackaged to correlate with the
// *original* downstream request (its own Kind/ByteBusID/TransactionNum),
// exactly like every other Router-facing Handler in this repo (see e.g.
// udp/ep0.go's responseFor) — the upstream remapping is entirely invisible
// to the downstream caller.
package proxy

//fusa:req REQ-PX-001
//fusa:req REQ-PX-002
//fusa:req REQ-PX-003
//fusa:req REQ-PX-004
//fusa:req REQ-PX-005
//fusa:req REQ-PX-006
//fusa:req REQ-PX-007
//fusa:req REQ-PX-008

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/udp"
)

// ErrClosed is returned when HandleRequest is called after Close.
var ErrClosed = errors.New("rcp/proxy: handler closed")

// DefaultForwardTimeout bounds how long Handler waits for the upstream
// controller's response, since request.Handler's own signature carries no
// per-call context a downstream Router.Route could hand it (see
// request.Handler's doc comment).
const DefaultForwardTimeout = 5 * time.Second

// TransformFunc may rewrite the byte_bus_id and/or body of an inbound
// request before Handler forwards it upstream. It receives the original
// downstream requester's avtp.StreamID (for logging or policy decisions —
// never itself forwarded upstream, see the package doc comment), the
// request's original addr, control flags, and body, and must return the
// (possibly rewritten) addr and body to forward, or an error to abort the
// forward. A nil TransformFunc forwards addr and body unchanged.
type TransformFunc func(requester avtp.StreamID, addr avtp.ByteBusID, control acf.ControlFlags, body []byte) (avtp.ByteBusID, []byte, error)

// Handler implements request.Handler by forwarding every request it
// receives to an upstream *udp.Controller, presenting that Controller's own
// StreamID identity rather than the original downstream requester's. It may
// be registered directly into a udp.Router (via Router.Register) the same
// as any native endpoint's Handler.
type Handler struct {
	upstream  *udp.Controller
	transform TransformFunc
	timeout   time.Duration
	closed    atomic.Bool
}

// NewHandler creates a Handler forwarding to upstream. If transform is nil,
// addr and body are forwarded unchanged. timeout bounds each forwarded
// request; a non-positive timeout uses DefaultForwardTimeout.
func NewHandler(upstream *udp.Controller, transform TransformFunc, timeout time.Duration) *Handler {
	if timeout <= 0 {
		timeout = DefaultForwardTimeout
	}
	return &Handler{upstream: upstream, transform: transform, timeout: timeout}
}

// HandleRequest implements request.Handler: it optionally transforms req's
// addr/body, forwards the result to the upstream controller, and repackages
// the upstream response to correlate with req (the original downstream
// request), not the (possibly different) upstream request actually sent.
func (h *Handler) HandleRequest(requester avtp.StreamID, req acf.Message) (acf.Message, error) {
	if h.closed.Load() {
		return acf.Message{}, ErrClosed
	}

	addr, body := req.ByteBusID, req.Body
	if h.transform != nil {
		var err error
		addr, body, err = h.transform(requester, req.ByteBusID, req.Control, req.Body)
		if err != nil {
			return acf.Message{}, fmt.Errorf("rcp/proxy: transform: %w", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), h.timeout)
	defer cancel()

	resp, err := h.upstream.Request(ctx, addr, req.Control, body)
	if err != nil {
		return acf.Message{}, err
	}

	return acf.Message{
		Kind:           req.Kind,
		ByteBusID:      req.ByteBusID,
		TransactionNum: req.TransactionNum,
		Control:        resp.Control,
		Pad:            resp.Pad,
		Body:           resp.Body,
	}, nil
}

// Upstream returns the upstream controller this Handler forwards to.
func (h *Handler) Upstream() *udp.Controller { return h.upstream }

// Close closes the upstream controller. Safe to call multiple times.
func (h *Handler) Close() error {
	if !h.closed.CompareAndSwap(false, true) {
		return nil
	}
	return h.upstream.Close()
}
