package spi

import (
	"sync"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/server"
)

// Transport performs one channel's full-duplex byte exchange: given the
// bytes to transmit, it returns the bytes received back over the same
// transfer. This package has no real SPI bus of its own to drive, so a
// caller supplies its own Transport per channel via Endpoint.SetTransport —
// e.g. backed by a simulated peripheral, a test double, or a real hardware
// driver on an embedded target. A channel with no Transport set defaults to
// a loopback echo (see Endpoint.transfer).
type Transport interface {
	Transfer(tx []byte) (rx []byte, err error)
}

// TriggerKind distinguishes the two trigger signals a SPI endpoint emits.
type TriggerKind uint8

const (
	// TriggerTransferComplete fires once a transfer finishes, carrying the
	// number of bytes received.
	TriggerTransferComplete TriggerKind = iota

	// TriggerChipSelectEdge fires twice per transfer: once as the channel
	// is selected (Asserted true) and once as it is deselected (Asserted
	// false) immediately afterward.
	TriggerChipSelectEdge
)

// TriggerEvent records one transfer-complete or chip-select-edge signal a
// SPI endpoint queued.
type TriggerEvent struct {
	Kind TriggerKind

	// Channel is the chip-select channel this event concerns.
	Channel Channel

	// ByteCount is the number of bytes received, meaningful only when Kind
	// is TriggerTransferComplete.
	ByteCount int

	// Asserted is true for a chip-select-assert edge and false for a
	// chip-select-deassert edge, meaningful only when Kind is
	// TriggerChipSelectEdge.
	Asserted bool
}

// Endpoint is one declared SPI endpoint layered on top of a server.Server:
// it reads/writes its Config through the server's register-map access path
// (server.Server.WriteFunctional/server.Server.ReadEndpoint), and owns each
// channel's Transport and the pending trigger queue itself, since those are
// runtime concerns rather than persisted configuration (see doc.go). All
// exported methods are safe for concurrent use.
type Endpoint struct {
	mu   sync.Mutex
	srv  *server.Server
	addr avtp.ByteBusID

	cfg        Config
	transports [MaxChannels]Transport
	triggers   []TriggerEvent
}

// NewEndpoint returns a SPI Endpoint for the already-declared endpoint addr
// on srv (see server.Server.AddEndpoint with EndpointType). It starts with a
// zero-value Config (every channel disabled) — call SetChannelConfig for at
// least one channel before handling any request.
func NewEndpoint(srv *server.Server, addr avtp.ByteBusID) *Endpoint {
	return &Endpoint{srv: srv, addr: addr}
}

// SetChannelConfig validates and persists the configuration for exactly one
// channel, leaving the other five channels' configuration untouched.
func (e *Endpoint) SetChannelConfig(requester avtp.StreamID, ch Channel, cc ChannelConfig) error {
	if !ch.Valid() {
		return ErrInvalidChannel
	}
	e.mu.Lock()
	cfg := e.cfg
	cfg.Channels[ch] = cc
	e.mu.Unlock()

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

// SetTransport installs the Transport that performs ch's actual full-duplex
// byte exchange. Passing nil restores the default loopback echo.
func (e *Endpoint) SetTransport(ch Channel, t Transport) error {
	if !ch.Valid() {
		return ErrInvalidChannel
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.transports[ch] = t
	return nil
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

// HandleRequest answers one plain, unconditional acf.Message transfer
// request addressed to this endpoint. Per ROADMAP.md Milestone 47's
// explicit scope, this is the plain request shape only — compound/
// triggered/chained/timed request kinds are Phase 15's job (ROADMAP.md
// Milestone 49) and are not decoded here.
//
// The request's evt[2:0] field selects which chip-select channel the
// transfer targets (TC18 §13.5 Table 30's SPI row): 000b through 101b name
// channels 0 to 5, 110b is reserved and rejected with UNSUPPORTED_CMD, and
// 111b routes the payload away from the bus entirely and into this
// endpoint's §12.7.1 configuration block. That decoding is not done here —
// this method asks acf.Message.EVTDisposition, the single shared
// implementation of Table 30, and acts on its answer.
//
// A transfer request must set acf.FlagWrite: a SPI transfer always carries
// an outgoing payload (even a zero-length one), so there is nothing to
// transfer without it. A configuration request may be either a read or a
// write, since §12.7.1 defines both directions.
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

	if !req.Control.Has(acf.FlagWrite) {
		return acf.Message{}, ErrRequestMustWrite
	}
	ch := Channel(disp.Channel)
	if !ch.Valid() { // unreachable: Table 30's SPI row only yields 0-5 here
		return acf.Message{}, ErrInvalidChannel
	}
	rx, err := e.transfer(ch, DecodeTransferRequest(req.Body))
	if err != nil {
		return acf.Message{}, err
	}
	return responseFor(req, EncodeTransferResponse(rx)), nil
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

// transfer performs one full-duplex byte exchange on ch, bracketed by a
// chip-select-assert/deassert trigger pair and followed by a
// transfer-complete trigger.
func (e *Endpoint) transfer(ch Channel, tx []byte) ([]byte, error) {
	e.mu.Lock()
	cc := e.cfg.Channels[ch]
	transport := e.transports[ch]
	e.mu.Unlock()

	if !cc.Enabled {
		return nil, ErrChannelNotConfigured
	}

	e.recordTrigger(TriggerEvent{Kind: TriggerChipSelectEdge, Channel: ch, Asserted: true})

	var rx []byte
	var err error
	if transport != nil {
		rx, err = transport.Transfer(tx)
	} else {
		rx = append([]byte(nil), tx...) // default loopback echo
	}

	e.recordTrigger(TriggerEvent{Kind: TriggerChipSelectEdge, Channel: ch, Asserted: false})
	if err != nil {
		return nil, err
	}
	e.recordTrigger(TriggerEvent{Kind: TriggerTransferComplete, Channel: ch, ByteCount: len(rx)})
	return rx, nil
}

func (e *Endpoint) recordTrigger(ev TriggerEvent) {
	e.mu.Lock()
	e.triggers = append(e.triggers, ev)
	e.mu.Unlock()
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
