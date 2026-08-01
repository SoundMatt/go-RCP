package adc

import (
	"sync"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/server"
)

// Transport supplies one raw ADC reading (up to 16 bits wide; the low
// ResolutionBits bits are the ones this package uses — see Config). This
// package has no real ADC hardware of its own to sample, so a caller
// supplies its own Transport via Endpoint.SetTransport — e.g. backed by a
// simulated signal source, a test double, or a real hardware driver. A
// channel with no Transport set defaults to always sampling zero (see
// Endpoint.Trigger).
type Transport interface {
	Sample() (raw uint16, err error)
}

// TriggerKind distinguishes the trigger signal an ADC endpoint emits.
type TriggerKind uint8

const (
	// TriggerMeasurementDone fires once a measurement (the full
	// sample/average/combine cycle) completes, carrying the resulting
	// value.
	TriggerMeasurementDone TriggerKind = iota
)

// TriggerEvent records one measurement-done signal an ADC endpoint queued.
type TriggerEvent struct {
	Kind  TriggerKind
	Value uint16
}

// Endpoint is one declared ADC endpoint layered on top of a server.Server: it
// reads/writes its Config through the server's register-map access path
// (server.Server.WriteFunctional/server.Server.ReadEndpoint), and owns the
// Transport, last-reported value, and pending trigger queue itself, since
// those are runtime concerns rather than persisted configuration (see
// doc.go). All exported methods are safe for concurrent use.
type Endpoint struct {
	mu   sync.Mutex
	srv  *server.Server
	addr avtp.ByteBusID

	cfg       Config
	transport Transport
	value     uint16
	triggers  []TriggerEvent
}

// NewEndpoint returns an ADC Endpoint for the already-declared endpoint addr
// on srv (see server.Server.AddEndpoint with EndpointType). It starts with a
// zero-value Config (channel disabled) — call Configure before handling any
// request.
func NewEndpoint(srv *server.Server, addr avtp.ByteBusID) *Endpoint {
	return &Endpoint{srv: srv, addr: addr}
}

// Configure validates cfg, persists it as this endpoint's functional
// configuration block through requester's server-level access, and adopts it
// for subsequent request handling. Config bits describing a resolution
// narrower than the endpoint's previously stored value are applied to that
// value immediately (masked down), the same active-mask-on-reconfigure
// posture gpio.Endpoint.Configure takes for its own live pin-value word.
func (e *Endpoint) Configure(requester avtp.StreamID, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := e.srv.WriteFunctional(requester, e.addr, EncodeConfig(cfg)); err != nil {
		return err
	}
	e.mu.Lock()
	e.cfg = cfg
	e.value &= resolutionMask(cfg.ResolutionBits)
	e.mu.Unlock()
	return nil
}

// SetTransport installs the Transport that supplies this endpoint's raw
// samples. Passing nil restores the default always-zero sample source.
func (e *Endpoint) SetTransport(t Transport) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.transport = t
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

// Trigger performs one full sample/average/combine measurement cycle: it
// takes Config.SampleCount raw readings from the configured Transport (or
// the default always-zero source with none set), arithmetic-means them, then
// combines that averaged reading with the endpoint's previous value under
// Config.Combine to produce the new reported value, masked to
// Config.ResolutionBits. It queues a TriggerMeasurementDone event carrying
// that value and returns it.
//
// Trigger is the single entry point both of this package's two
// continuous-sampling mechanisms drive (see TriggerMode and doc.go's Scope
// section) — a caller wiring this channel to another endpoint's own trigger
// signal, or a caller chaining off this endpoint's own DrainTriggers — as
// well as what a plain read request triggers when Config.TriggerMode is
// TriggerModeOnDemand (see HandleRequest). Trigger itself does not consult
// TriggerMode; it always performs one fresh measurement, regardless of which
// mechanism is calling it.
func (e *Endpoint) Trigger(requester avtp.StreamID) (uint16, error) {
	e.mu.Lock()
	cfg := e.cfg
	transport := e.transport
	prev := e.value
	e.mu.Unlock()

	if !cfg.Enabled {
		return 0, ErrChannelNotConfigured
	}

	mask := resolutionMask(cfg.ResolutionBits)
	var sum uint32
	for i := 0; i < int(cfg.SampleCount); i++ {
		var raw uint16
		var err error
		if transport != nil {
			raw, err = transport.Sample()
			if err != nil {
				return 0, err
			}
		}
		sum += uint32(raw) & uint32(mask)
	}
	avg := uint16(sum / uint32(cfg.SampleCount))

	var final uint16
	switch cfg.Combine {
	case CombineReplace:
		final = avg
	case CombineRollingAverage:
		final = uint16((uint32(prev) + uint32(avg)) / 2)
	default:
		return 0, ErrInvalidCombineMode
	}
	final &= mask

	e.mu.Lock()
	e.value = final
	e.triggers = append(e.triggers, TriggerEvent{Kind: TriggerMeasurementDone, Value: final})
	e.mu.Unlock()
	return final, nil
}

// HandleRequest answers one plain, unconditional acf.Message request
// addressed to this endpoint. Per ROADMAP.md Milestone 48's explicit scope,
// this is the read/manual-trigger request shape only — compound/triggered/
// chained/timed request kinds are Phase 15's job (ROADMAP.md Milestone 49)
// and are not decoded here. req must set exactly one of acf.FlagRead or
// acf.FlagWrite; a request with neither is rejected with
// ErrRequestMustReadOrWrite. A Write request always performs a fresh
// Trigger, regardless of Config.TriggerMode — a manual on-demand override. A
// Read request performs a fresh Trigger only when Config.TriggerMode is
// TriggerModeOnDemand; otherwise (TriggerModeExternal/TriggerModeSelf) it
// returns the latest already-measured value without sampling again itself,
// since in those modes the endpoint is expected to already be kept sampling
// by whatever is driving Trigger — a plain read must not additionally force
// a sample of its own on top of that.
func (e *Endpoint) HandleRequest(requester avtp.StreamID, req acf.Message) (acf.Message, error) {
	if req.ByteBusID != e.addr {
		return acf.Message{}, ErrWrongEndpoint
	}
	if _, err := e.srv.ReadEndpoint(requester, e.addr); err != nil {
		return acf.Message{}, err
	}

	// TC18 §13.5 Table 30 / §12.9.1: what happens to this request's
	// byte_msg_payload is decided by evt[2:0], not by this endpoint type.
	// For EVTClass, 111b routes the payload into this endpoint's §12.7.1
	// configuration block instead of presenting it at the interface, and
	// every other non-zero value is reserved and rejected with
	// UNSUPPORTED_CMD. The decoding itself lives in acf/evt.go, shared by
	// every endpoint type. It runs before any enabled/flag check, since a
	// configuration request is how a disabled endpoint is brought into
	// service in the first place.
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
		v, err := e.Trigger(requester)
		if err != nil {
			return acf.Message{}, err
		}
		return responseFor(req, EncodeValue(v)), nil
	case req.Control.Has(acf.FlagRead):
		e.mu.Lock()
		mode := e.cfg.TriggerMode
		e.mu.Unlock()
		if mode == TriggerModeOnDemand {
			v, err := e.Trigger(requester)
			if err != nil {
				return acf.Message{}, err
			}
			return responseFor(req, EncodeValue(v)), nil
		}
		e.mu.Lock()
		enabled := e.cfg.Enabled
		v := e.value
		e.mu.Unlock()
		if !enabled {
			return acf.Message{}, ErrChannelNotConfigured
		}
		return responseFor(req, EncodeValue(v)), nil
	default:
		return acf.Message{}, ErrRequestMustReadOrWrite
	}
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
	return nil
}
