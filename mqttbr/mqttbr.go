// Package mqttbr provides a pure-Go in-process MQTT-shaped cloud/telematics
// bridge for go-RCP, for the OPEN Alliance TC18 Remote Control Protocol
// (RCP), as described by the "OPEN Alliance TC18 Remote Control Protocol
// Specification v0.5.1_RC".
//
// This is ROADMAP.md Milestone 56 (v0.69.0)'s ADAPT-flagged rebuild: per
// Phase 17's disposition table, the reasoning is identical to ddsbr's — MQTT
// cloud/telematics integration is orthogonal to TC18 RCP and stays
// genuinely necessary, just re-pointed at the new addressing/request types
// in place of the retired rcp.Status broadcast. See ddsbr's own package doc
// comment for the fuller rationale (this package's Bridge mirrors ddsbr's
// shape exactly, one MQTT topic pair standing in for ddsbr's DDS topic
// pair): PublishResponse/PublishTrigger publish caller-supplied endpoint
// telemetry to the MQTT status topic, while messages arriving on the
// command topic are decoded and forwarded to an upstream *udp.Controller.
//
// Broker/Client are unchanged from the retired package: this in-process
// MQTT-like machinery never depended on the retired rcp API.
package mqttbr

//fusa:req REQ-MQTT-001
//fusa:req REQ-MQTT-002
//fusa:req REQ-MQTT-003
//fusa:req REQ-MQTT-004
//fusa:req REQ-MQTT-005
//fusa:req REQ-MQTT-006
//fusa:req REQ-MQTT-007
//fusa:req REQ-MQTT-008

import (
	"context"
	"encoding/binary"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/udp"
)

// ErrShortMessage is returned when a command-topic message is too short to
// contain the fixed wire header (see decodeCommand).
var ErrShortMessage = errors.New("rcp/mqttbr: command message too short")

// DefaultRequestTimeout bounds how long Bridge waits for the upstream
// controller's response to a forwarded command message.
const DefaultRequestTimeout = 5 * time.Second

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

// EncodeCommand/decodeCommand wire format for the command topic:
// [byte_bus_id(1)][control(1)][body...].
const commandHeaderLen = 2

func EncodeCommand(addr avtp.ByteBusID, control acf.ControlFlags, body []byte) []byte {
	out := make([]byte, commandHeaderLen+len(body))
	out[0] = byte(addr)
	out[1] = byte(control)
	copy(out[commandHeaderLen:], body)
	return out
}

func decodeCommand(msg []byte) (avtp.ByteBusID, acf.ControlFlags, []byte, error) {
	if len(msg) < commandHeaderLen {
		return 0, 0, nil, ErrShortMessage
	}
	return avtp.ByteBusID(msg[0]), acf.ControlFlags(msg[1]), msg[commandHeaderLen:], nil
}

// Status-topic message tags, distinguishing a published endpoint response
// from a published trigger-event count on the same topic.
const (
	statusTagResponse uint8 = 0x00
	statusTagTrigger  uint8 = 0x01
)

// StatusMessage is a decoded status-topic message (see DecodeStatus): either
// an endpoint ResponseSample (IsTrigger false) or a trigger-event count
// (IsTrigger true, Count meaningful, Control/Body/TransactionNum unused).
type StatusMessage struct {
	IsTrigger      bool
	ByteBusID      avtp.ByteBusID
	TransactionNum avtp.TransactionNum
	Control        acf.ControlFlags
	Body           []byte
	Count          int
}

// responseHeaderLen is tag(1) + byte_bus_id(1) + transaction_num(2) + control(1).
const responseHeaderLen = 5

// triggerMsgLen is tag(1) + byte_bus_id(1) + count(2).
const triggerMsgLen = 4

func encodeResponseStatus(resp acf.Message) []byte {
	out := make([]byte, responseHeaderLen+len(resp.Body))
	out[0] = statusTagResponse
	out[1] = byte(resp.ByteBusID)
	binary.BigEndian.PutUint16(out[2:4], uint16(resp.TransactionNum))
	out[4] = byte(resp.Control)
	copy(out[responseHeaderLen:], resp.Body)
	return out
}

func encodeTriggerStatus(source avtp.ByteBusID, count int) []byte {
	out := make([]byte, triggerMsgLen)
	out[0] = statusTagTrigger
	out[1] = byte(source)
	c := count
	if c < 0 {
		c = 0
	} else if c > 0xFFFF {
		c = 0xFFFF
	}
	binary.BigEndian.PutUint16(out[2:4], uint16(c)) //nolint:gosec // clamped above
	return out
}

// DecodeStatus parses a status-topic message published by PublishResponse
// or PublishTrigger back into a StatusMessage.
func DecodeStatus(msg []byte) (StatusMessage, error) {
	if len(msg) < 2 {
		return StatusMessage{}, ErrShortMessage
	}
	switch msg[0] {
	case statusTagTrigger:
		if len(msg) != triggerMsgLen {
			return StatusMessage{}, ErrShortMessage
		}
		return StatusMessage{
			IsTrigger: true,
			ByteBusID: avtp.ByteBusID(msg[1]),
			Count:     int(binary.BigEndian.Uint16(msg[2:4])),
		}, nil
	default:
		if len(msg) < responseHeaderLen {
			return StatusMessage{}, ErrShortMessage
		}
		return StatusMessage{
			ByteBusID:      avtp.ByteBusID(msg[1]),
			TransactionNum: avtp.TransactionNum(binary.BigEndian.Uint16(msg[2:4])),
			Control:        acf.ControlFlags(msg[4]),
			Body:           append([]byte(nil), msg[responseHeaderLen:]...),
		}, nil
	}
}

// Bridge connects a *udp.Controller to MQTT topics via a Client:
// PublishResponse/PublishTrigger publish caller-supplied endpoint telemetry
// to statusTopic, while messages arriving on cmdTopic are decoded and
// forwarded upstream as requests.
type Bridge struct {
	upstream    *udp.Controller
	client      *Client
	statusTopic string
	cmdTopic    string
	timeout     time.Duration
	closed      atomic.Bool
	stop        chan struct{}
	wg          sync.WaitGroup
}

// NewBridge creates a Bridge and starts the command-forwarding goroutine.
// Call Close to stop it.
func NewBridge(upstream *udp.Controller, client *Client, statusTopic, cmdTopic string) *Bridge {
	b := &Bridge{
		upstream:    upstream,
		client:      client,
		statusTopic: statusTopic,
		cmdTopic:    cmdTopic,
		timeout:     DefaultRequestTimeout,
		stop:        make(chan struct{}),
	}
	b.wg.Add(1)
	go b.runCmd()
	return b
}

// PublishResponse publishes resp to the MQTT status topic. A caller obtains
// resp however it likes (a direct Read/Write call, a request.Dispatcher
// poll result, or any other source) and calls this method for every
// response it wants fanned out to MQTT subscribers.
func (b *Bridge) PublishResponse(resp acf.Message) {
	b.client.Publish(b.statusTopic, encodeResponseStatus(resp))
}

// PublishTrigger publishes a trigger-event count observed on source to the
// MQTT status topic, tagged so a subscriber decoding with DecodeStatus can
// tell it apart from a PublishResponse message on the same topic.
func (b *Bridge) PublishTrigger(source avtp.ByteBusID, count int) {
	b.client.Publish(b.statusTopic, encodeTriggerStatus(source, count))
}

// Close stops the command-forwarding goroutine and waits for it to exit.
// Idempotent.
func (b *Bridge) Close() {
	if !b.closed.CompareAndSwap(false, true) {
		return
	}
	close(b.stop)
	b.wg.Wait()
}

// runCmd reads command messages from the MQTT command topic and forwards
// each as a request to the upstream controller.
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
			addr, control, body, err := decodeCommand(msg)
			if err != nil {
				continue
			}
			reqCtx, reqCancel := context.WithTimeout(ctx, b.timeout)
			_, _ = b.upstream.Request(reqCtx, addr, control, body)
			reqCancel()
		}
	}
}
