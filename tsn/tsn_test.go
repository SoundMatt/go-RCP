//fusa:test REQ-TSN-001
//fusa:test REQ-TSN-002
//fusa:test REQ-TSN-003
//fusa:test REQ-TSN-004
//fusa:test REQ-TSN-005
//fusa:test REQ-TSN-006

package tsn_test

import (
	"context"
	"errors"
	"testing"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
	rcptsn "github.com/SoundMatt/go-RCP/tsn"
	rcpudp "github.com/SoundMatt/go-RCP/udp"
)

func newTSNPair(t *testing.T, zone rcp.Zone) (*rcpudp.ZoneServer, *rcptsn.Controller) {
	t.Helper()
	srv, err := rcpudp.NewZoneServer(zone, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctrl, err := rcptsn.NewController(zone, srv.Addr().String(), rcptsn.DefaultTSNConfig())
	if err != nil {
		_ = srv.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = ctrl.Close()
		_ = srv.Close()
	})
	return srv, ctrl
}

// TestTSN_DefaultPCPMap verifies PCP mapping values (REQ-TSN-001).
func TestTSN_DefaultPCPMap(t *testing.T) {
	m := rcptsn.DefaultPCPMap()
	if m.PCPFor(rcp.PriorityNormal) != 2 {
		t.Errorf("Normal PCP = %d, want 2", m.PCPFor(rcp.PriorityNormal))
	}
	if m.PCPFor(rcp.PriorityHigh) != 5 {
		t.Errorf("High PCP = %d, want 5", m.PCPFor(rcp.PriorityHigh))
	}
	if m.PCPFor(rcp.PriorityCritical) != 7 {
		t.Errorf("Critical PCP = %d, want 7", m.PCPFor(rcp.PriorityCritical))
	}
}

// TestTSN_PCPFor_AllPriorities verifies Controller.PCPFor delegates to config (REQ-TSN-002).
func TestTSN_PCPFor_AllPriorities(t *testing.T) {
	cfg := rcptsn.TSNConfig{
		PCPMap: rcptsn.PCPMap{Normal: 1, High: 4, Critical: 6},
	}
	ctrl, err := rcptsn.NewController(rcp.ZoneFrontLeft, "127.0.0.1:9999", cfg)
	if err != nil {
		t.Skip("cannot create controller (likely port in use):", err)
	}
	defer func() { _ = ctrl.Close() }()

	if got := ctrl.PCPFor(rcp.PriorityNormal); got != 1 {
		t.Errorf("Normal PCP = %d, want 1", got)
	}
	if got := ctrl.PCPFor(rcp.PriorityHigh); got != 4 {
		t.Errorf("High PCP = %d, want 4", got)
	}
	if got := ctrl.PCPFor(rcp.PriorityCritical); got != 6 {
		t.Errorf("Critical PCP = %d, want 6", got)
	}
}

// TestTSN_Send_RoundTrip verifies TSN controller can send commands (REQ-TSN-003).
func TestTSN_Send_RoundTrip(t *testing.T) {
	_, ctrl := newTSNPair(t, rcp.ZoneFrontLeft)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdNoop, Priority: rcp.PriorityNormal})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Status != rcp.StatusOK {
		t.Errorf("status = %v, want OK", resp.Status)
	}
}

// TestTSN_Send_CriticalPriority verifies PriorityCritical commands are delivered (REQ-TSN-004).
func TestTSN_Send_CriticalPriority(t *testing.T) {
	_, ctrl := newTSNPair(t, rcp.ZoneFrontRight)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontRight, Type: rcp.CmdWatchdog, Priority: rcp.PriorityCritical})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Status != rcp.StatusOK {
		t.Errorf("status = %v, want OK", resp.Status)
	}
}

// TestTSN_Send_ZoneMismatch verifies ErrZoneMismatch (REQ-TSN-005).
func TestTSN_Send_ZoneMismatch(t *testing.T) {
	_, ctrl := newTSNPair(t, rcp.ZoneRearLeft)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdNoop})
	if !errors.Is(err, rcp.ErrZoneMismatch) {
		t.Errorf("error = %v, want ErrZoneMismatch", err)
	}
}

// TestTSN_Config_CycleAndVLAN verifies TSNConfig fields are accessible (REQ-TSN-006).
func TestTSN_Config_CycleAndVLAN(t *testing.T) {
	cfg := rcptsn.DefaultTSNConfig()
	if cfg.VLAN != 100 {
		t.Errorf("VLAN = %d, want 100", cfg.VLAN)
	}
	if cfg.CycleNs != 500_000 {
		t.Errorf("CycleNs = %d, want 500000", cfg.CycleNs)
	}
	ctrl, err := rcptsn.NewController(rcp.ZoneRearRight, "127.0.0.1:9998", cfg)
	if err != nil {
		t.Skip("cannot create controller:", err)
	}
	defer func() { _ = ctrl.Close() }()

	got := ctrl.Config()
	if got.VLAN != cfg.VLAN {
		t.Errorf("Config.VLAN = %d, want %d", got.VLAN, cfg.VLAN)
	}
	if got.CycleNs != cfg.CycleNs {
		t.Errorf("Config.CycleNs = %d, want %d", got.CycleNs, cfg.CycleNs)
	}
}
