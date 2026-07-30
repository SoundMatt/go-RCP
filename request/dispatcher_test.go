//fusa:test REQ-REQ-011
//fusa:test REQ-REQ-012
//fusa:test REQ-REQ-013
//fusa:test REQ-REQ-014
//fusa:test REQ-REQ-015
//fusa:test REQ-REQ-016
//fusa:test REQ-REQ-017
//fusa:test REQ-REQ-018
//fusa:test REQ-REQ-019
//fusa:test REQ-REQ-020

package request_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/gpio"
	"github.com/SoundMatt/go-RCP/request"
	"github.com/SoundMatt/go-RCP/server"
)

// rootStream and newGPIOEndpoint are this package's own test helpers,
// mirroring gpio_test's own newConfiguredEndpoint (request_test is an
// external test package and cannot reuse gpio_test's unexported helper).
func rootStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55}, 1)
}

func newGPIOEndpoint(t *testing.T, cfg gpio.Config) (*gpio.Endpoint, avtp.StreamID, avtp.ByteBusID) {
	t.Helper()
	root := rootStream()
	s := server.NewServer()
	if err := s.ClaimRoot(root); err != nil {
		t.Fatalf("ClaimRoot: %v", err)
	}
	addr := avtp.ByteBusID(1)
	if err := s.AddEndpoint(root, addr, gpio.EndpointType); err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}
	ep := gpio.NewEndpoint(s, addr)
	if err := ep.Configure(root, cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	return ep, root, addr
}

func gpioReadMsg(addr avtp.ByteBusID, txn avtp.TransactionNum) acf.Message {
	return acf.Message{Kind: acf.KindShort, ByteBusID: addr, TransactionNum: txn, Control: acf.FlagRead}
}

func gpioWriteMsg(addr avtp.ByteBusID, txn avtp.TransactionNum, sem gpio.WriteSemantic, operand uint32) acf.Message {
	return acf.Message{
		Kind: acf.KindShort, ByteBusID: addr, TransactionNum: txn, Control: acf.FlagWrite,
		Body: gpio.EncodeWriteRequest(sem, operand),
	}
}

// TestDispatcher_PlainRetrofit checks a KindShort Message (or a KindLong one
// with MTV set) is dispatched to the wrapped Handler completely unchanged —
// the Phase 14 plain-request path, retrofitted onto the lifecycle state
// machine without editing gpio itself (REQ-REQ-011). Only a KindLong message
// with MTV unset is treated as this package's own conditional/cancel
// envelope; see isConditionalEnvelope's doc comment in dispatcher.go.
func TestDispatcher_PlainRetrofit(t *testing.T) {
	ep, root, addr := newGPIOEndpoint(t, gpio.Config{PinCount: 4, Direction: 0b1111})
	d := request.NewDispatcher(ep, addr, request.NewSequencer(), nil)

	resp, err := d.Dispatch(root, gpioWriteMsg(addr, 1, gpio.SemanticOr, 0b0101), 0)
	if err != nil {
		t.Fatalf("Dispatch(write): %v", err)
	}
	v, err := gpio.DecodeValue(resp.Body)
	if err != nil || v != 0b0101 {
		t.Fatalf("write response = (%v, %v), want (0b0101, nil)", v, err)
	}

	resp, err = d.Dispatch(root, gpioReadMsg(addr, 2), 0)
	if err != nil {
		t.Fatalf("Dispatch(read): %v", err)
	}
	if !resp.Control.Has(acf.FlagResponse | acf.FlagRead) {
		t.Errorf("plain response Control = %v, want FlagResponse|FlagRead", resp.Control)
	}
	if resp.Kind != acf.KindShort {
		t.Errorf("plain response Kind = %v, want KindShort (not routed as a conditional envelope)", resp.Kind)
	}
}

// TestDispatcher_CompoundMatchAndAdvance checks a Compound request only
// reaches the wrapped Handler when its sequencer condition holds, and only
// advances the sequencer on a match (REQ-REQ-012).
func TestDispatcher_CompoundMatchAndAdvance(t *testing.T) {
	ep, root, addr := newGPIOEndpoint(t, gpio.Config{PinCount: 4, Direction: 0b1111})
	seq := request.NewSequencer()
	seq.Set(1, 10)
	d := request.NewDispatcher(ep, addr, seq, nil)

	// Condition false (10 != 20): the write must not reach the endpoint,
	// and the sequencer must stay at 10.
	unmatched := request.Conditional{Sequencer: 1, Op: request.CompareEqual, Operand: 20, AdvanceOnMatch: 5}
	body := request.EncodeCompound(unmatched, acf.FlagWrite, gpio.EncodeWriteRequest(gpio.SemanticOr, 0b0001))
	req := acf.Message{Kind: acf.KindLong, ByteBusID: addr, TransactionNum: 1, Control: acf.FlagWrite, Body: body}

	resp, err := d.Dispatch(root, req, 0)
	if err != nil {
		t.Fatalf("Dispatch(unmatched compound): %v", err)
	}
	res, _, err := request.DecodeConditionalResponse(resp.Body)
	if err != nil {
		t.Fatalf("DecodeConditionalResponse: %v", err)
	}
	if res.Matched {
		t.Errorf("Matched = true, want false (10 != 20)")
	}
	if got := seq.Get(1); got != 10 {
		t.Errorf("sequencer after unmatched compound = %d, want unchanged 10", got)
	}
	readResp, readErr := d.Dispatch(root, gpioReadMsg(addr, 2), 0)
	if readErr != nil {
		t.Fatalf("Dispatch(read): %v", readErr)
	}
	if v, _ := gpio.DecodeValue(readResp.Body); v != 0 {
		t.Errorf("endpoint value after unmatched compound = %d, want 0 (write never reached endpoint)", v)
	}

	// Condition true (10 == 10): the write must reach the endpoint, and the
	// sequencer must advance by AdvanceOnMatch.
	matched := request.Conditional{Sequencer: 1, Op: request.CompareEqual, Operand: 10, AdvanceOnMatch: 5}
	body = request.EncodeCompound(matched, acf.FlagWrite, gpio.EncodeWriteRequest(gpio.SemanticOr, 0b0001))
	req = acf.Message{Kind: acf.KindLong, ByteBusID: addr, TransactionNum: 3, Control: acf.FlagWrite, Body: body}

	resp, err = d.Dispatch(root, req, 0)
	if err != nil {
		t.Fatalf("Dispatch(matched compound): %v", err)
	}
	res, inner, err := request.DecodeConditionalResponse(resp.Body)
	if err != nil {
		t.Fatalf("DecodeConditionalResponse: %v", err)
	}
	if !res.Matched || res.SequencerValue != 15 {
		t.Errorf("ConditionalResult = %+v, want {Matched:true SequencerValue:15}", res)
	}
	if v, err := gpio.DecodeValue(inner); err != nil || v != 0b0001 {
		t.Errorf("wrapped inner response = (%v, %v), want (0b0001, nil)", v, err)
	}
	if got := seq.Get(1); got != 15 {
		t.Errorf("sequencer after matched compound = %d, want 15", got)
	}
}

// TestDispatcher_CompoundWaitNeverTouchesEndpoint checks a CompoundWait
// request never calls the wrapped Handler at all, regardless of whether its
// condition matches (REQ-REQ-013).
func TestDispatcher_CompoundWaitNeverTouchesEndpoint(t *testing.T) {
	ep, root, addr := newGPIOEndpoint(t, gpio.Config{PinCount: 4, Direction: 0b1111})
	seq := request.NewSequencer()
	seq.Set(2, 3)
	d := request.NewDispatcher(ep, addr, seq, nil)

	cond := request.Conditional{Sequencer: 2, Op: request.CompareGreaterOrEqual, Operand: 3, AdvanceOnMatch: 1}
	body := request.EncodeCompoundWait(cond)
	req := acf.Message{Kind: acf.KindLong, ByteBusID: addr, TransactionNum: 1, Control: 0, Body: body}

	resp, err := d.Dispatch(root, req, 0)
	if err != nil {
		t.Fatalf("Dispatch(compound-wait): %v", err)
	}
	res, inner, err := request.DecodeConditionalResponse(resp.Body)
	if err != nil {
		t.Fatalf("DecodeConditionalResponse: %v", err)
	}
	if !res.Matched || len(inner) != 0 {
		t.Errorf("ConditionalResult/inner = (%+v, %v), want (Matched:true, empty inner)", res, inner)
	}
	if got := seq.Get(2); got != 4 {
		t.Errorf("sequencer after matched compound-wait = %d, want 4 (still advances)", got)
	}

	// The endpoint's own pin value must still read zero: CompoundWait never
	// wrote to it.
	readResp, err := d.Dispatch(root, gpioReadMsg(addr, 2), 0)
	if err != nil {
		t.Fatalf("Dispatch(read): %v", err)
	}
	if v, _ := gpio.DecodeValue(readResp.Body); v != 0 {
		t.Errorf("endpoint value after compound-wait = %d, want 0", v)
	}
}

// TestDispatcher_Triggered checks a Triggered request stays pending until
// its registered TriggerPump reports a new event, consuming exactly one
// event per satisfied ticket (REQ-REQ-014).
func TestDispatcher_Triggered(t *testing.T) {
	ep, root, addr := newGPIOEndpoint(t, gpio.Config{PinCount: 4, Direction: 0b1111})
	d := request.NewDispatcher(ep, addr, request.NewSequencer(), nil)

	source := avtp.ByteBusID(9)
	events := 0
	d.RegisterTriggerSource(source, func() int {
		n := events
		events = 0
		return n
	})

	body := request.EncodeTriggered(source, acf.FlagWrite, gpio.EncodeWriteRequest(gpio.SemanticOr, 0b0001))
	req := acf.Message{Kind: acf.KindLong, ByteBusID: addr, TransactionNum: 1, Control: acf.FlagWrite, Body: body}

	id, err := d.Submit(root, req)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	d.Pump(0)
	if _, pollErr := d.Response(id); !errors.Is(pollErr, request.ErrPending) {
		t.Fatalf("Response before trigger fires = %v, want ErrPending", pollErr)
	}
	if st, _ := d.StateOf(id); st != request.StateStarted {
		t.Errorf("StateOf before trigger = %v, want StateStarted", st)
	}

	events = 1
	d.Pump(0)
	resp, err := d.Response(id)
	if err != nil {
		t.Fatalf("Response after trigger fires: %v", err)
	}
	if v, err := gpio.DecodeValue(resp.Body); err != nil || v != 0b0001 {
		t.Errorf("triggered response = (%v, %v), want (0b0001, nil)", v, err)
	}
}

// TestDispatcher_Timed checks a Timed request stays pending until Pump is
// called with now at or past its target time (REQ-REQ-015).
func TestDispatcher_Timed(t *testing.T) {
	ep, root, addr := newGPIOEndpoint(t, gpio.Config{PinCount: 4, Direction: 0b1111})
	d := request.NewDispatcher(ep, addr, request.NewSequencer(), nil)

	body := request.EncodeTimed(1000, acf.FlagWrite, gpio.EncodeWriteRequest(gpio.SemanticOr, 0b0010))
	req := acf.Message{Kind: acf.KindLong, ByteBusID: addr, TransactionNum: 1, Control: acf.FlagWrite, Body: body}

	id, err := d.Submit(root, req)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	d.Pump(500)
	if _, pollErr := d.Response(id); !errors.Is(pollErr, request.ErrPending) {
		t.Fatalf("Response before target time = %v, want ErrPending", pollErr)
	}

	d.Pump(1000)
	resp, err := d.Response(id)
	if err != nil {
		t.Fatalf("Response at target time: %v", err)
	}
	if v, err := gpio.DecodeValue(resp.Body); err != nil || v != 0b0010 {
		t.Errorf("timed response = (%v, %v), want (0b0010, nil)", v, err)
	}
}

// TestDispatcher_ChainedSequentialAndAbort checks a Chained request executes
// its segments strictly in order and aborts (without running later
// segments) the moment one segment's Handler call fails (REQ-REQ-016).
func TestDispatcher_ChainedSequentialAndAbort(t *testing.T) {
	ep, root, addr := newGPIOEndpoint(t, gpio.Config{PinCount: 4, Direction: 0b1111})
	d := request.NewDispatcher(ep, addr, request.NewSequencer(), nil)

	segs := []request.ChainedSegment{
		{Control: acf.FlagWrite, Body: gpio.EncodeWriteRequest(gpio.SemanticOr, 0b0001)},
		{Control: acf.FlagWrite, Body: gpio.EncodeWriteRequest(gpio.SemanticOr, 0b0010)},
		{Control: 0, Body: nil}, // neither Read nor Write: gpio rejects this
		{Control: acf.FlagWrite, Body: gpio.EncodeWriteRequest(gpio.SemanticOr, 0b1000)},
	}
	body, err := request.EncodeChained(segs)
	if err != nil {
		t.Fatalf("EncodeChained: %v", err)
	}
	req := acf.Message{Kind: acf.KindLong, ByteBusID: addr, TransactionNum: 1, Control: 0, Body: body}

	_, err = d.Dispatch(root, req, 0)
	if !errors.Is(err, request.ErrChainedSegmentFailed) {
		t.Fatalf("Dispatch(chained with failing segment) err = %v, want ErrChainedSegmentFailed", err)
	}

	// The first two segments must have actually run (pin bits 0 and 1 set);
	// the fourth must not have (pin bit 3 clear), since the chain aborted
	// at the third.
	readResp, err := d.Dispatch(root, gpioReadMsg(addr, 2), 0)
	if err != nil {
		t.Fatalf("Dispatch(read): %v", err)
	}
	if v, _ := gpio.DecodeValue(readResp.Body); v != 0b0011 {
		t.Errorf("endpoint value after aborted chain = %04b, want 0011 (only the two segments before the failure ran)", v)
	}
}

// TestDispatcher_CancelAll checks CancelAll clears every other pending
// ticket, executing before them even when submitted after, per the fixed
// cross-type priority ordering (REQ-REQ-017, REQ-REQ-019).
func TestDispatcher_CancelAll(t *testing.T) {
	ep, root, addr := newGPIOEndpoint(t, gpio.Config{PinCount: 4, Direction: 0b1111})
	d := request.NewDispatcher(ep, addr, request.NewSequencer(), nil)

	// Two Timed tickets, both not yet due, sit in StateStarted.
	body := request.EncodeTimed(1000, acf.FlagWrite, gpio.EncodeWriteRequest(gpio.SemanticOr, 0b0001))
	id1, err := d.Submit(root, acf.Message{Kind: acf.KindLong, ByteBusID: addr, TransactionNum: 1, Control: acf.FlagWrite, Body: body})
	if err != nil {
		t.Fatalf("Submit(timed 1): %v", err)
	}
	id2, err := d.Submit(root, acf.Message{Kind: acf.KindLong, ByteBusID: addr, TransactionNum: 2, Control: acf.FlagWrite, Body: body})
	if err != nil {
		t.Fatalf("Submit(timed 2): %v", err)
	}

	cancelReq := acf.Message{Kind: acf.KindLong, ByteBusID: addr, TransactionNum: 3, Control: 0, Body: request.EncodeCancelAll()}
	cancelID, err := d.Submit(root, cancelReq)
	if err != nil {
		t.Fatalf("Submit(cancel-all): %v", err)
	}

	// One Pump call, at a time both Timed tickets would otherwise be due:
	// cancellation's priority means they never execute.
	d.Pump(1000)

	resp, err := d.Response(cancelID)
	if err != nil {
		t.Fatalf("Response(cancel-all): %v", err)
	}
	n, err := request.DecodeCancelResponse(resp.Body)
	if err != nil || n != 2 {
		t.Fatalf("DecodeCancelResponse = (%d, %v), want (2, nil)", n, err)
	}

	for _, id := range []request.TicketID{id1, id2} {
		if _, pollErr := d.Response(id); !errors.Is(pollErr, request.ErrTicketCancelled) {
			t.Errorf("Response(%d) = %v, want ErrTicketCancelled", id, pollErr)
		}
	}

	// Confirm neither cancelled write actually reached the endpoint.
	readResp, err := d.Dispatch(root, gpioReadMsg(addr, 4), 0)
	if err != nil {
		t.Fatalf("Dispatch(read): %v", err)
	}
	if v, _ := gpio.DecodeValue(readResp.Body); v != 0 {
		t.Errorf("endpoint value after cancel-all = %04b, want 0000 (cancelled writes never ran)", v)
	}
}

// TestDispatcher_CancelTransactionAndSequencer checks the two optional
// narrower cancellation variants each clear only their specific target,
// leaving everything else pending (REQ-REQ-017).
func TestDispatcher_CancelTransactionAndSequencer(t *testing.T) {
	ep, root, addr := newGPIOEndpoint(t, gpio.Config{PinCount: 4, Direction: 0b1111})
	seq := request.NewSequencer()
	d := request.NewDispatcher(ep, addr, seq, nil)

	timedBody := request.EncodeTimed(1000, acf.FlagWrite, gpio.EncodeWriteRequest(gpio.SemanticOr, 0b0001))
	targetTxn := avtp.TransactionNum(42)
	target, err := d.Submit(root, acf.Message{Kind: acf.KindLong, ByteBusID: addr, TransactionNum: targetTxn, Control: acf.FlagWrite, Body: timedBody})
	if err != nil {
		t.Fatalf("Submit(target timed): %v", err)
	}
	bystander, err := d.Submit(root, acf.Message{Kind: acf.KindLong, ByteBusID: addr, TransactionNum: 43, Control: acf.FlagWrite, Body: timedBody})
	if err != nil {
		t.Fatalf("Submit(bystander timed): %v", err)
	}

	cancelTxnReq := acf.Message{Kind: acf.KindLong, ByteBusID: addr, TransactionNum: 44, Control: 0, Body: request.EncodeCancelTransaction(targetTxn)}
	cancelResp, err := d.Dispatch(root, cancelTxnReq, 0)
	if err != nil {
		t.Fatalf("Dispatch(cancel-transaction): %v", err)
	}
	if n, _ := request.DecodeCancelResponse(cancelResp.Body); n != 1 {
		t.Errorf("cancel-transaction cleared %d tickets, want 1", n)
	}
	if _, pollErr := d.Response(target); !errors.Is(pollErr, request.ErrTicketCancelled) {
		t.Errorf("Response(target) = %v, want ErrTicketCancelled", pollErr)
	}
	if _, pollErr := d.Response(bystander); !errors.Is(pollErr, request.ErrPending) {
		t.Errorf("Response(bystander) after cancel-transaction = %v, want ErrPending (untouched)", pollErr)
	}

	// Now exercise CancelSequencer: a Compound/CompoundWait pair gated on
	// sequencer 5, plus an unrelated Timed ticket that must survive.
	cond := request.Conditional{Sequencer: 5, Op: request.CompareEqual, Operand: 0, AdvanceOnMatch: 0}
	compoundWaitBody := request.EncodeCompoundWait(cond)
	gatedID, err := d.Submit(root, acf.Message{Kind: acf.KindLong, ByteBusID: addr, TransactionNum: 50, Control: 0, Body: compoundWaitBody})
	if err != nil {
		t.Fatalf("Submit(gated compound-wait): %v", err)
	}

	cancelSeqReq := acf.Message{Kind: acf.KindLong, ByteBusID: addr, TransactionNum: 51, Control: 0, Body: request.EncodeCancelSequencer(5)}
	cancelSeqResp, err := d.Dispatch(root, cancelSeqReq, 0)
	if err != nil {
		t.Fatalf("Dispatch(cancel-sequencer): %v", err)
	}
	if n, _ := request.DecodeCancelResponse(cancelSeqResp.Body); n != 1 {
		t.Errorf("cancel-sequencer cleared %d tickets, want 1", n)
	}
	if _, err := d.Response(gatedID); !errors.Is(err, request.ErrTicketCancelled) {
		t.Errorf("Response(gated) = %v, want ErrTicketCancelled", err)
	}
	if _, err := d.Response(bystander); !errors.Is(err, request.ErrPending) {
		t.Errorf("Response(bystander) after cancel-sequencer = %v, want still ErrPending", err)
	}
}

// TestDispatcher_AccessCheck checks Submit rejects a request outright when
// the configured AccessCheck fails, before any decoding or execution — in
// particular for KindCancelAll and KindCompoundWait, the two families that
// never reach the wrapped Handler and would otherwise bypass endpoint-level
// access control entirely (REQ-REQ-018).
func TestDispatcher_AccessCheck(t *testing.T) {
	ep, root, addr := newGPIOEndpoint(t, gpio.Config{PinCount: 4, Direction: 0b1111})
	denyAll := errors.New("denied")
	d := request.NewDispatcher(ep, addr, request.NewSequencer(), func(avtp.StreamID) error { return denyAll })

	cancelReq := acf.Message{Kind: acf.KindLong, ByteBusID: addr, TransactionNum: 1, Control: 0, Body: request.EncodeCancelAll()}
	if _, err := d.Submit(root, cancelReq); !errors.Is(err, denyAll) {
		t.Errorf("Submit(cancel-all, denied) err = %v, want %v", err, denyAll)
	}
	if _, err := d.Submit(root, gpioReadMsg(addr, 2)); !errors.Is(err, denyAll) {
		t.Errorf("Submit(plain, denied) err = %v, want %v", err, denyAll)
	}
}

// TestDispatcher_UnknownAndForget checks Response reports ErrUnknownTicket
// for an unissued id, Forget discards a finalized ticket's bookkeeping (and
// is a safe no-op otherwise), and Pending reflects only unresolved tickets
// (REQ-REQ-020).
func TestDispatcher_UnknownAndForget(t *testing.T) {
	ep, root, addr := newGPIOEndpoint(t, gpio.Config{PinCount: 4, Direction: 0b1111})
	d := request.NewDispatcher(ep, addr, request.NewSequencer(), nil)

	if _, err := d.Response(request.TicketID(999)); !errors.Is(err, request.ErrUnknownTicket) {
		t.Errorf("Response(unknown) = %v, want ErrUnknownTicket", err)
	}

	id, err := d.Submit(root, gpioReadMsg(addr, 1))
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if got := d.Pending(); got != 1 {
		t.Errorf("Pending() before Pump = %d, want 1", got)
	}
	d.Forget(id) // no-op: not yet finalized
	if _, err := d.Response(id); !errors.Is(err, request.ErrPending) {
		t.Errorf("Response after no-op Forget = %v, want still ErrPending", err)
	}

	d.Pump(0)
	if got := d.Pending(); got != 0 {
		t.Errorf("Pending() after Pump resolves the only ticket = %d, want 0", got)
	}
	d.Forget(id)
	if _, err := d.Response(id); !errors.Is(err, request.ErrUnknownTicket) {
		t.Errorf("Response after Forget = %v, want ErrUnknownTicket", err)
	}
}
