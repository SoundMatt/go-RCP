package fragment

import (
	"bytes"
	"sync"
	"time"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/e2e"
)

// sequence is one Reassembler-internal in-progress or completed reassembly.
// Callers never see this type directly.
type sequence struct {
	// Shared descriptor fields, taken from the first segment observed and
	// checked against every later one (apart from ReadSizeOrSegment, which
	// only the terminal/single segment's own value contributes to the final
	// combined Message — see headerMatches and Reassembler.Add).
	kind        acf.MessageKind
	timestamp   uint64
	baseControl acf.ControlFlags

	// finalReadSizeOrSegment is the terminal (or, for a single-segment
	// sequence, the only) segment's own ReadSizeOrSegment value — the real
	// read-size (or otherwise endpoint-meaningful) value that field carries
	// once acf.FlagMoreSegments is no longer set, not a segment number.
	finalReadSizeOrSegment uint16

	bodies  [][]byte // ascending segment order; each entry owned (copied) by this sequence
	ready   bool     // the terminal segment has been received
	updated time.Time
}

// headerMatches reports whether m's shared descriptor fields are consistent
// with s's already-buffered ones. m.ByteBusID and m.TransactionNum are not
// compared here — they are exactly the fields that select which sequence a
// caller looked up by Key in the first place.
func (s *sequence) headerMatches(m acf.Message) bool {
	return m.Kind == s.kind &&
		m.Timestamp == s.timestamp &&
		(m.Control&^acf.FlagMoreSegments) == s.baseControl
}

// messageFor builds the shared-field skeleton of key's reassembled Message
// (every field but Body).
func (s *sequence) messageFor(key Key) acf.Message {
	return acf.Message{
		Kind:              s.kind,
		ByteBusID:         key.Bus,
		TransactionNum:    key.Txn,
		Control:           s.baseControl,
		ReadSizeOrSegment: s.finalReadSizeOrSegment,
		Timestamp:         s.timestamp,
	}
}

// concatBodies returns the ascending-order concatenation of bodies as one
// fresh slice.
func concatBodies(bodies [][]byte) []byte {
	total := 0
	for _, b := range bodies {
		total += len(b)
	}
	out := make([]byte, 0, total)
	for _, b := range bodies {
		out = append(out, b...)
	}
	return out
}

// Reassembler is the receive-side half of ROADMAP.md Milestone 52: it
// accumulates acf.Message segments sharing one Key (see KeyOf) until the
// terminal one (acf.FlagMoreSegments clear) arrives, at which point Finish
// or FinishProtected returns the reassembled logical Message. It also
// accepts an ordinary, never-fragmented Message as a degenerate one-segment
// sequence that is immediately ready — every caller-facing method works the
// same way regardless of whether the traffic it sees is fragmented at all,
// so a caller (see Gateway) never has to special-case that decision itself.
//
// # This package's own reasoned reassembly policy (Guiding Principle 10)
//
// Neither ROADMAP.md nor the governing TC18 specification text spells
// out how a receiver should treat out-of-order segments, duplicate
// segments, or a stalled sequence that never completes — this package's own
// documented, reversible choices, flagged here per Guiding Principle 10:
//
//   - Segments of one sequence must arrive strictly in order (0, 1, 2, ...,
//     ending with the terminal segment). A segment whose own segment number
//     does not match the sequence's next expected index abandons the
//     sequence with ErrOutOfOrderSegment rather than buffering it for later
//     reordering — the AVTPDU layer's own per-stream sequence-number
//     bookkeeping (avtp.Header.SequenceNum, per avtp/doc.go) already exists
//     to detect loss/reordering at the transport level below this package;
//     this package chooses not to duplicate that as a second, independent
//     reordering buffer.
//   - An exact byte-for-byte repeat of the most recently accepted segment,
//     or of an already-completed sequence's terminal segment, is tolerated
//     silently (a harmless retransmission); anything else addressed to an
//     already-seen segment number is ErrDuplicateSegment.
//   - A sequence that receives no new segment within Config.Timeout is
//     purged by Sweep as abandoned — but only while still incomplete.
//     Sweep never discards a sequence that has already received its
//     terminal segment: silently destroying data that arrived completely,
//     merely because a caller has not yet called Finish/FinishProtected,
//     is a worse failure mode than the bounded memory cost of holding it
//     — a caller is expected to drain completed sequences promptly instead.
//   - Config.MaxSegments bounds memory against a sequence that never
//     terminates at all.
//
// One consequence of the wire format itself (avtp/message.go, not this
// package's own choice) rather than a policy decision above: the terminal
// segment of a sequence carries no segment number at all — once
// acf.FlagMoreSegments is clear, ReadSizeOrSegment reverts to its ordinary,
// non-fragmentation meaning (see acf.Message.SegmentNumber/ReadSize), so
// Add cannot check a terminal segment's position the way it checks every
// non-terminal one. A sender that ends a sequence early (sends a terminal
// segment where a non-terminal one was expected) is therefore
// indistinguishable, at this layer, from a sender that always intended a
// shorter sequence — both are accepted. Only a gap or reordering among
// non-terminal segments (each of which does carry an explicit, checked
// segment number) is detectable and rejected as ErrOutOfOrderSegment.
//
// This package runs no goroutine and sends nothing on the wire, the same
// posture e2e.Supervisor's watchdog takes: Sweep is a caller-driven,
// on-demand cleanup pass, not a background timer. All exported methods are
// safe for concurrent use.
type Reassembler struct {
	mu  sync.Mutex
	now func() time.Time
	cfg Config

	seqs map[Key]*sequence
}

// NewReassembler returns a Reassembler applying cfg, using time.Now as its
// clock. A non-positive cfg.Timeout is replaced with
// DefaultReassemblyTimeout; a non-positive cfg.MaxSegments is replaced with
// DefaultMaxSegments.
func NewReassembler(cfg Config) *Reassembler {
	return NewReassemblerWithClock(cfg, time.Now)
}

// NewReassemblerWithClock is like NewReassembler but accepts a custom clock
// function, used in tests to avoid real-time sleeps when exercising
// Config.Timeout — the same injectable-clock pattern
// e2e.NewSupervisorWithClock establishes.
func NewReassemblerWithClock(cfg Config, now func() time.Time) *Reassembler {
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultReassemblyTimeout
	}
	if cfg.MaxSegments <= 0 {
		cfg.MaxSegments = DefaultMaxSegments
	}
	return &Reassembler{
		now:  now,
		cfg:  cfg,
		seqs: make(map[Key]*sequence),
	}
}

// Add accepts one inbound Message addressed to/from stream, buffering it as
// part of the sequence identified by KeyOf(stream, m). It returns
// (true, nil) once m completes that sequence — whether m is an ordinary
// unfragmented message (immediately its own one-segment sequence) or the
// terminal segment of a multi-segment one — at which point a caller should
// call Finish or FinishProtected with the same Key to retrieve it. It
// returns (false, nil) for a non-terminal segment accepted into an
// in-progress sequence, and (false, err) when m cannot be accepted at all
// (see ErrOutOfOrderSegment, ErrDuplicateSegment, ErrHeaderMismatch,
// ErrSequenceComplete, ErrTooManySegments) — any error other than a
// tolerated duplicate abandons the sequence entirely, so a caller must
// treat the whole logical message as failed rather than retry just the
// rejected segment.
func (r *Reassembler) Add(stream avtp.StreamID, m acf.Message) (bool, error) {
	key := KeyOf(stream, m)
	now := r.now()
	isSeg := m.Control.Has(acf.FlagMoreSegments)

	r.mu.Lock()
	defer r.mu.Unlock()

	seq, ok := r.seqs[key]
	if !ok {
		if !isSeg {
			// An ordinary, never-fragmented message: a complete,
			// one-segment sequence from the start.
			r.seqs[key] = &sequence{
				kind:                   m.Kind,
				timestamp:              m.Timestamp,
				baseControl:            m.Control,
				finalReadSizeOrSegment: m.ReadSizeOrSegment,
				bodies:                 [][]byte{append([]byte(nil), m.Body...)},
				ready:                  true,
				updated:                now,
			}
			return true, nil
		}
		if m.ReadSizeOrSegment != 0 {
			return false, ErrOutOfOrderSegment
		}
		r.seqs[key] = &sequence{
			kind:        m.Kind,
			timestamp:   m.Timestamp,
			baseControl: m.Control &^ acf.FlagMoreSegments,
			bodies:      [][]byte{append([]byte(nil), m.Body...)},
			updated:     now,
		}
		return false, nil
	}

	if !seq.headerMatches(m) {
		delete(r.seqs, key)
		return false, ErrHeaderMismatch
	}

	if seq.ready {
		if !isSeg && bytes.Equal(seq.bodies[len(seq.bodies)-1], m.Body) && m.ReadSizeOrSegment == seq.finalReadSizeOrSegment {
			return true, nil
		}
		return false, ErrSequenceComplete
	}

	next := len(seq.bodies)
	if isSeg {
		idx := int(m.ReadSizeOrSegment)
		switch {
		case idx == next-1:
			if bytes.Equal(seq.bodies[idx], m.Body) {
				return false, nil
			}
			delete(r.seqs, key)
			return false, ErrDuplicateSegment
		case idx != next:
			delete(r.seqs, key)
			return false, ErrOutOfOrderSegment
		}
	}

	if next+1 > r.cfg.MaxSegments {
		delete(r.seqs, key)
		return false, ErrTooManySegments
	}

	seq.bodies = append(seq.bodies, append([]byte(nil), m.Body...))
	seq.updated = now
	if !isSeg {
		seq.finalReadSizeOrSegment = m.ReadSizeOrSegment
		seq.ready = true
		return true, nil
	}
	return false, nil
}

// Finish returns the reassembled Message for key — the ascending-order
// concatenation of every accepted segment's Body, with no CRC handling of
// any kind — and removes key's sequence from this Reassembler. It returns
// ErrUnknownSequence when key names no sequence this Reassembler currently
// holds, and ErrIncomplete when that sequence has not yet received its
// terminal segment.
func (r *Reassembler) Finish(key Key) (acf.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	seq, ok := r.seqs[key]
	if !ok {
		return acf.Message{}, ErrUnknownSequence
	}
	if !seq.ready {
		return acf.Message{}, ErrIncomplete
	}
	out := seq.messageFor(key)
	out.Body = concatBodies(seq.bodies)
	delete(r.seqs, key)
	return out, nil
}

// FinishProtected is Finish's CRC-aware counterpart (ROADMAP.md Milestone
// 52's third named integration point): it treats key's terminal segment's
// trailing bytes as a CRC32 safe point (0-3 zero pad bytes immediately
// followed by a Len-byte CRC32 trailer, per e2e.Protect) covering the whole
// combined message, verified via e2e.VerifyFragmented over this sequence's
// own per-segment Body slices — the exact function e2e/crc.go's own doc
// comment names as the one a fragmentation-aware reassembly path is meant
// to call. On success it returns the reassembled Message with that trailing
// field (and any pad bytes) stripped, exactly as e2e.Verify does for an
// already-whole (unfragmented) message. It returns
// ErrUnknownSequence/ErrIncomplete under the same conditions Finish does,
// e2e.ErrShortSafePoint when the terminal segment's Body is too short to
// contain a safe point, and (wrapping) e2e.ErrCRCMismatch when no candidate
// pad length's recomputed CRC matches — either way removing key's sequence
// from this Reassembler, the same fail-closed posture e2e.Guard takes.
func (r *Reassembler) FinishProtected(stream avtp.StreamID, key Key) (acf.Message, error) {
	r.mu.Lock()
	seq, ok := r.seqs[key]
	if !ok {
		r.mu.Unlock()
		return acf.Message{}, ErrUnknownSequence
	}
	if !seq.ready {
		r.mu.Unlock()
		return acf.Message{}, ErrIncomplete
	}

	bodies := make([][]byte, len(seq.bodies))
	copy(bodies, seq.bodies)
	header := seq.messageFor(key)
	delete(r.seqs, key)
	r.mu.Unlock()

	return e2e.VerifyFragmented(stream, header, bodies)
}

// Sweep purges every sequence that is still incomplete (has not yet
// received its terminal segment) and has not accepted a new segment within
// this Reassembler's configured Config.Timeout, and returns the Keys it
// purged. A completed sequence awaiting Finish/FinishProtected is never
// purged by Sweep — see the type doc comment's reasoning. A caller drives
// Sweep on whatever cadence it judges appropriate; nothing in this package
// calls it automatically.
func (r *Reassembler) Sweep() []Key {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now()
	var purged []Key
	for key, seq := range r.seqs {
		if seq.ready {
			continue
		}
		if now.Sub(seq.updated) > r.cfg.Timeout {
			delete(r.seqs, key)
			purged = append(purged, key)
		}
	}
	return purged
}

// Pending returns the number of sequences this Reassembler currently holds,
// complete or not.
func (r *Reassembler) Pending() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.seqs)
}
