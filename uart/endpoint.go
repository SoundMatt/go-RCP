package uart

import (
	"sync"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/server"
)

// Transport performs the endpoint's actual TX byte emission: given the bytes
// to transmit, it returns the number of bytes actually accepted (n <=
// len(tx)) and any error. This package has no real UART line of its own to
// drive, so a caller supplies its own Transport via Endpoint.SetTransport —
// e.g. backed by a simulated peripheral, a test double, or a real hardware
// driver. An endpoint with no Transport set defaults to looping every
// transmitted byte directly into its own RX FIFO (see Endpoint.transmit).
type Transport interface {
	Write(tx []byte) (n int, err error)
}

// TriggerKind distinguishes the two trigger signals a UART endpoint emits.
type TriggerKind uint8

const (
	// TriggerTXComplete fires once a TX write request's transmit finishes,
	// carrying the number of bytes accepted.
	TriggerTXComplete TriggerKind = iota

	// TriggerRXDataAvailable fires once new bytes are appended to the RX
	// FIFO (see Endpoint.Receive), carrying the number of newly arrived
	// bytes.
	TriggerRXDataAvailable
)

// TriggerEvent records one TX-complete or RX-data-available signal a UART
// endpoint queued.
type TriggerEvent struct {
	Kind      TriggerKind
	ByteCount int
}

// Endpoint is one declared UART endpoint layered on top of a server.Server:
// it reads/writes its Config through the server's register-map access path
// (server.Server.WriteFunctional/server.Server.ReadEndpoint), and owns the
// Transport, RX FIFO, and pending trigger queue itself, since those are
// runtime concerns rather than persisted configuration (see doc.go). TX and
// RX requests are handled independently of one another (see
// HandleRequest) even though they share one Config block. All exported
// methods are safe for concurrent use.
type Endpoint struct {
	mu   sync.Mutex
	srv  *server.Server
	addr avtp.ByteBusID

	cfg       Config
	transport Transport
	rxFIFO    []byte
	triggers  []TriggerEvent
}

// NewEndpoint returns a UART Endpoint for the already-declared endpoint addr
// on srv (see server.Server.AddEndpoint with EndpointType). It starts with a
// zero-value Config (endpoint disabled) — call Configure before handling any
// request.
func NewEndpoint(srv *server.Server, addr avtp.ByteBusID) *Endpoint {
	return &Endpoint{srv: srv, addr: addr}
}

// Configure validates cfg, persists it as this endpoint's functional
// configuration block through requester's server-level access, and adopts it
// for subsequent request handling. Configure does not touch the RX FIFO or
// pending trigger queue — those are independent runtime state.
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
// TX byte emission. Passing nil restores the default TX-to-RX loopback.
func (e *Endpoint) SetTransport(t Transport) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.transport = t
}

// Receive appends data to the endpoint's RX FIFO, modeling bytes that
// arrived on the physical RX line from outside this package (the only way
// the RX FIFO grows other than the default loopback Transport — see
// Endpoint.transmit), and queues a TriggerRXDataAvailable event.
func (e *Endpoint) Receive(data []byte) {
	if len(data) == 0 {
		return
	}
	e.mu.Lock()
	e.rxFIFO = append(e.rxFIFO, data...)
	e.triggers = append(e.triggers, TriggerEvent{Kind: TriggerRXDataAvailable, ByteCount: len(data)})
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

// HandleRequest answers one plain, unconditional acf.Message TX or RX
// request addressed to this endpoint. Per ROADMAP.md Milestone 48's explicit
// scope, this is the plain request shape only — compound/triggered/chained/
// timed request kinds are Phase 15's job (ROADMAP.md Milestone 49) and are
// not decoded here. req must set exactly one of acf.FlagRead or
// acf.FlagWrite; a request with neither is rejected with
// ErrRequestMustReadOrWrite. A Write request is TX: its Body is the bytes to
// transmit. A Read request is RX and must be payload-less (see doc.go's note
// on this asymmetry versus gpio/pwm's read requests); its
// acf.Message.ReadSizeOrSegment field carries the requested read size, per
// the shared request-descriptor header every endpoint type uses for a plain
// read.
func (e *Endpoint) HandleRequest(requester avtp.StreamID, req acf.Message) (acf.Message, error) {
	if req.ByteBusID != e.addr {
		return acf.Message{}, ErrWrongEndpoint
	}
	if _, err := e.srv.ReadEndpoint(requester, e.addr); err != nil {
		return acf.Message{}, err
	}

	switch {
	case req.Control.Has(acf.FlagWrite):
		tx := DecodeWriteRequest(req.Body)
		n, err := e.transmit(tx)
		if err != nil {
			return acf.Message{}, err
		}
		return responseFor(req, EncodeWriteResponse(n)), nil
	case req.Control.Has(acf.FlagRead):
		if len(req.Body) != 0 {
			return acf.Message{}, ErrReadRequestNotPayloadLess
		}
		e.mu.Lock()
		enabled := e.cfg.Enabled
		e.mu.Unlock()
		if !enabled {
			return acf.Message{}, ErrUARTNotConfigured
		}
		complete, data := e.read(req.ReadSizeOrSegment)
		return responseFor(req, EncodeReadResponse(complete, data)), nil
	default:
		return acf.Message{}, ErrRequestMustReadOrWrite
	}
}

// transmit performs one TX write through the configured Transport (or the
// default TX-to-RX loopback with none set), followed by a TX-complete
// trigger.
func (e *Endpoint) transmit(tx []byte) (int, error) {
	e.mu.Lock()
	cfg := e.cfg
	transport := e.transport
	e.mu.Unlock()

	if !cfg.Enabled {
		return 0, ErrUARTNotConfigured
	}

	var n int
	var err error
	if transport != nil {
		n, err = transport.Write(tx)
		if err != nil {
			return 0, err
		}
	} else {
		n = len(tx)
		e.Receive(tx) // default loopback: TX bytes reappear on this endpoint's own RX FIFO
	}

	e.mu.Lock()
	e.triggers = append(e.triggers, TriggerEvent{Kind: TriggerTXComplete, ByteCount: n})
	e.mu.Unlock()
	return n, nil
}

// read drains up to want bytes from the RX FIFO (FIFO order), reporting
// complete=true only when the full requested count was available. This
// package has no real timer of its own, so a read that cannot fill the full
// request returns whatever is available immediately rather than blocking —
// this package's synchronous stand-in for the FIFO-drain-or-timeout
// completion ROADMAP.md Milestone 48 describes (see doc.go); a caller
// driving a real timeout is expected to poll with follow-up read requests.
func (e *Endpoint) read(want uint16) (complete bool, data []byte) {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := int(want)
	if n > len(e.rxFIFO) {
		n = len(e.rxFIFO)
	}
	data = append([]byte(nil), e.rxFIFO[:n]...)
	e.rxFIFO = e.rxFIFO[n:]
	return n == int(want), data
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
