//fusa:test REQ-RD-001
//fusa:test REQ-RD-002
//fusa:test REQ-RD-003
//fusa:test REQ-RD-004
//fusa:test REQ-RD-005
//fusa:test REQ-RD-006
//fusa:test REQ-RD-007
//fusa:test REQ-RD-008

package redundancy_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/mock"
	"github.com/SoundMatt/go-RCP/redundancy"
)

// errPrimary is the sentinel used to simulate primary failure.
var errPrimary = errors.New("primary failed")

// failOnce is a mock controller that returns errPrimary on the first Send then delegates.
type failOnce struct {
	inner   *mock.Controller
	failed  bool
	mu      sync.Mutex
}

func (f *failOnce) Send(ctx context.Context, cmd *rcp.Command) (*rcp.Response, error) {
	f.mu.Lock()
	if !f.failed {
		f.failed = true
		f.mu.Unlock()
		return nil, errPrimary
	}
	f.mu.Unlock()
	return f.inner.Send(ctx, cmd)
}
func (f *failOnce) Zone() rcp.Zone                                        { return f.inner.Zone() }
func (f *failOnce) Subscribe(ctx context.Context) (<-chan *rcp.Status, error) { return f.inner.Subscribe(ctx) }
func (f *failOnce) Close() error                                          { return f.inner.Close() }

// alwaysFail returns errPrimary on every Send.
type alwaysFail struct{ zone rcp.Zone }

func (a *alwaysFail) Send(_ context.Context, _ *rcp.Command) (*rcp.Response, error) {
	return nil, errPrimary
}
func (a *alwaysFail) Zone() rcp.Zone                                        { return a.zone }
func (a *alwaysFail) Subscribe(_ context.Context) (<-chan *rcp.Status, error) { ch := make(chan *rcp.Status); return ch, nil }
func (a *alwaysFail) Close() error                                          { return nil }

// TestRedundancy_PrimarySucceeds sends via primary when healthy (REQ-RD-001).
func TestRedundancy_PrimarySucceeds(t *testing.T) {
	prim := mock.NewController(rcp.ZoneFrontLeft, nil)
	stby := mock.NewController(rcp.ZoneFrontLeft, nil)
	c := redundancy.NewController(prim, stby, nil)
	t.Cleanup(func() { _ = c.Close() })

	resp, err := c.Send(context.Background(), &rcp.Command{Zone: rcp.ZoneFrontLeft})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Status != rcp.StatusOK {
		t.Errorf("Status = %v, want OK", resp.Status)
	}
	if c.Failovers() != 0 {
		t.Errorf("Failovers = %d, want 0", c.Failovers())
	}
}

// TestRedundancy_FailoverOnPrimaryError standby takes over after primary fails (REQ-RD-002).
func TestRedundancy_FailoverOnPrimaryError(t *testing.T) {
	prim := &failOnce{inner: mock.NewController(rcp.ZoneFrontLeft, nil)}
	stby := mock.NewController(rcp.ZoneFrontLeft, nil)
	c := redundancy.NewController(prim, stby, nil)
	t.Cleanup(func() { _ = c.Close() })

	// First send: primary fails → failover triggered, error returned.
	_, err := c.Send(context.Background(), &rcp.Command{Zone: rcp.ZoneFrontLeft})
	if !errors.Is(err, errPrimary) {
		t.Fatalf("first send: err = %v, want errPrimary", err)
	}
	if c.Failovers() != 1 {
		t.Errorf("Failovers = %d, want 1", c.Failovers())
	}

	// Second send: now goes to standby → OK.
	resp, err := c.Send(context.Background(), &rcp.Command{Zone: rcp.ZoneFrontLeft})
	if err != nil {
		t.Fatalf("second send: %v", err)
	}
	if resp.Status != rcp.StatusOK {
		t.Errorf("second send Status = %v, want OK", resp.Status)
	}
}

// TestRedundancy_ActiveAfterFailover returns the standby as active (REQ-RD-003).
func TestRedundancy_ActiveAfterFailover(t *testing.T) {
	prim := &failOnce{inner: mock.NewController(rcp.ZoneFrontLeft, nil)}
	stby := mock.NewController(rcp.ZoneFrontLeft, nil)
	c := redundancy.NewController(prim, stby, nil)
	t.Cleanup(func() { _ = c.Close() })

	_, _ = c.Send(context.Background(), &rcp.Command{Zone: rcp.ZoneFrontLeft})
	if c.Active() != stby {
		t.Errorf("Active() after failover is not the standby")
	}
}

// TestRedundancy_PolicyPreventsFailover policy returning false keeps primary (REQ-RD-004).
func TestRedundancy_PolicyPreventsFailover(t *testing.T) {
	prim := &failOnce{inner: mock.NewController(rcp.ZoneFrontLeft, nil)}
	stby := mock.NewController(rcp.ZoneFrontLeft, nil)

	// Policy: never failover.
	noFailover := redundancy.FailoverPolicy(func(_ error) bool { return false })
	c := redundancy.NewController(prim, stby, noFailover)
	t.Cleanup(func() { _ = c.Close() })

	_, err := c.Send(context.Background(), &rcp.Command{Zone: rcp.ZoneFrontLeft})
	if err == nil {
		t.Fatal("expected error from primary, got nil")
	}
	if c.Failovers() != 0 {
		t.Errorf("Failovers = %d, want 0 (policy suppressed failover)", c.Failovers())
	}
}

// TestRedundancy_Zone returns the zone of the active controller (REQ-RD-005).
func TestRedundancy_Zone(t *testing.T) {
	prim := mock.NewController(rcp.ZoneRearLeft, nil)
	stby := mock.NewController(rcp.ZoneRearLeft, nil)
	c := redundancy.NewController(prim, stby, nil)
	t.Cleanup(func() { _ = c.Close() })

	if got := c.Zone(); got != rcp.ZoneRearLeft {
		t.Errorf("Zone() = %v, want ZoneRearLeft", got)
	}
}

// TestRedundancy_Subscribe delegates to active controller (REQ-RD-005).
func TestRedundancy_Subscribe(t *testing.T) {
	prim := mock.NewController(rcp.ZoneFrontLeft, nil)
	stby := mock.NewController(rcp.ZoneFrontLeft, nil)
	c := redundancy.NewController(prim, stby, nil)
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := c.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	_ = ch
}

// TestRedundancy_Close_Idempotent safe to call twice (REQ-RD-006).
func TestRedundancy_Close_Idempotent(t *testing.T) {
	prim := mock.NewController(rcp.ZoneFrontLeft, nil)
	stby := mock.NewController(rcp.ZoneFrontLeft, nil)
	c := redundancy.NewController(prim, stby, nil)
	if err := c.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

// TestRedundancy_Close_RejectsSend returns ErrClosed after Close (REQ-RD-006).
func TestRedundancy_Close_RejectsSend(t *testing.T) {
	prim := mock.NewController(rcp.ZoneFrontLeft, nil)
	stby := mock.NewController(rcp.ZoneFrontLeft, nil)
	c := redundancy.NewController(prim, stby, nil)
	_ = c.Close()

	_, err := c.Send(context.Background(), &rcp.Command{Zone: rcp.ZoneFrontLeft})
	if !errors.Is(err, rcp.ErrClosed) {
		t.Errorf("err = %v, want ErrClosed", err)
	}
}

// TestRedundancy_Concurrent no race under concurrent sends with failover (REQ-RD-007).
func TestRedundancy_Concurrent(t *testing.T) {
	prim := &alwaysFail{zone: rcp.ZoneFrontLeft}
	stby := mock.NewController(rcp.ZoneFrontLeft, nil)
	c := redundancy.NewController(prim, stby, nil)
	t.Cleanup(func() { _ = c.Close() })

	ctx := context.Background()
	const n = 40
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_, _ = c.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft})
		}()
	}
	wg.Wait()
}

// TestRedundancy_FailoverCount increments Failovers on each swap (REQ-RD-008).
func TestRedundancy_FailoverCount(t *testing.T) {
	prim := &failOnce{inner: mock.NewController(rcp.ZoneFrontLeft, nil)}
	stby := &failOnce{inner: mock.NewController(rcp.ZoneFrontLeft, nil)}
	c := redundancy.NewController(prim, stby, nil)
	t.Cleanup(func() { _ = c.Close() })

	// First failover: primary → standby.
	_, _ = c.Send(context.Background(), &rcp.Command{Zone: rcp.ZoneFrontLeft})
	if c.Failovers() != 1 {
		t.Errorf("after first failover: count = %d, want 1", c.Failovers())
	}

	// Second failover: standby → primary (both are failOnce, standby now fails once).
	_, _ = c.Send(context.Background(), &rcp.Command{Zone: rcp.ZoneFrontLeft})
	if c.Failovers() != 2 {
		t.Errorf("after second failover: count = %d, want 2", c.Failovers())
	}

	// Third send: original primary (now standby, already failed once) → OK.
	resp, err := c.Send(context.Background(), &rcp.Command{Zone: rcp.ZoneFrontLeft})
	if err != nil {
		t.Fatalf("third send: %v", err)
	}
	if resp.Status != rcp.StatusOK {
		t.Errorf("third send Status = %v, want OK", resp.Status)
	}
}
