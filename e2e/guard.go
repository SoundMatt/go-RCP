package e2e

import (
	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/request"
)

// Guard wraps another request.Handler, adding the CRC32 safe-point
// mechanism (ROADMAP.md Milestone 50) as an explicit per-endpoint opt-in
// mode — the same "wrap, don't edit" composition request.Dispatcher itself
// uses to retrofit the six Phase 14 endpoint types (see request/doc.go): an
// endpoint opts into safe-point protection by handing
// e2e.NewGuard(theRealHandler) to request.NewDispatcher in place of the
// endpoint itself, without theRealHandler needing to know this mechanism
// exists at all.
//
// Every inbound Message must carry a valid trailing CRC32 safe point (see
// Verify) or Guard skips calling the wrapped Handler entirely and returns
// ErrCRCMismatch — this milestone's dedicated CRC error code, surfaced to
// callers the same way every other protocol-level rejection in this repo
// is: as a distinguishable Go error a Dispatcher ticket resolves to (see
// Dispatcher.Response), not a bespoke wire-level error-code body. A
// successful call re-Protects the wrapped Handler's own response with a
// fresh safe point, since an endpoint that has opted into this mode is
// opting in symmetrically — both the requests it receives and the
// responses it sends are protected, not just one direction.
type Guard struct {
	Handler request.Handler
}

// NewGuard returns a Guard wrapping h.
func NewGuard(h request.Handler) *Guard {
	return &Guard{Handler: h}
}

// HandleRequest implements request.Handler. requester doubles as the
// addressing identity Verify/Protect use for the CRC's stream-addressing
// coverage: the AVTPDU carrying req is addressed to/from that same stream,
// on both the request and (symmetrically) the response leg.
func (g *Guard) HandleRequest(requester avtp.StreamID, req acf.Message) (acf.Message, error) {
	inner, err := Verify(requester, req)
	if err != nil {
		return acf.Message{}, err
	}
	resp, err := g.Handler.HandleRequest(requester, inner)
	if err != nil {
		return acf.Message{}, err
	}
	return Protect(requester, resp), nil
}
