package gpio

import (
	"sync"

	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/server"
)

// TriggerEvent records one per-pin change/edge signal a GPIO endpoint
// queued: which pins changed (restricted to Config.TriggerEnable), and the
// endpoint's full pin-value word immediately after the change.
type TriggerEvent struct {
	ChangedMask uint32
	Value       uint32
}

// Endpoint is one declared GPIO endpoint layered on top of a server.Server:
// it reads/writes its Config through the server's register-map access path
// (server.Server.WriteFunctional/server.Server.ReadEndpoint), and owns the
// live pin-value word and pending trigger queue itself, since those are
// runtime I/O state rather than persisted configuration (see doc.go). All
// exported methods are safe for concurrent use.
type Endpoint struct {
	mu   sync.Mutex
	srv  *server.Server
	addr avtp.ByteBusID

	cfg      Config
	value    uint32
	triggers []TriggerEvent
}

// NewEndpoint returns a GPIO Endpoint for the already-declared endpoint addr
// on srv (see server.Server.AddEndpoint with EndpointType). It starts with a
// zero-value Config (no pins in use) — call Configure before handling any
// request.
func NewEndpoint(srv *server.Server, addr avtp.ByteBusID) *Endpoint {
	return &Endpoint{srv: srv, addr: addr}
}

// Configure validates cfg, persists it as this endpoint's functional
// configuration block through requester's server-level access, and adopts
// it for subsequent request handling. Config bits describing pins beyond
// cfg's new PinCount are dropped from the live pin-value and trigger state,
// consistent with Config.Validate's active-pin masking.
func (e *Endpoint) Configure(requester avtp.StreamID, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := e.srv.WriteFunctional(requester, e.addr, EncodeConfig(cfg)); err != nil {
		return err
	}
	e.mu.Lock()
	e.cfg = cfg
	active := cfg.activeMask()
	e.value &= active
	e.mu.Unlock()
	return nil
}

// SetInputs drives the endpoint's input-direction pins (Config.Direction
// bits clear) from mask, leaving every output-direction pin untouched, and
// returns the endpoint's resulting full pin-value word. It models an
// external physical input transition — the only way an input pin's value
// changes, since a write request only ever affects output pins (see
// applyWrite). A change on a TriggerEnable pin is queued exactly as a
// write-triggered change would be.
func (e *Endpoint) SetInputs(mask uint32) uint32 {
	e.mu.Lock()
	defer e.mu.Unlock()
	active := e.cfg.activeMask()
	inMask := ^e.cfg.Direction & active
	before := e.value
	e.value = (e.value &^ inMask) | (mask & inMask)
	e.recordChangeLocked(before, e.value)
	return e.value
}

// DrainTriggers returns every TriggerEvent queued since the last call (FIFO
// order) and clears the queue, or returns nil if none are pending.
func (e *Endpoint) DrainTriggers() []TriggerEvent {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.triggers) == 0 {
		return nil
	}
	out := e.triggers
	e.triggers = nil
	return out
}

// recordChangeLocked queues a TriggerEvent if any bit that changed between
// before and after is also set in the endpoint's current TriggerEnable mask.
// Callers must hold e.mu.
func (e *Endpoint) recordChangeLocked(before, after uint32) {
	changed := (before ^ after) & e.cfg.TriggerEnable
	if changed == 0 {
		return
	}
	e.triggers = append(e.triggers, TriggerEvent{ChangedMask: changed, Value: after})
}

// HandleRequest answers one plain, unconditional avtp.Message request
// addressed to this endpoint. Per ROADMAP.md Milestone 47's explicit scope,
// this is the read/write/reconfigure request shape only — compound/
// triggered/chained/timed request kinds are Phase 15's job (ROADMAP.md
// Milestone 49) and are not decoded here. req must set exactly one of
// avtp.FlagRead or avtp.FlagWrite; a request with neither is rejected with
// ErrRequestMustReadOrWrite, since a GPIO endpoint has nothing else to do
// with it at this milestone.
func (e *Endpoint) HandleRequest(requester avtp.StreamID, req avtp.Message) (avtp.Message, error) {
	if req.ByteBusID != e.addr {
		return avtp.Message{}, ErrWrongEndpoint
	}
	if _, err := e.srv.ReadEndpoint(requester, e.addr); err != nil {
		return avtp.Message{}, err
	}

	switch {
	case req.Control.Has(avtp.FlagWrite):
		sem, operand, err := DecodeWriteRequest(req.Body)
		if err != nil {
			return avtp.Message{}, err
		}
		result, err := e.applyWrite(requester, sem, operand)
		if err != nil {
			return avtp.Message{}, err
		}
		return responseFor(req, EncodeValue(result)), nil
	case req.Control.Has(avtp.FlagRead):
		e.mu.Lock()
		v := e.value
		e.mu.Unlock()
		return responseFor(req, EncodeValue(v)), nil
	default:
		return avtp.Message{}, ErrRequestMustReadOrWrite
	}
}

// applyWrite applies one write (or reconfigure) request and returns the
// endpoint's resulting pin-value word.
func (e *Endpoint) applyWrite(requester avtp.StreamID, sem WriteSemantic, operand uint32) (uint32, error) {
	if !sem.Valid() {
		return 0, ErrInvalidSemantic
	}

	if sem == SemanticReconfigure {
		e.mu.Lock()
		cfg := e.cfg
		cfg.Direction = operand & cfg.activeMask()
		e.cfg = cfg
		value := e.value
		e.mu.Unlock()
		if err := e.srv.WriteFunctional(requester, e.addr, EncodeConfig(cfg)); err != nil {
			return 0, err
		}
		return value, nil
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	active := e.cfg.activeMask()
	outMask := e.cfg.Direction & active
	candidate, err := applyValue(e.value, operand, sem, active)
	if err != nil {
		return 0, err
	}
	before := e.value
	e.value = (e.value &^ outMask) | (candidate & outMask)
	e.recordChangeLocked(before, e.value)
	return e.value, nil
}

// responseFor builds the response Message for req: FlagResponse set, the
// originating Read/Write flag preserved so a caller can tell which request
// shape it answers, same Kind/ByteBusID/TransactionNum as req for
// correlation, and body as the caller-supplied payload.
func responseFor(req avtp.Message, body []byte) avtp.Message {
	return avtp.Message{
		Kind:           req.Kind,
		ByteBusID:      req.ByteBusID,
		TransactionNum: req.TransactionNum,
		Control:        avtp.FlagResponse | (req.Control & (avtp.FlagRead | avtp.FlagWrite)),
		Body:           body,
	}
}
