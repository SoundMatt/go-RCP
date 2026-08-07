//fusa:req REQ-ADAPT-001
//fusa:req REQ-ADAPT-002
//fusa:req REQ-ADAPT-003
//fusa:req REQ-ADAPT-004
//fusa:req REQ-ADAPT-005
//fusa:req REQ-ADAPT-006
//fusa:req REQ-ADAPT-007
//fusa:req REQ-ADAPT-008
//fusa:req REQ-MSG-001
//fusa:req REQ-MSG-002
//fusa:req REQ-MSG-003
//fusa:req REQ-MSG-004
//fusa:req REQ-MSG-005
//fusa:req REQ-MSG-006
//fusa:req REQ-MSG-007
//fusa:req REQ-MSG-008

package rcp

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	relay "github.com/SoundMatt/RELAY/v2"
	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
)

// Controller is the narrow interface Adapt wraps: a TC18 client capable of
// one synchronous request/response round trip against a single dialed RC
// Server, presenting its own avtp.StreamID identity on every request it
// sends. This is the same "narrow local interface matching *udp.Controller's
// own shape" pattern capi.Controller and observe's own Controller-equivalent
// interface already established (ROADMAP.md Milestone 57) — *udp.Controller
// (production) and *mock.Client (the reference test double, see the mock
// package) both already satisfy this shape exactly and need no adapter of
// their own to be passed to Adapt.
//
//fusa:req REQ-ADAPT-001
type Controller interface {
	// StreamID returns this Controller's own avtp.StreamID identity.
	StreamID() avtp.StreamID

	// Request submits one request to addr with the given control flags and
	// body, and blocks for the matching response or ctx's expiry, whichever
	// comes first.
	Request(ctx context.Context, addr avtp.ByteBusID, control acf.ControlFlags, body []byte) (acf.Message, error)

	// Close releases all resources held by the Controller. Safe to call
	// multiple times.
	Close() error
}

// Adapt wraps c as a relay.Caller so application code can use it
// protocol-agnostically. It is non-blocking and does not connect (c is
// assumed already dialed).
//
// The returned value also implements the optional relay interfaces
// HealthProvider, MetricsProvider, and Drainer (spec §9); application code
// may type-assert for them.
//
//fusa:req REQ-ADAPT-001
func Adapt(c Controller) relay.Caller {
	return &rcpAdapter{ctrl: c}
}

type rcpAdapter struct {
	ctrl Controller

	// Counters feeding MetricsProvider.Metrics() (spec §9).
	writeCount     atomic.Uint64
	deliverCount   atomic.Uint64
	dropCount      atomic.Uint64
	bytesWritten   atomic.Uint64
	bytesDelivered atomic.Uint64
	errorCount     atomic.Uint64

	// inFlight tracks Send/Call dispatches in progress, drained by
	// CloseWithDrain. closed feeds HealthProvider.Health().
	inFlight atomic.Int64
	closed   atomic.Bool

	// subs holds every still-open Subscribe channel, closed by Close (see
	// Subscribe's own doc comment for why nothing is ever sent to one).
	subMu sync.Mutex
	subs  []chan relay.Message
}

// Protocol returns relay.RCP.
//
//fusa:req REQ-ADAPT-002
func (a *rcpAdapter) Protocol() relay.Protocol { return relay.RCP }

// Send converts msg to a request, dispatches it via Controller.Request, and
// discards the response.
//
//fusa:req REQ-ADAPT-003
func (a *rcpAdapter) Send(ctx context.Context, msg relay.Message) error {
	addr, control, body, err := RequestFromMessage(msg)
	if err != nil {
		a.errorCount.Add(1)
		return err
	}
	a.inFlight.Add(1)
	defer a.inFlight.Add(-1)
	a.writeCount.Add(1)
	a.bytesWritten.Add(uint64(len(body)))
	if _, err = a.ctrl.Request(ctx, addr, control, body); err != nil {
		a.errorCount.Add(1)
	}
	return err
}

// Call converts req to a request, dispatches it via Controller.Request, and
// returns the endpoint's response as a relay.Message.
//
//fusa:req REQ-ADAPT-004
func (a *rcpAdapter) Call(ctx context.Context, req relay.Message) (relay.Message, error) {
	addr, control, body, err := RequestFromMessage(req)
	if err != nil {
		a.errorCount.Add(1)
		return relay.Message{}, err
	}
	a.inFlight.Add(1)
	defer a.inFlight.Add(-1)
	a.writeCount.Add(1)
	a.bytesWritten.Add(uint64(len(body)))
	resp, err := a.ctrl.Request(ctx, addr, control, body)
	if err != nil {
		a.errorCount.Add(1)
		return relay.Message{}, err
	}
	a.deliverCount.Add(1)
	a.bytesDelivered.Add(uint64(len(resp.Body)))
	return ResponseToMessage(addr, resp), nil
}

// Subscribe returns a channel that obeys every RELAY §6 lifecycle rule
// (independent per call, closed when the adapter closes, ErrClosed once
// already closed) but never delivers a message: the OPEN Alliance TC18
// Remote Control Protocol this package implements has no server-initiated
// broadcast counterpart to the retired bespoke protocol's periodic Status
// push — the same conclusion ROADMAP.md Milestone 56 reached for every
// protocol-bridge package's own Subscribe (ddsbr, mqttbr, grpcbridge,
// restbridge) and Milestone 57 reached for capi/observe's Controller-
// equivalent interfaces. A caller that type-asserts relay.Caller and calls
// Subscribe gets a well-behaved, empty stream rather than a nil channel or
// a panic; opts is accepted and validated (an unrecognised BackPressurePolicy
// still panics via relay.ApplySubscriberOpts, matching every other RELAY
// adapter) but otherwise unused, since nothing is ever enqueued.
//
//fusa:req REQ-ADAPT-005
//fusa:req REQ-ADAPT-006
//fusa:req REQ-ADAPT-007
func (a *rcpAdapter) Subscribe(opts ...relay.SubscriberOption) (<-chan relay.Message, error) {
	if a.closed.Load() {
		return nil, ErrClosed
	}
	cfg := relay.ApplySubscriberOpts(opts)
	ch := make(chan relay.Message, cfg.ChanDepth(64))

	a.subMu.Lock()
	defer a.subMu.Unlock()
	if a.closed.Load() {
		close(ch)
		return ch, nil
	}
	a.subs = append(a.subs, ch)
	return ch, nil
}

// Close closes every open Subscribe channel and the underlying Controller.
//
//fusa:req REQ-ADAPT-008
func (a *rcpAdapter) Close() error {
	if a.closed.Swap(true) {
		return nil
	}
	a.subMu.Lock()
	for _, ch := range a.subs {
		close(ch)
	}
	a.subs = nil
	a.subMu.Unlock()
	return a.ctrl.Close()
}

// EndpointIDString renders addr as the relay.Message.ID string
// RequestFromMessage/ResponseToMessage use to address one endpoint on
// whichever RC Server the Controller in use is already dialed against — a
// Controller presents exactly one avtp.StreamID identity to exactly one
// destination server (see Controller's own doc comment), so only the
// ByteBusID varies message to message.
//
//fusa:req REQ-MSG-004
func EndpointIDString(addr avtp.ByteBusID) string {
	return strconv.Itoa(int(addr))
}

// ParseEndpointID parses a relay.Message.ID string produced by
// EndpointIDString back into an avtp.ByteBusID. Returns ErrNotFound for a
// malformed string or a value outside ByteBusID's 0-255 range.
//
//fusa:req REQ-MSG-001
//fusa:req REQ-MSG-002
func ParseEndpointID(id string) (avtp.ByteBusID, error) {
	n, err := strconv.Atoi(id)
	if err != nil || n < 0 || n > 255 {
		return 0, fmt.Errorf("rcp: endpoint id %q: %w", id, ErrNotFound)
	}
	return avtp.ByteBusID(n), nil
}

// RequestFromMessage converts a relay.Message into the (addr, control,
// body) triple Controller.Request needs. addr comes from m.ID (see
// ParseEndpointID). control comes from the "rcp.op" Meta key ("read" or
// "write"); when absent or unrecognised it defaults to write when
// m.Payload is non-empty, else read — the same "infer intent from payload
// presence" default this package's retired CommandFromMessage applied to
// its own rcp.cmd_type default. body is m.Payload, unchanged.
//
// m's "rcp.transaction_num"/"rcp.read_size_or_segment" Meta keys (see
// ResponseToMessage) are intentionally not read here: Controller.Request
// has no parameter for either — this package's narrow Controller interface
// (see Controller's own doc comment) leaves transaction-number assignment
// and read-size selection to the concrete Controller implementation
// (*udp.Controller, *mock.Client) rather than exposing them to a caller
// converting a generic relay.Message. This is a one-directional, disclosed
// subset of RELAY's own reference rcp.Message.FromMessage() mapping, not an
// oversight.
//
//fusa:req REQ-MSG-007
//fusa:req REQ-MSG-008
func RequestFromMessage(m relay.Message) (avtp.ByteBusID, acf.ControlFlags, []byte, error) {
	addr, err := ParseEndpointID(m.ID)
	if err != nil {
		return 0, 0, nil, err
	}
	var control acf.ControlFlags
	switch m.Meta["rcp.op"] {
	case "read":
		control = acf.FlagRead
	case "write":
		control = acf.FlagWrite
	default:
		if len(m.Payload) > 0 {
			control = acf.FlagWrite
		} else {
			control = acf.FlagRead
		}
	}
	return addr, control, m.Payload, nil
}

// ResponseToMessage converts addr's acf.Message response into a
// relay.Message — this package's RELAY spec §15.7.5 canonical-conversion
// analogue for the TC18 request/response model. ID is
// EndpointIDString(addr), Payload is the response Body. Four Meta keys are
// always set, mirroring RELAY's own reference rcp.Message.ToMessage()
// (direction-agnostic: both op and error are set regardless of whether resp
// represents a response or, as here, is also used to render a bare request
// for interop/testing purposes): "rcp.op" mirrors resp.Control's
// Read/Write bit, "rcp.error" mirrors its Error bit, and
// "rcp.transaction_num"/"rcp.read_size_or_segment" carry resp.TransactionNum
// and resp.ReadSizeOrSegment as decimal strings — closing the §15.7.5
// "MUST be lossless for all mandatory fields" gap those two previously
// unmapped acf.Message fields left open.
//
//fusa:req REQ-MSG-003
//fusa:req REQ-MSG-005
//fusa:req REQ-MSG-006
//fusa:req REQ-CONF-001
func ResponseToMessage(addr avtp.ByteBusID, resp acf.Message) relay.Message {
	op := "read"
	if resp.Control.Has(acf.FlagWrite) {
		op = "write"
	}
	return relay.Message{
		Protocol:  relay.RCP,
		ID:        EndpointIDString(addr),
		Payload:   resp.Body,
		Timestamp: time.Now(),
		Meta: map[string]string{
			"rcp.op":                   op,
			"rcp.error":                strconv.FormatBool(resp.Control.Has(acf.FlagError)),
			"rcp.transaction_num":      strconv.FormatUint(uint64(resp.TransactionNum), 10),
			"rcp.read_size_or_segment": strconv.FormatUint(uint64(resp.ReadSizeOrSegment), 10),
		},
	}
}
