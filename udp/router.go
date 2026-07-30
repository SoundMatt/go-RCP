package udp

import (
	"sync"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/regmap"
	"github.com/SoundMatt/go-RCP/request"
)

// EP0 is the per-request handler for the reserved regmap.EP0 address:
// configuration read/write and Milestone 46 discovery. It takes the full
// avtp.Header (not just the requester's avtp.StreamID, the way
// request.Handler does) because HandleDiscoveryRequest's untimed-header
// requirement needs it — see ep0.go.
type EP0 interface {
	HandleRequest(hdr avtp.Header, req acf.Message) (acf.Message, error)
}

// Router dispatches a decoded avtp.Header + acf.Message request to the
// right place: EP0.HandleRequest for byte_bus_id EP0, or a
// caller-registered request.Handler for every other declared endpoint,
// looked up by avtp.ByteBusID. Router is the single place this package's
// Server applies avtp.Header.Disposition — every registered Handler
// answers a plain (requester, acf.Message) call and never has to reimplement
// the timed/untimed drop rule itself. All exported methods are safe for
// concurrent use.
type Router struct {
	ep0 EP0

	// timeSyncSupported reports whether the server side of this Router has
	// any time-synchronization capability at all, per avtp.Header.Disposition.
	// This milestone has no gPTP/clock integration to query, so it is a
	// caller-supplied static capability flag (see NewRouter) rather than a
	// live check — a real time-sync integration is a follow-on, tracked in
	// doc.go's "Explicit non-goals" rather than guessed at here.
	timeSyncSupported bool

	mu       sync.RWMutex
	handlers map[avtp.ByteBusID]request.Handler
}

// NewRouter returns a Router that answers EP0 requests via ep0 (typically
// an *EP0Handler wrapping a *server.Server — see NewEP0Handler) and treats
// timeSyncSupported as this server's answer to avtp.Header.Disposition's
// own question for every request it routes.
func NewRouter(ep0 EP0, timeSyncSupported bool) *Router {
	return &Router{
		ep0:               ep0,
		timeSyncSupported: timeSyncSupported,
		handlers:          make(map[avtp.ByteBusID]request.Handler),
	}
}

// Register installs handler as the responder for every request addressed
// to addr. It returns ErrReservedAddress for addr == regmap.EP0 (always
// answered by this Router's own EP0 handler, never a registered one) and
// ErrDuplicateEndpoint if addr already has a registered Handler.
func (r *Router) Register(addr avtp.ByteBusID, handler request.Handler) error {
	if addr == regmap.EP0 {
		return ErrReservedAddress
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.handlers[addr]; ok {
		return ErrDuplicateEndpoint
	}
	r.handlers[addr] = handler
	return nil
}

// Deregister removes addr's registered Handler, if any. It is a no-op for
// an address with nothing registered.
func (r *Router) Deregister(addr avtp.ByteBusID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.handlers, addr)
}

// Route answers one decoded request. The bool return reports whether a
// reply should be sent at all: it is false only when hdr.Disposition
// resolves to avtp.DispositionDrop (a timestamped AVTPDU this Router's
// configured time-sync capability cannot honor), matching this package's
// documented drop-silently posture (see doc.go) rather than sending an
// error response a sender with no trusted clock may not even be able to
// correlate. Any other failure (unknown endpoint, a Handler's own error) is
// reported as a wire-level error response, not a dropped reply.
func (r *Router) Route(hdr avtp.Header, req acf.Message) (acf.Message, bool) {
	if hdr.Disposition(r.timeSyncSupported) == avtp.DispositionDrop {
		return acf.Message{}, false
	}

	var resp acf.Message
	var err error
	if req.ByteBusID == regmap.EP0 {
		resp, err = r.ep0.HandleRequest(hdr, req)
	} else {
		r.mu.RLock()
		h, ok := r.handlers[req.ByteBusID]
		r.mu.RUnlock()
		if !ok {
			err = ErrUnknownEndpoint
		} else {
			resp, err = h.HandleRequest(hdr.StreamID, req)
		}
	}

	if err != nil {
		return errorResponse(req, err), true
	}
	return resp, true
}

// errorResponse builds the wire-level error-response shape this package
// defines (see doc.go's spec-fidelity note): FlagResponse and FlagError
// set, the originating Read/Write flag preserved (mirroring every Phase 14
// endpoint type's own responseFor helper, e.g. gpio.Endpoint's), Kind/
// ByteBusID/TransactionNum carried over from req for correlation, and Body
// as the numeric ErrorCode errorCodeFor(err) maps err onto, followed by
// err's message text as an optional trailing diagnostic (see
// EncodeErrorBody) — the code is the primary, authoritative payload; the
// diagnostic text is for local debugging only.
func errorResponse(req acf.Message, err error) acf.Message {
	return acf.Message{
		Kind:           req.Kind,
		ByteBusID:      req.ByteBusID,
		TransactionNum: req.TransactionNum,
		Control:        acf.FlagResponse | acf.FlagError | (req.Control & (acf.FlagRead | acf.FlagWrite)),
		Body:           EncodeErrorBody(errorCodeFor(err), err.Error()),
	}
}
