// Package ddsbr provides a DDS (Data Distribution Service) publish-subscribe
// telemetry bridge for go-RCP, for the OPEN Alliance TC18 Remote Control
// Protocol (RCP), as described by the "OPEN Alliance TC18 Remote Control
// Protocol Specification v0.5.1_RC".
//
// This is ROADMAP.md Milestone 56 (v0.69.0)'s ADAPT-flagged rebuild: per
// Phase 17's disposition table, DDS pub/sub telemetry fan-out is not
// something TC18 RCP does natively, so this bridge remains genuinely
// necessary — unlike canbr/linbr, it does not narrow into a thin wrapper
// around a native endpoint type. What does change is what it fans out: the
// retired package's Bridge subscribed directly to an rcp.Controller's own
// Subscribe method and republished each incoming rcp.Status. This
// milestone's addressing model has no equivalent native broadcast (see
// ROADMAP.md's Phase 13 framing, "no server-side safety net"): a
// *udp.Controller only ever answers a request it itself sent. So telemetry
// publication becomes caller-driven, PublishResponse/PublishTrigger calls a
// caller makes as it obtains endpoint responses (a direct Read/Write, a
// request.Dispatcher poll result, or a native trigger drain) by whatever
// means it already does — the same "caller supplies the polling loop"
// posture request.Dispatcher's own TriggerPump already establishes for this
// repo (see request/dispatcher.go).
//
// The Domain/Topic/DataWriter/DataReader publish-subscribe machinery below
// is unchanged from the retired package: it never depended on the retired
// rcp API in the first place, so it needed no rebuild of its own.
package ddsbr

//fusa:req REQ-DDS-001
//fusa:req REQ-DDS-002
//fusa:req REQ-DDS-003
//fusa:req REQ-DDS-004
//fusa:req REQ-DDS-005
//fusa:req REQ-DDS-006
//fusa:req REQ-DDS-007
//fusa:req REQ-DDS-008

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/udp"
)

// ErrTopicNotFound is returned when a topic name is not registered in the domain.
var ErrTopicNotFound = errors.New("rcp/ddsbr: topic not found")

// DefaultRequestTimeout bounds how long Bridge waits for the upstream
// controller's response to a forwarded command sample, since a DDS sample
// carries no per-call context of its own.
const DefaultRequestTimeout = 5 * time.Second

// ─── Domain / Topic ───────────────────────────────────────────────────────────

// Topic is a named, typed publish-subscribe channel within a Domain.
// Any number of DataWriters can write and DataReaders can read concurrently.
type Topic struct {
	name string

	mu   sync.RWMutex
	subs map[chan any]struct{}
}

// newTopic allocates an empty Topic with the given name.
func newTopic(name string) *Topic {
	return &Topic{name: name, subs: make(map[chan any]struct{})}
}

// Name returns the topic name.
func (t *Topic) Name() string { return t.name }

// Write delivers sample to all current subscribers without blocking the caller.
func (t *Topic) Write(sample any) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for ch := range t.subs {
		select {
		case ch <- sample:
		default:
		}
	}
}

// subscribe returns a buffered channel that receives subsequent writes.
func (t *Topic) subscribe() chan any {
	ch := make(chan any, 32)
	t.mu.Lock()
	t.subs[ch] = struct{}{}
	t.mu.Unlock()
	return ch
}

// unsubscribe removes ch and closes it.
func (t *Topic) unsubscribe(ch chan any) {
	t.mu.Lock()
	delete(t.subs, ch)
	t.mu.Unlock()
	close(ch)
}

// Domain manages a set of named Topics.
type Domain struct {
	mu     sync.RWMutex
	topics map[string]*Topic
}

// NewDomain returns an empty Domain.
func NewDomain() *Domain { return &Domain{topics: make(map[string]*Topic)} }

// NewTopic creates and registers a Topic with name. If a topic with that name
// already exists the existing one is returned.
func (d *Domain) NewTopic(name string) *Topic {
	d.mu.Lock()
	defer d.mu.Unlock()
	if t, ok := d.topics[name]; ok {
		return t
	}
	t := newTopic(name)
	d.topics[name] = t
	return t
}

// Lookup returns the Topic with name, or ErrTopicNotFound.
func (d *Domain) Lookup(name string) (*Topic, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	t, ok := d.topics[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrTopicNotFound, name)
	}
	return t, nil
}

// ─── DataWriter / DataReader ──────────────────────────────────────────────────

// DataWriter writes typed samples to a Topic.
type DataWriter struct{ topic *Topic }

// NewDataWriter returns a DataWriter bound to t.
func NewDataWriter(t *Topic) *DataWriter { return &DataWriter{topic: t} }

// Write publishes sample to all DataReaders on the topic.
func (w *DataWriter) Write(sample any) { w.topic.Write(sample) }

// DataReader reads samples from a Topic.
type DataReader struct {
	topic *Topic
	ch    chan any
}

// NewDataReader returns a DataReader subscribed to t.
func NewDataReader(t *Topic) *DataReader {
	return &DataReader{topic: t, ch: t.subscribe()}
}

// Read returns a channel of samples written to the topic.
func (r *DataReader) Read() <-chan any { return r.ch }

// Close removes the DataReader from the topic.
func (r *DataReader) Close() { r.topic.unsubscribe(r.ch) }

// ─── Bridge ───────────────────────────────────────────────────────────────────

// ResponseSample is a telemetry sample published for one endpoint response
// (see Bridge.PublishResponse).
type ResponseSample struct {
	ByteBusID      avtp.ByteBusID
	TransactionNum avtp.TransactionNum
	Control        acf.ControlFlags
	Body           []byte
}

// TriggerSample is a telemetry sample published for a trigger-event count
// observed on source (see Bridge.PublishTrigger, and request.TriggerPump for
// the shape a caller typically drains this from).
type TriggerSample struct {
	ByteBusID avtp.ByteBusID
	Count     int
}

// CommandSample is a DDS command sample a Bridge forwards to its upstream
// controller as one plain request.
type CommandSample struct {
	ByteBusID avtp.ByteBusID
	Control   acf.ControlFlags
	Body      []byte
}

// Bridge connects a *udp.Controller to DDS topics: PublishResponse/
// PublishTrigger fan out caller-supplied endpoint telemetry to
// telemetryWriter, while CommandSamples arriving on cmdReader are forwarded
// upstream as requests.
type Bridge struct {
	upstream        *udp.Controller
	telemetryWriter *DataWriter
	cmdReader       *DataReader
	timeout         time.Duration
	closed          atomic.Bool
	stop            chan struct{}
	wg              sync.WaitGroup
}

// NewBridge creates a Bridge forwarding CommandSamples from cmdReader to
// upstream, and publishing telemetry to telemetryWriter. Call Bridge.Close
// to stop the command-forwarding goroutine.
func NewBridge(upstream *udp.Controller, telemetryWriter *DataWriter, cmdReader *DataReader) *Bridge {
	b := &Bridge{
		upstream:        upstream,
		telemetryWriter: telemetryWriter,
		cmdReader:       cmdReader,
		timeout:         DefaultRequestTimeout,
		stop:            make(chan struct{}),
	}
	b.wg.Add(1)
	go b.runCmd()
	return b
}

// PublishResponse publishes resp as a ResponseSample to the telemetry
// topic. A caller obtains resp however it likes — a direct Read/Write call
// against upstream, a request.Dispatcher.Response poll, or any other
// source — and calls this method for every response it wants fanned out to
// DDS subscribers.
func (b *Bridge) PublishResponse(resp acf.Message) {
	b.telemetryWriter.Write(ResponseSample{
		ByteBusID:      resp.ByteBusID,
		TransactionNum: resp.TransactionNum,
		Control:        resp.Control,
		Body:           resp.Body,
	})
}

// PublishTrigger publishes a TriggerSample reporting count new trigger
// events observed on source to the telemetry topic.
func (b *Bridge) PublishTrigger(source avtp.ByteBusID, count int) {
	b.telemetryWriter.Write(TriggerSample{ByteBusID: source, Count: count})
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

// runCmd reads CommandSamples from cmdReader and forwards each as a plain
// request to the upstream controller.
func (b *Bridge) runCmd() {
	defer b.wg.Done()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { <-b.stop; cancel() }()
	defer cancel()

	for {
		select {
		case <-b.stop:
			return
		case sample, ok := <-b.cmdReader.Read():
			if !ok {
				return
			}
			cs, ok := sample.(CommandSample)
			if !ok {
				continue
			}
			reqCtx, reqCancel := context.WithTimeout(ctx, b.timeout)
			_, _ = b.upstream.Request(reqCtx, cs.ByteBusID, cs.Control, cs.Body)
			reqCancel()
		}
	}
}
