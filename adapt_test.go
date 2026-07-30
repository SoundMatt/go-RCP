package rcp_test

//fusa:test REQ-ADAPT-001
//fusa:test REQ-ADAPT-002
//fusa:test REQ-ADAPT-003
//fusa:test REQ-ADAPT-004
//fusa:test REQ-ADAPT-005
//fusa:test REQ-ADAPT-006
//fusa:test REQ-ADAPT-007
//fusa:test REQ-ADAPT-008
//fusa:test REQ-MSG-001
//fusa:test REQ-MSG-002
//fusa:test REQ-MSG-003
//fusa:test REQ-MSG-004
//fusa:test REQ-MSG-005
//fusa:test REQ-MSG-006
//fusa:test REQ-MSG-007
//fusa:test REQ-MSG-008

import (
	"context"
	"testing"

	relay "github.com/SoundMatt/RELAY/v2"
	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
)

// ── Adapt ─────────────────────────────────────────────────────────────────────

func TestAdapt_ReturnsCaller(t *testing.T) {
	_, ctrl := newTestController(t, nil)
	caller := rcp.Adapt(ctrl)
	if caller == nil {
		t.Error("Adapt() returned nil")
	}
	// Compile-time: relay.Caller embeds relay.Node; both are satisfied.
	var _ relay.Node = caller
}

func TestAdapter_Protocol(t *testing.T) {
	_, ctrl := newTestController(t, nil)
	if got := rcp.Adapt(ctrl).Protocol(); got != relay.RCP {
		t.Errorf("Protocol() = %v, want relay.RCP", got)
	}
}

func TestAdapter_Send_DispatchesRequest(t *testing.T) {
	_, ctrl := newTestController(t, nil)
	node := rcp.Adapt(ctrl)
	err := node.Send(context.Background(), relay.Message{
		Protocol: relay.RCP,
		ID:       rcp.EndpointIDString(testAddr),
		Meta:     map[string]string{"rcp.op": "write"},
		Payload:  []byte("hello"),
	})
	if err != nil {
		t.Errorf("Send() error: %v", err)
	}
}

func TestAdapter_Send_UnparseableID_ReturnsError(t *testing.T) {
	_, ctrl := newTestController(t, nil)
	err := rcp.Adapt(ctrl).Send(context.Background(), relay.Message{
		Protocol: relay.RCP,
		ID:       "nowhere",
	})
	if err == nil {
		t.Error("Send(unparseable ID) did not return error")
	}
}

func TestAdapter_Call_ReturnsRelayMessage(t *testing.T) {
	_, ctrl := newTestController(t, nil)
	caller := rcp.Adapt(ctrl)
	resp, err := caller.Call(context.Background(), relay.Message{
		Protocol: relay.RCP,
		ID:       rcp.EndpointIDString(testAddr),
		Meta:     map[string]string{"rcp.op": "read"},
	})
	if err != nil {
		t.Fatalf("Call() error: %v", err)
	}
	if resp.Protocol != relay.RCP {
		t.Errorf("resp.Protocol = %v, want relay.RCP", resp.Protocol)
	}
	if string(resp.Payload) != "ack" {
		t.Errorf("resp.Payload = %q, want %q", resp.Payload, "ack")
	}
}

// TestAdapter_Subscribe_LifecycleCompliant verifies Subscribe returns
// independent channels that are closed on adapter Close, per RELAY §6 — see
// rcpAdapter.Subscribe's own doc comment in adapt.go for why nothing is
// ever delivered on either channel.
func TestAdapter_Subscribe_LifecycleCompliant(t *testing.T) {
	_, ctrl := newTestController(t, nil)
	node := rcp.Adapt(ctrl)

	ch1, err := node.Subscribe(relay.WithChannelDepth(8))
	if err != nil {
		t.Fatalf("Subscribe() error: %v", err)
	}
	ch2, err := node.Subscribe()
	if err != nil {
		t.Fatalf("Subscribe() error: %v", err)
	}

	select {
	case _, ok := <-ch1:
		t.Fatalf("ch1 ready before adapter.Close() (ok=%v); TC18 Subscribe should never deliver or close early", ok)
	default:
	}

	if err := node.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}
	if _, ok := <-ch1; ok {
		t.Error("ch1 not closed after adapter.Close()")
	}
	if _, ok := <-ch2; ok {
		t.Error("ch2 not closed after adapter.Close()")
	}
}

func TestAdapter_Subscribe_AfterClose_ReturnsErrClosed(t *testing.T) {
	_, ctrl := newTestController(t, nil)
	node := rcp.Adapt(ctrl)
	if err := node.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}
	if _, err := node.Subscribe(); err == nil {
		t.Error("Subscribe() after Close() did not return an error")
	}
}

func TestAdapter_Close_DelegatesToController(t *testing.T) {
	_, ctrl := newTestController(t, nil)
	node := rcp.Adapt(ctrl)
	if err := node.Close(); err != nil {
		t.Errorf("Close() error: %v", err)
	}
}

// ── ResponseToMessage ─────────────────────────────────────────────────────────

func TestResponseToMessage_Protocol(t *testing.T) {
	msg := rcp.ResponseToMessage(testAddr, acf.Message{Control: acf.FlagResponse, Body: []byte("data")})
	if msg.Protocol != relay.RCP {
		t.Errorf("msg.Protocol = %v, want relay.RCP", msg.Protocol)
	}
}

func TestResponseToMessage_ID(t *testing.T) {
	msg := rcp.ResponseToMessage(7, acf.Message{})
	if msg.ID != rcp.EndpointIDString(7) {
		t.Errorf("msg.ID = %q, want %q", msg.ID, rcp.EndpointIDString(7))
	}
}

func TestResponseToMessage_Payload(t *testing.T) {
	msg := rcp.ResponseToMessage(testAddr, acf.Message{Body: []byte("health-check")})
	if string(msg.Payload) != "health-check" {
		t.Errorf("msg.Payload = %q, want %q", msg.Payload, "health-check")
	}
}

func TestResponseToMessage_ErrorMeta(t *testing.T) {
	for _, isErr := range []bool{true, false} {
		control := acf.FlagResponse
		if isErr {
			control |= acf.FlagError
		}
		msg := rcp.ResponseToMessage(testAddr, acf.Message{Control: control})
		want := "false"
		if isErr {
			want = "true"
		}
		if got := msg.Meta["rcp.error"]; got != want {
			t.Errorf("isErr=%v: Meta[rcp.error] = %q, want %q", isErr, got, want)
		}
	}
}

func TestResponseToMessage_OpMeta(t *testing.T) {
	cases := []struct {
		name    string
		control acf.ControlFlags
		want    string
	}{
		{"read bit set", acf.FlagRead, "read"},
		{"write bit set", acf.FlagWrite, "write"},
		{"neither bit set", acf.FlagResponse, "read"},
	}
	for _, tc := range cases {
		msg := rcp.ResponseToMessage(testAddr, acf.Message{Control: tc.control})
		if got := msg.Meta["rcp.op"]; got != tc.want {
			t.Errorf("%s: Meta[rcp.op] = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestResponseToMessage_TransactionNumAndReadSizeMeta(t *testing.T) {
	msg := rcp.ResponseToMessage(testAddr, acf.Message{
		TransactionNum:    42,
		Control:           acf.FlagWrite,
		ReadSizeOrSegment: 4,
	})
	if got := msg.Meta["rcp.transaction_num"]; got != "42" {
		t.Errorf("Meta[rcp.transaction_num] = %q, want %q", got, "42")
	}
	if got := msg.Meta["rcp.read_size_or_segment"]; got != "4" {
		t.Errorf("Meta[rcp.read_size_or_segment] = %q, want %q", got, "4")
	}
}

func TestResponseToMessage_TransactionNumAndReadSizeMeta_ZeroDefaults(t *testing.T) {
	msg := rcp.ResponseToMessage(testAddr, acf.Message{})
	if got := msg.Meta["rcp.transaction_num"]; got != "0" {
		t.Errorf("Meta[rcp.transaction_num] = %q, want %q", got, "0")
	}
	if got := msg.Meta["rcp.read_size_or_segment"]; got != "0" {
		t.Errorf("Meta[rcp.read_size_or_segment] = %q, want %q", got, "0")
	}
}

// ── RequestFromMessage ────────────────────────────────────────────────────────

func TestRequestFromMessage_Addr(t *testing.T) {
	msg := relay.Message{Protocol: relay.RCP, ID: "3"}
	addr, _, _, err := rcp.RequestFromMessage(msg)
	if err != nil {
		t.Fatalf("RequestFromMessage() error: %v", err)
	}
	if addr != 3 {
		t.Errorf("addr = %d, want 3", addr)
	}
}

func TestRequestFromMessage_UnparseableID(t *testing.T) {
	_, _, _, err := rcp.RequestFromMessage(relay.Message{Protocol: relay.RCP, ID: "not-a-number"})
	if err == nil {
		t.Fatal("RequestFromMessage(unparseable ID) did not return error")
	}
}

func TestRequestFromMessage_ControlFlags(t *testing.T) {
	cases := []struct {
		name    string
		meta    map[string]string
		payload []byte
		want    acf.ControlFlags
	}{
		{"explicit read", map[string]string{"rcp.op": "read"}, nil, acf.FlagRead},
		{"explicit write", map[string]string{"rcp.op": "write"}, nil, acf.FlagWrite},
		{"default with payload", nil, []byte("x"), acf.FlagWrite},
		{"default without payload", nil, nil, acf.FlagRead},
	}
	for _, tc := range cases {
		msg := relay.Message{Protocol: relay.RCP, ID: "1", Meta: tc.meta, Payload: tc.payload}
		_, control, _, err := rcp.RequestFromMessage(msg)
		if err != nil {
			t.Fatalf("%s: RequestFromMessage() error: %v", tc.name, err)
		}
		if control != tc.want {
			t.Errorf("%s: control = %v, want %v", tc.name, control, tc.want)
		}
	}
}

func TestRequestFromMessage_Body(t *testing.T) {
	payload := []byte("body-bytes")
	_, _, body, err := rcp.RequestFromMessage(relay.Message{Protocol: relay.RCP, ID: "1", Payload: payload})
	if err != nil {
		t.Fatalf("RequestFromMessage() error: %v", err)
	}
	if string(body) != string(payload) {
		t.Errorf("body = %q, want %q", body, payload)
	}
}

// ── EndpointIDString / ParseEndpointID ───────────────────────────────────────

func TestEndpointID_RoundTrip(t *testing.T) {
	for _, addr := range []int{0, 1, 42, 255} {
		s := rcp.EndpointIDString(avtp.ByteBusID(addr))
		got, err := rcp.ParseEndpointID(s)
		if err != nil {
			t.Fatalf("ParseEndpointID(%q) error: %v", s, err)
		}
		if int(got) != addr {
			t.Errorf("ParseEndpointID(%q) = %d, want %d", s, got, addr)
		}
	}
}

func TestParseEndpointID_OutOfRange(t *testing.T) {
	for _, s := range []string{"-1", "256", "abc", ""} {
		if _, err := rcp.ParseEndpointID(s); err == nil {
			t.Errorf("ParseEndpointID(%q) did not return an error", s)
		}
	}
}
