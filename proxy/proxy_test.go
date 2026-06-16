//fusa:test REQ-PX-001
//fusa:test REQ-PX-002
//fusa:test REQ-PX-003
//fusa:test REQ-PX-004
//fusa:test REQ-PX-005
//fusa:test REQ-PX-006
//fusa:test REQ-PX-007
//fusa:test REQ-PX-008

package proxy_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/mock"
	"github.com/SoundMatt/go-RCP/proxy"
)

// TestProxy_TransparentForward forwards command and response unchanged (REQ-PX-001).
func TestProxy_TransparentForward(t *testing.T) {
	upstream := mock.NewController(rcp.ZoneFrontLeft, nil)
	p := proxy.NewController(upstream, nil)
	t.Cleanup(func() { _ = p.Close() })

	resp, err := p.Send(context.Background(), &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdSet})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Status != rcp.StatusOK {
		t.Errorf("Status = %v, want OK", resp.Status)
	}
}

// TestProxy_NilTransform behaves identically to transparent forward (REQ-PX-001).
func TestProxy_NilTransform(t *testing.T) {
	upstream := mock.NewController(rcp.ZoneFrontLeft, nil)
	p := proxy.NewController(upstream, nil)
	t.Cleanup(func() { _ = p.Close() })

	cmd := &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdGet, Priority: rcp.PriorityHigh}
	resp, err := p.Send(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Status != rcp.StatusOK {
		t.Errorf("Status = %v, want OK", resp.Status)
	}
}

// TestProxy_TransformRewritesCommand the transform may alter the command (REQ-PX-002).
func TestProxy_TransformRewritesCommand(t *testing.T) {
	var received *rcp.Command
	recordUpstream := &captureController{inner: mock.NewController(rcp.ZoneFrontLeft, nil), capture: &received}

	transform := func(cmd *rcp.Command) (*rcp.Command, error) {
		c := *cmd
		c.Priority = rcp.PriorityCritical // escalate priority
		return &c, nil
	}
	p := proxy.NewController(recordUpstream, transform)
	t.Cleanup(func() { _ = p.Close() })

	_, err := p.Send(context.Background(), &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdSet, Priority: rcp.PriorityNormal})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if received == nil {
		t.Fatal("upstream never received the command")
	}
	if received.Priority != rcp.PriorityCritical {
		t.Errorf("upstream got Priority=%v, want PriorityCritical", received.Priority)
	}
}

// TestProxy_TransformError aborts Send (REQ-PX-003).
func TestProxy_TransformError(t *testing.T) {
	upstream := mock.NewController(rcp.ZoneFrontLeft, nil)
	transformErr := errors.New("forbidden by transform")
	p := proxy.NewController(upstream, func(_ *rcp.Command) (*rcp.Command, error) {
		return nil, transformErr
	})
	t.Cleanup(func() { _ = p.Close() })

	_, err := p.Send(context.Background(), &rcp.Command{Zone: rcp.ZoneFrontLeft})
	if err == nil {
		t.Fatal("expected error from transform, got nil")
	}
	if !errors.Is(err, transformErr) {
		t.Errorf("err = %v, does not wrap transformErr", err)
	}
}

// TestProxy_Zone delegates to upstream (REQ-PX-004).
func TestProxy_Zone(t *testing.T) {
	upstream := mock.NewController(rcp.ZoneRearRight, nil)
	p := proxy.NewController(upstream, nil)
	t.Cleanup(func() { _ = p.Close() })

	if got := p.Zone(); got != rcp.ZoneRearRight {
		t.Errorf("Zone() = %v, want ZoneRearRight", got)
	}
}

// TestProxy_Subscribe delegates to upstream (REQ-PX-005).
func TestProxy_Subscribe(t *testing.T) {
	upstream := mock.NewController(rcp.ZoneFrontLeft, nil)
	p := proxy.NewController(upstream, nil)
	t.Cleanup(func() { _ = p.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := p.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	_ = ch
}

// TestProxy_Close_Idempotent safe to call twice (REQ-PX-006).
func TestProxy_Close_Idempotent(t *testing.T) {
	upstream := mock.NewController(rcp.ZoneFrontLeft, nil)
	p := proxy.NewController(upstream, nil)
	if err := p.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

// TestProxy_Close_RejectsSend returns ErrClosed after Close (REQ-PX-006).
func TestProxy_Close_RejectsSend(t *testing.T) {
	upstream := mock.NewController(rcp.ZoneFrontLeft, nil)
	p := proxy.NewController(upstream, nil)
	_ = p.Close()

	_, err := p.Send(context.Background(), &rcp.Command{Zone: rcp.ZoneFrontLeft})
	if !errors.Is(err, rcp.ErrClosed) {
		t.Errorf("err = %v, want ErrClosed", err)
	}
}

// TestProxy_Concurrent no race under concurrent sends (REQ-PX-007).
func TestProxy_Concurrent(t *testing.T) {
	upstream := mock.NewController(rcp.ZoneFrontLeft, nil)
	p := proxy.NewController(upstream, nil)
	t.Cleanup(func() { _ = p.Close() })

	ctx := context.Background()
	const n = 40
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_, _ = p.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft})
		}()
	}
	wg.Wait()
}

// TestProxy_Composable wraps proxy in a second proxy (REQ-PX-008).
func TestProxy_Composable(t *testing.T) {
	upstream := mock.NewController(rcp.ZoneFrontLeft, nil)

	var hops int
	countTransform := func(cmd *rcp.Command) (*rcp.Command, error) {
		hops++
		return cmd, nil
	}

	inner := proxy.NewController(upstream, countTransform)
	outer := proxy.NewController(inner, countTransform)
	t.Cleanup(func() { _ = outer.Close() })

	_, err := outer.Send(context.Background(), &rcp.Command{Zone: rcp.ZoneFrontLeft})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if hops != 2 {
		t.Errorf("hops = %d, want 2 (each proxy fires transform once)", hops)
	}
}

// captureController records the last command received by Send.
type captureController struct {
	inner   rcp.Controller
	capture **rcp.Command
}

func (c *captureController) Send(ctx context.Context, cmd *rcp.Command) (*rcp.Response, error) {
	*c.capture = cmd
	return c.inner.Send(ctx, cmd)
}
func (c *captureController) Zone() rcp.Zone                                        { return c.inner.Zone() }
func (c *captureController) Subscribe(ctx context.Context) (<-chan *rcp.Status, error) { return c.inner.Subscribe(ctx) }
func (c *captureController) Close() error                                           { return c.inner.Close() }
