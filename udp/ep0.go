package udp

import (
	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/regmap"
	"github.com/SoundMatt/go-RCP/server"
)

// EP0Handler adapts a *server.Server's already-shipped configuration/
// discovery surface (ROADMAP.md Milestones 45-46) to the wire-level
// avtp.Header + acf.Message shape Router.Route hands EP0 requests. It is
// the first place in this repo any package turns those Go-level server.Server
// calls into on-wire RCP-over-ACF traffic; every earlier milestone's own
// test suite called server.Server's methods directly (see server/ep0_test.go,
// server/discovery_test.go).
//
// A write (acf.FlagWrite set) always goes through server.Server.WriteEP0 —
// root-client-only, exactly as WriteEP0 itself already enforces. A read
// (acf.FlagRead set, FlagWrite clear) always goes through
// server.Server.HandleDiscoveryRequest, per ROADMAP.md Milestone 46's own
// framing that discovery is "the mandatory, self-contained discovery
// mechanism ... every conformant server must answer in any lifecycle
// state" (Phase 17's disposition table) — this Router-level EP0Handler
// treats every EP0 read as that broadcastable, grant-independent,
// lifecycle-state-independent discovery read, not the separate,
// access-gated server.Server.ReadEP0. A caller that specifically wants the
// gated read (e.g. because it already holds an EP0 grant and wants to
// distinguish a normal read from a discovery probe at the application
// layer) calls server.Server.ReadEP0 directly rather than through this
// transport-level EP0Handler; that distinction has no separate wire-level
// signal to route on today, an open item left for a follow-on discovery/
// configuration-claim client protocol (ROADMAP.md Milestone 55) to define.
type EP0Handler struct {
	srv *server.Server
}

// NewEP0Handler returns an EP0Handler answering EP0 requests against srv.
func NewEP0Handler(srv *server.Server) *EP0Handler {
	return &EP0Handler{srv: srv}
}

// HandleRequest implements the Router.EP0 interface.
func (h *EP0Handler) HandleRequest(hdr avtp.Header, req acf.Message) (acf.Message, error) {
	if req.ByteBusID != regmap.EP0 {
		return acf.Message{}, ErrWrongEndpoint
	}

	switch {
	case req.Control.Has(acf.FlagWrite):
		if err := h.srv.WriteEP0(hdr.StreamID, req.Body); err != nil {
			return acf.Message{}, err
		}
		return responseFor(req, nil), nil
	case req.Control.Has(acf.FlagRead):
		body, err := h.srv.HandleDiscoveryRequest(hdr, req.Kind == acf.KindLong)
		if err != nil {
			return acf.Message{}, err
		}
		return responseFor(req, body), nil
	default:
		return acf.Message{}, ErrRequestMustReadOrWrite
	}
}

// responseFor builds a successful response Message for req: FlagResponse
// set, the originating Read/Write flag preserved, Kind/ByteBusID/
// TransactionNum carried over from req for correlation, and body as the
// result payload — the same shape every Phase 14 endpoint type's own
// responseFor helper (e.g. gpio.Endpoint's) already establishes.
func responseFor(req acf.Message, body []byte) acf.Message {
	return acf.Message{
		Kind:           req.Kind,
		ByteBusID:      req.ByteBusID,
		TransactionNum: req.TransactionNum,
		Control:        acf.FlagResponse | (req.Control & (acf.FlagRead | acf.FlagWrite)),
		Body:           body,
	}
}
