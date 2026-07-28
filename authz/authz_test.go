//fusa:test REQ-AZ-001
//fusa:test REQ-AZ-002
//fusa:test REQ-AZ-003
//fusa:test REQ-AZ-004
//fusa:test REQ-AZ-005
//fusa:test REQ-AZ-006
//fusa:test REQ-AZ-007
//fusa:test REQ-AZ-008

package authz_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/authz"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/server"
	"github.com/SoundMatt/go-RCP/udp"
)

// stubHandler answers every request with FlagResponse set and an echoed
// body, mirroring udp_test.go's own fixture.
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

func clientStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55}, 1)
}

func serverStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE}, 1)
}

const testEndpoint = avtp.ByteBusID(1)

// newHarness starts a udp.Server answering testEndpoint via stubHandler and
// dials a *udp.Controller against it, returning a ready-to-wrap inner
// controller.
func newHarness(t *testing.T) *udp.Controller {
	t.Helper()
	router := udp.NewRouter(udp.NewEP0Handler(server.NewServer()), false)
	if err := router.Register(testEndpoint, stubHandler{}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	srv, err := udp.NewServer(serverStream(), "127.0.0.1:0", router)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	ctrl, err := udp.NewController(clientStream(), srv.Addr())
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	t.Cleanup(func() { _ = ctrl.Close() })
	return ctrl
}

// TestAuthz_AllowExact permits an exact-match principal/stream/endpoint
// triple (REQ-AZ-001).
func TestAuthz_AllowExact(t *testing.T) {
	inner := newHarness(t)
	p := authz.NewPolicy()
	p.Allow("ecm", inner.StreamID(), testEndpoint)

	ctrl := authz.NewController(inner, p, "ecm")
	resp, err := ctrl.Read(context.Background(), testEndpoint)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !resp.Control.Has(acf.FlagResponse) {
		t.Errorf("Control = %v, want FlagResponse set", resp.Control)
	}
}

// TestAuthz_DenyNoMatch denies when no policy entry matches (REQ-AZ-002).
func TestAuthz_DenyNoMatch(t *testing.T) {
	inner := newHarness(t)
	p := authz.NewPolicy()
	p.Allow("ecm", inner.StreamID(), testEndpoint+1) // never matches testEndpoint

	ctrl := authz.NewController(inner, p, "ecm")
	_, err := ctrl.Read(context.Background(), testEndpoint)
	if !errors.Is(err, authz.ErrDenied) {
		t.Errorf("err = %v, want ErrDenied", err)
	}
}

// TestAuthz_DenyExplicit an explicit Deny entry, evaluated before a later
// Allow, wins (REQ-AZ-002).
func TestAuthz_DenyExplicit(t *testing.T) {
	inner := newHarness(t)
	p := authz.NewPolicy()
	p.Deny("ecm", inner.StreamID(), testEndpoint)
	p.Allow("ecm", inner.StreamID(), authz.EndpointAny)

	ctrl := authz.NewController(inner, p, "ecm")
	_, err := ctrl.Read(context.Background(), testEndpoint)
	if !errors.Is(err, authz.ErrDenied) {
		t.Errorf("err = %v, want ErrDenied", err)
	}
}

// TestAuthz_WildcardPrincipal empty principal matches any caller (REQ-AZ-003).
func TestAuthz_WildcardPrincipal(t *testing.T) {
	inner := newHarness(t)
	p := authz.NewPolicy()
	p.Allow("", inner.StreamID(), testEndpoint)

	ctrl := authz.NewController(inner, p, "anyone")
	if _, err := ctrl.Read(context.Background(), testEndpoint); err != nil {
		t.Fatalf("Read: %v", err)
	}
}

// TestAuthz_WildcardStream StreamAny matches any requester stream (REQ-AZ-003).
func TestAuthz_WildcardStream(t *testing.T) {
	inner := newHarness(t)
	p := authz.NewPolicy()
	p.Allow("diag", authz.StreamAny, testEndpoint)

	ctrl := authz.NewController(inner, p, "diag")
	if _, err := ctrl.Read(context.Background(), testEndpoint); err != nil {
		t.Fatalf("Read: %v", err)
	}
}

// TestAuthz_WildcardEndpoint EndpointAny matches every endpoint (REQ-AZ-003).
func TestAuthz_WildcardEndpoint(t *testing.T) {
	inner := newHarness(t)
	p := authz.NewPolicy()
	p.Allow("admin", inner.StreamID(), authz.EndpointAny)

	ctrl := authz.NewController(inner, p, "admin")
	if _, err := ctrl.Read(context.Background(), testEndpoint); err != nil {
		t.Fatalf("Read: %v", err)
	}
	// testEndpoint+1 has no registered server-side handler: the policy
	// admits the request (EndpointAny), but the transport reports the
	// unknown-endpoint failure as a wire-level error response rather than a
	// Go error, exactly as udp.Router.Route documents.
	resp, err := ctrl.Read(context.Background(), testEndpoint+1)
	if err != nil {
		t.Fatalf("Read(testEndpoint+1): %v", err)
	}
	if !resp.Control.Has(acf.FlagError) {
		t.Errorf("Control = %v, want FlagError set for an unregistered endpoint", resp.Control)
	}
}

// TestAuthz_ContextPrincipal WithPrincipal overrides the static principal
// (REQ-AZ-004).
func TestAuthz_ContextPrincipal(t *testing.T) {
	inner := newHarness(t)
	p := authz.NewPolicy()
	p.Allow("privileged", inner.StreamID(), testEndpoint)

	ctrl := authz.NewController(inner, p, "limited")

	_, err := ctrl.Read(context.Background(), testEndpoint)
	if !errors.Is(err, authz.ErrDenied) {
		t.Errorf("without ctx principal: err = %v, want ErrDenied", err)
	}

	ctx := authz.WithPrincipal(context.Background(), "privileged")
	if _, err := ctrl.Read(ctx, testEndpoint); err != nil {
		t.Fatalf("with ctx principal: %v", err)
	}
}

// TestAuthz_SetEntries replaces policy atomically (REQ-AZ-005).
func TestAuthz_SetEntries(t *testing.T) {
	inner := newHarness(t)
	p := authz.NewPolicy()
	p.Allow("ecm", inner.StreamID(), testEndpoint)

	ctrl := authz.NewController(inner, p, "ecm")

	if _, err := ctrl.Read(context.Background(), testEndpoint); err != nil {
		t.Fatalf("before SetEntries: %v", err)
	}

	p.SetEntries(nil)

	if _, err := ctrl.Read(context.Background(), testEndpoint); !errors.Is(err, authz.ErrDenied) {
		t.Errorf("after SetEntries(nil): err = %v, want ErrDenied", err)
	}
}

// TestAuthz_DefaultDeny empty policy denies all requests (REQ-AZ-002).
func TestAuthz_DefaultDeny(t *testing.T) {
	inner := newHarness(t)
	p := authz.NewPolicy()
	ctrl := authz.NewController(inner, p, "anyone")

	_, err := ctrl.Read(context.Background(), testEndpoint)
	if !errors.Is(err, authz.ErrDenied) {
		t.Errorf("err = %v, want ErrDenied", err)
	}
}

// TestAuthz_Concurrent verifies no race under concurrent policy evaluation
// (REQ-AZ-006).
func TestAuthz_Concurrent(t *testing.T) {
	inner := newHarness(t)
	p := authz.NewPolicy()
	p.Allow("ecm", inner.StreamID(), authz.EndpointAny)

	ctrl := authz.NewController(inner, p, "ecm")

	ctx := context.Background()
	const n = 40
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_, _ = ctrl.Read(ctx, testEndpoint)
		}()
	}
	go func() {
		p.SetEntries([]authz.Entry{
			{Principal: "ecm", Stream: inner.StreamID(), Endpoint: authz.EndpointAny, Action: authz.Allow},
		})
	}()
	wg.Wait()
}

// TestAuthz_Discover bypasses the policy and delegates to the inner
// controller (REQ-AZ-007).
func TestAuthz_Discover(t *testing.T) {
	inner := newHarness(t)
	p := authz.NewPolicy() // empty: would deny everything else
	ctrl := authz.NewController(inner, p, "")

	if _, err := ctrl.Discover(context.Background()); err != nil {
		t.Fatalf("Discover: %v", err)
	}
}

// TestAuthz_StreamID delegates to the inner controller (REQ-AZ-007).
func TestAuthz_StreamID(t *testing.T) {
	inner := newHarness(t)
	p := authz.NewPolicy()
	ctrl := authz.NewController(inner, p, "")

	if got, want := ctrl.StreamID(), inner.StreamID(); got != want {
		t.Errorf("StreamID() = %v, want %v", got, want)
	}
}

// TestAuthz_Close_Idempotent Close is safe to call twice (REQ-AZ-008).
func TestAuthz_Close_Idempotent(t *testing.T) {
	inner := newHarness(t)
	p := authz.NewPolicy()
	ctrl := authz.NewController(inner, p, "")
	if err := ctrl.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := ctrl.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

// TestAuthz_Close_RejectsRequest Request after Close returns ErrClosed
// (REQ-AZ-008).
func TestAuthz_Close_RejectsRequest(t *testing.T) {
	inner := newHarness(t)
	p := authz.NewPolicy()
	ctrl := authz.NewController(inner, p, "")
	_ = ctrl.Close()

	_, err := ctrl.Read(context.Background(), testEndpoint)
	if !errors.Is(err, udp.ErrClosed) {
		t.Errorf("err = %v, want ErrClosed", err)
	}
}
