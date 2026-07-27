package loan

// White-box test (package loan, not loan_test): it needs direct access to the
// unexported loaned/pool fields to deterministically observe that SendLoaned
// actually recycles a buffer, without depending on sync.Pool's GC- and
// scheduler-sensitive caching. (A Put-then-Get identity check through the
// public API is flaky under the race detector even single-goroutine/single-P,
// since a Put/Get pair is only guaranteed to observe the same object while
// uninterrupted by an intervening GC cycle — sync.Pool gives no such
// guarantee across arbitrary code in between. Asserting on c.loaned instead
// is deterministic: in SendLoaned, the bookkeeping delete and the pool.Put
// call are unconditionally coupled in the same branch, so observing the
// bookkeeping entry disappear is a reliable proxy for the buffer having been
// handed back to the pool.)

import (
	"context"
	"testing"

	rcp "github.com/SoundMatt/go-RCP"
	mocktransport "github.com/SoundMatt/go-RCP/mock"
)

// TestController_SendLoaned_RecyclesBufferToPool verifies that SendLoaned
// actually returns the loaned buffer to c.pool for reuse, not just delegating
// to the inner Controller and dropping it to the GC.
func TestController_SendLoaned_RecyclesBufferToPool(t *testing.T) {
	inner := mocktransport.NewController(rcp.ZoneFrontLeft, nil)
	defer func() { _ = inner.Close() }()
	c := New(inner)

	l, err := c.Loan(32)
	if err != nil {
		t.Fatalf("Loan: %v", err)
	}
	if n := len(c.loaned); n != 1 {
		t.Fatalf("len(c.loaned) after Loan = %d, want 1 (loan not tracked)", n)
	}

	cmd := &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdSet, Payload: l.Payload}
	if _, err := c.SendLoaned(context.Background(), cmd); err != nil {
		t.Fatalf("SendLoaned: %v", err)
	}

	// The loan bookkeeping used to look up the pool slot must be cleared once
	// the buffer is recycled (otherwise it leaks and would also risk a
	// double-Put on a future SendLoaned call with a coincidentally-reused
	// address). Since the delete and c.pool.Put(bp) are unconditionally
	// coupled in SendLoaned, this also proves the buffer was recycled.
	if n := len(c.loaned); n != 0 {
		t.Errorf("len(c.loaned) after SendLoaned = %d, want 0 (SendLoaned did not recycle the buffer to the pool)", n)
	}
}
