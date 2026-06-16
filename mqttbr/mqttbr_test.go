//fusa:test REQ-MQTT-001
//fusa:test REQ-MQTT-002
//fusa:test REQ-MQTT-003
//fusa:test REQ-MQTT-004
//fusa:test REQ-MQTT-005
//fusa:test REQ-MQTT-006
//fusa:test REQ-MQTT-007
//fusa:test REQ-MQTT-008

package mqttbr_test

import (
	"testing"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/mock"
	"github.com/SoundMatt/go-RCP/mqttbr"
)

// REQ-MQTT-001: Broker routes PUBLISH to all matching subscribers.
func TestBroker_Routes(t *testing.T) {
	b := mqttbr.NewBroker()
	c1 := mqttbr.NewClient(b)
	c2 := mqttbr.NewClient(b)
	defer c1.Close()
	defer c2.Close()

	ch1 := c1.Subscribe("rcp/status")
	ch2 := c2.Subscribe("rcp/status")

	c1.Publish("rcp/status", []byte("hello"))

	select {
	case msg := <-ch2:
		if string(msg) != "hello" {
			t.Errorf("c2 got %q, want %q", msg, "hello")
		}
	case <-time.After(time.Second):
		t.Error("c2: timeout waiting for routed message")
	}

	// c1 also published, so it receives its own message via the broker
	select {
	case msg := <-ch1:
		if string(msg) != "hello" {
			t.Errorf("c1 got %q, want %q", msg, "hello")
		}
	case <-time.After(time.Second):
		t.Error("c1: timeout waiting for own message")
	}
}

// REQ-MQTT-002: Client.Publish delivers payload to all subscribers.
func TestClient_Publish(t *testing.T) {
	b := mqttbr.NewBroker()
	pub := mqttbr.NewClient(b)
	sub := mqttbr.NewClient(b)
	defer pub.Close()
	defer sub.Close()

	ch := sub.Subscribe("test/topic")
	pub.Publish("test/topic", []byte("payload"))

	select {
	case got := <-ch:
		if string(got) != "payload" {
			t.Errorf("got %q, want %q", got, "payload")
		}
	case <-time.After(time.Second):
		t.Error("timeout")
	}
}

// REQ-MQTT-003: Client.Subscribe returns a channel of payloads.
func TestClient_Subscribe(t *testing.T) {
	b := mqttbr.NewBroker()
	c := mqttbr.NewClient(b)
	defer c.Close()

	ch := c.Subscribe("my/topic")
	if ch == nil {
		t.Fatal("Subscribe returned nil channel")
	}
	// second call same topic returns same channel
	ch2 := c.Subscribe("my/topic")
	if ch != ch2 {
		t.Error("Subscribe on same topic returned different channel")
	}
}

// REQ-MQTT-004: Client.Unsubscribe removes the subscription.
func TestClient_Unsubscribe(t *testing.T) {
	b := mqttbr.NewBroker()
	pub := mqttbr.NewClient(b)
	sub := mqttbr.NewClient(b)
	defer pub.Close()
	defer sub.Close()

	ch := sub.Subscribe("unsub/test")
	sub.Unsubscribe("unsub/test")
	pub.Publish("unsub/test", []byte("after-unsub"))

	select {
	case <-ch:
		t.Error("received message after Unsubscribe")
	case <-time.After(50 * time.Millisecond):
		// expected: no message
	}
}

// REQ-MQTT-005: Bridge publishes rcp.Status to the status topic.
func TestBridge_PublishesStatus(t *testing.T) {
	b := mqttbr.NewBroker()
	inner := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer inner.Close()

	client := mqttbr.NewClient(b)
	defer client.Close()
	listener := mqttbr.NewClient(b)
	defer listener.Close()

	statusCh := listener.Subscribe("rcp/status")

	bridge := mqttbr.NewBridge(inner, client, "rcp/status", "rcp/cmd")
	defer bridge.Close()

	// Publish repeatedly until the bridge goroutine has subscribed
	go func() {
		for {
			inner.Publish([]byte("data"))
			time.Sleep(10 * time.Millisecond)
		}
	}()

	select {
	case msg := <-statusCh:
		if len(msg) < 2 {
			t.Errorf("status message too short: %d bytes", len(msg))
		}
	case <-time.After(3 * time.Second):
		t.Error("timeout waiting for status publish")
	}
}

// REQ-MQTT-006: Bridge dispatches command topic messages via rcp.Controller.Send.
func TestBridge_DispatchesCmd(t *testing.T) {
	b := mqttbr.NewBroker()
	dispatched := make(chan rcp.CommandType, 1)
	inner := mock.NewController(rcp.ZoneFrontLeft, func(cmd *rcp.Command) *rcp.Response {
		dispatched <- cmd.Type
		return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusOK}
	})
	defer inner.Close()

	client := mqttbr.NewClient(b)
	defer client.Close()
	bridge := mqttbr.NewBridge(inner, client, "rcp/status", "rcp/cmd")
	defer bridge.Close()

	sender := mqttbr.NewClient(b)
	defer sender.Close()

	// Give the bridge goroutine time to subscribe
	time.Sleep(20 * time.Millisecond)

	msg := []byte{byte(rcp.ZoneFrontLeft), byte(rcp.CmdSet)}
	sender.Publish("rcp/cmd", msg)

	select {
	case got := <-dispatched:
		if got != rcp.CmdSet {
			t.Errorf("got %v, want CmdSet", got)
		}
	case <-time.After(3 * time.Second):
		t.Error("timeout waiting for dispatch")
	}
}

// REQ-MQTT-007: Client.Close unsubscribes from all topics.
func TestClient_CloseUnsubscribesAll(t *testing.T) {
	b := mqttbr.NewBroker()
	pub := mqttbr.NewClient(b)
	sub := mqttbr.NewClient(b)
	defer pub.Close()

	ch := sub.Subscribe("a/topic")
	sub.Close()

	pub.Publish("a/topic", []byte("after-close"))

	select {
	case <-ch:
		t.Error("received message after client Close")
	case <-time.After(50 * time.Millisecond):
		// expected: channel closed, no new messages
	}
}

// REQ-MQTT-008: Bridge.Close is idempotent.
func TestBridge_CloseIdempotent(t *testing.T) {
	b := mqttbr.NewBroker()
	inner := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer inner.Close()

	client := mqttbr.NewClient(b)
	defer client.Close()

	bridge := mqttbr.NewBridge(inner, client, "rcp/status", "rcp/cmd")
	bridge.Close()
	bridge.Close() // must not panic or deadlock
}
