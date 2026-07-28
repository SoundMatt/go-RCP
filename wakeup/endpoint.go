package wakeup

import (
	"sync"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/server"
)

// TriggerKind distinguishes the trigger signals a Wakeup endpoint emits.
type TriggerKind uint8

const (
	// TriggerPowerStateChanged fires on every successful power-state
	// transition (see Endpoint.transitionTo), including a Sleep→Normal
	// wake.
	TriggerPowerStateChanged TriggerKind = iota

	// TriggerWakeHandshake fires once per repeat of the wake-handshake
	// message a Sleep→Normal wake queues up front (see doc.go's Scope
	// section and Endpoint.AcknowledgeWake).
	TriggerWakeHandshake
)

// TriggerEvent records one power-state-changed or wake-handshake signal a
// Wakeup endpoint queued. State is meaningful only for
// TriggerPowerStateChanged; Handshake is meaningful only for
// TriggerWakeHandshake.
type TriggerEvent struct {
	Kind      TriggerKind
	State     PowerState
	Handshake WakeHandshake
}

// Endpoint is one declared Wakeup endpoint layered on top of a
// server.Server: it reads/writes its Config through the server's
// register-map access path
// (server.Server.WriteFunctional/server.Server.ReadEndpoint), and owns its
// current PowerState, cold/hot-start bookkeeping, and pending trigger queue
// itself, since those are runtime concerns rather than persisted
// configuration (see doc.go). All exported methods are safe for concurrent
// use.
type Endpoint struct {
	mu   sync.Mutex
	srv  *server.Server
	addr avtp.ByteBusID

	cfg   Config
	state PowerState

	lastStartKind StartKind
	retentionLost bool

	triggers []TriggerEvent
}

// NewEndpoint returns a Wakeup Endpoint for the already-declared endpoint
// addr on srv (see server.Server.AddEndpoint with EndpointType). It starts
// with a zero-value Config (endpoint disabled) and PowerNormal as its
// current PowerState — call Configure before handling any request.
func NewEndpoint(srv *server.Server, addr avtp.ByteBusID) *Endpoint {
	return &Endpoint{srv: srv, addr: addr, state: PowerNormal}
}

// Configure validates cfg, persists it as this endpoint's functional
// configuration block through requester's server-level access, and adopts it
// for subsequent request handling.
func (e *Endpoint) Configure(requester avtp.StreamID, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := e.srv.WriteFunctional(requester, e.addr, EncodeConfig(cfg)); err != nil {
		return err
	}
	e.mu.Lock()
	e.cfg = cfg
	e.mu.Unlock()
	return nil
}

// State returns the endpoint's current PowerState.
func (e *Endpoint) State() PowerState {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.state
}

// LastStartKind reports the cold/hot-start determination made by the most
// recent Sleep→Normal wake, or StartUnknown if none has occurred yet.
func (e *Endpoint) LastStartKind() StartKind {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.lastStartKind
}

// SetRetentionLost marks that retained context was lost during the current
// PowerSleep period (e.g. a simulated brown-out), so the next Sleep→Normal
// wake determines StartCold rather than StartHot. It has no effect outside
// of PowerSleep beyond arming that determination for whenever the next wake
// occurs, and is cleared automatically once that wake happens.
func (e *Endpoint) SetRetentionLost() {
	e.mu.Lock()
	e.retentionLost = true
	e.mu.Unlock()
}

// AcknowledgeWake discards every not-yet-drained TriggerWakeHandshake event
// still queued, stopping the current wake cycle's repeats early — a caller
// (representing the requester that received and answered a wake-handshake
// message) calls this once it no longer needs the message repeated. Every
// other queued TriggerEvent is left untouched.
func (e *Endpoint) AcknowledgeWake() {
	e.mu.Lock()
	defer e.mu.Unlock()
	filtered := e.triggers[:0]
	for _, t := range e.triggers {
		if t.Kind != TriggerWakeHandshake {
			filtered = append(filtered, t)
		}
	}
	e.triggers = filtered
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

// HandleRequest answers one plain, unconditional acf.Message request
// addressed to this endpoint (see doc.go's "Explicit non-goal" section for
// the conditional-request kinds this deliberately leaves to
// request.Dispatcher). req must set exactly one of acf.FlagRead or
// acf.FlagWrite; a request with neither is rejected with
// ErrRequestMustReadOrWrite. A write request's body is the target
// PowerState (see EncodePowerStateRequest); the response echoes the newly
// applied state back. A read request returns the current PowerState.
func (e *Endpoint) HandleRequest(requester avtp.StreamID, req acf.Message) (acf.Message, error) {
	if req.ByteBusID != e.addr {
		return acf.Message{}, ErrWrongEndpoint
	}
	if _, err := e.srv.ReadEndpoint(requester, e.addr); err != nil {
		return acf.Message{}, err
	}

	e.mu.Lock()
	cfg := e.cfg
	e.mu.Unlock()
	if !cfg.Enabled {
		return acf.Message{}, ErrNotConfigured
	}

	switch {
	case req.Control.Has(acf.FlagWrite):
		target, err := DecodePowerStateRequest(req.Body)
		if err != nil {
			return acf.Message{}, err
		}
		if err := e.transitionTo(target); err != nil {
			return acf.Message{}, err
		}
		return responseFor(req, acf.FlagWrite, EncodePowerStateResponse(target)), nil
	case req.Control.Has(acf.FlagRead):
		return responseFor(req, acf.FlagRead, EncodePowerStateResponse(e.State())), nil
	default:
		return acf.Message{}, ErrRequestMustReadOrWrite
	}
}

// transitionTo drives the endpoint to target: PowerUnpowered is always
// rejected with ErrCannotRequestUnpowered, an otherwise-unrecognized target
// is rejected with ErrInvalidPowerState, transitioning to the state the
// endpoint is already in is a no-op, and every other transition queues a
// TriggerPowerStateChanged event — with a Sleep→Normal transition
// additionally determining this wake's StartKind (see SetRetentionLost) and
// queuing Config.WakeHandshakeRepeatCount TriggerWakeHandshake events (see
// doc.go's Scope section).
func (e *Endpoint) transitionTo(target PowerState) error {
	if target == PowerUnpowered {
		return ErrCannotRequestUnpowered
	}
	if !target.Valid() {
		return ErrInvalidPowerState
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.state == target {
		return nil
	}
	waking := e.state == PowerSleep && target == PowerNormal
	e.state = target

	e.triggers = append(e.triggers, TriggerEvent{Kind: TriggerPowerStateChanged, State: target})

	if waking {
		startKind := StartHot
		if e.retentionLost {
			startKind = StartCold
		}
		e.retentionLost = false
		e.lastStartKind = startKind

		for seq := uint16(0); seq < e.cfg.WakeHandshakeRepeatCount; seq++ {
			e.triggers = append(e.triggers, TriggerEvent{
				Kind:      TriggerWakeHandshake,
				Handshake: WakeHandshake{Start: startKind, Sequence: seq},
			})
		}
	}
	return nil
}

// responseFor builds the response Message for req: FlagResponse set plus
// the originating Read/Write flag preserved so a caller can tell which
// request shape it answers, same Kind/ByteBusID/TransactionNum as req for
// correlation, and body as the caller-supplied payload.
func responseFor(req acf.Message, rw acf.ControlFlags, body []byte) acf.Message {
	return acf.Message{
		Kind:           req.Kind,
		ByteBusID:      req.ByteBusID,
		TransactionNum: req.TransactionNum,
		Control:        acf.FlagResponse | rw,
		Body:           body,
	}
}
