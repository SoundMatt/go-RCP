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

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/authz"
	"github.com/SoundMatt/go-RCP/mock"
)

func newCtrl(policy *authz.Policy, principal string) (*authz.Controller, *mock.Controller) {
	inner := mock.NewController(rcp.ZoneFrontLeft, nil)
	return authz.NewController(inner, policy, principal), inner
}

// TestAuthz_AllowExact permits an exact-match principal/zone/cmd triple (REQ-AZ-001).
func TestAuthz_AllowExact(t *testing.T) {
	p := authz.NewPolicy()
	p.Allow("ecm", rcp.ZoneFrontLeft, rcp.CmdSet)

	ctrl, _ := newCtrl(p, "ecm")
	t.Cleanup(func() { _ = ctrl.Close() })

	resp, err := ctrl.Send(context.Background(), &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdSet})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Status != rcp.StatusOK {
		t.Errorf("Status = %v, want OK", resp.Status)
	}
}

// TestAuthz_DenyNoMatch denies when no policy entry matches (REQ-AZ-002).
func TestAuthz_DenyNoMatch(t *testing.T) {
	p := authz.NewPolicy()
	// Allow ecm only on CmdGet, not CmdSet
	p.Allow("ecm", rcp.ZoneFrontLeft, rcp.CmdGet)

	ctrl, _ := newCtrl(p, "ecm")
	t.Cleanup(func() { _ = ctrl.Close() })

	_, err := ctrl.Send(context.Background(), &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdSet})
	if !errors.Is(err, authz.ErrDenied) {
		t.Errorf("err = %v, want ErrDenied", err)
	}
}

// TestAuthz_DenyExplicit explicit Deny entry returns ErrDenied (REQ-AZ-002).
func TestAuthz_DenyExplicit(t *testing.T) {
	p := authz.NewPolicy()
	p.Allow("ecm", rcp.ZoneFrontLeft, authz.CmdTypeAny)
	p.Deny("ecm", rcp.ZoneFrontLeft, rcp.CmdReset) // inserted after Allow-all; won't fire due to order

	// For deny-first ordering, put Deny before Allow:
	p2 := authz.NewPolicy()
	p2.Deny("ecm", rcp.ZoneFrontLeft, rcp.CmdReset)
	p2.Allow("ecm", rcp.ZoneFrontLeft, authz.CmdTypeAny)

	ctrl, _ := newCtrl(p2, "ecm")
	t.Cleanup(func() { _ = ctrl.Close() })

	// CmdReset should be denied
	_, err := ctrl.Send(context.Background(), &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdReset})
	if !errors.Is(err, authz.ErrDenied) {
		t.Errorf("CmdReset err = %v, want ErrDenied", err)
	}

	// CmdSet should be allowed
	resp, err := ctrl.Send(context.Background(), &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdSet})
	if err != nil {
		t.Fatalf("CmdSet: %v", err)
	}
	if resp.Status != rcp.StatusOK {
		t.Errorf("CmdSet Status = %v, want OK", resp.Status)
	}
}

// TestAuthz_WildcardPrincipal empty principal matches any caller (REQ-AZ-003).
func TestAuthz_WildcardPrincipal(t *testing.T) {
	p := authz.NewPolicy()
	p.Allow("", rcp.ZoneFrontLeft, rcp.CmdGet) // any principal

	ctrl, _ := newCtrl(p, "anyone")
	t.Cleanup(func() { _ = ctrl.Close() })

	resp, err := ctrl.Send(context.Background(), &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdGet})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Status != rcp.StatusOK {
		t.Errorf("Status = %v, want OK", resp.Status)
	}
}

// TestAuthz_WildcardZone ZoneUnknown matches any zone (REQ-AZ-003).
func TestAuthz_WildcardZone(t *testing.T) {
	p := authz.NewPolicy()
	p.Allow("diag", rcp.ZoneUnknown, rcp.CmdGet) // any zone

	inner := mock.NewController(rcp.ZoneRearRight, nil)
	ctrl := authz.NewController(inner, p, "diag")
	t.Cleanup(func() { _ = ctrl.Close() })

	resp, err := ctrl.Send(context.Background(), &rcp.Command{Zone: rcp.ZoneRearRight, Type: rcp.CmdGet})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Status != rcp.StatusOK {
		t.Errorf("Status = %v, want OK", resp.Status)
	}
}

// TestAuthz_WildcardCmdType CmdTypeAny matches every command type (REQ-AZ-003).
func TestAuthz_WildcardCmdType(t *testing.T) {
	p := authz.NewPolicy()
	p.Allow("admin", rcp.ZoneFrontLeft, authz.CmdTypeAny)

	ctrl, _ := newCtrl(p, "admin")
	t.Cleanup(func() { _ = ctrl.Close() })

	for _, ct := range []rcp.CommandType{rcp.CmdSet, rcp.CmdGet, rcp.CmdReset, rcp.CmdWatchdog} {
		resp, err := ctrl.Send(context.Background(), &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: ct})
		if err != nil {
			t.Errorf("cmd %d: %v", ct, err)
			continue
		}
		if resp.Status != rcp.StatusOK {
			t.Errorf("cmd %d Status = %v, want OK", ct, resp.Status)
		}
	}
}

// TestAuthz_ContextPrincipal WithPrincipal overrides the static principal (REQ-AZ-004).
func TestAuthz_ContextPrincipal(t *testing.T) {
	p := authz.NewPolicy()
	p.Allow("privileged", rcp.ZoneFrontLeft, rcp.CmdReset)

	// Static principal "limited" has no reset permission.
	ctrl, _ := newCtrl(p, "limited")
	t.Cleanup(func() { _ = ctrl.Close() })

	// Without context principal: denied.
	_, err := ctrl.Send(context.Background(), &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdReset})
	if !errors.Is(err, authz.ErrDenied) {
		t.Errorf("without ctx principal: err = %v, want ErrDenied", err)
	}

	// With context principal: allowed.
	ctx := authz.WithPrincipal(context.Background(), "privileged")
	resp, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdReset})
	if err != nil {
		t.Fatalf("with ctx principal: %v", err)
	}
	if resp.Status != rcp.StatusOK {
		t.Errorf("Status = %v, want OK", resp.Status)
	}
}

// TestAuthz_SetEntries replaces policy atomically (REQ-AZ-005).
func TestAuthz_SetEntries(t *testing.T) {
	p := authz.NewPolicy()
	p.Allow("ecm", rcp.ZoneFrontLeft, rcp.CmdSet)

	ctrl, _ := newCtrl(p, "ecm")
	t.Cleanup(func() { _ = ctrl.Close() })

	// Allowed before replacement.
	_, err := ctrl.Send(context.Background(), &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdSet})
	if err != nil {
		t.Fatalf("before SetEntries: %v", err)
	}

	// Replace with empty policy — deny all.
	p.SetEntries(nil)

	_, err = ctrl.Send(context.Background(), &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdSet})
	if !errors.Is(err, authz.ErrDenied) {
		t.Errorf("after SetEntries(nil): err = %v, want ErrDenied", err)
	}
}

// TestAuthz_DefaultDeny empty policy denies all commands (REQ-AZ-002).
func TestAuthz_DefaultDeny(t *testing.T) {
	p := authz.NewPolicy()
	ctrl, _ := newCtrl(p, "anyone")
	t.Cleanup(func() { _ = ctrl.Close() })

	_, err := ctrl.Send(context.Background(), &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdSet})
	if !errors.Is(err, authz.ErrDenied) {
		t.Errorf("err = %v, want ErrDenied", err)
	}
}

// TestAuthz_Concurrent verifies no race under concurrent policy evaluation (REQ-AZ-006).
func TestAuthz_Concurrent(t *testing.T) {
	p := authz.NewPolicy()
	p.Allow("ecm", rcp.ZoneFrontLeft, authz.CmdTypeAny)

	ctrl, _ := newCtrl(p, "ecm")
	t.Cleanup(func() { _ = ctrl.Close() })

	ctx := context.Background()
	const n = 40
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_, _ = ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdSet})
		}()
	}
	// Concurrently replace the policy.
	go func() {
		p.SetEntries([]authz.Entry{
			{Principal: "ecm", Zone: rcp.ZoneFrontLeft, CmdType: authz.CmdTypeAny, Action: authz.Allow},
		})
	}()
	wg.Wait()
}

// TestAuthz_Zone delegates to inner controller (REQ-AZ-007).
func TestAuthz_Zone(t *testing.T) {
	inner := mock.NewController(rcp.ZoneRearLeft, nil)
	p := authz.NewPolicy()
	ctrl := authz.NewController(inner, p, "")
	t.Cleanup(func() { _ = ctrl.Close() })

	if got := ctrl.Zone(); got != rcp.ZoneRearLeft {
		t.Errorf("Zone() = %v, want ZoneRearLeft", got)
	}
}

// TestAuthz_Subscribe delegates to inner controller (REQ-AZ-007).
func TestAuthz_Subscribe(t *testing.T) {
	p := authz.NewPolicy()
	ctrl, _ := newCtrl(p, "")
	t.Cleanup(func() { _ = ctrl.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := ctrl.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	_ = ch
}

// TestAuthz_Close_Idempotent Close is safe to call twice (REQ-AZ-008).
func TestAuthz_Close_Idempotent(t *testing.T) {
	p := authz.NewPolicy()
	ctrl, _ := newCtrl(p, "")
	if err := ctrl.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := ctrl.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

// TestAuthz_Close_RejectsSend Send after Close returns ErrClosed (REQ-AZ-008).
func TestAuthz_Close_RejectsSend(t *testing.T) {
	p := authz.NewPolicy()
	ctrl, _ := newCtrl(p, "")
	_ = ctrl.Close()

	_, err := ctrl.Send(context.Background(), &rcp.Command{Zone: rcp.ZoneFrontLeft})
	if !errors.Is(err, rcp.ErrClosed) {
		t.Errorf("err = %v, want ErrClosed", err)
	}
}
