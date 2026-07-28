//fusa:test REQ-PQ-001
//fusa:test REQ-PQ-002
//fusa:test REQ-PQ-003
//fusa:test REQ-PQ-004
//fusa:test REQ-PQ-005
//fusa:test REQ-PQ-006

package prioqueue_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/prioqueue"
	"github.com/SoundMatt/go-RCP/request"
)

func testStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x05, 0, 0, 0, 0, 1}, 1)
}

// TestQueue_PushRejectsInvalidKind checks Push rejects a Kind that is
// neither request.KindPlain nor Kind.Valid (REQ-PQ-001).
func TestQueue_PushRejectsInvalidKind(t *testing.T) {
	q := prioqueue.NewQueue()
	stream := testStream()

	if err := q.Push(request.KindPlain, stream, acf.Message{}); err != nil {
		t.Errorf("Push(KindPlain) = %v, want nil (explicitly accepted, see doc)", err)
	}
	if err := q.Push(request.KindCompound, stream, acf.Message{}); err != nil {
		t.Errorf("Push(KindCompound) = %v, want nil", err)
	}

	invalid := request.Kind(200) // well past kindCount, and not the safety-tag bit either
	if err := q.Push(invalid, stream, acf.Message{}); !errors.Is(err, prioqueue.ErrInvalidKind) {
		t.Errorf("Push(invalid) = %v, want ErrInvalidKind", err)
	}
	if q.Len() != 2 {
		t.Errorf("Len() = %d, want 2 (the invalid Push must not have enqueued anything)", q.Len())
	}
}

// TestQueue_PopOrdersByKindPriority checks Pop always returns the pending
// Item with the lowest request.Kind.Priority first, across every non-safety
// Kind the request package defines (REQ-PQ-002).
func TestQueue_PopOrdersByKindPriority(t *testing.T) {
	q := prioqueue.NewQueue()
	stream := testStream()

	// Enqueued deliberately out of priority order.
	kinds := []request.Kind{
		request.KindPlain,
		request.KindCompound,
		request.KindCompoundWait,
		request.KindTimed,
		request.KindTriggered,
		request.KindChained,
		request.KindCancelAll,
	}
	for _, k := range kinds {
		if err := q.Push(k, stream, acf.Message{}); err != nil {
			t.Fatalf("Push(%v): %v", k, err)
		}
	}

	wantOrder := []request.Kind{
		request.KindCancelAll,
		request.KindChained,
		request.KindTriggered,
		request.KindTimed,
		request.KindCompoundWait,
		request.KindCompound,
		request.KindPlain,
	}
	for i, want := range wantOrder {
		got, ok := q.Pop()
		if !ok {
			t.Fatalf("Pop() #%d: queue unexpectedly empty", i)
		}
		if got.Kind != want {
			t.Errorf("Pop() #%d = %v, want %v", i, got.Kind, want)
		}
	}
	if _, ok := q.Pop(); ok {
		t.Errorf("Pop() after draining = ok, want empty")
	}
}

// TestQueue_FIFOWithinEqualPriority checks Items at equal
// request.Kind.Priority are released in insertion order (REQ-PQ-003).
func TestQueue_FIFOWithinEqualPriority(t *testing.T) {
	q := prioqueue.NewQueue()
	stream := testStream()

	for txn := avtp.TransactionNum(1); txn <= 3; txn++ {
		msg := acf.Message{TransactionNum: txn}
		if err := q.Push(request.KindCompound, stream, msg); err != nil {
			t.Fatalf("Push(%d): %v", txn, err)
		}
	}

	for want := avtp.TransactionNum(1); want <= 3; want++ {
		got, ok := q.Pop()
		if !ok || got.Message.TransactionNum != want {
			t.Errorf("Pop() = (txn=%d, ok=%v), want (txn=%d, ok=true)", got.Message.TransactionNum, ok, want)
		}
	}
}

// TestQueue_SafetyVariantMatchesBasePriority checks a safety-request Kind
// variant (IsSafety) ranks identically to — and interleaves FIFO with — its
// Base kind, mirroring Kind.Priority's own guarantee (REQ-PQ-004).
func TestQueue_SafetyVariantMatchesBasePriority(t *testing.T) {
	q := prioqueue.NewQueue()
	stream := testStream()

	if err := q.Push(request.KindCompound, stream, acf.Message{TransactionNum: 1}); err != nil {
		t.Fatalf("Push(base): %v", err)
	}
	if err := q.Push(request.KindCompoundSafety, stream, acf.Message{TransactionNum: 2}); err != nil {
		t.Fatalf("Push(safety): %v", err)
	}
	// Both should outrank Plain and be released in the FIFO order they were
	// pushed, since Priority() is identical for a Kind and its safety
	// variant.
	if err := q.Push(request.KindPlain, stream, acf.Message{TransactionNum: 3}); err != nil {
		t.Fatalf("Push(plain): %v", err)
	}

	first, ok := q.Pop()
	if !ok || first.Message.TransactionNum != 1 {
		t.Fatalf("Pop() #1 = %+v, want txn=1", first)
	}
	second, ok := q.Pop()
	if !ok || second.Message.TransactionNum != 2 {
		t.Fatalf("Pop() #2 = %+v, want txn=2", second)
	}
	third, ok := q.Pop()
	if !ok || third.Message.TransactionNum != 3 {
		t.Fatalf("Pop() #3 = %+v, want txn=3 (Plain last)", third)
	}
}

// TestQueue_PopAllDrainsInPriorityOrder checks PopAll drains every pending
// Item in the same priority order Pop would, leaving the queue empty
// (REQ-PQ-005).
func TestQueue_PopAllDrainsInPriorityOrder(t *testing.T) {
	q := prioqueue.NewQueue()
	stream := testStream()

	if got := q.PopAll(); got != nil {
		t.Fatalf("PopAll() on empty queue = %v, want nil", got)
	}

	_ = q.Push(request.KindPlain, stream, acf.Message{TransactionNum: 1})
	_ = q.Push(request.KindCancelAll, stream, acf.Message{TransactionNum: 2})
	_ = q.Push(request.KindTimed, stream, acf.Message{TransactionNum: 3})

	items := q.PopAll()
	if len(items) != 3 {
		t.Fatalf("PopAll() len = %d, want 3", len(items))
	}
	wantTxns := []avtp.TransactionNum{2, 3, 1} // CancelAll, Timed, Plain
	for i, want := range wantTxns {
		if items[i].Message.TransactionNum != want {
			t.Errorf("PopAll()[%d].Message.TransactionNum = %d, want %d", i, items[i].Message.TransactionNum, want)
		}
	}
	if q.Len() != 0 {
		t.Errorf("Len() after PopAll = %d, want 0", q.Len())
	}
}

// TestQueue_ConcurrentUse checks Queue's exported methods are safe to call
// concurrently (REQ-PQ-006).
func TestQueue_ConcurrentUse(t *testing.T) {
	q := prioqueue.NewQueue()
	stream := testStream()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = q.Push(request.KindPlain, stream, acf.Message{TransactionNum: avtp.TransactionNum(i)})
		}(i)
	}
	wg.Wait()
	if q.Len() != 50 {
		t.Fatalf("Len() after concurrent pushes = %d, want 50", q.Len())
	}

	var drained int
	var mu sync.Mutex
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, ok := q.Pop(); ok {
				mu.Lock()
				drained++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if drained != 50 {
		t.Errorf("drained = %d, want 50", drained)
	}
	if q.Len() != 0 {
		t.Errorf("Len() after concurrent pops = %d, want 0", q.Len())
	}
}
