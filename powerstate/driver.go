package powerstate

import (
	"sync"

	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/wakeup"
)

// Transmitter sends one wake-handshake message to target on behalf of a
// wakeup.Endpoint. A caller adapts this to whatever this server's actual
// transport for target's stream is; this package has no transport of its
// own, the same "no transport, caller supplies it" posture request.Handler
// establishes for the request package.
type Transmitter func(target avtp.StreamID, handshake wakeup.WakeHandshake) error

// Event reports one power-state-changed signal Driver.Pump observed while
// draining an Endpoint's trigger queue.
type Event struct {
	// State is the PowerState the endpoint transitioned to.
	State wakeup.PowerState
}

// Driver paces one wakeup.Endpoint's queued wake-handshake retransmissions
// and relays its power-state-changed triggers as Events. See doc.go for why
// this package exists at all: wakeup.Endpoint queues the full repeat count
// up front but paces/transmits none of it itself. All exported methods are
// safe for concurrent use.
type Driver struct {
	ep     *wakeup.Endpoint
	target avtp.StreamID
	send   Transmitter

	mu      sync.Mutex
	pending []wakeup.WakeHandshake
}

// NewDriver returns a Driver pacing ep's wake-handshake retransmissions
// toward target via send.
func NewDriver(ep *wakeup.Endpoint, target avtp.StreamID, send Transmitter) *Driver {
	return &Driver{ep: ep, target: target, send: send}
}

// Pump drains ep's trigger queue, returns every TriggerPowerStateChanged
// event observed (oldest first), and transmits at most one pending
// wake-handshake repeat via Transmitter — either one freshly drained this
// call, or one left over from a previous call whose Transmitter invocation
// failed (see below). A repeat is only removed from this Driver's own
// pending queue once send succeeds; a returned error leaves it at the front
// of that queue for the next Pump call to retry, so a transient transport
// failure costs a retry delay, not a lost repeat. A caller normally calls
// Pump again on its own next tick regardless of error — Acknowledge, not a
// clean send, is what stops the retries.
//
// Pump holds this Driver's own lock for its entire body, including the
// Transmitter call — the same "callback runs inside the critical section"
// posture request.Dispatcher.Pump already establishes for its own Handler
// calls, chosen here for the same reason: it is what makes "at most one
// send in flight, and no send can ever be popped twice by two concurrent
// Pump calls" trivially true rather than a separate invariant to maintain.
func (d *Driver) Pump() ([]Event, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	drained := d.ep.DrainTriggers()
	var events []Event
	for _, tr := range drained {
		switch tr.Kind {
		case wakeup.TriggerPowerStateChanged:
			events = append(events, Event{State: tr.State})
		case wakeup.TriggerWakeHandshake:
			d.pending = append(d.pending, tr.Handshake)
		}
	}

	if len(d.pending) == 0 {
		return events, nil
	}
	next := d.pending[0]
	if err := d.send(d.target, next); err != nil {
		return events, err
	}
	d.pending = d.pending[1:]
	return events, nil
}

// Acknowledge stops this wake cycle's retries: it discards every
// wake-handshake repeat ep has not yet queued out to this Driver (via
// ep.AcknowledgeWake) and every repeat this Driver had already pulled into
// its own pending buffer but not yet transmitted.
func (d *Driver) Acknowledge() {
	d.ep.AcknowledgeWake()
	d.mu.Lock()
	d.pending = nil
	d.mu.Unlock()
}

// Pending returns the number of wake-handshake repeats this Driver has
// pulled from ep but not yet successfully transmitted.
func (d *Driver) Pending() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.pending)
}
