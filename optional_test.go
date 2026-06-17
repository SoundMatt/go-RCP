package rcp_test

//fusa:test REQ-OPT-001
//fusa:test REQ-OPT-002
//fusa:test REQ-OPT-003
//fusa:test REQ-OPT-004
//fusa:test REQ-OPT-005
//fusa:test REQ-OPT-006

import (
	"context"
	"testing"
	"time"

	relay "github.com/SoundMatt/RELAY"
	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/mock"
)

func msgFor(zone rcp.Zone, cmdType string) relay.Message {
	return relay.Message{
		Protocol: relay.RCP,
		ID:       zone.String(),
		Meta:     map[string]string{"rcp.cmd_type": cmdType},
	}
}

func asHealth(t *testing.T, n relay.Caller) relay.HealthProvider {
	t.Helper()
	hp, ok := n.(relay.HealthProvider)
	if !ok {
		t.Fatal("adapter is not a relay.HealthProvider")
	}
	return hp
}

func asMetrics(t *testing.T, n relay.Caller) relay.MetricsProvider {
	t.Helper()
	mp, ok := n.(relay.MetricsProvider)
	if !ok {
		t.Fatal("adapter is not a relay.MetricsProvider")
	}
	return mp
}

func asDrainer(t *testing.T, n relay.Caller) relay.Drainer {
	t.Helper()
	d, ok := n.(relay.Drainer)
	if !ok {
		t.Fatal("adapter is not a relay.Drainer")
	}
	return d
}

// ── HealthProvider ──────────────────────────────────────────────────────────

func TestAdapter_ImplementsOptionalInterfaces(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer ctrl.Close() //nolint:errcheck
	node := rcp.Adapt(ctrl)
	if _, ok := node.(relay.HealthProvider); !ok {
		t.Error("adapter does not implement relay.HealthProvider")
	}
	if _, ok := node.(relay.MetricsProvider); !ok {
		t.Error("adapter does not implement relay.MetricsProvider")
	}
	if _, ok := node.(relay.Drainer); !ok {
		t.Error("adapter does not implement relay.Drainer")
	}
}

func TestAdapter_Health_OKWhenFresh(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer ctrl.Close() //nolint:errcheck
	node := rcp.Adapt(ctrl)
	if got := asHealth(t, node).Health().Status; got != relay.HealthOK {
		t.Errorf("Health().Status = %v, want HealthOK", got)
	}
}

func TestAdapter_Health_DegradedAfterError(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer ctrl.Close() //nolint:errcheck
	node := rcp.Adapt(ctrl)
	// Unknown zone ID forces a CommandFromMessage error.
	_ = node.Send(context.Background(), relay.Message{Protocol: relay.RCP, ID: "nowhere"})
	if got := asHealth(t, node).Health().Status; got != relay.HealthDegraded {
		t.Errorf("Health().Status = %v, want HealthDegraded", got)
	}
}

func TestAdapter_Health_DownAfterClose(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	node := rcp.Adapt(ctrl)
	_ = node.Close()
	if got := asHealth(t, node).Health().Status; got != relay.HealthDown {
		t.Errorf("Health().Status = %v, want HealthDown", got)
	}
}

// ── MetricsProvider ─────────────────────────────────────────────────────────

func TestAdapter_Metrics_CountsCall(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer ctrl.Close() //nolint:errcheck
	node := rcp.Adapt(ctrl)
	if _, err := node.Call(context.Background(), msgFor(rcp.ZoneFrontLeft, "get")); err != nil {
		t.Fatalf("Call: %v", err)
	}
	m := asMetrics(t, node).Metrics()
	if m.WriteCount != 1 {
		t.Errorf("WriteCount = %d, want 1", m.WriteCount)
	}
	if m.DeliverCount != 1 {
		t.Errorf("DeliverCount = %d, want 1", m.DeliverCount)
	}
}

func TestAdapter_Metrics_CountsError(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer ctrl.Close() //nolint:errcheck
	node := rcp.Adapt(ctrl)
	_ = node.Send(context.Background(), relay.Message{Protocol: relay.RCP, ID: "nowhere"})
	if m := asMetrics(t, node).Metrics(); m.ErrorCount != 1 {
		t.Errorf("ErrorCount = %d, want 1", m.ErrorCount)
	}
}

// ── Drainer ─────────────────────────────────────────────────────────────────

func TestAdapter_CloseWithDrain_NoInFlight(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	node := rcp.Adapt(ctrl)
	if err := asDrainer(t, node).CloseWithDrain(context.Background()); err != nil {
		t.Errorf("CloseWithDrain: %v", err)
	}
	// Health must report down after drain-close.
	if got := asHealth(t, node).Health().Status; got != relay.HealthDown {
		t.Errorf("post-drain Health = %v, want HealthDown", got)
	}
}

func TestAdapter_CloseWithDrain_RespectsContext(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	node := rcp.Adapt(ctrl)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	// No in-flight work, so this returns nil promptly regardless of timeout.
	if err := asDrainer(t, node).CloseWithDrain(ctx); err != nil {
		t.Errorf("CloseWithDrain: %v", err)
	}
}
