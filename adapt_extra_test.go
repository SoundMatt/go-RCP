package rcp_test

//fusa:test REQ-ADAPT-003
//fusa:test REQ-ADAPT-004
//fusa:test REQ-ADAPT-005
//fusa:test REQ-ADAPT-006
//fusa:test REQ-ADAPT-007
//fusa:test REQ-OPT-005
//fusa:test REQ-OPT-006

import (
	"context"
	"errors"
	"testing"
	"time"

	relay "github.com/SoundMatt/RELAY"
	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
)

// fakeController is a hand-rolled rcp.Controller that lets a test force the
// branches the mock cannot: a Request that errors and a Request that blocks
// (to keep an adapter dispatch in-flight).
type fakeController struct {
	stream      avtp.StreamID
	reqErr      error         // returned by Request when set
	reqGate     chan struct{} // when non-nil, Request blocks until it is closed
	reqEntered  chan struct{} // signalled once when Request is entered
	respBody    []byte        // returned by Request when reqErr is nil
	respControl acf.ControlFlags
}

func (f *fakeController) StreamID() avtp.StreamID { return f.stream }

func (f *fakeController) Request(ctx context.Context, addr avtp.ByteBusID, control acf.ControlFlags, body []byte) (acf.Message, error) {
	if f.reqEntered != nil {
		select {
		case f.reqEntered <- struct{}{}:
		default:
		}
	}
	if f.reqGate != nil {
		select {
		case <-f.reqGate:
		case <-ctx.Done():
			return acf.Message{}, ctx.Err()
		}
	}
	if f.reqErr != nil {
		return acf.Message{}, f.reqErr
	}
	return acf.Message{ByteBusID: addr, Control: f.respControl, Body: f.respBody}, nil
}

func (f *fakeController) Close() error { return nil }

// ── Send / Call error paths (controller returns an error) ──────────────────────

func TestAdapter_Send_ControllerError(t *testing.T) {
	fc := &fakeController{reqErr: errors.New("boom")}
	node := rcp.Adapt(fc)
	if err := node.Send(context.Background(), relay.Message{Protocol: relay.RCP, ID: "1", Payload: []byte("x")}); err == nil {
		t.Fatal("Send: expected controller error, got nil")
	}
	// The dispatch attempt is still counted, and the error is recorded.
	m := asMetrics(t, node).Metrics()
	if m.WriteCount != 1 {
		t.Errorf("WriteCount = %d, want 1", m.WriteCount)
	}
	if m.ErrorCount != 1 {
		t.Errorf("ErrorCount = %d, want 1", m.ErrorCount)
	}
}

func TestAdapter_Call_UnparseableID(t *testing.T) {
	fc := &fakeController{}
	node := rcp.Adapt(fc)
	if _, err := node.Call(context.Background(), relay.Message{Protocol: relay.RCP, ID: "nowhere"}); err == nil {
		t.Fatal("Call(unparseable ID): expected error, got nil")
	}
}

func TestAdapter_Call_ControllerError(t *testing.T) {
	fc := &fakeController{reqErr: errors.New("boom")}
	node := rcp.Adapt(fc)
	if _, err := node.Call(context.Background(), relay.Message{Protocol: relay.RCP, ID: "1"}); err == nil {
		t.Fatal("Call: expected controller error, got nil")
	}
	if m := asMetrics(t, node).Metrics(); m.ErrorCount != 1 {
		t.Errorf("ErrorCount = %d, want 1", m.ErrorCount)
	}
}

// ── CloseWithDrain with work in flight ─────────────────────────────────────────

func TestAdapter_CloseWithDrain_WaitsForInFlight(t *testing.T) {
	fc := &fakeController{
		reqGate:    make(chan struct{}),
		reqEntered: make(chan struct{}, 1),
	}
	node := rcp.Adapt(fc)
	go func() {
		_, _ = node.Call(context.Background(), relay.Message{Protocol: relay.RCP, ID: "1"})
	}()
	<-fc.reqEntered // dispatch is now in flight (inFlight == 1)

	// Release the blocked Request shortly so the drain loop observes inFlight → 0.
	go func() {
		time.Sleep(10 * time.Millisecond)
		close(fc.reqGate)
	}()
	if err := asDrainer(t, node).CloseWithDrain(context.Background()); err != nil {
		t.Fatalf("CloseWithDrain: %v", err)
	}
}

func TestAdapter_CloseWithDrain_ContextExpiresWhileInFlight(t *testing.T) {
	fc := &fakeController{
		reqGate:    make(chan struct{}), // never released until cleanup
		reqEntered: make(chan struct{}, 1),
	}
	node := rcp.Adapt(fc)
	go func() {
		_, _ = node.Call(context.Background(), relay.Message{Protocol: relay.RCP, ID: "1"})
	}()
	<-fc.reqEntered

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := asDrainer(t, node).CloseWithDrain(ctx)
	// Per spec §9.2, a drain timeout MUST be reported as relay.ErrTimeout, not
	// the raw ctx.Err() (context.DeadlineExceeded).
	if !errors.Is(err, relay.ErrTimeout) {
		t.Fatalf("CloseWithDrain err = %v, want errors.Is(err, relay.ErrTimeout) == true", err)
	}
	close(fc.reqGate) // let the in-flight Call unwind
}
