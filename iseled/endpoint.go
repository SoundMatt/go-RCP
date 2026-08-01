package iseled

import (
	"sync"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/server"
)

// Transport performs the endpoint's actual exchange of one Command over the
// physical 4b/5b-encoded chain, returning every device's DeviceResponse
// that answered (one entry for a targeted Command, up to Config.DeviceCount
// entries for a Command addressed to DeviceBroadcast). This package has no
// real ISELED chain of its own to drive, so a caller supplies its own
// Transport via Endpoint.SetTransport — e.g. backed by a simulated
// peripheral, a test double, or a real hardware driver. An endpoint with no
// Transport set defaults to a loopback echo (see Endpoint.exchange).
type Transport interface {
	Exchange(cmd Command) (AggregatedResponse, error)
}

// TriggerEvent records one command-complete signal an ISELED endpoint
// queued, carrying the number of devices that answered.
type TriggerEvent struct {
	ResponseCount int
}

// Endpoint is one declared ISELED endpoint layered on top of a
// server.Server: it reads/writes its Config through the server's
// register-map access path
// (server.Server.WriteFunctional/server.Server.ReadEndpoint), and owns the
// Transport and pending trigger queue itself, since those are runtime
// concerns rather than persisted configuration (see doc.go). All exported
// methods are safe for concurrent use.
type Endpoint struct {
	mu   sync.Mutex
	srv  *server.Server
	addr avtp.ByteBusID

	cfg       Config
	transport Transport
	triggers  []TriggerEvent
}

// NewEndpoint returns an ISELED Endpoint for the already-declared endpoint
// addr on srv (see server.Server.AddEndpoint with EndpointType). It starts
// with a zero-value Config (chain disabled) — call Configure before
// handling any request.
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
// chain exchange. Passing nil restores the default loopback echo.
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

// HandleRequest answers one plain, unconditional acf.Message command
// request addressed to this endpoint (see doc.go's "Explicit non-goal"
// section for the conditional-request kinds this deliberately leaves to
// request.Dispatcher). req must set acf.FlagWrite: an ISELED command
// always carries an outgoing payload, so there is nothing to send down the
// chain without it. The response body is an EncodeAggregatedResponse
// encoding of every device that answered — one entry for a targeted
// Command, up to Config.DeviceCount entries for one addressed to
// DeviceBroadcast (see doc.go's "Multi-device response aggregation"
// section).
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
	if !req.Control.Has(acf.FlagWrite) {
		return acf.Message{}, ErrRequestMustWrite
	}

	cmd, err := DecodeCommand(req.Body)
	if err != nil {
		return acf.Message{}, err
	}
	resp, err := e.exchange(cmd)
	if err != nil {
		return acf.Message{}, err
	}
	return responseFor(req, EncodeAggregatedResponse(resp)), nil
}

// exchange performs one command exchange over the chain, followed by a
// command-complete trigger.
func (e *Endpoint) exchange(cmd Command) (AggregatedResponse, error) {
	e.mu.Lock()
	cfg := e.cfg
	transport := e.transport
	e.mu.Unlock()

	if !cfg.Enabled {
		return nil, ErrChainNotConfigured
	}
	if cmd.Address != DeviceBroadcast && cmd.Address >= cfg.DeviceCount {
		return nil, ErrDeviceAddressOutOfRange
	}

	var resp AggregatedResponse
	var err error
	if transport != nil {
		resp, err = transport.Exchange(cmd)
		if err != nil {
			return nil, err
		}
	} else {
		resp = defaultLoopback(cmd, cfg.DeviceCount)
	}

	e.mu.Lock()
	e.triggers = append(e.triggers, TriggerEvent{ResponseCount: len(resp)})
	e.mu.Unlock()
	return resp, nil
}

// defaultLoopback is the endpoint's default (no Transport configured)
// exchange behavior: cmd.Data echoed back by cmd.Address itself for a
// targeted Command, or by every device 0..deviceCount-1 for a Command
// addressed to DeviceBroadcast.
func defaultLoopback(cmd Command, deviceCount uint8) AggregatedResponse {
	if cmd.Address != DeviceBroadcast {
		return AggregatedResponse{{Address: cmd.Address, Data: append([]byte(nil), cmd.Data...)}}
	}
	resp := make(AggregatedResponse, 0, deviceCount)
	for addr := uint8(0); addr < deviceCount; addr++ {
		resp = append(resp, DeviceResponse{Address: addr, Data: append([]byte(nil), cmd.Data...)})
	}
	return resp
}

// responseFor builds the response Message for req: FlagResponse set, same
// Kind/ByteBusID/TransactionNum as req for correlation, and body as the
// caller-supplied payload.
func responseFor(req acf.Message, body []byte) acf.Message {
	return acf.Message{
		Kind:           req.Kind,
		ByteBusID:      req.ByteBusID,
		TransactionNum: req.TransactionNum,
		Control:        acf.FlagResponse | acf.FlagWrite,
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
