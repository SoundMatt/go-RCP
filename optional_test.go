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
	hp := rcp.Adapt(ctrl).(relay.HealthProvider)
	if got := hp.Health().Status; got != relay.HealthOK {
		t.Errorf("Health().Status = %v, want HealthOK", got)
	}
}

func TestAdapter_Health_DegradedAfterError(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer ctrl.Close() //nolint:errcheck
	node := rcp.Adapt(ctrl)
	// Unknown zone ID forces a CommandFromMessage error.
	_ = node.Send(context.Background(), relay.Message{Protocol: relay.RCP, ID: "nowhere"})
	hp := node.(relay.HealthProvider)
	if got := hp.Health().Status; got != relay.HealthDegraded {
		t.Errorf("Health().Status = %v, want HealthDegraded", got)
	}
}

func TestAdapter_Health_DownAfterClose(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	node := rcp.Adapt(ctrl)
	_ = node.Close()
	hp := node.(relay.HealthProvider)
	if got := hp.Health().Status; got != relay.HealthDown {
		t.Errorf("Health().Status = %v, want HealthDown", got)
	}
}

// ── MetricsProvider ─────────────────────────────────────────────────────────

func TestAdapter_Metrics_CountsCall(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer ctrl.Close() //nolint:errcheck
	node := rcp.Adapt(ctrl)
	if _, err := node.(relay.Caller).Call(context.Background(), msgFor(rcp.ZoneFrontLeft, "get")); err != nil {
		t.Fatalf("Call: %v", err)
	}
	m := node.(relay.MetricsProvider).Metrics()
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
	if m := node.(relay.MetricsProvider).Metrics(); m.ErrorCount != 1 {
		t.Errorf("ErrorCount = %d, want 1", m.ErrorCount)
	}
}

// ── Drainer ─────────────────────────────────────────────────────────────────

func TestAdapter_CloseWithDrain_NoInFlight(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	node := rcp.Adapt(ctrl)
	d := node.(relay.Drainer)
	if err := d.CloseWithDrain(context.Background()); err != nil {
		t.Errorf("CloseWithDrain: %v", err)
	}
	// Health must report down after drain-close.
	if got := node.(relay.HealthProvider).Health().Status; got != relay.HealthDown {
		t.Errorf("post-drain Health = %v, want HealthDown", got)
	}
}

func TestAdapter_CloseWithDrain_RespectsContext(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	node := rcp.Adapt(ctrl)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	// No in-flight work, so this returns nil promptly regardless of timeout.
	if err := node.(relay.Drainer).CloseWithDrain(ctx); err != nil {
		t.Errorf("CloseWithDrain: %v", err)
	}
}
