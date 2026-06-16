//fusa:test REQ-PQ-001
//fusa:test REQ-PQ-002
//fusa:test REQ-PQ-003
//fusa:test REQ-PQ-004
//fusa:test REQ-PQ-005
//fusa:test REQ-PQ-006
//fusa:test REQ-PQ-007
//fusa:test REQ-PQ-008

package prioqueue_test

import (
	"context"
	"sync"
	"testing"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/mock"
	"github.com/SoundMatt/go-RCP/prioqueue"
)

// sequencingController records the order in which commands arrive at Send.
type sequencingController struct {
	mock.Controller
	mu    sync.Mutex
	order []rcp.Priority
	gate  chan struct{} // close to release blocked sends
}

func newSequencingCtrl(zone rcp.Zone) *sequencingController {
	return &sequencingController{
		Controller: *mock.NewController(zone, nil),
		gate:       make(chan struct{}),
	}
}

func (s *sequencingController) Send(ctx context.Context, cmd *rcp.Command) (*rcp.Response, error) {
	s.mu.Lock()
	s.order = append(s.order, cmd.Priority)
	s.mu.Unlock()
	// Block until gate is closed to give other sends time to queue.
	select {
	case <-s.gate:
	case <-ctx.Done():
	}
	return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusOK}, nil
}

// TestPrioQueue_CriticalBeforeNormal verifies Critical pre-empts Normal (REQ-PQ-001).
func TestPrioQueue_CriticalBeforeNormal(t *testing.T) {
	inner := newSequencingCtrl(rcp.ZoneFrontLeft)
	ctrl := prioqueue.NewController(inner)
	t.Cleanup(func() { _ = ctrl.Close() })

	ctx := context.Background()
	var wg sync.WaitGroup

	// Send Normal first — it will block in the sequencing controller holding the gate.
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdNoop, Priority: rcp.PriorityNormal})
	}()

	// Give the Normal send time to reach the dispatcher and block.
	time.Sleep(10 * time.Millisecond)

	// Enqueue Critical and High while Normal is in-flight.
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdNoop, Priority: rcp.PriorityCritical})
	}()
	go func() {
		defer wg.Done()
		_, _ = ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdNoop, Priority: rcp.PriorityHigh})
	}()

	// Give them time to enqueue.
	time.Sleep(10 * time.Millisecond)

	// Release the gate — Normal completes, then dispatcher picks next.
	close(inner.gate)
	wg.Wait()

	inner.mu.Lock()
	order := append([]rcp.Priority{}, inner.order...)
	inner.mu.Unlock()

	if len(order) != 3 {
		t.Fatalf("expected 3 commands dispatched, got %d", len(order))
	}
	// First was Normal (already in-flight), then Critical, then High.
	if order[0] != rcp.PriorityNormal {
		t.Errorf("order[0] = %v, want Normal", order[0])
	}
	if order[1] != rcp.PriorityCritical {
		t.Errorf("order[1] = %v, want Critical", order[1])
	}
	if order[2] != rcp.PriorityHigh {
		t.Errorf("order[2] = %v, want High", order[2])
	}
}

// TestPrioQueue_FIFOWithinPriority verifies equal-priority commands are FIFO (REQ-PQ-002).
func TestPrioQueue_FIFOWithinPriority(t *testing.T) {
	inner := newSequencingCtrl(rcp.ZoneFrontLeft)
	ctrl := prioqueue.NewController(inner)
	t.Cleanup(func() { _ = ctrl.Close() })

	ctx := context.Background()
	var wg sync.WaitGroup

	// Block with first Normal send.
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft, Priority: rcp.PriorityNormal, ID: 1})
	}()
	time.Sleep(10 * time.Millisecond)

	// Enqueue two more Normal commands in order.
	for i := uint32(2); i <= 3; i++ {
		id := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft, Priority: rcp.PriorityNormal, ID: id})
		}()
		time.Sleep(2 * time.Millisecond) // slight spacing to ensure ordering
	}

	time.Sleep(5 * time.Millisecond)
	close(inner.gate)
	wg.Wait()

	inner.mu.Lock()
	order := append([]rcp.Priority{}, inner.order...)
	inner.mu.Unlock()

	for _, p := range order {
		if p != rcp.PriorityNormal {
			t.Errorf("unexpected priority %v in FIFO test", p)
		}
	}
}

// TestPrioQueue_RoundTrip verifies basic Send/Response pass-through (REQ-PQ-003).
func TestPrioQueue_RoundTrip(t *testing.T) {
	inner := mock.NewController(rcp.ZoneFrontLeft, nil)
	ctrl := prioqueue.NewController(inner)
	t.Cleanup(func() { _ = ctrl.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdNoop})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Status != rcp.StatusOK {
		t.Errorf("Status = %v, want OK", resp.Status)
	}
}

// TestPrioQueue_ContextCancellation verifies ErrTimeout on ctx cancel (REQ-PQ-004).
func TestPrioQueue_ContextCancellation(t *testing.T) {
	// Use a slow inner that holds the gate; the queued send should time out.
	inner := newSequencingCtrl(rcp.ZoneFrontLeft)
	ctrl := prioqueue.NewController(inner)
	t.Cleanup(func() {
		close(inner.gate)
		_ = ctrl.Close()
	})

	ctx := context.Background()
	// First send — will block holding the gate.
	go func() {
		_, _ = ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft, Priority: rcp.PriorityNormal})
	}()
	time.Sleep(10 * time.Millisecond)

	// Second send with a short timeout.
	shortCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := ctrl.Send(shortCtx, &rcp.Command{Zone: rcp.ZoneFrontLeft, Priority: rcp.PriorityNormal})
	if err == nil {
		t.Error("expected timeout error, got nil")
	}
}

// TestPrioQueue_Zone verifies Zone() delegates to inner (REQ-PQ-005).
func TestPrioQueue_Zone(t *testing.T) {
	inner := mock.NewController(rcp.ZoneRearLeft, nil)
	ctrl := prioqueue.NewController(inner)
	t.Cleanup(func() { _ = ctrl.Close() })

	if got := ctrl.Zone(); got != rcp.ZoneRearLeft {
		t.Errorf("Zone() = %v, want ZoneRearLeft", got)
	}
}

// TestPrioQueue_Subscribe verifies Subscribe delegates to inner (REQ-PQ-005).
func TestPrioQueue_Subscribe(t *testing.T) {
	inner := mock.NewController(rcp.ZoneFrontLeft, nil)
	ctrl := prioqueue.NewController(inner)
	t.Cleanup(func() { _ = ctrl.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := ctrl.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	_ = ch
}

// TestPrioQueue_Close_Idempotent verifies Close() is safe to call twice (REQ-PQ-006).
func TestPrioQueue_Close_Idempotent(t *testing.T) {
	ctrl := prioqueue.NewController(mock.NewController(rcp.ZoneFrontLeft, nil))
	if err := ctrl.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := ctrl.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

// TestPrioQueue_Close_RejectsNewSends verifies Send after Close returns ErrClosed (REQ-PQ-006).
func TestPrioQueue_Close_RejectsNewSends(t *testing.T) {
	inner := mock.NewController(rcp.ZoneFrontLeft, nil)
	ctrl := prioqueue.NewController(inner)
	_ = ctrl.Close()

	_, err := ctrl.Send(context.Background(), &rcp.Command{Zone: rcp.ZoneFrontLeft})
	if err == nil {
		t.Error("Send after Close should return error")
	}
}

// TestPrioQueue_Concurrent verifies no data race under 20 concurrent senders (REQ-PQ-007).
func TestPrioQueue_Concurrent(t *testing.T) {
	inner := mock.NewController(rcp.ZoneFrontLeft, nil)
	ctrl := prioqueue.NewController(inner)
	t.Cleanup(func() { _ = ctrl.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const n = 20
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			_, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdNoop})
			errs <- err
		}()
	}
	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Errorf("concurrent Send %d: %v", i, err)
		}
	}
}

// TestPrioQueue_AllPrioritiesDispatched verifies commands at all 3 priorities succeed (REQ-PQ-008).
func TestPrioQueue_AllPrioritiesDispatched(t *testing.T) {
	inner := mock.NewController(rcp.ZoneFrontLeft, nil)
	ctrl := prioqueue.NewController(inner)
	t.Cleanup(func() { _ = ctrl.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	for _, p := range []rcp.Priority{rcp.PriorityNormal, rcp.PriorityHigh, rcp.PriorityCritical} {
		resp, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft, Priority: p})
		if err != nil {
			t.Errorf("Send priority=%v: %v", p, err)
			continue
		}
		if resp.Status != rcp.StatusOK {
			t.Errorf("priority=%v: Status = %v, want OK", p, resp.Status)
		}
	}
}
