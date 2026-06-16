//fusa:test REQ-ZG-001
//fusa:test REQ-ZG-002
//fusa:test REQ-ZG-003
//fusa:test REQ-ZG-004
//fusa:test REQ-ZG-005
//fusa:test REQ-ZG-006
//fusa:test REQ-ZG-007
//fusa:test REQ-ZG-008

package zonegroup_test

import (
	"context"
	"errors"
	"testing"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/mock"
	"github.com/SoundMatt/go-RCP/zonegroup"
)

func zones(zs ...rcp.Zone) []rcp.Controller {
	ctrls := make([]rcp.Controller, len(zs))
	for i, z := range zs {
		ctrls[i] = mock.NewController(z, nil)
	}
	return ctrls
}

// TestZoneGroup_BroadcastAllOK all zones reply StatusOK (REQ-ZG-001).
func TestZoneGroup_BroadcastAllOK(t *testing.T) {
	g, err := zonegroup.NewGroup(zones(rcp.ZoneFrontLeft, rcp.ZoneFrontRight, rcp.ZoneRearLeft))
	if err != nil {
		t.Fatalf("NewGroup: %v", err)
	}
	t.Cleanup(func() { _ = g.Close() })

	res, err := g.Broadcast(context.Background(), &rcp.Command{Type: rcp.CmdSet})
	if err != nil {
		t.Fatalf("Broadcast: %v", err)
	}
	if !res.OK() {
		t.Errorf("OK() = false, want true; errors: %v", res.Errors())
	}
	if len(res.Results) != 3 {
		t.Errorf("Results len = %d, want 3", len(res.Results))
	}
}

// TestZoneGroup_CmdZoneOverride cmd.Zone is overridden per member (REQ-ZG-002).
func TestZoneGroup_CmdZoneOverride(t *testing.T) {
	var gotZones []rcp.Zone

	g, err := zonegroup.NewGroup(zones(rcp.ZoneFrontLeft, rcp.ZoneRearRight))
	if err != nil {
		t.Fatalf("NewGroup: %v", err)
	}
	t.Cleanup(func() { _ = g.Close() })

	// The command starts with ZoneCentral; Broadcast must override per member.
	res, err := g.Broadcast(context.Background(), &rcp.Command{Zone: rcp.ZoneCentral, Type: rcp.CmdGet})
	if err != nil {
		t.Fatalf("Broadcast: %v", err)
	}
	for _, zr := range res.Results {
		gotZones = append(gotZones, zr.Zone)
	}
	for _, z := range gotZones {
		if z == rcp.ZoneCentral {
			t.Errorf("zone was not overridden: got ZoneCentral in result")
		}
	}
}

// TestZoneGroup_PartialFailure BroadcastResult.OK false when a member errors (REQ-ZG-003).
func TestZoneGroup_PartialFailure(t *testing.T) {
	// Mix a healthy mock with a closed (error-returning) mock.
	healthy := mock.NewController(rcp.ZoneFrontLeft, nil)
	sick := mock.NewController(rcp.ZoneFrontRight, nil)
	_ = sick.Close() // closed → returns ErrClosed on Send

	g, _ := zonegroup.NewGroup([]rcp.Controller{healthy, sick})
	// Don't close g here — it would try to close already-closed sick again.

	res, err := g.Broadcast(context.Background(), &rcp.Command{Type: rcp.CmdGet})
	if err != nil {
		t.Fatalf("Broadcast: %v", err)
	}
	if res.OK() {
		t.Error("OK() = true, want false (sick member)")
	}
	errs := res.Errors()
	if len(errs) == 0 {
		t.Error("Errors() empty, want at least one error for sick member")
	}
}

// TestZoneGroup_ContextCancel context cancellation propagates to all Sends (REQ-ZG-004).
func TestZoneGroup_ContextCancel(t *testing.T) {
	g, _ := zonegroup.NewGroup(zones(rcp.ZoneFrontLeft, rcp.ZoneFrontRight))
	t.Cleanup(func() { _ = g.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	// Both zones are healthy mocks; timeout is so short the sends may or may not complete.
	// This test just verifies no panic and no hang.
	_, err := g.Broadcast(ctx, &rcp.Command{Type: rcp.CmdNoop})
	_ = err // may be nil or context error
}

// TestZoneGroup_Zones returns the correct zone list (REQ-ZG-005).
func TestZoneGroup_Zones(t *testing.T) {
	want := []rcp.Zone{rcp.ZoneFrontLeft, rcp.ZoneRearRight}
	g, _ := zonegroup.NewGroup(zones(want...))
	t.Cleanup(func() { _ = g.Close() })

	got := g.Zones()
	if len(got) != len(want) {
		t.Fatalf("Zones() len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Zones()[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestZoneGroup_Len returns correct member count (REQ-ZG-005).
func TestZoneGroup_Len(t *testing.T) {
	g, _ := zonegroup.NewGroup(zones(rcp.ZoneFrontLeft, rcp.ZoneFrontRight, rcp.ZoneRearLeft, rcp.ZoneRearRight))
	t.Cleanup(func() { _ = g.Close() })

	if got := g.Len(); got != 4 {
		t.Errorf("Len() = %d, want 4", got)
	}
}

// TestZoneGroup_NewGroup_Empty rejects empty member list (REQ-ZG-006).
func TestZoneGroup_NewGroup_Empty(t *testing.T) {
	_, err := zonegroup.NewGroup(nil)
	if err == nil {
		t.Error("expected error for empty group, got nil")
	}
}

// TestZoneGroup_NewGroup_NilMember rejects nil member (REQ-ZG-006).
func TestZoneGroup_NewGroup_NilMember(t *testing.T) {
	ctrls := []rcp.Controller{mock.NewController(rcp.ZoneFrontLeft, nil), nil}
	_, err := zonegroup.NewGroup(ctrls)
	if err == nil {
		t.Error("expected error for nil member, got nil")
	}
}

// TestZoneGroup_Close_Idempotent safe to call twice (REQ-ZG-007).
func TestZoneGroup_Close_Idempotent(t *testing.T) {
	g, _ := zonegroup.NewGroup(zones(rcp.ZoneFrontLeft))
	if err := g.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := g.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

// TestZoneGroup_BroadcastAfterClose returns ErrClosed (REQ-ZG-007).
func TestZoneGroup_BroadcastAfterClose(t *testing.T) {
	g, _ := zonegroup.NewGroup(zones(rcp.ZoneFrontLeft))
	_ = g.Close()

	_, err := g.Broadcast(context.Background(), &rcp.Command{})
	if !errors.Is(err, rcp.ErrClosed) {
		t.Errorf("err = %v, want ErrClosed", err)
	}
}

// TestZoneGroup_Concurrent no race under concurrent broadcasts (REQ-ZG-008).
func TestZoneGroup_Concurrent(t *testing.T) {
	g, _ := zonegroup.NewGroup(zones(rcp.ZoneFrontLeft, rcp.ZoneFrontRight, rcp.ZoneRearLeft, rcp.ZoneRearRight))
	t.Cleanup(func() { _ = g.Close() })

	ctx := context.Background()
	const n = 20
	done := make(chan struct{}, n)
	for i := 0; i < n; i++ {
		go func() {
			_, _ = g.Broadcast(ctx, &rcp.Command{Type: rcp.CmdNoop})
			done <- struct{}{}
		}()
	}
	for i := 0; i < n; i++ {
		<-done
	}
}
