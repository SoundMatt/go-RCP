//fusa:test REQ-REQ-019

package request_test

import (
	"testing"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/gpio"
	"github.com/SoundMatt/go-RCP/request"
)

// orderRecorder wraps a gpio.Endpoint and records the order in which
// HandleRequest is actually invoked, by inspecting each write's operand —
// this test's way of observing Dispatcher.Pump's execution order across
// several different Kinds that all become ready within the same call.
type orderRecorder struct {
	ep    *gpio.Endpoint
	order *[]uint32
}

func (r orderRecorder) HandleRequest(requester avtp.StreamID, req acf.Message) (acf.Message, error) {
	resp, err := r.ep.HandleRequest(requester, req)
	if err == nil && req.Control.Has(acf.FlagWrite) {
		if operand, decErr := gpio.DecodeWriteRequest(req.Body); decErr == nil && req.EVTSelector() == acf.EVTSelector1 {
			*r.order = append(*r.order, operand)
		}
	}
	return resp, err
}

// TestDispatcher_PriorityOrdering checks that when a Timed ticket, a
// Compound ticket, and a Plain ticket all become ready to execute within
// the same Pump call, they run in the fixed cross-type order Kind.Priority
// documents (Timed before Compound before Plain), regardless of submission
// order (REQ-REQ-019).
func TestDispatcher_PriorityOrdering(t *testing.T) {
	ep, root, addr := newGPIOEndpoint(t, gpio.Config{PinCount: 8, Direction: 0xFF})
	var order []uint32
	handler := orderRecorder{ep: ep, order: &order}
	d := request.NewDispatcher(handler, addr, request.NewSequencer(), nil)

	// Submitted in Plain, Compound, Timed order — the reverse of their
	// execution-priority rank — so a passing test can only mean priority
	// ordering, not submission-order coincidence.
	plainOperand := uint32(0x01)
	if _, err := d.Submit(root, gpioWriteMsg(addr, 1, acf.EVTSelector1, plainOperand)); err != nil {
		t.Fatalf("Submit(plain): %v", err)
	}

	compoundOperand := uint32(0x02)
	matched := request.Conditional{Sequencer: 1, Op: request.CompareEqual, Operand: 0, AdvanceOnMatch: 0}
	compoundBody := request.EncodeCompound(matched, acf.FlagWrite, gpio.EncodeWriteRequest(compoundOperand))
	if _, err := d.Submit(root, acf.Message{Kind: acf.KindLong, ByteBusID: addr, TransactionNum: 2, EVT: uint8(acf.EVTSelector1), Control: acf.FlagWrite, Body: compoundBody}); err != nil {
		t.Fatalf("Submit(compound): %v", err)
	}

	timedOperand := uint32(0x04)
	timedBody := request.EncodeTimed(1000, acf.FlagWrite, gpio.EncodeWriteRequest(timedOperand))
	if _, err := d.Submit(root, acf.Message{Kind: acf.KindLong, ByteBusID: addr, TransactionNum: 3, EVT: uint8(acf.EVTSelector1), Control: acf.FlagWrite, Body: timedBody}); err != nil {
		t.Fatalf("Submit(timed): %v", err)
	}

	// One Pump call, at a time the Timed ticket is also due: all three
	// become ready simultaneously.
	d.Pump(1000)

	want := []uint32{timedOperand, compoundOperand, plainOperand}
	if len(order) != len(want) {
		t.Fatalf("execution order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Errorf("execution order = %v, want %v (Timed before Compound before Plain)", order, want)
			break
		}
	}
}
