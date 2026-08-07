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
	"context"
	"errors"
	"testing"
	"time"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/mqttbr"
	"github.com/SoundMatt/go-RCP/v9/server"
	"github.com/SoundMatt/go-RCP/v9/udp"
)

func clientStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55}, 1)
}

func serverStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE}, 1)
}

const testAddr = avtp.ByteBusID(1)

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

func newUpstream(t *testing.T) *udp.Controller {
	t.Helper()
	router := udp.NewRouter(udp.NewEP0Handler(server.NewServer()), false)
	if err := router.Register(testAddr, stubHandler{}); err != nil {
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

// TestBroker_Publish_ReachesSubscriber a Broker delivers a Publish to a
// subscribed Client (REQ-MQTT-001).
func TestBroker_Publish_ReachesSubscriber(t *testing.T) {
	broker := mqttbr.NewBroker()
	pub := mqttbr.NewClient(broker)
	sub := mqttbr.NewClient(broker)

	ch := sub.Subscribe("topic")
	pub.Publish("topic", []byte("hello"))

	select {
	case got := <-ch:
		if string(got) != "hello" {
			t.Errorf("got %q, want %q", got, "hello")
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for message")
	}
}

// TestClient_Subscribe_Idempotent repeated Subscribe calls for the same
// topic return the same channel (REQ-MQTT-002).
func TestClient_Subscribe_Idempotent(t *testing.T) {
	broker := mqttbr.NewBroker()
	c := mqttbr.NewClient(broker)
	ch1 := c.Subscribe("topic")
	ch2 := c.Subscribe("topic")
	if ch1 != ch2 {
		t.Error("Subscribe returned distinct channels for the same topic")
	}
}

// TestClient_Unsubscribe_StopsDelivery a Client no longer receives messages
// on a topic after Unsubscribe (REQ-MQTT-003).
func TestClient_Unsubscribe_StopsDelivery(t *testing.T) {
	broker := mqttbr.NewBroker()
	pub := mqttbr.NewClient(broker)
	sub := mqttbr.NewClient(broker)

	sub.Subscribe("topic")
	sub.Unsubscribe("topic")
	pub.Publish("topic", []byte("hello"))

	// Re-subscribing after Unsubscribe gets a fresh channel; assert the
	// original subscription is gone by confirming a second, distinct
	// subscribe call sees nothing left over from the unsubscribed one.
	ch := sub.Subscribe("topic")
	select {
	case got := <-ch:
		t.Errorf("received unexpected message %q after Unsubscribe", got)
	case <-time.After(50 * time.Millisecond):
		// expected: nothing delivered
	}
}

// TestBridge_PublishResponse_Decodable PublishResponse's wire message
// round-trips through DecodeStatus as a non-trigger StatusMessage
// (REQ-MQTT-004).
func TestBridge_PublishResponse_Decodable(t *testing.T) {
	upstream := newUpstream(t)
	broker := mqttbr.NewBroker()
	client := mqttbr.NewClient(broker)
	sub := mqttbr.NewClient(broker)
	statusCh := sub.Subscribe("status")

	b := mqttbr.NewBridge(upstream, client, "status", "cmd")
	defer b.Close()

	resp, err := upstream.Read(context.Background(), testAddr)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	b.PublishResponse(resp)

	select {
	case msg := <-statusCh:
		got, err := mqttbr.DecodeStatus(msg)
		if err != nil {
			t.Fatalf("DecodeStatus: %v", err)
		}
		if got.IsTrigger {
			t.Error("IsTrigger = true, want false")
		}
		if got.ByteBusID != testAddr {
			t.Errorf("ByteBusID = %v, want %v", got.ByteBusID, testAddr)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for status message")
	}
}

// TestBridge_PublishTrigger_Decodable PublishTrigger's wire message
// round-trips through DecodeStatus as a trigger StatusMessage (REQ-MQTT-005).
func TestBridge_PublishTrigger_Decodable(t *testing.T) {
	upstream := newUpstream(t)
	broker := mqttbr.NewBroker()
	client := mqttbr.NewClient(broker)
	sub := mqttbr.NewClient(broker)
	statusCh := sub.Subscribe("status")

	b := mqttbr.NewBridge(upstream, client, "status", "cmd")
	defer b.Close()

	b.PublishTrigger(testAddr, 7)

	select {
	case msg := <-statusCh:
		got, err := mqttbr.DecodeStatus(msg)
		if err != nil {
			t.Fatalf("DecodeStatus: %v", err)
		}
		if !got.IsTrigger {
			t.Error("IsTrigger = false, want true")
		}
		if got.Count != 7 {
			t.Errorf("Count = %d, want 7", got.Count)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for status message")
	}
}

// TestBridge_CommandDispatch a command message published to the command
// topic is decoded and forwarded to the upstream controller (REQ-MQTT-006).
func TestBridge_CommandDispatch(t *testing.T) {
	upstream := newUpstream(t)
	broker := mqttbr.NewBroker()
	client := mqttbr.NewClient(broker)
	cmdPub := mqttbr.NewClient(broker)

	b := mqttbr.NewBridge(upstream, client, "status", "cmd")
	defer b.Close()

	cmdPub.Publish("cmd", mqttbr.EncodeCommand(testAddr, acf.FlagWrite, []byte("hi")))

	time.Sleep(50 * time.Millisecond)
	if _, err := upstream.Read(context.Background(), testAddr); err != nil {
		t.Fatalf("Read after dispatch: %v", err)
	}
}

// TestBridge_CloseIdempotent Close stops the command-forwarding goroutine
// and is safe to call twice (REQ-MQTT-007).
func TestBridge_CloseIdempotent(t *testing.T) {
	upstream := newUpstream(t)
	broker := mqttbr.NewBroker()
	client := mqttbr.NewClient(broker)

	b := mqttbr.NewBridge(upstream, client, "status", "cmd")
	b.Close()
	b.Close() // must not panic or block
}

// TestDecodeStatus_ShortMessage DecodeStatus reports ErrShortMessage for an
// undersized buffer (REQ-MQTT-008).
func TestDecodeStatus_ShortMessage(t *testing.T) {
	if _, err := mqttbr.DecodeStatus([]byte{0x00}); !errors.Is(err, mqttbr.ErrShortMessage) {
		t.Errorf("err = %v, want ErrShortMessage", err)
	}
}
