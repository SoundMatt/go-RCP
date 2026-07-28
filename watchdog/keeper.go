package watchdog

import (
	"sync"

	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/crcsafe"
	"github.com/SoundMatt/go-RCP/request"
)

// PurgeEvent records one stream's watchdog-driven purge outcome from a
// single Keeper.Tick call: Purged is the concatenation, across every
// Dispatcher Watch registered for Stream, of that Dispatcher's own
// PurgeNonSafety return value. Purged is nil (not simply empty) when the
// purge cleared no tickets at all — the event itself still appears in
// Tick's returned slice, so a caller can tell "this stream's watchdog
// tripped, but there was nothing to purge" apart from "this stream's
// watchdog never tripped this call".
type PurgeEvent struct {
	Stream avtp.StreamID
	Purged []request.TicketID
}

// Keeper centralizes the crcsafe.Supervisor-driven watchdog check-and-purge
// pattern crcsafe/doc.go documents, across every (stream, Dispatcher)
// association a caller registers with Watch. All exported methods are safe
// for concurrent use.
type Keeper struct {
	sup *crcsafe.Supervisor

	mu    sync.Mutex
	watch map[avtp.StreamID][]*request.Dispatcher
}

// NewKeeper returns a Keeper whose Tick checks sup for every stream Watch
// registers. sup must not be nil.
func NewKeeper(sup *crcsafe.Supervisor) *Keeper {
	return &Keeper{sup: sup, watch: make(map[avtp.StreamID][]*request.Dispatcher)}
}

// Watch registers d as a Dispatcher whose PurgeNonSafety Tick calls whenever
// stream's watchdog trips (per sup.InSafeState(stream)). One Dispatcher may
// be registered against several streams — an endpoint several requesters
// are permitted to address — and one stream may be registered against
// several Dispatchers — several endpoints one requester addresses. Watch
// appends; it never replaces a previous registration, and registering the
// exact same (stream, d) pair more than once is a harmless no-op (Watch
// deduplicates against exactly that pair, not against d alone).
func (k *Keeper) Watch(stream avtp.StreamID, d *request.Dispatcher) {
	k.mu.Lock()
	defer k.mu.Unlock()
	for _, existing := range k.watch[stream] {
		if existing == d {
			return
		}
	}
	k.watch[stream] = append(k.watch[stream], d)
}

// Streams returns every stream this Keeper currently watches, in no
// particular order.
func (k *Keeper) Streams() []avtp.StreamID {
	k.mu.Lock()
	defer k.mu.Unlock()
	out := make([]avtp.StreamID, 0, len(k.watch))
	for s := range k.watch {
		out = append(out, s)
	}
	return out
}

// Tick checks sup.InSafeState for every stream Watch has registered and, for
// each one currently reporting true, calls PurgeNonSafety on every
// Dispatcher registered against it — returning one PurgeEvent per stream
// whose watchdog was tripped this call (see PurgeEvent's own doc comment for
// why a tripped-but-nothing-to-purge stream still produces an event). A
// Dispatcher registered under two streams that both trip in the same Tick
// call has PurgeNonSafety called once per stream, which is safe: the second
// call simply finds nothing left to purge, since PurgeNonSafety only ever
// acts on tickets that have not already reached StateFinalized.
func (k *Keeper) Tick() []PurgeEvent {
	k.mu.Lock()
	streams := make([]avtp.StreamID, 0, len(k.watch))
	dispatchers := make(map[avtp.StreamID][]*request.Dispatcher, len(k.watch))
	for s, ds := range k.watch {
		streams = append(streams, s)
		dispatchers[s] = append([]*request.Dispatcher(nil), ds...)
	}
	k.mu.Unlock()

	var events []PurgeEvent
	for _, s := range streams {
		if !k.sup.InSafeState(s) {
			continue
		}
		var purged []request.TicketID
		for _, d := range dispatchers[s] {
			purged = append(purged, d.PurgeNonSafety()...)
		}
		events = append(events, PurgeEvent{Stream: s, Purged: purged})
	}
	return events
}
