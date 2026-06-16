// Package prioqueue provides a per-zone priority queue that serialises
// concurrent command senders while honouring PriorityCritical > PriorityHigh > PriorityNormal.
//
// Commands at equal priority are dispatched FIFO. Critical commands always
// pre-empt queued Normal and High commands, ensuring watchdog kicks and
// safety-critical actuation are never head-of-line blocked by lower-priority traffic.
//
// A single dispatch goroutine issues inner.Send calls one at a time, preserving
// the inner controller's single-outstanding-request contract.
package prioqueue

//fusa:req REQ-PQ-001
//fusa:req REQ-PQ-002
//fusa:req REQ-PQ-003
//fusa:req REQ-PQ-004
//fusa:req REQ-PQ-005
//fusa:req REQ-PQ-006
//fusa:req REQ-PQ-007
//fusa:req REQ-PQ-008

import (
	"container/heap"
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	rcp "github.com/SoundMatt/go-RCP"
)

// entry holds a queued command, its caller's context, and the result channel.
type entry struct {
	ctx    context.Context
	cmd    *rcp.Command
	result chan result
	seq    uint64 // insertion order — tie-breaks equal priorities (FIFO)
}

type result struct {
	resp *rcp.Response
	err  error
}

// priorityHeap implements heap.Interface.
// Higher Priority = higher urgency; ties broken by earlier seq (FIFO).
type priorityHeap []*entry

func (h priorityHeap) Len() int { return len(h) }
func (h priorityHeap) Less(i, j int) bool {
	if h[i].cmd.Priority != h[j].cmd.Priority {
		return h[i].cmd.Priority > h[j].cmd.Priority
	}
	return h[i].seq < h[j].seq
}
func (h priorityHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *priorityHeap) Push(x interface{}) {
	e, ok := x.(*entry)
	if !ok {
		panic("prioqueue: Push received non-*entry")
	}
	*h = append(*h, e)
}
func (h *priorityHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	old[n-1] = nil
	*h = old[:n-1]
	return x
}

// Controller wraps any rcp.Controller and serialises Send calls through a
// priority queue. A single background goroutine dispatches commands so the
// inner controller never sees concurrent Sends.
type Controller struct {
	inner  rcp.Controller
	done   chan struct{}
	closed atomic.Bool

	mu     sync.Mutex
	h      priorityHeap
	seq    atomic.Uint64
	notify chan struct{} // non-blocking signal: queue has work
}

// NewController wraps inner with priority-queue scheduling.
func NewController(inner rcp.Controller) *Controller {
	c := &Controller{
		inner:  inner,
		done:   make(chan struct{}),
		notify: make(chan struct{}, 1),
	}
	heap.Init(&c.h)
	go c.dispatch()
	return c
}

// Send enqueues cmd at its priority and waits for the response.
// Returns rcp.ErrClosed if the Controller has been closed, or
// rcp.ErrTimeout if ctx expires before the command is dispatched and responded to.
func (c *Controller) Send(ctx context.Context, cmd *rcp.Command) (*rcp.Response, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("rcp/prioqueue: zone %s: %w", c.inner.Zone(), rcp.ErrClosed)
	}
	e := &entry{
		ctx:    ctx,
		cmd:    cmd,
		result: make(chan result, 1),
		seq:    c.seq.Add(1),
	}
	c.mu.Lock()
	heap.Push(&c.h, e)
	c.mu.Unlock()

	select {
	case c.notify <- struct{}{}:
	default:
	}

	select {
	case r := <-e.result:
		return r.resp, r.err
	case <-ctx.Done():
		return nil, fmt.Errorf("rcp/prioqueue: zone %s: %w", c.inner.Zone(), rcp.ErrTimeout)
	case <-c.done:
		return nil, fmt.Errorf("rcp/prioqueue: zone %s: %w", c.inner.Zone(), rcp.ErrClosed)
	}
}

// Zone delegates to the inner controller.
func (c *Controller) Zone() rcp.Zone { return c.inner.Zone() }

// Subscribe delegates to the inner controller.
func (c *Controller) Subscribe(ctx context.Context) (<-chan *rcp.Status, error) {
	return c.inner.Subscribe(ctx)
}

// Close stops the dispatch goroutine and closes the inner controller.
// Safe to call multiple times.
func (c *Controller) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	close(c.done)
	return c.inner.Close()
}

// dispatch is the single goroutine that dequeues and issues inner.Send.
func (c *Controller) dispatch() {
	for {
		select {
		case <-c.done:
			return
		case <-c.notify:
			for {
				c.mu.Lock()
				if c.h.Len() == 0 {
					c.mu.Unlock()
					break
				}
				e, ok := heap.Pop(&c.h).(*entry)
				if !ok {
					break // heap only contains *entry; should never happen
				}
				c.mu.Unlock()

				// If the caller already cancelled, skip the inner send
				// and report the error directly.
				select {
				case <-e.ctx.Done():
					e.result <- result{nil, fmt.Errorf("rcp/prioqueue: %w", rcp.ErrTimeout)}
					continue
				default:
				}

				resp, err := c.inner.Send(e.ctx, e.cmd)
				e.result <- result{resp, err}
			}
		}
	}
}
