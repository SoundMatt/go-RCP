package can

import (
	"sync"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/server"
)

// Transport performs the endpoint's actual transmission of a Frame onto the
// bus. This package has no real CAN bus of its own to drive, so a caller
// supplies its own Transport via Endpoint.SetTransport — e.g. backed by a
// simulated peripheral, a test double, or a real hardware driver. An
// endpoint with no Transport set defaults to accepting the transmission
// without error and otherwise doing nothing (see Endpoint.transmit) — per
// doc.go's Scope section, this package does not loop a transmitted frame
// back into the RX path by default, since a real bus's RX path is
// independent of what this controller itself last sent.
type Transport interface {
	Transmit(f Frame) error
}

// Endpoint is one declared CAN endpoint layered on top of a server.Server:
// it reads/writes its Config through the server's register-map access path
// (server.Server.WriteFunctional/server.Server.ReadEndpoint), and owns the
// Transport and most-recently-received Frame itself, since those are
// runtime concerns rather than persisted configuration (see doc.go). All
// exported methods are safe for concurrent use.
//
// Unlike every other Phase 14/16 endpoint type in this repo, Endpoint has no
// DrainTriggers method — see doc.go's "Open gap: no trigger-signal table"
// section for why that is a deliberate, documented omission rather than an
// oversight.
type Endpoint struct {
	mu   sync.Mutex
	srv  *server.Server
	addr avtp.ByteBusID

	cfg       Config
	transport Transport

	received    Frame
	hasReceived bool
}

// NewEndpoint returns a CAN Endpoint for the already-declared endpoint addr
// on srv (see server.Server.AddEndpoint with EndpointType). It starts with a
// zero-value Config (bus disabled) — call Configure before handling any
// request.
func NewEndpoint(srv *server.Server, addr avtp.ByteBusID) *Endpoint {
	return &Endpoint{srv: srv, addr: addr}
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
// frame transmission. Passing nil restores the default accept-and-discard
// behavior.
func (e *Endpoint) SetTransport(t Transport) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.transport = t
}

// SetReceivedFrame records f as the most recently received frame on this
// bus, for a subsequent read request to return (see Endpoint.HandleRequest).
// This is how a caller — a simulated peripheral, a test, or a real hardware
// driver's RX callback — feeds incoming bus traffic into this endpoint; it
// is never inferred from a transmitted frame (see doc.go's Scope section).
func (e *Endpoint) SetReceivedFrame(f Frame) {
	e.mu.Lock()
	e.received = f
	e.hasReceived = true
	e.mu.Unlock()
}

// HandleRequest answers one plain, unconditional acf.Message request
// addressed to this endpoint (see doc.go's Scope section for the
// conditional-request kinds this deliberately leaves to
// request.Dispatcher). req must set exactly one of acf.FlagRead or
// acf.FlagWrite; a request with neither is rejected with
// ErrRequestMustReadOrWrite. A write request's body is an encoded Frame to
// transmit (see EncodeFrame); the response echoes it back once accepted. A
// read request returns the most recently received Frame (see
// SetReceivedFrame), failing explicitly with ErrNoFrameReceived rather than
// returning a stale or zero-value Frame when none has been received yet.
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
		return responseFor(req, req.Control&(acf.FlagRead|acf.FlagWrite), body), nil
	}

	e.mu.Lock()
	cfg := e.cfg
	e.mu.Unlock()
	if !cfg.Enabled {
		return acf.Message{}, ErrNotConfigured
	}

	switch {
	case req.Control.Has(acf.FlagWrite):
		f, err := DecodeFrame(req.Body)
		if err != nil {
			return acf.Message{}, err
		}
		if err := f.Validate(); err != nil {
			return acf.Message{}, err
		}
		if err := e.transmit(f); err != nil {
			return acf.Message{}, err
		}
		return responseFor(req, acf.FlagWrite, EncodeFrame(f)), nil
	case req.Control.Has(acf.FlagRead):
		f, err := e.lastReceived()
		if err != nil {
			return acf.Message{}, err
		}
		return responseFor(req, acf.FlagRead, EncodeFrame(f)), nil
	default:
		return acf.Message{}, ErrRequestMustReadOrWrite
	}
}

// transmit hands f to the configured Transport (or the default
// accept-and-discard behavior with none set).
func (e *Endpoint) transmit(f Frame) error {
	e.mu.Lock()
	transport := e.transport
	e.mu.Unlock()

	if transport == nil {
		return nil
	}
	return transport.Transmit(f)
}

// lastReceived returns the most recently received Frame, or
// ErrNoFrameReceived if none has been recorded via SetReceivedFrame.
func (e *Endpoint) lastReceived() (Frame, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.hasReceived {
		return Frame{}, ErrNoFrameReceived
	}
	return e.received, nil
}

// responseFor builds the response Message for req: FlagResponse set plus the
// originating Read/Write flag preserved so a caller can tell which request
// shape it answers, same Kind/ByteBusID/TransactionNum as req for
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
