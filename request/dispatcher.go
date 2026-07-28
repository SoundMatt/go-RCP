package request

import (
	"fmt"
	"sort"
	"sync"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
)

// Handler is the shape every Phase 14 endpoint type's Endpoint.HandleRequest
// method already satisfies (gpio.Endpoint, spi.Endpoint, i2c.Endpoint,
// uart.Endpoint, adc.Endpoint, pwm.Endpoint), used here without requiring
// any of those packages to import this one or change their own signature —
// this is how Dispatcher "retrofits" their already-shipped plain-request
// path onto the request-lifecycle state machine (ROADMAP.md Milestone 49):
// by wrapping, not editing.
type Handler interface {
	HandleRequest(requester avtp.StreamID, req acf.Message) (acf.Message, error)
}

// TriggerPump is a caller-supplied adapter reporting how many new trigger
// events another endpoint has queued since TriggerPump was last invoked. A
// caller wires a Phase 14 endpoint's own typed DrainTriggers method into
// this shape, e.g. for a GPIO trigger source:
//
//	dispatcher.RegisterTriggerSource(gpioAddr, func() int {
//	    return len(gpioEndpoint.DrainTriggers())
//	})
//
// Dispatcher.Pump calls a registered TriggerPump at most once per Pump call,
// even when multiple KindTriggered tickets wait on the same source — the
// resulting count is then consumed across those waiting tickets, oldest
// first, one event per satisfied ticket.
type TriggerPump func() int

// SafeStateCheck is an optional caller-supplied gate reporting whether the
// endpoint a Dispatcher wraps is currently in its configured safe state, for
// the given requester stream (ROADMAP.md Milestone 50). It is the readiness
// condition every safety-request ("MSB-set") Kind adds on top of its base
// kind's own readiness rule: Dispatcher.Pump never lets a safety-request
// ticket advance past StateStarted while this reports false. A caller
// normally backs this with a e2e.Supervisor's InSafeState method — this
// package has no wall-clock or sequence-monotonicity tracking of its own
// (see doc.go). It takes the requester rather than being a single endpoint-
// wide bool because the watchdog condition driving safe-state entry is
// tracked per stream, not per endpoint (see e2e.Supervisor).
type SafeStateCheck func(requester avtp.StreamID) bool

// AccessCheck is an optional caller-supplied gate Dispatcher.Submit runs for
// every request, before any kind-specific decoding or handling: it must
// return nil for the request to be admitted at all. A caller normally adapts
// this from the same access-control path an endpoint's own HandleRequest
// already uses (e.g. server.Server.ReadEndpoint), since two conditional-
// request kinds — KindCompoundWait and every cancellation variant — never
// reach the wrapped Handler at all and would otherwise bypass that endpoint-
// level check entirely.
type AccessCheck func(requester avtp.StreamID) error

// Dispatcher is the request-lifecycle state machine for one endpoint
// (ROADMAP.md Milestone 49): it decodes every inbound acf.Message —
// plain or conditional — into a ticket, advances that ticket through
// StateQueued -> StateStarted -> StateExecuting -> StateFinalized, and
// applies the fixed cross-type execution-priority ordering (Kind.Priority)
// whenever more than one ticket becomes eligible to execute within the same
// Pump call. It wraps exactly one Handler (one endpoint) and one Sequencer
// bank; a server exposing several endpoint types runs one Dispatcher per
// endpoint, each with its own Sequencer, mirroring how each Phase 14
// package's Endpoint is already one-per-declared-endpoint. All exported
// methods are safe for concurrent use.
type Dispatcher struct {
	mu        sync.Mutex
	handler   Handler
	addr      avtp.ByteBusID
	seq       *Sequencer
	access    AccessCheck
	safeState SafeStateCheck
	pumps     map[avtp.ByteBusID]TriggerPump

	tickets map[TicketID]*ticket
	order   []TicketID
	nextID  TicketID
}

// NewDispatcher returns a Dispatcher for the endpoint addr, wrapping handler
// and backed by seq (create one with NewSequencer; pass the same *Sequencer
// to every Dispatcher that should share one sequencer-register namespace,
// or a fresh one to keep this endpoint's sequencers private — either is a
// legitimate deployment choice this package does not prescribe). access may
// be nil, in which case Submit performs no admission check of its own
// beyond what handler's own HandleRequest enforces for the kinds that reach
// it.
func NewDispatcher(handler Handler, addr avtp.ByteBusID, seq *Sequencer, access AccessCheck) *Dispatcher {
	return &Dispatcher{
		handler: handler,
		addr:    addr,
		seq:     seq,
		access:  access,
		pumps:   make(map[avtp.ByteBusID]TriggerPump),
		tickets: make(map[TicketID]*ticket),
	}
}

// RegisterTriggerSource wires pump as the TriggerPump for source. Later
// calls for the same source replace the previous registration.
func (d *Dispatcher) RegisterTriggerSource(source avtp.ByteBusID, pump TriggerPump) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pumps[source] = pump
}

// SetSafeStateCheck wires check as this Dispatcher's SafeStateCheck
// (ROADMAP.md Milestone 50), replacing any previous registration. Pass nil
// to withdraw a Dispatcher's opt-in to the safety-request Kinds — once
// withdrawn, Submit rejects every new safety-request Kind with
// ErrSafeStateNotConfigured again, though any already-admitted safety
// ticket stays pending exactly as if the check were still configured but
// permanently false, rather than being silently discarded.
func (d *Dispatcher) SetSafeStateCheck(check SafeStateCheck) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.safeState = check
}

// Submit decodes req into a new ticket and admits it (StateQueued), running
// AccessCheck first if one was configured. It does not itself advance the
// ticket past StateQueued — call Pump (directly, or via Dispatch) to make
// progress. Submit returns the assigned TicketID once admission succeeds.
func (d *Dispatcher) Submit(requester avtp.StreamID, req acf.Message) (TicketID, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.access != nil {
		if err := d.access(requester); err != nil {
			return 0, err
		}
	}

	t := &ticket{id: d.nextID, requester: requester, original: req, state: StateQueued}

	if !req.Control.Has(acf.FlagExtended) {
		t.kind = KindPlain
		t.innerControl = req.Control
		t.innerBody = req.Body
	} else if err := decodeInto(t, req.Body); err != nil {
		return 0, err
	}

	if t.kind.IsSafety() && d.safeState == nil {
		return 0, ErrSafeStateNotConfigured
	}

	d.nextID++
	d.tickets[t.id] = t
	d.order = append(d.order, t.id)
	return t.id, nil
}

// decodeInto decodes req's extended-request envelope into t's kind-specific
// fields. It dispatches on k.Base() throughout: a safety-request ("MSB-set")
// envelope shares its base kind's exact body layout (see envelope.go's
// EncodeXxxSafety functions), so decoding never needs to special-case the
// tag bit — only readiness evaluation and execution eligibility do (see
// Dispatcher.Pump and Dispatcher.Submit).
func decodeInto(t *ticket, body []byte) error {
	k, err := PeekKind(body)
	if err != nil {
		return err
	}
	t.kind = k
	switch k.Base() {
	case KindCompound:
		c, ic, ib, err := DecodeCompound(body)
		if err != nil {
			return err
		}
		t.conditional, t.innerControl, t.innerBody = c, ic, ib
	case KindCompoundWait:
		c, err := DecodeCompoundWait(body)
		if err != nil {
			return err
		}
		t.conditional = c
	case KindTriggered:
		src, ic, ib, err := DecodeTriggered(body)
		if err != nil {
			return err
		}
		t.triggerSource, t.innerControl, t.innerBody = src, ic, ib
	case KindChained:
		segs, err := DecodeChained(body)
		if err != nil {
			return err
		}
		t.segments = segs
	case KindTimed:
		at, ic, ib, err := DecodeTimed(body)
		if err != nil {
			return err
		}
		t.executeAtMicros, t.innerControl, t.innerBody = at, ic, ib
	case KindCancelAll:
		if err := DecodeCancelAll(body); err != nil {
			return err
		}
	case KindCancelTransaction:
		txn, err := DecodeCancelTransaction(body)
		if err != nil {
			return err
		}
		t.cancelTxn = txn
	case KindCancelSequencer:
		id, err := DecodeCancelSequencer(body)
		if err != nil {
			return err
		}
		t.cancelSeq = id
	default:
		return ErrInvalidKind
	}
	return nil
}

// Pump advances every StateQueued ticket to StateStarted, determines which
// StateStarted tickets are ready to execute given now (a caller-supplied
// monotonic microsecond clock — its epoch and units are a matter of caller/
// Dispatcher agreement, since real time synchronization is out of this
// package's scope, per avtp/doc.go's own posture on the subject), each
// registered TriggerPump's current count, and — for a safety-request
// ("MSB-set") ticket — the configured SafeStateCheck's current answer for
// that ticket's requester, executes every ready ticket in fixed cross-type
// priority order (Kind.Priority, FIFO within a rank), and returns the
// TicketIDs that reached StateFinalized during this call — including any a
// cancellation ticket in the same batch cleared before its own turn to
// execute. A safety-request ticket whose SafeStateCheck currently reports
// false simply stays in StateStarted, re-evaluated on every later Pump call,
// the same way an unmet KindTimed target time or an empty KindTriggered
// source does.
func (d *Dispatcher) Pump(now uint64) []TicketID {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, id := range d.order {
		if t := d.tickets[id]; t.state == StateQueued {
			t.state = StateStarted
		}
	}

	// Call each distinct trigger source's pump at most once this call.
	// A safety-request KindTriggeredSafety ticket that is not (yet) ready
	// per the safe-state gate below still counts toward this: the pump is
	// still called so the source's event count stays accurate, it simply
	// isn't consumed from `available` until the ticket also clears the
	// safe-state check.
	available := make(map[avtp.ByteBusID]int)
	for _, id := range d.order {
		t := d.tickets[id]
		if t.state != StateStarted || t.kind.Base() != KindTriggered {
			continue
		}
		if _, seen := available[t.triggerSource]; seen {
			continue
		}
		if pump, ok := d.pumps[t.triggerSource]; ok {
			available[t.triggerSource] = pump()
		} else {
			available[t.triggerSource] = 0
		}
	}

	var ready []*ticket
	for _, id := range d.order {
		t := d.tickets[id]
		if t.state != StateStarted {
			continue
		}
		if t.kind.IsSafety() && !d.safeStateReadyLocked(t.requester) {
			continue
		}
		switch t.kind.Base() {
		case KindTriggered:
			if available[t.triggerSource] > 0 {
				available[t.triggerSource]--
				ready = append(ready, t)
			}
		case KindTimed:
			if now >= t.executeAtMicros {
				ready = append(ready, t)
			}
		default:
			ready = append(ready, t)
		}
	}

	sort.SliceStable(ready, func(i, j int) bool {
		return ready[i].kind.Priority() < ready[j].kind.Priority()
	})

	var finalized []TicketID
	for _, t := range ready {
		if t.state == StateFinalized {
			// A cancellation ticket earlier in this same priority-ordered
			// batch already cleared it.
			continue
		}
		t.state = StateExecuting
		cancelled := d.execute(t)
		t.state = StateFinalized
		finalized = append(finalized, t.id)
		finalized = append(finalized, cancelled...)
	}
	return finalized
}

// safeStateReadyLocked reports whether a safety-request ticket submitted by
// requester is currently allowed to advance to StateExecuting. Callers must
// hold d.mu. It only ever returns true when a SafeStateCheck is configured
// and reports true — Submit already refuses to admit a safety-request Kind
// at all when d.safeState is nil (see ErrSafeStateNotConfigured), but
// SetSafeStateCheck(nil) can withdraw that configuration later while a
// safety ticket it already admitted is still pending, so this stays
// defensive rather than assuming non-nil.
func (d *Dispatcher) safeStateReadyLocked(requester avtp.StreamID) bool {
	return d.safeState != nil && d.safeState(requester)
}

// Dispatch is the synchronous convenience path: Submit followed by one Pump
// call and a Response lookup, giving callers of the conditional kinds that
// always resolve immediately (everything except a not-yet-satisfied
// KindTriggered/KindTimed) the same synchronous shape Phase 14 endpoints'
// own HandleRequest already has. A Triggered/Timed ticket not yet due
// returns (acf.Message{}, ErrPending) wrapping the TicketID for later
// polling — see Response.
func (d *Dispatcher) Dispatch(requester avtp.StreamID, req acf.Message, now uint64) (acf.Message, error) {
	id, err := d.Submit(requester, req)
	if err != nil {
		return acf.Message{}, err
	}
	d.Pump(now)
	resp, err := d.Response(id)
	if err == ErrPending {
		return acf.Message{}, fmt.Errorf("%w (ticket %d)", ErrPending, id)
	}
	return resp, err
}

// Response returns ticket id's outcome. It reports ErrUnknownTicket if id
// was never issued (or has been Forgotten), and ErrPending if the ticket
// exists but has not yet reached StateFinalized.
func (d *Dispatcher) Response(id TicketID) (acf.Message, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	t, ok := d.tickets[id]
	if !ok {
		return acf.Message{}, ErrUnknownTicket
	}
	if t.state != StateFinalized {
		return acf.Message{}, ErrPending
	}
	return t.response, t.err
}

// StateOf reports ticket id's current lifecycle State, and whether id is
// known to this Dispatcher at all.
func (d *Dispatcher) StateOf(id TicketID) (State, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	t, ok := d.tickets[id]
	if !ok {
		return 0, false
	}
	return t.state, true
}

// Forget discards a finalized ticket's bookkeeping, freeing the memory a
// long-running Dispatcher would otherwise accumulate. It is a no-op if id is
// unknown or not yet finalized — callers that want to abandon a still-
// pending ticket should cancel it instead (see EncodeCancelTransaction).
func (d *Dispatcher) Forget(id TicketID) {
	d.mu.Lock()
	defer d.mu.Unlock()
	t, ok := d.tickets[id]
	if !ok || t.state != StateFinalized {
		return
	}
	delete(d.tickets, id)
	for i, oid := range d.order {
		if oid == id {
			d.order = append(d.order[:i], d.order[i+1:]...)
			break
		}
	}
}

// Pending returns the number of tickets this Dispatcher still holds that
// have not reached StateFinalized.
func (d *Dispatcher) Pending() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := 0
	for _, id := range d.order {
		if d.tickets[id].state != StateFinalized {
			n++
		}
	}
	return n
}

// innerMessage rebuilds the plain acf.Message a Phase 14 endpoint's own
// HandleRequest expects, from t's original request-descriptor fields plus
// the given inner Control/Body — used for every kind that ultimately calls
// d.handler.HandleRequest (Plain, a matched Compound, Triggered, Timed, and
// each Chained segment).
func innerMessage(t *ticket, control acf.ControlFlags, body []byte) acf.Message {
	return acf.Message{
		Kind:              t.original.Kind,
		ByteBusID:         t.original.ByteBusID,
		TransactionNum:    t.original.TransactionNum,
		Control:           control,
		ReadSizeOrSegment: t.original.ReadSizeOrSegment,
		Timestamp:         t.original.Timestamp,
		Body:              body,
	}
}

// responseFor builds t's outer response Message: same Kind/ByteBusID/
// TransactionNum as the original request for correlation, FlagResponse
// always set, acf.FlagExtended set for every kind whose response body is
// this package's own envelope shape (everything except KindPlain,
// KindTriggered, and KindTimed, whose responses are the wrapped Handler
// call's own response verbatim — see execute).
func responseFor(t *ticket, body []byte) acf.Message {
	control := acf.FlagResponse | acf.FlagExtended | (t.innerControl & (acf.FlagRead | acf.FlagWrite))
	return acf.Message{
		Kind:           t.original.Kind,
		ByteBusID:      t.original.ByteBusID,
		TransactionNum: t.original.TransactionNum,
		Control:        control,
		Body:           body,
	}
}

// execute runs t's kind-specific StateExecuting action, populating
// t.response/t.err. Callers must hold d.mu. It returns the TicketIDs of any
// other tickets a cancellation kind finalized as a side effect. It switches
// on t.kind.Base() throughout: a safety-request ticket's execution behavior
// is identical to its base kind's once Pump has already gated it on the
// safe-state readiness check (see safeStateReadyLocked) — the safety tag
// only changes when a ticket is allowed to reach this method, never what it
// does once it's here.
func (d *Dispatcher) execute(t *ticket) []TicketID {
	switch t.kind.Base() {
	case KindPlain:
		t.response, t.err = d.handler.HandleRequest(t.requester, innerMessage(t, t.innerControl, t.innerBody))
		return nil

	case KindCompound:
		matched, val := t.conditional.evaluate(d.seq)
		var innerBody []byte
		if matched {
			resp, err := d.handler.HandleRequest(t.requester, innerMessage(t, t.innerControl, t.innerBody))
			if err != nil {
				t.err = err
				return nil
			}
			innerBody = resp.Body
		}
		t.response = responseFor(t, EncodeConditionalResponse(ConditionalResult{Matched: matched, SequencerValue: val}, innerBody))
		return nil

	case KindCompoundWait:
		matched, val := t.conditional.evaluate(d.seq)
		t.response = responseFor(t, EncodeConditionalResponse(ConditionalResult{Matched: matched, SequencerValue: val}, nil))
		return nil

	case KindTriggered, KindTimed:
		t.response, t.err = d.handler.HandleRequest(t.requester, innerMessage(t, t.innerControl, t.innerBody))
		return nil

	case KindChained:
		result := ChainedResult{Total: len(t.segments)}
		for _, seg := range t.segments {
			resp, err := d.handler.HandleRequest(t.requester, innerMessage(t, seg.Control, seg.Body))
			if err != nil {
				result.Responses = append(result.Responses, ChainedResponse{Failed: true})
				t.err = fmt.Errorf("%w: %v", ErrChainedSegmentFailed, err)
				break
			}
			result.Responses = append(result.Responses, ChainedResponse{Body: resp.Body})
		}
		t.response = responseFor(t, EncodeChainedResponse(result))
		return nil

	case KindCancelAll:
		cancelled := d.cancelWhereLocked(t.id, func(*ticket) bool { return true })
		t.response = responseFor(t, EncodeCancelResponse(uint16(len(cancelled))))
		return cancelled

	case KindCancelTransaction:
		txn := t.cancelTxn
		cancelled := d.cancelWhereLocked(t.id, func(other *ticket) bool {
			return other.original.TransactionNum == txn
		})
		t.response = responseFor(t, EncodeCancelResponse(uint16(len(cancelled))))
		return cancelled

	case KindCancelSequencer:
		seqID := t.cancelSeq
		cancelled := d.cancelWhereLocked(t.id, func(other *ticket) bool {
			return (other.kind == KindCompound || other.kind == KindCompoundWait) && other.conditional.Sequencer == seqID
		})
		t.response = responseFor(t, EncodeCancelResponse(uint16(len(cancelled))))
		return cancelled

	default:
		t.err = ErrInvalidKind
		return nil
	}
}

// cancelWhereLocked finalizes (as StateFinalized/ErrTicketCancelled) every
// ticket other than exclude for which match reports true and that has not
// already reached StateFinalized, returning their TicketIDs. Callers must
// hold d.mu.
func (d *Dispatcher) cancelWhereLocked(exclude TicketID, match func(*ticket) bool) []TicketID {
	return d.finalizeWhereLocked(ErrTicketCancelled, func(other *ticket) bool {
		return other.id != exclude && match(other)
	})
}

// finalizeWhereLocked finalizes (as StateFinalized/finalErr) every
// not-yet-finalized ticket for which match reports true, returning their
// TicketIDs. Callers must hold d.mu. It is the shared mechanism behind both
// a client's own cancellation requests (cancelWhereLocked, finalErr
// ErrTicketCancelled) and this Dispatcher's own watchdog-driven purge
// (PurgeNonSafety, finalErr ErrPurgedByWatchdog) — the two are kept as
// distinct errors precisely because they are semantically different events
// for a caller polling Response to distinguish (a client asked for the
// first; this Dispatcher's own fault response produced the second).
func (d *Dispatcher) finalizeWhereLocked(finalErr error, match func(*ticket) bool) []TicketID {
	var cleared []TicketID
	for _, id := range d.order {
		t := d.tickets[id]
		if t.state == StateFinalized || !match(t) {
			continue
		}
		t.state = StateFinalized
		t.err = finalErr
		cleared = append(cleared, id)
	}
	return cleared
}

// PurgeNonSafety finalizes (as StateFinalized/ErrPurgedByWatchdog) every
// not-yet-finalized ticket whose Kind is not a safety-request ("MSB-set")
// variant, leaving every safety-request ticket untouched regardless of its
// current state or readiness (ROADMAP.md Milestone 50's "watchdog-driven
// purge of ordinary pending requests"). It returns the TicketIDs it
// finalized.
//
// This Dispatcher never calls PurgeNonSafety on its own: it has no wall-
// clock or per-stream sequence-monotonicity tracking of its own (see
// SafeStateCheck's doc comment) to decide when a purge is warranted. A
// caller wires a e2e.Supervisor's watchdog trip to this method — the
// same asymmetry SafeStateCheck has, where this package only ever consumes
// the watchdog's verdict, never computes it.
func (d *Dispatcher) PurgeNonSafety() []TicketID {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.finalizeWhereLocked(ErrPurgedByWatchdog, func(t *ticket) bool {
		return !t.kind.IsSafety()
	})
}
