package pwm

import (
	"sync"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/server"
)

// OutputTransport drives a RoleOutput endpoint's actual waveform generation.
// This package has no real PWM hardware of its own to drive, so a caller
// supplies its own OutputTransport via Endpoint.SetOutputTransport — e.g.
// backed by a simulated peripheral, a test double, or a real hardware
// driver. An endpoint with none set defaults to simply storing the applied
// waveform for readback (see Endpoint.applyOutput).
type OutputTransport interface {
	SetOutput(periodMicros, activeMicros uint32) error
}

// InputTransport supplies a RoleInput endpoint's most recently measured
// incoming waveform, or ErrSignalLost (or any other error) if no valid
// waveform is currently observable. An endpoint with none set defaults to
// reporting whatever was last fed through Endpoint.SetCapturedWaveform, or
// ErrSignalLost if that has never been called or Endpoint.SetSignalLost was
// called more recently (see Endpoint.capture).
type InputTransport interface {
	Capture() (periodMicros, activeMicros uint32, err error)
}

// TriggerKind distinguishes the trigger signals a PWM endpoint emits.
type TriggerKind uint8

const (
	// TriggerOutputUpdated fires whenever a RoleOutput endpoint's applied
	// waveform changes (a write request, or Configure applying its default).
	TriggerOutputUpdated TriggerKind = iota

	// TriggerCaptureUpdated fires whenever a RoleInput endpoint's captured
	// waveform changes.
	TriggerCaptureUpdated

	// TriggerSignalLost fires whenever a RoleInput endpoint's incoming
	// signal is explicitly marked lost.
	TriggerSignalLost
)

// TriggerEvent records one output-updated, capture-updated, or signal-lost
// signal a PWM endpoint queued. PeriodMicros/ActiveMicros are meaningful only
// for TriggerOutputUpdated/TriggerCaptureUpdated.
type TriggerEvent struct {
	Kind         TriggerKind
	PeriodMicros uint32
	ActiveMicros uint32
}

// Endpoint is one declared PWM endpoint layered on top of a server.Server: it
// reads/writes its Config through the server's register-map access path
// (server.Server.WriteFunctional/server.Server.ReadEndpoint), and owns its
// Transports and runtime waveform/capture state itself, since those are
// runtime concerns rather than persisted configuration (see doc.go). All
// exported methods are safe for concurrent use.
type Endpoint struct {
	mu   sync.Mutex
	srv  *server.Server
	addr avtp.ByteBusID

	cfg Config

	outputTransport OutputTransport
	appliedPeriod   uint32
	appliedActive   uint32

	inputTransport InputTransport
	capturedPeriod uint32
	capturedActive uint32
	hasCapture     bool
	signalLost     bool

	triggers []TriggerEvent
}

// NewEndpoint returns a PWM Endpoint for the already-declared endpoint addr
// on srv (see server.Server.AddEndpoint with EndpointType). It starts with a
// zero-value Config (endpoint disabled) — call Configure before handling any
// request.
func NewEndpoint(srv *server.Server, addr avtp.ByteBusID) *Endpoint {
	return &Endpoint{srv: srv, addr: addr}
}

// Configure validates cfg, persists it as this endpoint's functional
// configuration block through requester's server-level access, and adopts it
// for subsequent request handling. For a RoleOutput endpoint, Configure also
// immediately applies cfg's default waveform (see Config's doc comment),
// exactly as a write request would.
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

	if cfg.Enabled && cfg.Role == RoleOutput {
		if err := e.applyOutput(cfg.DefaultPeriodMicros, cfg.DefaultActiveMicros); err != nil {
			return err
		}
	}
	return nil
}

// SetOutputTransport installs the OutputTransport that drives a RoleOutput
// endpoint's actual waveform generation. Passing nil restores the default
// store-for-readback behavior.
func (e *Endpoint) SetOutputTransport(t OutputTransport) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.outputTransport = t
}

// SetInputTransport installs the InputTransport that supplies a RoleInput
// endpoint's measured waveform. Passing nil restores the default
// SetCapturedWaveform/SetSignalLost-driven behavior.
func (e *Endpoint) SetInputTransport(t InputTransport) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.inputTransport = t
}

// SetCapturedWaveform records the most recently observed incoming waveform
// for the default (no InputTransport configured) RoleInput behavior, clears
// any prior signal-lost state, and queues a TriggerCaptureUpdated event. It
// has no effect on the value Endpoint.HandleRequest reports while an
// InputTransport is configured via SetInputTransport.
func (e *Endpoint) SetCapturedWaveform(periodMicros, activeMicros uint32) {
	e.mu.Lock()
	e.capturedPeriod = periodMicros
	e.capturedActive = activeMicros
	e.hasCapture = true
	e.signalLost = false
	e.triggers = append(e.triggers, TriggerEvent{Kind: TriggerCaptureUpdated, PeriodMicros: periodMicros, ActiveMicros: activeMicros})
	e.mu.Unlock()
}

// SetSignalLost marks the default (no InputTransport configured) RoleInput
// behavior as having lost its incoming signal, so a subsequent read request
// fails explicitly with ErrSignalLost rather than returning the last
// captured waveform, and queues a TriggerSignalLost event. It has no effect
// while an InputTransport is configured via SetInputTransport.
func (e *Endpoint) SetSignalLost() {
	e.mu.Lock()
	e.signalLost = true
	e.triggers = append(e.triggers, TriggerEvent{Kind: TriggerSignalLost})
	e.mu.Unlock()
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
// addressed to this endpoint. Per ROADMAP.md Milestone 48's explicit scope,
// this is the read/write request shape only — compound/triggered/chained/
// timed request kinds are Phase 15's job (ROADMAP.md Milestone 49) and are
// not decoded here. req must set exactly one of acf.FlagRead or
// acf.FlagWrite; a request with neither is rejected with
// ErrRequestMustReadOrWrite. A write request is only accepted for a
// RoleOutput endpoint (ErrWriteNotSupportedForInput otherwise): its body is
// the waveform to apply (see EncodeWaveform), and the response echoes it
// back. A read request on a RoleOutput endpoint returns the currently
// applied waveform; on a RoleInput endpoint it returns the most recently
// captured waveform, or fails explicitly with ErrSignalLost on signal loss
// rather than returning stale data or hanging.
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
		if cfg.Role != RoleOutput {
			return acf.Message{}, ErrWriteNotSupportedForInput
		}
		period, active, err := DecodeWaveform(req.Body)
		if err != nil {
			return acf.Message{}, err
		}
		if err := e.applyOutput(period, active); err != nil {
			return acf.Message{}, err
		}
		return responseFor(req, EncodeWaveform(period, active)), nil
	case req.Control.Has(acf.FlagRead):
		switch cfg.Role {
		case RoleOutput:
			e.mu.Lock()
			p, a := e.appliedPeriod, e.appliedActive
			e.mu.Unlock()
			return responseFor(req, EncodeWaveform(p, a)), nil
		default: // RoleInput
			p, a, err := e.capture()
			if err != nil {
				return acf.Message{}, err
			}
			return responseFor(req, EncodeWaveform(p, a)), nil
		}
	default:
		return acf.Message{}, ErrRequestMustReadOrWrite
	}
}

// applyOutput applies periodMicros/activeMicros through the configured
// OutputTransport (or the default store-for-readback behavior with none
// set), followed by a TriggerOutputUpdated event.
func (e *Endpoint) applyOutput(periodMicros, activeMicros uint32) error {
	if activeMicros > periodMicros {
		return ErrActiveExceedsPeriod
	}
	e.mu.Lock()
	transport := e.outputTransport
	e.mu.Unlock()

	if transport != nil {
		if err := transport.SetOutput(periodMicros, activeMicros); err != nil {
			return err
		}
	}

	e.mu.Lock()
	e.appliedPeriod = periodMicros
	e.appliedActive = activeMicros
	e.triggers = append(e.triggers, TriggerEvent{Kind: TriggerOutputUpdated, PeriodMicros: periodMicros, ActiveMicros: activeMicros})
	e.mu.Unlock()
	return nil
}

// capture returns the endpoint's currently measured incoming waveform
// through the configured InputTransport (or the default
// SetCapturedWaveform/SetSignalLost-driven behavior with none set), failing
// with ErrSignalLost (or the Transport's own error) rather than returning
// stale data.
func (e *Endpoint) capture() (uint32, uint32, error) {
	e.mu.Lock()
	transport := e.inputTransport
	e.mu.Unlock()

	if transport != nil {
		return transport.Capture()
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if e.signalLost || !e.hasCapture {
		return 0, 0, ErrSignalLost
	}
	return e.capturedPeriod, e.capturedActive, nil
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
