// Package loan implements a Controller wrapper extending udp.Controller
// with zero-copy payload loaning via a sync.Pool, for the OPEN Alliance
// TC18 Remote Control Protocol (RCP).
//
// This is ROADMAP.md Milestone 54 (v0.67.0)'s ADAPT-flagged rebuild: per
// Phase 17's disposition table, "the sync.Pool-backed zero-copy loaning
// pattern is not protocol-specific; only the pooled type changes." The
// retired package wrapped the root module's rcp.Controller interface and
// implemented rcp.LoaningController — both root-module contracts Phase 17's
// disposition table explicitly leaves alone ("Root-module files ... are
// not in this table ... Their replacement is Phases 13-16 (the types
// themselves) plus Phase 18"). At the time of this rebuild udp.Controller
// did not (and, until Phase 18 defined a TC18-shaped equivalent of
// rcp.Controller, could not meaningfully) implement that old interface, so
// this rebuild wraps *udp.Controller concretely instead of an interface,
// and exposes RequestLoaned/Loan in place of the old SendLoaned/Loan pair.
// The pooled buffer type itself is unchanged: this package still hands out
// and recycles *rcp.Loan values (root package's own already-generic
// Payload + release-func struct — see rcp.NewLoan's doc comment, "intended
// for use by loaning-pool implementations in external packages"), the same
// "only the pooled type changes" continuity the disposition table calls
// for, just no longer wrapped by the retired Command/Response-shaped
// interface.
//
// Phase 18's cutover (Milestone 59, v1.0.0) has since defined that
// TC18-shaped rcp.Controller — StreamID/Request/Close, the same shape
// *udp.Controller and mock.Client already presented — and this package's
// own Controller happens to satisfy it too (see the compile-time assertion
// below), a side effect of already matching *udp.Controller's shape rather
// than a change made for this milestone.
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
	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/udp"
)

// Controller wraps a *udp.Controller, adding zero-copy request-body loaning
// via a sync.Pool.
type Controller struct {
	inner  *udp.Controller
	pool   sync.Pool
	closed atomic.Bool

	// loaned tracks buffers currently on loan, keyed by the address of
	// their first byte, so RequestLoaned — which is handed only a bare
	// []byte body, with no reference back to the pool slot — can still
	// look up and recycle the right *[]byte. Guarded by mu since
	// Loan/RequestLoaned/Loan.Return may be called concurrently.
	mu     sync.Mutex
	loaned map[*byte]*[]byte
}

// Controller satisfies rcp.Controller (see this file's own package doc
// comment).
var _ rcp.Controller = (*Controller)(nil)

// New wraps inner as a loaning Controller.
func New(inner *udp.Controller) *Controller {
	return &Controller{
		inner:  inner,
		pool:   sync.Pool{New: func() any { b := make([]byte, 0, 256); return &b }},
		loaned: make(map[*byte]*[]byte),
	}
}

// StreamID returns the inner Controller's own avtp.StreamID identity.
func (c *Controller) StreamID() avtp.StreamID { return c.inner.StreamID() }

// Request delegates to the inner Controller (no loaning).
func (c *Controller) Request(ctx context.Context, addr avtp.ByteBusID, control acf.ControlFlags, body []byte) (acf.Message, error) {
	return c.inner.Request(ctx, addr, control, body)
}

// Close marks this Controller closed and closes the inner Controller.
func (c *Controller) Close() error {
	c.closed.Store(true)
	return c.inner.Close()
}

// Loan returns a zeroed buffer of exactly size bytes obtained from the
// pool. The caller must either pass the buffer to RequestLoaned or call
// (*rcp.Loan).Return() to release it.
func (c *Controller) Loan(size int) (*rcp.Loan, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("rcp/loan: stream %s: %w", c.StreamID(), udp.ErrClosed)
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

	// Record the loan under the buffer's identity (its first byte's
	// address) so RequestLoaned can find and recycle it. A zero-length
	// buffer has no addressable byte and is simply not tracked — Return()
	// still frees it via the closure below, it just isn't recyclable from
	// RequestLoaned.
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

// untrack removes key from the in-flight loan table, if present. No-op for
// a nil key (zero-length loans, which are never tracked).
func (c *Controller) untrack(key *byte) {
	if key == nil {
		return
	}
	c.mu.Lock()
	delete(c.loaned, key)
	c.mu.Unlock()
}

// RequestLoaned sends a request to addr whose body is a buffer obtained
// via Loan, and returns the buffer to the pool for reuse once the
// underlying Controller.Request call returns (which, per udp.Controller's
// own contract, has by then already copied the body onto the wire — the
// same "inner controller copies the payload" precondition the retired
// package's own SendLoaned relied on). The caller must not access body
// after this call.
func (c *Controller) RequestLoaned(ctx context.Context, addr avtp.ByteBusID, control acf.ControlFlags, body []byte) (acf.Message, error) {
	if c.closed.Load() {
		return acf.Message{}, fmt.Errorf("rcp/loan: stream %s: %w", c.StreamID(), udp.ErrClosed)
	}
	resp, err := c.inner.Request(ctx, addr, control, body)

	if len(body) > 0 {
		key := &body[0]
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
