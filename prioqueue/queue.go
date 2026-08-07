package prioqueue

import (
	"container/heap"
	"sync"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/request"
)

// Item is one request queued for later release, carrying enough for a
// caller to submit it to a request.Dispatcher (Submit or Dispatch) once
// popped: which stream is requesting (Requester) and the already-encoded
// acf.Message (Message), envelope included if Kind is anything other than
// request.KindPlain. Kind is carried separately (rather than re-derived
// from Message) purely so Queue never needs to decode Message's body itself
// to rank it.
type Item struct {
	Kind      request.Kind
	Requester avtp.StreamID
	Message   acf.Message

	seq uint64
}

// Queue is a client-side, single-outstanding-request-at-a-time dispatch
// queue ordering pending Items purely by request.Kind.Priority — the
// specification's own fixed cross-type execution-priority ordering — rather
// than the old prioqueue package's client-assigned
// PriorityCritical/High/Normal enum, which the new protocol has no
// equivalent of at all (see doc.go). Items at equal priority are released
// FIFO by insertion order. All exported methods are safe for concurrent
// use.
type Queue struct {
	mu  sync.Mutex
	h   itemHeap
	seq uint64
}

// NewQueue returns an empty Queue.
func NewQueue() *Queue {
	q := &Queue{}
	heap.Init(&q.h)
	return q
}

// Push enqueues an Item requesting kind against requester carrying msg,
// returning ErrInvalidKind if kind is not one of request's recognized
// values. request.KindPlain is accepted here even though it is never itself
// an encoded envelope byte on the wire (see request.Kind's own doc
// comment) — a plain, unconditional request still has a well-defined
// priority rank (request.Kind.Priority), and this queue's job is ordering
// pending work, not wire validation.
func (q *Queue) Push(kind request.Kind, requester avtp.StreamID, msg acf.Message) error {
	if !validKind(kind) {
		return ErrInvalidKind
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.seq++
	heap.Push(&q.h, &Item{Kind: kind, Requester: requester, Message: msg, seq: q.seq})
	return nil
}

// Pop removes and returns the highest-priority pending Item (lowest
// request.Kind.Priority, FIFO within a tie), or reports ok=false if the
// queue is currently empty.
func (q *Queue) Pop() (item Item, ok bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.h.Len() == 0 {
		return Item{}, false
	}
	it, _ := heap.Pop(&q.h).(*Item)
	return *it, true
}

// PopAll drains every pending Item in priority order (highest priority
// first), for a caller that wants to release everything currently queued in
// one pass — e.g. once per request.Dispatcher.Pump cycle. It returns nil,
// not an empty non-nil slice, when the queue was already empty.
func (q *Queue) PopAll() []Item {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.h.Len() == 0 {
		return nil
	}
	out := make([]Item, 0, q.h.Len())
	for q.h.Len() > 0 {
		it, _ := heap.Pop(&q.h).(*Item)
		out = append(out, *it)
	}
	return out
}

// Len returns the number of Items currently queued.
func (q *Queue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.h.Len()
}

// validKind reports whether k is an acceptable Push argument: either
// request.KindPlain (see Push's own doc comment for why that is accepted
// despite Kind.Valid excluding it) or any other Kind for which Kind.Valid
// reports true.
func validKind(k request.Kind) bool {
	return k == request.KindPlain || k.Valid()
}

// itemHeap implements heap.Interface, ordering by request.Kind.Priority()
// (lower rank runs first) then by insertion order (seq) for a stable FIFO
// tie-break.
type itemHeap []*Item

func (h itemHeap) Len() int { return len(h) }

func (h itemHeap) Less(i, j int) bool {
	pi, pj := h[i].Kind.Priority(), h[j].Kind.Priority()
	if pi != pj {
		return pi < pj
	}
	return h[i].seq < h[j].seq
}

func (h itemHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *itemHeap) Push(x any) {
	it, ok := x.(*Item)
	if !ok {
		panic("prioqueue: Push received non-*Item")
	}
	*h = append(*h, it)
}

func (h *itemHeap) Pop() any {
	old := *h
	n := len(old)
	it := old[n-1]
	old[n-1] = nil
	*h = old[:n-1]
	return it
}
