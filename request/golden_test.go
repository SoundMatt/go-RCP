//fusa:test REQ-REQ-021

package request_test

import (
	"bytes"
	"testing"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/gpio"
	"github.com/SoundMatt/go-RCP/request"
)

// ── REQ-REQ-021: frozen conditional-request envelope byte layouts ─────────
//
// These fixtures pin the exact wire bytes this package's encoders produce
// today, the same posture gpio/golden_test.go, spi/golden_test.go, and
// their siblings established for their own packages — so Phase 16's
// remaining endpoint types and Milestone 50's E2E-CRC safety work can
// regression-test against a frozen encoding rather than re-deriving it from
// current behaviour.

// goldenCompoundInnerBody is a valid gpio write request body (SemanticOr,
// operand 0x00000005), so TestGolden_EndToEndDispatch below can dispatch
// goldenCompound against a real gpio.Endpoint, not just round-trip the
// envelope in isolation.
var goldenCompoundInnerBody = []byte{0x01, 0x00, 0x00, 0x00, 0x05}

// goldenCompound is Sequencer=0x0001, Op=CompareEqual(0), Operand=0x0000000A,
// AdvanceOnMatch=0x00000001, inner Control=FlagWrite, inner body =
// goldenCompoundInnerBody.
var goldenCompound = []byte{
	byte(request.KindCompound),
	0x00, 0x01, // Sequencer
	0x00,                   // Op = CompareEqual
	0x00, 0x00, 0x00, 0x0A, // Operand
	0x00, 0x00, 0x00, 0x01, // AdvanceOnMatch
	byte(acf.FlagWrite),          // inner Control
	0x01, 0x00, 0x00, 0x00, 0x05, // inner Body (goldenCompoundInnerBody)
}

func TestGolden_Compound(t *testing.T) {
	c := request.Conditional{Sequencer: 1, Op: request.CompareEqual, Operand: 10, AdvanceOnMatch: 1}
	got := request.EncodeCompound(c, acf.FlagWrite, goldenCompoundInnerBody)
	if !bytes.Equal(got, goldenCompound) {
		t.Fatalf("EncodeCompound changed:\n got  % X\n want % X", got, goldenCompound)
	}
	gotC, gotControl, gotBody, err := request.DecodeCompound(goldenCompound)
	if err != nil {
		t.Fatalf("DecodeCompound(golden): %v", err)
	}
	if gotC != c || gotControl != acf.FlagWrite || !bytes.Equal(gotBody, goldenCompoundInnerBody) {
		t.Errorf("DecodeCompound(golden) = (%+v, %v, % X), want (%+v, %v, % X)", gotC, gotControl, gotBody, c, acf.FlagWrite, goldenCompoundInnerBody)
	}
}

// goldenCancelAll is just the Kind byte.
var goldenCancelAll = []byte{byte(request.KindCancelAll)}

func TestGolden_CancelAll(t *testing.T) {
	got := request.EncodeCancelAll()
	if !bytes.Equal(got, goldenCancelAll) {
		t.Fatalf("EncodeCancelAll changed:\n got  % X\n want % X", got, goldenCancelAll)
	}
}

// TestGolden_EndToEndDispatch checks a golden Compound envelope, dispatched
// end-to-end against a real gpio.Endpoint, produces the expected wrapped
// write and ConditionalResult.
func TestGolden_EndToEndDispatch(t *testing.T) {
	ep, root, addr := newGPIOEndpoint(t, gpio.Config{PinCount: 8, Direction: 0xFF})
	seq := request.NewSequencer()
	seq.Set(1, 10) // matches goldenCompound's Operand
	d := request.NewDispatcher(ep, addr, seq, nil)

	req := acf.Message{Kind: acf.KindShort, ByteBusID: addr, TransactionNum: 1, Control: acf.FlagWrite | acf.FlagExtended, Body: goldenCompound}
	resp, err := d.Dispatch(root, req, 0)
	if err != nil {
		t.Fatalf("Dispatch(golden compound): %v", err)
	}
	res, inner, err := request.DecodeConditionalResponse(resp.Body)
	if err != nil {
		t.Fatalf("DecodeConditionalResponse: %v", err)
	}
	if !res.Matched || res.SequencerValue != 11 {
		t.Errorf("ConditionalResult = %+v, want {Matched:true SequencerValue:11}", res)
	}
	if v, err := gpio.DecodeValue(inner); err != nil || v != 0x05 {
		t.Errorf("wrapped inner response = (%v, %v), want (5, nil)", v, err)
	}
}
