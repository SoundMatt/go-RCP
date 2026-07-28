//fusa:test REQ-FRAG-010
//fusa:test REQ-FRAG-011

package fragment_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/fragment"
	"github.com/SoundMatt/go-RCP/request"
)

// echoHandler is a minimal request.Handler test double recording the
// (fully reassembled) request it was handed and echoing its Body back.
type echoHandler struct {
	got acf.Message
	n   int
}

func (h *echoHandler) HandleRequest(_ avtp.StreamID, req acf.Message) (acf.Message, error) {
	h.got = req
	h.n++
	resp := req
	resp.Control = acf.FlagResponse
	return resp, nil
}

func newGatewayFixture() (*fragment.Gateway, *echoHandler, *request.Dispatcher) {
	h := &echoHandler{}
	d := request.NewDispatcher(h, avtp.ByteBusID(1), request.NewSequencer(), nil)
	gw := fragment.NewGateway(d, fragment.NewReassembler(fragment.Config{}), 10)
	return gw, h, d
}

// TestGateway_Submit_PassesUnfragmentedRequestThrough checks an ordinary
// (unfragmented) request reaches the wrapped request.Dispatcher directly,
// with a real request.TicketID resolvable through the normal
// Submit/Pump/Response lifecycle (REQ-FRAG-010).
func TestGateway_Submit_PassesUnfragmentedRequestThrough(t *testing.T) {
	gw, h, d := newGatewayFixture()
	stream := testStream()
	req := acf.Message{ByteBusID: 1, TransactionNum: 1, Control: acf.FlagWrite, Body: []byte{0x01, 0x02}}

	id, err := gw.Submit(stream, req)
	if err != nil {
		t.Fatalf("Submit(unfragmented): %v", err)
	}
	d.Pump(0)
	resp, err := d.Response(id)
	if err != nil {
		t.Fatalf("Response: %v", err)
	}
	if !bytes.Equal(resp.Body, req.Body) {
		t.Errorf("Response body = % X, want echoed % X", resp.Body, req.Body)
	}
	if h.n != 1 || !bytes.Equal(h.got.Body, req.Body) {
		t.Errorf("wrapped Handler saw %+v (n=%d), want one call with the original body", h.got, h.n)
	}
}

// TestGateway_Submit_ReassemblesBeforeDispatching checks a fragmented
// request is fully reassembled before the wrapped request.Dispatcher (and,
// beneath it, the wrapped Handler) ever sees it, and that non-terminal
// segments report ErrAwaitingSegments with no ticket admitted
// (REQ-FRAG-010).
func TestGateway_Submit_ReassemblesBeforeDispatching(t *testing.T) {
	gw, h, d := newGatewayFixture()
	stream := testStream()
	body := bytes.Repeat([]byte{0x07}, 35)
	original := acf.Message{ByteBusID: 1, TransactionNum: 2, Control: acf.FlagWrite, Body: body}

	segs, err := fragment.Split(original, 10)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(segs) < 2 {
		t.Fatalf("test setup: want multiple segments, got %d", len(segs))
	}

	for _, seg := range segs[:len(segs)-1] {
		if _, submitErr := gw.Submit(stream, seg); !errors.Is(submitErr, fragment.ErrAwaitingSegments) {
			t.Fatalf("Submit(non-terminal segment) err = %v, want ErrAwaitingSegments", submitErr)
		}
		if h.n != 0 {
			t.Fatalf("wrapped Handler called before reassembly completed (n=%d)", h.n)
		}
	}

	id, err := gw.Submit(stream, segs[len(segs)-1])
	if err != nil {
		t.Fatalf("Submit(terminal segment): %v", err)
	}
	d.Pump(0)
	resp, err := d.Response(id)
	if err != nil {
		t.Fatalf("Response: %v", err)
	}
	if !bytes.Equal(resp.Body, body) {
		t.Errorf("Response body = % X, want reassembled % X", resp.Body, body)
	}
	if h.n != 1 || !bytes.Equal(h.got.Body, body) {
		t.Errorf("wrapped Handler saw %+v (n=%d), want exactly one call with the reassembled body", h.got, h.n)
	}
}

// TestGateway_Response_SplitsLargeResponse checks Response splits a
// too-large resolved response the same way Split does, and leaves a small
// response unchanged (REQ-FRAG-011).
func TestGateway_Response_SplitsLargeResponse(t *testing.T) {
	gw, _, _ := newGatewayFixture()

	small := acf.Message{Body: []byte{0x01, 0x02}}
	segs, err := gw.Response(small)
	if err != nil {
		t.Fatalf("Response(small): %v", err)
	}
	if len(segs) != 1 || !bytes.Equal(segs[0].Body, small.Body) {
		t.Fatalf("Response(small) = %+v, want unchanged single segment", segs)
	}

	large := acf.Message{Body: bytes.Repeat([]byte{0x09}, 25)}
	segs, err = gw.Response(large)
	if err != nil {
		t.Fatalf("Response(large): %v", err)
	}
	if len(segs) != 3 {
		t.Fatalf("Response(large) produced %d segments, want 3", len(segs))
	}
	var recombined []byte
	for _, seg := range segs {
		recombined = append(recombined, seg.Body...)
	}
	if !bytes.Equal(recombined, large.Body) {
		t.Errorf("recombined Response segments = % X, want % X", recombined, large.Body)
	}
}
