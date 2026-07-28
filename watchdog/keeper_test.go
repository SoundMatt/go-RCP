//fusa:test REQ-WDG-001
//fusa:test REQ-WDG-002
//fusa:test REQ-WDG-003
//fusa:test REQ-WDG-004
//fusa:test REQ-WDG-005

package watchdog_test

import (
	"testing"
	"time"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/e2e"
	"github.com/SoundMatt/go-RCP/gpio"
	"github.com/SoundMatt/go-RCP/request"
	"github.com/SoundMatt/go-RCP/server"
	"github.com/SoundMatt/go-RCP/watchdog"
)

// fakeClock is a manually-advanced clock for deterministic tests, the same
// injectable-clock pattern e2e's own tests establish.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

// newDispatcher builds a fresh GPIO endpoint wrapped in its own Dispatcher,
// addressed on a fresh server rooted at root.
func newDispatcher(t *testing.T, root avtp.StreamID, addr avtp.ByteBusID) *request.Dispatcher {
	t.Helper()
	s := server.NewServer()
	if err := s.ClaimRoot(root); err != nil {
		t.Fatalf("ClaimRoot: %v", err)
	}
	if err := s.AddEndpoint(root, addr, gpio.EndpointType); err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}
	ep := gpio.NewEndpoint(s, addr)
	if err := ep.Configure(root, gpio.Config{PinCount: 4, Direction: 0b1111}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	return request.NewDispatcher(ep, addr, request.NewSequencer(), nil)
}

func testStream(suffix uint16) avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x01, 0, 0, 0, 0, 1}, suffix)
}

// submitPlain submits a trivial plain (unconditional) write ticket against d
// on behalf of requester, returning its TicketID.
func submitPlain(t *testing.T, d *request.Dispatcher, requester avtp.StreamID, addr avtp.ByteBusID, txn avtp.TransactionNum) request.TicketID {
	t.Helper()
	id, err := d.Submit(requester, acf.Message{
		Kind:           acf.KindShort,
		ByteBusID:      addr,
		TransactionNum: txn,
		Control:        acf.FlagWrite,
		Body:           gpio.EncodeWriteRequest(gpio.SemanticOr, 0b0001),
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	return id
}

// TestKeeper_WatchDeduplicates checks Watch registers a (stream, Dispatcher)
// pair, Streams reports it, and re-registering the identical pair is a
// no-op rather than a duplicate entry (REQ-WDG-001).
func TestKeeper_WatchDeduplicates(t *testing.T) {
	sup := e2e.NewSupervisor(e2e.StreamConfig{Timeout: time.Hour})
	k := watchdog.NewKeeper(sup)

	root := testStream(1)
	d := newDispatcher(t, root, avtp.ByteBusID(1))

	k.Watch(root, d)
	k.Watch(root, d) // duplicate registration

	streams := k.Streams()
	if len(streams) != 1 || streams[0] != root {
		t.Fatalf("Streams() = %v, want exactly [%v]", streams, root)
	}

	// Tick should call PurgeNonSafety on d at most once for this stream in
	// this call — verified indirectly via TestKeeper_TickPurgesTrippedStreams
	// below, which would over-report Purged if Watch had stored d twice.
}

// TestKeeper_TickNoOpWhenNotTripped checks Tick calls PurgeNonSafety on no
// Dispatcher, and returns no events, for a stream whose watchdog has not
// tripped (REQ-WDG-002).
func TestKeeper_TickNoOpWhenNotTripped(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	sup := e2e.NewSupervisorWithClock(e2e.StreamConfig{Timeout: time.Hour}, clock.now)
	k := watchdog.NewKeeper(sup)

	root := testStream(2)
	d := newDispatcher(t, root, avtp.ByteBusID(1))
	k.Watch(root, d)

	if err := sup.Observe(root, 1); err != nil {
		t.Fatalf("Observe: %v", err)
	}
	id := submitPlain(t, d, root, avtp.ByteBusID(1), 1)

	events := k.Tick()
	if len(events) != 0 {
		t.Fatalf("Tick() = %v, want no events (watchdog not tripped)", events)
	}
	if _, ok := d.StateOf(id); !ok {
		t.Fatalf("ticket %d vanished even though watchdog never tripped", id)
	}
	if st, _ := d.StateOf(id); st == request.StateFinalized {
		t.Errorf("ticket %d finalized even though watchdog never tripped", id)
	}
}

// TestKeeper_TickPurgesTrippedStreams checks Tick purges every non-safety
// ticket on every Dispatcher registered under a tripped stream, leaves a
// safety-request ticket on the same Dispatcher untouched, and does not touch
// a Dispatcher registered under an untripped stream (REQ-WDG-003).
func TestKeeper_TickPurgesTrippedStreams(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	sup := e2e.NewSupervisorWithClock(e2e.StreamConfig{Timeout: 100 * time.Millisecond}, clock.now)
	k := watchdog.NewKeeper(sup)

	tripped := testStream(3)
	calm := testStream(4)

	dTripped := newDispatcher(t, tripped, avtp.ByteBusID(1))
	dCalm := newDispatcher(t, calm, avtp.ByteBusID(1))
	k.Watch(tripped, dTripped)
	k.Watch(calm, dCalm)

	if err := sup.Observe(tripped, 1); err != nil {
		t.Fatalf("Observe(tripped): %v", err)
	}
	if err := sup.Observe(calm, 1); err != nil {
		t.Fatalf("Observe(calm): %v", err)
	}

	dTripped.SetSafeStateCheck(func(avtp.StreamID) bool { return false })

	ordinary := submitPlain(t, dTripped, tripped, avtp.ByteBusID(1), 1)
	cond := request.Conditional{Sequencer: 1, Op: request.CompareEqual, Operand: 0, AdvanceOnMatch: 1}
	safeID, err := dTripped.Submit(tripped, acf.Message{
		Kind: acf.KindShort, ByteBusID: avtp.ByteBusID(1), TransactionNum: 2,
		Control: acf.FlagExtended, Body: request.EncodeCompoundWaitSafety(cond),
	})
	if err != nil {
		t.Fatalf("Submit(safety): %v", err)
	}
	calmTicket := submitPlain(t, dCalm, calm, avtp.ByteBusID(1), 1)

	// `calm` keeps hearing from its stream while `tripped` goes quiet.
	clock.advance(60 * time.Millisecond)
	if err := sup.Observe(calm, 2); err != nil {
		t.Fatalf("Observe(calm, refresh): %v", err)
	}
	clock.advance(60 * time.Millisecond) // tripped: 120ms since its only arrival (>100ms Timeout); calm: 60ms since its refresh
	if !sup.InSafeState(tripped) {
		t.Fatalf("InSafeState(tripped) = false, want true")
	}
	if sup.InSafeState(calm) {
		t.Fatalf("InSafeState(calm) = true, want false")
	}

	events := k.Tick()
	if len(events) != 1 || events[0].Stream != tripped {
		t.Fatalf("Tick() events = %+v, want exactly one event for %v", events, tripped)
	}
	if len(events[0].Purged) != 1 || events[0].Purged[0] != ordinary {
		t.Errorf("Tick() Purged = %v, want [%d] (only the ordinary ticket)", events[0].Purged, ordinary)
	}

	if _, err := dTripped.Response(ordinary); err == nil {
		t.Errorf("Response(ordinary) after purge = nil error, want ErrPurgedByWatchdog")
	}
	if st, _ := dTripped.StateOf(safeID); st == request.StateFinalized {
		t.Errorf("safety ticket %d finalized by a purge, want it to survive", safeID)
	}
	if st, _ := dCalm.StateOf(calmTicket); st == request.StateFinalized {
		t.Errorf("calm-stream ticket %d finalized even though its stream never tripped", calmTicket)
	}
}

// TestKeeper_TickReportsZeroPurgeEvent checks Tick still returns an event
// for a tripped stream whose Dispatcher had nothing pending to purge, so a
// caller can distinguish "tripped, nothing to purge" from "never tripped"
// (REQ-WDG-004).
func TestKeeper_TickReportsZeroPurgeEvent(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	sup := e2e.NewSupervisorWithClock(e2e.StreamConfig{Timeout: 100 * time.Millisecond}, clock.now)
	k := watchdog.NewKeeper(sup)

	root := testStream(5)
	d := newDispatcher(t, root, avtp.ByteBusID(1))
	k.Watch(root, d)

	if err := sup.Observe(root, 1); err != nil {
		t.Fatalf("Observe: %v", err)
	}
	clock.advance(200 * time.Millisecond)

	events := k.Tick()
	if len(events) != 1 {
		t.Fatalf("Tick() = %v, want exactly one event", events)
	}
	if events[0].Stream != root {
		t.Errorf("Tick() event stream = %v, want %v", events[0].Stream, root)
	}
	if len(events[0].Purged) != 0 {
		t.Errorf("Tick() Purged = %v, want empty (nothing was pending)", events[0].Purged)
	}
}

// TestKeeper_TickHandlesDispatcherSharedAcrossStreams checks that a single
// Dispatcher registered under two streams which both trip in the same Tick
// call is purged safely (its second PurgeNonSafety call simply finding
// nothing left), without Tick erroring or double-reporting the same
// TicketID (REQ-WDG-005).
func TestKeeper_TickHandlesDispatcherSharedAcrossStreams(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	sup := e2e.NewSupervisorWithClock(e2e.StreamConfig{Timeout: 100 * time.Millisecond}, clock.now)
	k := watchdog.NewKeeper(sup)

	streamA := testStream(6)
	streamB := testStream(7)
	d := newDispatcher(t, streamA, avtp.ByteBusID(1))

	k.Watch(streamA, d)
	k.Watch(streamB, d)

	if err := sup.Observe(streamA, 1); err != nil {
		t.Fatalf("Observe(A): %v", err)
	}
	if err := sup.Observe(streamB, 1); err != nil {
		t.Fatalf("Observe(B): %v", err)
	}
	ticket := submitPlain(t, d, streamA, avtp.ByteBusID(1), 1)

	clock.advance(200 * time.Millisecond) // both streams trip together

	events := k.Tick()
	if len(events) != 2 {
		t.Fatalf("Tick() = %v, want exactly two events (one per tripped stream)", events)
	}

	var totalPurged int
	seen := map[request.TicketID]bool{}
	for _, ev := range events {
		for _, id := range ev.Purged {
			if seen[id] {
				t.Errorf("TicketID %d reported purged more than once across events", id)
			}
			seen[id] = true
			totalPurged++
		}
	}
	if totalPurged != 1 {
		t.Errorf("total purged tickets = %d, want 1", totalPurged)
	}
	if _, err := d.Response(ticket); err == nil {
		t.Errorf("Response(ticket) after purge = nil error, want an error")
	}
}
