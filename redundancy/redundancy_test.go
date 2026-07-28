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

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/redundancy"
	"github.com/SoundMatt/go-RCP/server"
	"github.com/SoundMatt/go-RCP/udp"
)

type stubHandler struct{}

func (stubHandler) HandleRequest(_ avtp.StreamID, req acf.Message) (acf.Message, error) {
	return acf.Message{
		Kind:           req.Kind,
		ByteBusID:      req.ByteBusID,
		TransactionNum: req.TransactionNum,
		Control:        acf.FlagResponse | (req.Control & (acf.FlagRead | acf.FlagWrite)),
		Body:           req.Body,
	}, nil
}

const testEndpoint = avtp.ByteBusID(1)

// newWorkingController starts a fresh server and returns a dialed
// *udp.Controller against it, presenting the given suffix to keep multiple
// harness controllers' StreamIDs distinct.
func newWorkingController(t *testing.T, suffix uint16) *udp.Controller {
	t.Helper()
	router := udp.NewRouter(udp.NewEP0Handler(server.NewServer()), false)
	if err := router.Register(testEndpoint, stubHandler{}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	srv, err := udp.NewServer(avtp.NewStreamID([6]byte{0x02, 0xAA, 0xBB, 0xCC, 0xDD, byte(suffix)}, suffix), "127.0.0.1:0", router)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	ctrl, err := udp.NewController(avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, byte(suffix)}, suffix), srv.Addr())
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	t.Cleanup(func() { _ = ctrl.Close() })
	return ctrl
}

// newFailingController returns an already-closed *udp.Controller, whose
// every Request deterministically fails with udp.ErrClosed — a simple,
// synchronous way to exercise failover without relying on network timeouts.
func newFailingController(t *testing.T, suffix uint16) *udp.Controller {
	t.Helper()
	ctrl := newWorkingController(t, suffix)
	_ = ctrl.Close()
	return ctrl
}

// TestRedundancy_HealthyPrimary Requests go to the primary while it is
// healthy (REQ-RD-001).
func TestRedundancy_HealthyPrimary(t *testing.T) {
	primary := newWorkingController(t, 1)
	standby := newWorkingController(t, 2)
	rc := redundancy.NewController(primary, standby, nil)
	t.Cleanup(func() { _ = rc.Close() })

	if _, err := rc.Read(context.Background(), testEndpoint); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if rc.Failovers() != 0 {
		t.Errorf("Failovers() = %d, want 0", rc.Failovers())
	}
	if rc.Active() != primary {
		t.Errorf("Active() != primary before any failure")
	}
}

// TestRedundancy_Failover a failing primary triggers a failover to the
// standby, incrementing the counter and returning the primary's error
// (REQ-RD-002). The next Request uses the new primary (REQ-RD-003).
func TestRedundancy_Failover(t *testing.T) {
	primary := newFailingController(t, 1)
	standby := newWorkingController(t, 2)
	rc := redundancy.NewController(primary, standby, nil)
	t.Cleanup(func() { _ = rc.Close() })

	_, err := rc.Read(context.Background(), testEndpoint)
	if !errors.Is(err, udp.ErrClosed) {
		t.Fatalf("first Read err = %v, want ErrClosed (primary failure surfaced)", err)
	}
	if rc.Failovers() != 1 {
		t.Fatalf("Failovers() = %d, want 1", rc.Failovers())
	}
	if rc.Active() != standby {
		t.Fatalf("Active() != standby after failover")
	}

	if _, err := rc.Read(context.Background(), testEndpoint); err != nil {
		t.Errorf("second Read (via new primary): %v", err)
	}
}

// TestRedundancy_PolicySuppresses a FailoverPolicy returning false leaves
// the controllers unswapped (REQ-RD-004).
func TestRedundancy_PolicySuppresses(t *testing.T) {
	primary := newFailingController(t, 1)
	standby := newWorkingController(t, 2)
	rc := redundancy.NewController(primary, standby, func(error) bool { return false })
	t.Cleanup(func() { _ = rc.Close() })

	if _, err := rc.Read(context.Background(), testEndpoint); err == nil {
		t.Fatalf("Read = nil error, want the suppressed primary error")
	}
	if rc.Failovers() != 0 {
		t.Errorf("Failovers() = %d, want 0 (policy suppressed failover)", rc.Failovers())
	}
	if rc.Active() != primary {
		t.Errorf("Active() != primary; policy should have left it unswapped")
	}
}

// TestRedundancy_StreamID delegates to whichever controller is currently
// primary (REQ-RD-005).
func TestRedundancy_StreamID(t *testing.T) {
	primary := newFailingController(t, 1)
	standby := newWorkingController(t, 2)
	rc := redundancy.NewController(primary, standby, nil)
	t.Cleanup(func() { _ = rc.Close() })

	if got, want := rc.StreamID(), primary.StreamID(); got != want {
		t.Errorf("StreamID() before failover = %v, want primary's %v", got, want)
	}

	_, _ = rc.Read(context.Background(), testEndpoint) // triggers failover

	if got, want := rc.StreamID(), standby.StreamID(); got != want {
		t.Errorf("StreamID() after failover = %v, want new primary's (standby's) %v", got, want)
	}
}

// TestRedundancy_Close_Idempotent Close closes both controllers and is safe
// to call multiple times; Request after Close returns ErrClosed (REQ-RD-006).
func TestRedundancy_Close_Idempotent(t *testing.T) {
	primary := newWorkingController(t, 1)
	standby := newWorkingController(t, 2)
	rc := redundancy.NewController(primary, standby, nil)

	if err := rc.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := rc.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
	if _, err := rc.Read(context.Background(), testEndpoint); !errors.Is(err, udp.ErrClosed) {
		t.Errorf("Read after Close err = %v, want ErrClosed", err)
	}
}

// TestRedundancy_Concurrent concurrent Requests during a failover are
// race-free; the swap happens at most once (REQ-RD-007).
func TestRedundancy_Concurrent(t *testing.T) {
	primary := newFailingController(t, 1)
	standby := newWorkingController(t, 2)
	rc := redundancy.NewController(primary, standby, nil)
	t.Cleanup(func() { _ = rc.Close() })

	const n = 40
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_, _ = rc.Read(context.Background(), testEndpoint)
		}()
	}
	wg.Wait()

	if rc.Failovers() != 1 {
		t.Errorf("Failovers() = %d, want exactly 1 despite concurrent callers (REQ-RD-008)", rc.Failovers())
	}
}
