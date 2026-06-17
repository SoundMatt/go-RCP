package rcp_test

//fusa:test REQ-ADAPT-001
//fusa:test REQ-ADAPT-002
//fusa:test REQ-ADAPT-003
//fusa:test REQ-ADAPT-004
//fusa:test REQ-ADAPT-005
//fusa:test REQ-ADAPT-006
//fusa:test REQ-ADAPT-007
//fusa:test REQ-ADAPT-008
//fusa:test REQ-MSG-003
//fusa:test REQ-MSG-004
//fusa:test REQ-MSG-005
//fusa:test REQ-MSG-006
//fusa:test REQ-MSG-007
//fusa:test REQ-MSG-008
//fusa:test REQ-MSG-009
//fusa:test REQ-MSG-010

import (
	"context"
	"testing"
	"time"

	relay "github.com/SoundMatt/RELAY"
	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/mock"
)

// ── Adapt ─────────────────────────────────────────────────────────────────────

func TestAdapt_ReturnsCaller(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer ctrl.Close() //nolint:errcheck
	caller := rcp.Adapt(ctrl)
	if caller == nil {
		t.Error("Adapt() returned nil")
	}
	// Compile-time: relay.Caller embeds relay.Node; both are satisfied.
	var _ relay.Node = caller
}

func TestAdapter_Protocol(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer ctrl.Close() //nolint:errcheck
	if got := rcp.Adapt(ctrl).Protocol(); got != relay.RCP {
		t.Errorf("Protocol() = %v, want relay.RCP", got)
	}
}

func TestAdapter_Send_DispatchesCommand(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer ctrl.Close() //nolint:errcheck
	node := rcp.Adapt(ctrl)
	err := node.Send(context.Background(), relay.Message{
		Protocol: relay.RCP,
		ID:       "front-left",
		Meta:     map[string]string{"rcp.cmd_type": "get"},
	})
	if err != nil {
		t.Errorf("Send() error: %v", err)
	}
}

func TestAdapter_Send_UnknownZone_ReturnsError(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer ctrl.Close() //nolint:errcheck
	err := rcp.Adapt(ctrl).Send(context.Background(), relay.Message{
		Protocol: relay.RCP,
		ID:       "nowhere",
	})
	if err == nil {
		t.Error("Send(unknown zone) did not return error")
	}
}

func TestAdapter_Call_ReturnsRelayMessage(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer ctrl.Close() //nolint:errcheck
	caller := rcp.Adapt(ctrl)
	resp, err := caller.Call(context.Background(), relay.Message{
		Protocol: relay.RCP,
		ID:       "front-left",
		Meta:     map[string]string{"rcp.cmd_type": "get"},
	})
	if err != nil {
		t.Fatalf("Call() error: %v", err)
	}
	if resp.Protocol != relay.RCP {
		t.Errorf("resp.Protocol = %v, want relay.RCP", resp.Protocol)
	}
}

func TestAdapter_Subscribe_ReceivesMessages(t *testing.T) {
	mc := mock.NewController(rcp.ZoneCentral, nil)
	defer mc.Close() //nolint:errcheck
	node := rcp.Adapt(mc)
	ch, err := node.Subscribe(relay.WithChannelDepth(8))
	if err != nil {
		t.Fatalf("Subscribe() error: %v", err)
	}
	mc.Publish([]byte("ping"))
	select {
	case msg, ok := <-ch:
		if !ok {
			t.Fatal("channel closed unexpectedly")
		}
		if msg.Protocol != relay.RCP {
			t.Errorf("msg.Protocol = %v, want relay.RCP", msg.Protocol)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for message")
	}
}

func TestAdapter_Close_DelegatesToController(t *testing.T) {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
	node := rcp.Adapt(ctrl)
	if err := node.Close(); err != nil {
		t.Errorf("Close() error: %v", err)
	}
}

// ── Status.ToMessage ──────────────────────────────────────────────────────────

func TestStatus_ToMessage_Protocol(t *testing.T) {
	s := &rcp.Status{Zone: rcp.ZoneFrontLeft, Seq: 3, Healthy: true, Payload: []byte("data")}
	msg := s.ToMessage()
	if msg.Protocol != relay.RCP {
		t.Errorf("ToMessage().Protocol = %v, want relay.RCP", msg.Protocol)
	}
}

func TestStatus_ToMessage_ID(t *testing.T) {
	s := &rcp.Status{Zone: rcp.ZoneRearRight}
	msg := s.ToMessage()
	if msg.ID != rcp.ZoneRearRight.String() {
		t.Errorf("ToMessage().ID = %q, want %q", msg.ID, rcp.ZoneRearRight.String())
	}
}

func TestStatus_ToMessage_Payload(t *testing.T) {
	payload := []byte("health-check")
	s := &rcp.Status{Zone: rcp.ZoneCentral, Seq: 7, Payload: payload}
	msg := s.ToMessage()
	if string(msg.Payload) != string(payload) {
		t.Errorf("ToMessage().Payload = %q, want %q", msg.Payload, payload)
	}
	if msg.Seq != 7 {
		t.Errorf("ToMessage().Seq = %d, want 7", msg.Seq)
	}
}

func TestStatus_ToMessage_HealthyMeta(t *testing.T) {
	for _, healthy := range []bool{true, false} {
		s := &rcp.Status{Zone: rcp.ZoneFrontLeft, Healthy: healthy}
		msg := s.ToMessage()
		want := "false"
		if healthy {
			want = "true"
		}
		if got := msg.Meta["rcp.healthy"]; got != want {
			t.Errorf("Meta[rcp.healthy] = %q, want %q", got, want)
		}
	}
}

// ── CommandFromMessage ────────────────────────────────────────────────────────

func TestCommandFromMessage_Zone(t *testing.T) {
	msg := relay.Message{Protocol: relay.RCP, ID: "front-right"}
	cmd, err := rcp.CommandFromMessage(msg)
	if err != nil {
		t.Fatalf("CommandFromMessage() error: %v", err)
	}
	if cmd.Zone != rcp.ZoneFrontRight {
		t.Errorf("cmd.Zone = %v, want ZoneFrontRight", cmd.Zone)
	}
}

func TestCommandFromMessage_Priority(t *testing.T) {
	cases := []struct {
		meta string
		want rcp.Priority
	}{
		{"normal", rcp.PriorityNormal},
		{"high", rcp.PriorityHigh},
		{"critical", rcp.PriorityCritical},
	}
	for _, tc := range cases {
		msg := relay.Message{
			Protocol: relay.RCP,
			ID:       "central",
			Meta:     map[string]string{"rcp.priority": tc.meta},
		}
		cmd, err := rcp.CommandFromMessage(msg)
		if err != nil {
			t.Fatalf("CommandFromMessage() error: %v", err)
		}
		if cmd.Priority != tc.want {
			t.Errorf("priority %q: cmd.Priority = %v, want %v", tc.meta, cmd.Priority, tc.want)
		}
	}
}

func TestCommandFromMessage_CmdType(t *testing.T) {
	cases := []struct {
		meta string
		want rcp.CommandType
	}{
		{"noop", rcp.CmdNoop},
		{"set", rcp.CmdSet},
		{"get", rcp.CmdGet},
		{"reset", rcp.CmdReset},
		{"watchdog", rcp.CmdWatchdog},
		{"sleep", rcp.CmdSleep},
		{"wake", rcp.CmdWake},
	}
	for _, tc := range cases {
		msg := relay.Message{
			Protocol: relay.RCP,
			ID:       "central",
			Meta:     map[string]string{"rcp.cmd_type": tc.meta},
		}
		cmd, err := rcp.CommandFromMessage(msg)
		if err != nil {
			t.Fatalf("CommandFromMessage() error: %v", err)
		}
		if cmd.Type != tc.want {
			t.Errorf("cmd_type %q: cmd.Type = %v, want %v", tc.meta, cmd.Type, tc.want)
		}
	}
}

// ── ResponseToMessage ─────────────────────────────────────────────────────────

func TestResponseToMessage(t *testing.T) {
	resp := &rcp.Response{
		CommandID: 42,
		Zone:      rcp.ZoneRearLeft,
		Status:    rcp.StatusOK,
		Payload:   []byte("ok"),
	}
	msg := rcp.ResponseToMessage(resp)
	if msg.Protocol != relay.RCP {
		t.Errorf("msg.Protocol = %v, want relay.RCP", msg.Protocol)
	}
	if msg.ID != rcp.ZoneRearLeft.String() {
		t.Errorf("msg.ID = %q, want %q", msg.ID, rcp.ZoneRearLeft.String())
	}
	if msg.Meta["rcp.status"] != "0" {
		t.Errorf("msg.Meta[rcp.status] = %q, want \"0\"", msg.Meta["rcp.status"])
	}
}
