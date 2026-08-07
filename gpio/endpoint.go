package gpio

import (
	"sync"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/server"
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

// HandleRequest answers one plain, unconditional acf.Message request
// addressed to this endpoint. Per ROADMAP.md Milestone 47's explicit scope,
// this is the read/write/configuration request shape only — compound/
// triggered/chained/timed request kinds are Phase 15's job (ROADMAP.md
// Milestone 49) and are not decoded here. req must set exactly one of
// acf.FlagRead or acf.FlagWrite; a request with neither is rejected with
// ErrRequestMustReadOrWrite, since a GPIO endpoint has nothing else to do
// with it at this milestone.
//
// A request's evt[2:0] field decides what happens to its byte_msg_payload,
// per TC18 §13.5 Table 30's GPIO/PWM_OUT row: which combining rule the
// operand bitmask is written to the pins under (set / OR / AND / XOR /
// saturating add / saturating subtract), whether the request is reserved and
// must be rejected with UNSUPPORTED_CMD (100b), or whether the payload is
// kept away from the pins entirely and used to change this endpoint's
// configuration instead (111b, §12.7.1). That decoding is not done here —
// this method asks acf.Message.EVTDisposition, the single shared
// implementation of Table 30, and acts on its answer.
func (e *Endpoint) HandleRequest(requester avtp.StreamID, req acf.Message) (acf.Message, error) {
	if req.ByteBusID != e.addr {
		return acf.Message{}, ErrWrongEndpoint
	}
	if _, err := e.srv.ReadEndpoint(requester, e.addr); err != nil {
		return acf.Message{}, err
	}

	disp, err := req.EVTDisposition(EVTClass)
	if err != nil {
		return acf.Message{}, err
	}
	if disp.Action == acf.EVTActionConfigure {
		body, cfgErr := e.srv.ApplyConfigRequest(requester, e.addr, req, e.encodeConfigBlock, e.adoptConfigBlock)
		if cfgErr != nil {
			return acf.Message{}, cfgErr
		}
		return responseFor(req, body), nil
	}

	switch {
	case req.Control.Has(acf.FlagWrite):
		operand, err := DecodeWriteRequest(req.Body)
		if err != nil {
			return acf.Message{}, err
		}
		result, err := e.applyWrite(disp.WriteOp, operand)
		if err != nil {
			return acf.Message{}, err
		}
		return responseFor(req, EncodeValue(result)), nil
	case req.Control.Has(acf.FlagRead):
		e.mu.Lock()
		v := e.value
		e.mu.Unlock()
		return responseFor(req, EncodeValue(v)), nil
	default:
		return acf.Message{}, ErrRequestMustReadOrWrite
	}
}

// applyWrite applies one write request's operand under op and returns the
// endpoint's resulting pin-value word. Only output-direction pins are
// affected: every input-direction pin keeps whatever value SetInputs last
// drove into it, whichever combining rule op names.
func (e *Endpoint) applyWrite(op acf.EVTWriteOp, operand uint32) (uint32, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	active := e.cfg.activeMask()
	outMask := e.cfg.Direction & active
	candidate, err := applyValue(e.value, operand, op, active)
	if err != nil {
		return 0, err
	}
	before := e.value
	e.value = (e.value &^ outMask) | (candidate & outMask)
	e.recordChangeLocked(before, e.value)
	return e.value, nil
}

// encodeConfigBlock and adoptConfigBlock are this endpoint type's half of
// server.Server.ApplyConfigRequest's §12.7.1 configuration-access contract:
// render the current configuration as this endpoint's EP_func block, and
// decode/validate/adopt a patched one. See ApplyConfigRequest.
func (e *Endpoint) encodeConfigBlock() []byte {
	e.mu.Lock()
	defer e.mu.Unlock()
	return EncodeConfig(e.cfg)
}

func (e *Endpoint) adoptConfigBlock(raw []byte) error {
	cfg, err := DecodeConfig(raw)
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cfg = cfg
	e.value &= cfg.activeMask()
	return nil
}

// responseFor builds the response Message for req: FlagResponse set, the
// originating Read/Write flag preserved so a caller can tell which request
// shape it answers, same Kind/ByteBusID/TransactionNum as req for
// correlation, and body as the caller-supplied payload.
func responseFor(req acf.Message, body []byte) acf.Message {
	return acf.Message{
		Kind:           req.Kind,
		ByteBusID:      req.ByteBusID,
		TransactionNum: req.TransactionNum,
		Control:        acf.FlagResponse | (req.Control & (acf.FlagRead | acf.FlagWrite)),
		Body:           body,
	}
}
