//fusa:req REQ-MQTT-001
//fusa:req REQ-MQTT-002
//fusa:req REQ-MQTT-003
//fusa:req REQ-MQTT-004
//fusa:req REQ-MQTT-005
//fusa:req REQ-MQTT-006
//fusa:req REQ-MQTT-007
//fusa:req REQ-MQTT-008

// Package mqttbr provides a pure-Go in-process MQTT broker bridge for go-RCP.
//
// Broker routes PUBLISH messages to all subscribers on a topic. Client connects
// to a Broker and can Publish or Subscribe to topics. Bridge wires an
// rcp.Controller to a pair of MQTT topics: rcp.Status events are published to
// the status topic, and command payloads arriving on the command topic are
// forwarded to the controller via Send.
package mqttbr

import (
	"context"
	"sync"
	"sync/atomic"

	rcp "github.com/SoundMatt/go-RCP"
)

// ─── Broker ───────────────────────────────────────────────────────────────────

type subscription struct {
	ch chan []byte
}

// Broker is an in-process MQTT-like message broker.
type Broker struct {
	mu   sync.RWMutex
	subs map[string][]*subscription
}

// NewBroker returns an empty Broker.
func NewBroker() *Broker {
	return &Broker{subs: make(map[string][]*subscription)}
}

// publish delivers msg to every subscriber registered for topic.
func (b *Broker) publish(topic string, msg []byte) {
	b.mu.RLock()
	list := b.subs[topic]
	b.mu.RUnlock()
	for _, s := range list {
		select {
		case s.ch <- msg:
		default:
		}
	}
}

// subscribe registers ch for topic.
func (b *Broker) subscribe(topic string, s *subscription) {
	b.mu.Lock()
	b.subs[topic] = append(b.subs[topic], s)
	b.mu.Unlock()
}

// unsubscribe removes s from topic.
func (b *Broker) unsubscribe(topic string, s *subscription) {
	b.mu.Lock()
	defer b.mu.Unlock()
	list := b.subs[topic]
	for i, sub := range list {
		if sub == s {
			b.subs[topic] = append(list[:i], list[i+1:]...)
			return
		}
	}
}

// ─── Client ───────────────────────────────────────────────────────────────────

// Client connects to a Broker and provides Publish / Subscribe.
type Client struct {
	broker *Broker
	mu     sync.Mutex
	subs   map[string]*subscription
	closed atomic.Bool
}

// NewClient returns a Client connected to broker.
func NewClient(broker *Broker) *Client {
	return &Client{
		broker: broker,
		subs:   make(map[string]*subscription),
	}
}

// Publish sends msg to all subscribers on topic.
// Returns without error if the client is closed.
func (c *Client) Publish(topic string, msg []byte) {
	if c.closed.Load() {
		return
	}
	c.broker.publish(topic, msg)
}

// Subscribe returns a channel that receives messages published to topic.
// The same topic may only be subscribed once per Client; subsequent calls
// return the existing channel.
func (c *Client) Subscribe(topic string) <-chan []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	if s, ok := c.subs[topic]; ok {
		return s.ch
	}
	s := &subscription{ch: make(chan []byte, 32)}
	c.broker.subscribe(topic, s)
	c.subs[topic] = s
	return s.ch
}

// Unsubscribe removes the subscription for topic.
func (c *Client) Unsubscribe(topic string) {
	c.mu.Lock()
	s, ok := c.subs[topic]
	if !ok {
		c.mu.Unlock()
		return
	}
	delete(c.subs, topic)
	c.mu.Unlock()
	c.broker.unsubscribe(topic, s)
}

// Close unsubscribes from all topics.
func (c *Client) Close() {
	if !c.closed.CompareAndSwap(false, true) {
		return
	}
	c.mu.Lock()
	topics := make([]string, 0, len(c.subs))
	subs := make([]*subscription, 0, len(c.subs))
	for topic, s := range c.subs {
		topics = append(topics, topic)
		subs = append(subs, s)
	}
	c.subs = make(map[string]*subscription)
	c.mu.Unlock()
	for i, topic := range topics {
		c.broker.unsubscribe(topic, subs[i])
	}
}

// ─── Bridge ───────────────────────────────────────────────────────────────────

// Bridge connects an rcp.Controller to MQTT topics via a Client.
// Status events are published to statusTopic; messages on cmdTopic are decoded
// as rcp.Command Zone + Type (single byte each) and dispatched via Send.
type Bridge struct {
	ctrl        rcp.Controller
	client      *Client
	statusTopic string
	cmdTopic    string
	closed      atomic.Bool
	stop        chan struct{}
	wg          sync.WaitGroup
}

// NewBridge creates a Bridge and starts background goroutines.
// Call Close to stop them.
func NewBridge(ctrl rcp.Controller, client *Client, statusTopic, cmdTopic string) *Bridge {
	b := &Bridge{
		ctrl:        ctrl,
		client:      client,
		statusTopic: statusTopic,
		cmdTopic:    cmdTopic,
		stop:        make(chan struct{}),
	}
	b.wg.Add(2)
	go b.runStatus()
	go b.runCmd()
	return b
}

// Close stops the bridge goroutines and waits for them to exit. Idempotent.
func (b *Bridge) Close() {
	if !b.closed.CompareAndSwap(false, true) {
		return
	}
	close(b.stop)
	b.wg.Wait()
}

// runStatus subscribes to the controller and publishes encoded Status payloads.
func (b *Bridge) runStatus() {
	defer b.wg.Done()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { <-b.stop; cancel() }()

	ch, err := b.ctrl.Subscribe(ctx)
	if err != nil {
		return
	}
	for {
		select {
		case st, ok := <-ch:
			if !ok {
				return
			}
			msg := make([]byte, 2+len(st.Payload))
			msg[0] = byte(st.Zone)
			msg[1] = byte(st.Seq & 0xFF)
			copy(msg[2:], st.Payload)
			b.client.Publish(b.statusTopic, msg)
		case <-b.stop:
			return
		}
	}
}

// runCmd reads command messages from the MQTT topic and dispatches via Send.
// Wire format: [zone byte][type byte][payload...].
func (b *Bridge) runCmd() {
	defer b.wg.Done()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { <-b.stop; cancel() }()
	defer cancel()

	ch := b.client.Subscribe(b.cmdTopic)
	for {
		select {
		case <-b.stop:
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			if len(msg) < 2 {
				continue
			}
			cmd := &rcp.Command{
				Zone:    rcp.Zone(msg[0]),
				Type:    rcp.CommandType(msg[1]),
				Payload: msg[2:],
			}
			_, _ = b.ctrl.Send(ctx, cmd)
		}
	}
}
