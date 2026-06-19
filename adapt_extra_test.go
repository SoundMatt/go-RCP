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
)

// fakeController is a hand-rolled rcp.Controller that lets a test force the
// branches the mock cannot: a Send that errors, a Send that blocks (to keep an
// adapter dispatch in-flight), a Subscribe that errors, and a Subscribe whose
// Status stream the test feeds directly to exercise back-pressure policies.
type fakeController struct {
	zone        rcp.Zone
	sendErr     error            // returned by Send when set
	sendGate    chan struct{}    // when non-nil, Send blocks until it is closed
	sendEntered chan struct{}    // signalled once when Send is entered
	feed        chan *rcp.Status // returned by Subscribe when subErr is nil
	subErr      error            // returned by Subscribe when set
}

func (f *fakeController) Zone() rcp.Zone { return f.zone }

func (f *fakeController) Send(ctx context.Context, cmd *rcp.Command) (*rcp.Response, error) {
	if f.sendEntered != nil {
		select {
		case f.sendEntered <- struct{}{}:
		default:
		}
	}
	if f.sendGate != nil {
		select {
		case <-f.sendGate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.sendErr != nil {
		return nil, f.sendErr
	}
	return &rcp.Response{Zone: f.zone, CommandID: cmd.ID}, nil
}

func (f *fakeController) Subscribe(_ context.Context) (<-chan *rcp.Status, error) {
	if f.subErr != nil {
		return nil, f.subErr
	}
	return f.feed, nil
}

func (f *fakeController) Close() error { return nil }

// ── Send / Call error paths (controller returns an error) ──────────────────────

func TestAdapter_Send_ControllerError(t *testing.T) {
	fc := &fakeController{zone: rcp.ZoneFrontLeft, sendErr: errors.New("boom")}
	node := rcp.Adapt(fc)
	if err := node.Send(context.Background(), msgFor(rcp.ZoneFrontLeft, "get")); err == nil {
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

func TestAdapter_Call_UnknownZone(t *testing.T) {
	fc := &fakeController{zone: rcp.ZoneFrontLeft}
	node := rcp.Adapt(fc)
	if _, err := node.Call(context.Background(), relay.Message{Protocol: relay.RCP, ID: "nowhere"}); err == nil {
		t.Fatal("Call(unknown zone): expected error, got nil")
	}
}

func TestAdapter_Call_ControllerError(t *testing.T) {
	fc := &fakeController{zone: rcp.ZoneFrontLeft, sendErr: errors.New("boom")}
	node := rcp.Adapt(fc)
	if _, err := node.Call(context.Background(), msgFor(rcp.ZoneFrontLeft, "get")); err == nil {
		t.Fatal("Call: expected controller error, got nil")
	}
	if m := asMetrics(t, node).Metrics(); m.ErrorCount != 1 {
		t.Errorf("ErrorCount = %d, want 1", m.ErrorCount)
	}
}

// ── Subscribe error and back-pressure policies ─────────────────────────────────

func TestAdapter_Subscribe_ControllerError(t *testing.T) {
	fc := &fakeController{zone: rcp.ZoneFrontLeft, subErr: errors.New("no subs")}
	node := rcp.Adapt(fc)
	if _, err := node.Subscribe(); err == nil {
		t.Fatal("Subscribe: expected controller error, got nil")
	}
}

func TestAdapter_Subscribe_DropNewest(t *testing.T) {
	// Unbuffered feed: each successful `feed <- st` proves the goroutine has
	// finished processing the previous Status, making drops deterministic.
	fc := &fakeController{zone: rcp.ZoneFrontLeft, feed: make(chan *rcp.Status)}
	node := rcp.Adapt(fc)
	ch, err := node.Subscribe(relay.WithChannelDepth(1), relay.WithBackPressure(relay.DropNewest))
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	fc.feed <- &rcp.Status{Zone: fc.zone, Seq: 1} // delivered to ch (now full)
	fc.feed <- &rcp.Status{Zone: fc.zone, Seq: 2} // ch full → dropped
	fc.feed <- &rcp.Status{Zone: fc.zone, Seq: 3} // proves Seq 2 was processed
	close(fc.feed)
	for range ch { //nolint:revive // drain until the adapter closes ch
	}
	if m := asMetrics(t, node).Metrics(); m.DropCount == 0 {
		t.Error("DropCount = 0, want > 0 under DropNewest with a full channel")
	}
}

func TestAdapter_Subscribe_DropOldest(t *testing.T) {
	fc := &fakeController{zone: rcp.ZoneFrontLeft, feed: make(chan *rcp.Status)}
	node := rcp.Adapt(fc)
	ch, err := node.Subscribe(relay.WithChannelDepth(1), relay.WithBackPressure(relay.DropOldest))
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	fc.feed <- &rcp.Status{Zone: fc.zone, Seq: 1} // delivered (ch full)
	fc.feed <- &rcp.Status{Zone: fc.zone, Seq: 2} // ch full → drop oldest, enqueue Seq 2
	fc.feed <- &rcp.Status{Zone: fc.zone, Seq: 3} // proves Seq 2 was processed
	close(fc.feed)
	for range ch { //nolint:revive
	}
	if m := asMetrics(t, node).Metrics(); m.DropCount == 0 {
		t.Error("DropCount = 0, want > 0 under DropOldest with a full channel")
	}
}

func TestAdapter_Subscribe_Block(t *testing.T) {
	fc := &fakeController{zone: rcp.ZoneFrontLeft, feed: make(chan *rcp.Status)}
	node := rcp.Adapt(fc)
	ch, err := node.Subscribe(relay.WithChannelDepth(4), relay.WithBackPressure(relay.Block))
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	fc.feed <- &rcp.Status{Zone: fc.zone, Seq: 1, Payload: []byte("blk")}
	select {
	case msg, ok := <-ch:
		if !ok {
			t.Fatal("channel closed unexpectedly")
		}
		if msg.Protocol != relay.RCP {
			t.Errorf("msg.Protocol = %v, want relay.RCP", msg.Protocol)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for blocked delivery")
	}
	close(fc.feed)
	for range ch { //nolint:revive
	}
	if m := asMetrics(t, node).Metrics(); m.DeliverCount == 0 {
		t.Error("DeliverCount = 0, want > 0 under Block")
	}
}

// ── CloseWithDrain with work in flight ─────────────────────────────────────────

func TestAdapter_CloseWithDrain_WaitsForInFlight(t *testing.T) {
	fc := &fakeController{
		zone:        rcp.ZoneFrontLeft,
		sendGate:    make(chan struct{}),
		sendEntered: make(chan struct{}, 1),
	}
	node := rcp.Adapt(fc)
	go func() {
		_, _ = node.Call(context.Background(), msgFor(rcp.ZoneFrontLeft, "get"))
	}()
	<-fc.sendEntered // dispatch is now in flight (inFlight == 1)

	// Release the blocked Send shortly so the drain loop observes inFlight → 0.
	go func() {
		time.Sleep(10 * time.Millisecond)
		close(fc.sendGate)
	}()
	if err := asDrainer(t, node).CloseWithDrain(context.Background()); err != nil {
		t.Fatalf("CloseWithDrain: %v", err)
	}
}

func TestAdapter_CloseWithDrain_ContextExpiresWhileInFlight(t *testing.T) {
	fc := &fakeController{
		zone:        rcp.ZoneFrontLeft,
		sendGate:    make(chan struct{}), // never released until cleanup
		sendEntered: make(chan struct{}, 1),
	}
	node := rcp.Adapt(fc)
	go func() {
		_, _ = node.Call(context.Background(), msgFor(rcp.ZoneFrontLeft, "get"))
	}()
	<-fc.sendEntered

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := asDrainer(t, node).CloseWithDrain(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("CloseWithDrain err = %v, want context.DeadlineExceeded", err)
	}
	close(fc.sendGate) // let the in-flight Call unwind
}
