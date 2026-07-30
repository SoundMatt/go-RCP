package mdio

import (
	"sync"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/server"
)

// Transport performs the endpoint's actual MDIO register access. This
// package has no real MDIO interface of its own to drive, so a caller
// supplies its own Transport via Endpoint.SetTransport — e.g. backed by a
// simulated PHY, a test double, or a real hardware driver (including one
// exposing an integrated on-die PHY with no physical MDIO pins wired, per
// doc.go's Scope section). An endpoint with no Transport set defaults to an
// in-memory per-(Mode,PhyAddr,DevAddr,RegAddr) register store that reads as
// zero until written (see Endpoint.read/Endpoint.write). The register
// value is a uint32 so it can hold either the 16-bit or 32-bit width a
// Request's DataWidth selects; a 16-bit-wide access only ever populates the
// value's low 16 bits.
type Transport interface {
	ReadRegister(r Request) (uint32, error)
	WriteRegister(r Request, data uint32) error
}

// TriggerEvent records one register-access-complete signal an MDIO
// endpoint queued.
type TriggerEvent struct {
	Request Request
	Data    uint32
	Write   bool
}

// registerKey identifies one addressable register across every field a
// Request's addressing can vary: Mode, PhyAddr, DevAddr, RegAddr.
type registerKey struct {
	mode    Mode
	phyAddr uint8
	devAddr uint8
	regAddr uint16
}

func keyFor(r Request) registerKey {
	return registerKey{mode: r.Mode, phyAddr: r.PhyAddr, devAddr: r.DevAddr, regAddr: r.RegAddr}
}

// Endpoint is one declared MDIO endpoint layered on top of a server.Server:
// it reads/writes its Config through the server's register-map access path
// (server.Server.WriteFunctional/server.Server.ReadEndpoint), and owns the
// Transport, default register store, and pending trigger queue itself,
// since those are runtime concerns rather than persisted configuration (see
// doc.go). All exported methods are safe for concurrent use.
type Endpoint struct {
	mu   sync.Mutex
	srv  *server.Server
	addr avtp.ByteBusID

	cfg       Config
	transport Transport
	registers map[registerKey]uint32
	triggers  []TriggerEvent
}

// NewEndpoint returns an MDIO Endpoint for the already-declared endpoint
// addr on srv (see server.Server.AddEndpoint with EndpointType). It starts
// with a zero-value Config (endpoint disabled) — call Configure before
// handling any request.
func NewEndpoint(srv *server.Server, addr avtp.ByteBusID) *Endpoint {
	return &Endpoint{srv: srv, addr: addr, registers: make(map[registerKey]uint32)}
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

// SetTransport installs the Transport that performs this endpoint's actual
// register access. Passing nil restores the default in-memory register
// store.
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

// HandleRequest answers one plain, unconditional acf.Message request
// addressed to this endpoint (see doc.go's "Explicit non-goal" section for
// the conditional-request kinds this deliberately leaves to
// request.Dispatcher). req must set exactly one of acf.FlagRead or
// acf.FlagWrite; a request with neither is rejected with
// ErrRequestMustReadOrWrite. A read request's body is an encoded Request
// (see EncodeReadRequest); the response is the 16-bit register value (see
// EncodeResponse). A write request's body is an encoded Request plus the
// 16-bit value to write (see EncodeWriteRequest); the response echoes the
// written value back.
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
		r, data, err := DecodeWriteRequest(req.Body)
		if err != nil {
			return acf.Message{}, err
		}
		if err := r.Validate(); err != nil {
			return acf.Message{}, err
		}
		if err := e.write(r, data); err != nil {
			return acf.Message{}, err
		}
		return responseFor(req, acf.FlagWrite, EncodeResponse(r, data)), nil
	case req.Control.Has(acf.FlagRead):
		r, err := DecodeReadRequest(req.Body)
		if err != nil {
			return acf.Message{}, err
		}
		if verr := r.Validate(); verr != nil {
			return acf.Message{}, verr
		}
		data, err := e.read(r)
		if err != nil {
			return acf.Message{}, err
		}
		return responseFor(req, acf.FlagRead, EncodeResponse(r, data)), nil
	default:
		return acf.Message{}, ErrRequestMustReadOrWrite
	}
}

// read performs one register read through the configured Transport (or the
// default in-memory register store with none set), followed by a
// register-access-complete trigger.
func (e *Endpoint) read(r Request) (uint32, error) {
	e.mu.Lock()
	transport := e.transport
	e.mu.Unlock()

	var data uint32
	var err error
	if transport != nil {
		data, err = transport.ReadRegister(r)
		if err != nil {
			return 0, err
		}
	} else {
		e.mu.Lock()
		data = e.registers[keyFor(r)] // zero value (0) if never written
		e.mu.Unlock()
	}

	e.mu.Lock()
	e.triggers = append(e.triggers, TriggerEvent{Request: r, Data: data, Write: false})
	e.mu.Unlock()
	return data, nil
}

// write performs one register write through the configured Transport (or
// the default in-memory register store with none set), followed by a
// register-access-complete trigger.
func (e *Endpoint) write(r Request, data uint32) error {
	e.mu.Lock()
	transport := e.transport
	e.mu.Unlock()

	if transport != nil {
		if err := transport.WriteRegister(r, data); err != nil {
			return err
		}
	} else {
		e.mu.Lock()
		e.registers[keyFor(r)] = data
		e.mu.Unlock()
	}

	e.mu.Lock()
	e.triggers = append(e.triggers, TriggerEvent{Request: r, Data: data, Write: true})
	e.mu.Unlock()
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
