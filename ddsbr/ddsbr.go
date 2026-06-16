//fusa:req REQ-DDS-001
//fusa:req REQ-DDS-002
//fusa:req REQ-DDS-003
//fusa:req REQ-DDS-004
//fusa:req REQ-DDS-005
//fusa:req REQ-DDS-006
//fusa:req REQ-DDS-007
//fusa:req REQ-DDS-008

// Package ddsbr provides a DDS (Data Distribution Service) bridge for go-RCP.
//
// DDS is the publish-subscribe middleware used by AUTOSAR Adaptive and
// ROS 2 for real-time data distribution. This package implements a pure-Go
// in-process DDS domain so rcp.Status updates can be published to DDS topics
// and DDS command samples can be forwarded to an rcp.Controller.
//
// Domain manages named Topics. DataWriter writes typed samples to a Topic;
// DataReader receives them. Bridge wires a pair of Topics to an rcp.Controller:
// status events flow from the controller to the status topic, while command
// samples arriving on the command topic are dispatched via the controller.
package ddsbr

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	rcp "github.com/SoundMatt/go-RCP"
)

// ErrTopicNotFound is returned when a topic name is not registered in the domain.
var ErrTopicNotFound = errors.New("rcp/ddsbr: topic not found")

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

// Bridge connects an rcp.Controller to DDS topics.
// Status events from the controller are published to statusWriter.
// Samples from cmdReader are dispatched via the controller's Send method.
type Bridge struct {
	ctrl         rcp.Controller
	statusWriter *DataWriter
	cmdReader    *DataReader
	closed       atomic.Bool
	stopStatus   chan struct{}
	stopCmd      chan struct{}
	wg           sync.WaitGroup
}

// NewBridge creates a Bridge and starts background goroutines.
// Call Bridge.Close to shut them down.
func NewBridge(ctrl rcp.Controller, statusWriter *DataWriter, cmdReader *DataReader) *Bridge {
	b := &Bridge{
		ctrl:         ctrl,
		statusWriter: statusWriter,
		cmdReader:    cmdReader,
		stopStatus:   make(chan struct{}),
		stopCmd:      make(chan struct{}),
	}
	b.wg.Add(2)
	go b.runStatus()
	go b.runCmd()
	return b
}

// Close stops the background goroutines and waits for them to exit.
// Idempotent.
func (b *Bridge) Close() {
	if !b.closed.CompareAndSwap(false, true) {
		return
	}
	close(b.stopStatus)
	close(b.stopCmd)
	b.wg.Wait()
}

// runStatus subscribes to the controller and publishes Status samples.
func (b *Bridge) runStatus() {
	defer b.wg.Done()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { <-b.stopStatus; cancel() }()

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
			b.statusWriter.Write(st)
		case <-b.stopStatus:
			return
		}
	}
}

// runCmd reads command samples from the DDS reader and calls Send.
func (b *Bridge) runCmd() {
	defer b.wg.Done()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { <-b.stopCmd; cancel() }()
	defer cancel()
	for {
		select {
		case <-b.stopCmd:
			return
		case sample, ok := <-b.cmdReader.Read():
			if !ok {
				return
			}
			cmd, ok := sample.(*rcp.Command)
			if !ok {
				continue
			}
			_, _ = b.ctrl.Send(ctx, cmd)
		}
	}
}
