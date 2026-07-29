package mock

//fusa:req REQ-MEP-001
//fusa:req REQ-MEP-002
//fusa:req REQ-MEP-003
//fusa:req REQ-MEP-004
//fusa:req REQ-MEP-005
//fusa:req REQ-MEP-006

import (
	"fmt"
	"sync/atomic"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
)

// EndpointFunc is a caller-supplied function implementing one endpoint's
// request-handling logic. If nil, Endpoint answers every request with a
// fixed success response carrying an empty body — the same "default OK
// unless a Handler says otherwise" posture this package's legacy Controller
// (mock.go) already established, retargeted at this milestone's
// request.Handler shape.
type EndpointFunc func(requester avtp.StreamID, req acf.Message) (acf.Message, error)

// Endpoint is the reference in-process test double for one declared
// endpoint address, per ROADMAP.md Milestone 57's disposition-table call on
// this package ("the reference test double must actually implement the new
// server/endpoint/register-map model to be useful for testing anything
// built in Phases 13-16"). It implements request.Handler directly, so it is
// registrable into a *udp.Router (via Router.Register) exactly like any
// real Phase 14/16 endpoint-type package's own Endpoint — a caller wanting
// to unit-test a Dispatcher, a bridge package, or a Milestone 55
// control-plane adapter without pulling in gpio/spi/adc/... wires this in
// their place instead. All exported methods are safe for concurrent use.
type Endpoint struct {
	addr   avtp.ByteBusID
	fn     EndpointFunc
	closed atomic.Bool
}

// NewEndpoint returns an Endpoint answering requests addressed to addr. fn
// may be nil (see EndpointFunc).
func NewEndpoint(addr avtp.ByteBusID, fn EndpointFunc) *Endpoint {
	return &Endpoint{addr: addr, fn: fn}
}

// Addr returns the endpoint address this Endpoint answers requests for.
func (e *Endpoint) Addr() avtp.ByteBusID { return e.addr }

// HandleRequest implements request.Handler. It rejects a request addressed
// to a different byte_bus_id (ErrWrongEndpoint) and a request received
// after Close (ErrClosed), matching every Phase 14 endpoint type's own
// HandleRequest posture (see e.g. gpio.Endpoint.HandleRequest).
func (e *Endpoint) HandleRequest(requester avtp.StreamID, req acf.Message) (acf.Message, error) {
	if e.closed.Load() {
		return acf.Message{}, fmt.Errorf("rcp/mock: endpoint %d: %w", e.addr, ErrClosed)
	}
	if req.ByteBusID != e.addr {
		return acf.Message{}, fmt.Errorf("rcp/mock: endpoint %d: %w", e.addr, ErrWrongEndpoint)
	}
	if e.fn != nil {
		return e.fn(requester, req)
	}
	return acf.Message{
		Kind:           req.Kind,
		ByteBusID:      req.ByteBusID,
		TransactionNum: req.TransactionNum,
		Control:        acf.FlagResponse | (req.Control & (acf.FlagRead | acf.FlagWrite)),
	}, nil
}

// Close marks the Endpoint closed; subsequent HandleRequest calls report
// ErrClosed. Safe to call multiple times.
func (e *Endpoint) Close() error {
	e.closed.Store(true)
	return nil
}
