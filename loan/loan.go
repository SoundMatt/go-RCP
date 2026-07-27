// Package loan implements the LoaningController wrapper, extending any rcp.Controller
// with zero-copy payload loaning via a sync.Pool.
package loan

//fusa:req REQ-LOAN-001
//fusa:req REQ-LOAN-002
//fusa:req REQ-LOAN-003
//fusa:req REQ-LOAN-004
//fusa:req REQ-LOAN-005
//fusa:req REQ-LOAN-006

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	rcp "github.com/SoundMatt/go-RCP"
)

// Controller wraps any rcp.Controller and implements rcp.LoaningController.
// Payloads for SendLoaned are obtained from a sync.Pool, avoiding allocation
// on the hot path when size fits within the pool's typical buffer capacity.
type Controller struct {
	inner  rcp.Controller
	pool   sync.Pool
	closed atomic.Bool

	// loaned tracks buffers currently on loan, keyed by the address of their
	// first byte, so SendLoaned — which the spec-mandated rcp.LoaningController
	// signature hands only a bare cmd.Payload []byte, with no reference back to
	// the pool slot — can still look up and recycle the right *[]byte. Guarded
	// by mu since Loan/SendLoaned/Loan.Return may be called concurrently.
	mu     sync.Mutex
	loaned map[*byte]*[]byte
}

// New wraps inner as a LoaningController.
func New(inner rcp.Controller) *Controller {
	return &Controller{
		inner:  inner,
		pool:   sync.Pool{New: func() any { b := make([]byte, 0, 256); return &b }},
		loaned: make(map[*byte]*[]byte),
	}
}

// Zone implements rcp.Controller.
func (c *Controller) Zone() rcp.Zone { return c.inner.Zone() }

// Send implements rcp.Controller (delegates to inner).
func (c *Controller) Send(ctx context.Context, cmd *rcp.Command) (*rcp.Response, error) {
	return c.inner.Send(ctx, cmd)
}

// Subscribe implements rcp.Controller (delegates to inner).
func (c *Controller) Subscribe(ctx context.Context) (<-chan *rcp.Status, error) {
	return c.inner.Subscribe(ctx)
}

// Close implements rcp.Controller (delegates to inner).
func (c *Controller) Close() error {
	c.closed.Store(true)
	return c.inner.Close()
}

// Loan implements rcp.LoaningController. It returns a zeroed buffer of exactly
// size bytes obtained from the pool. The caller must either pass the buffer to
// SendLoaned or call rcp.Loan.Return() to release it.
func (c *Controller) Loan(size int) (*rcp.Loan, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("rcp/loan: zone %s: %w", c.Zone(), rcp.ErrClosed)
	}
	if size < 0 {
		return nil, fmt.Errorf("rcp/loan: negative size %d", size)
	}

	bp, _ := c.pool.Get().(*[]byte)
	var buf []byte
	if bp != nil && cap(*bp) >= size {
		*bp = (*bp)[:size]
		buf = *bp
	} else {
		buf = make([]byte, size)
		if bp == nil {
			tmp := buf[:0]
			bp = &tmp
		}
	}
	for i := range buf {
		buf[i] = 0
	}

	// Record the loan under the buffer's identity (its first byte's address)
	// so SendLoaned can find and recycle it. A zero-length buffer has no
	// addressable byte and is simply not tracked — Return() still frees it
	// via the closure below, it just isn't recyclable from SendLoaned.
	var key *byte
	if len(buf) > 0 {
		key = &buf[0]
		c.mu.Lock()
		c.loaned[key] = bp
		c.mu.Unlock()
	}

	release := func() {
		c.untrack(key)
		*bp = buf[:0]
		c.pool.Put(bp)
	}
	return rcp.NewLoan(buf, release), nil
}

// untrack removes key from the in-flight loan table, if present. No-op for a
// nil key (zero-length loans, which are never tracked).
func (c *Controller) untrack(key *byte) {
	if key == nil {
		return
	}
	c.mu.Lock()
	delete(c.loaned, key)
	c.mu.Unlock()
}

// SendLoaned implements rcp.LoaningController. It sends cmd using cmd.Payload
// (which must be a buffer obtained via Loan) and returns the buffer to the
// pool for reuse by a later Loan call. The caller must not access
// cmd.Payload after this call.
func (c *Controller) SendLoaned(ctx context.Context, cmd *rcp.Command) (*rcp.Response, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("rcp/loan: zone %s: %w", c.Zone(), rcp.ErrClosed)
	}
	// Send delegates to the inner controller. The inner controller copies the payload
	// (as required by REQ-CTRL-026), so we can safely return the loaned buffer now,
	// regardless of the send outcome.
	resp, err := c.inner.Send(ctx, cmd)

	if len(cmd.Payload) > 0 {
		key := &cmd.Payload[0]
		c.mu.Lock()
		bp, ok := c.loaned[key]
		if ok {
			delete(c.loaned, key)
		}
		c.mu.Unlock()
		if ok {
			*bp = (*bp)[:0]
			c.pool.Put(bp)
		}
	}

	return resp, err
}
