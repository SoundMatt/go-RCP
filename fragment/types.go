package fragment

import (
	"time"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
)

// Key identifies one logical, potentially multi-segment RCP message: the
// same addressing/correlation fields e2e.Compute already treats as a
// message's own identity (see acf.Message.ByteBusID and
// acf.Message.TransactionNum), scoped to the enclosing AVTPDU's StreamID
// exactly as avtp/doc.go describes both fields being scoped. Two segments
// with equal Key are always part of the same logical message; two with
// unequal Key never are.
type Key struct {
	Stream avtp.StreamID
	Bus    avtp.ByteBusID
	Txn    avtp.TransactionNum
}

// KeyOf derives m's reassembly Key relative to the enclosing AVTPDU's
// StreamID.
func KeyOf(stream avtp.StreamID, m acf.Message) Key {
	return Key{Stream: stream, Bus: m.ByteBusID, Txn: m.TransactionNum}
}

// DefaultMaxSegmentBody is Split's default per-segment body-length cap when
// a caller does not supply its own. It is deliberately smaller than the
// AVTPDU wire format's own ~2047-byte hard ceiling (avtp.Header.DataLength's
// 11-bit field, minus the RCP message descriptor header) — this package's
// own reasoned, conservative choice sized to typical Ethernet-class link
// MTUs (1500 bytes) rather than the protocol's theoretical maximum, per
// ROADMAP.md Milestone 52's own "cannot fit ... on realistic MTUs"
// rationale. A caller with a smaller or larger link budget passes its own
// value to Split/Config instead.
const DefaultMaxSegmentBody = 1400

// DefaultReassemblyTimeout is Config's default Timeout when a caller
// supplies a non-positive value. This is this package's own reasoned
// choice, not a value transcribed from the TC18 specification text, which
// (per ROADMAP.md Milestone 52) does not spell out a reassembly-timeout
// policy at all.
const DefaultReassemblyTimeout = 5 * time.Second

// DefaultMaxSegments bounds the number of segments a single Reassembler
// sequence may accumulate before it is abandoned as ErrTooManySegments.
// This guards a receiver's memory against a runaway or malicious sender
// that never sends a terminal segment; it is this package's own reasoned
// choice, not a specification-mandated limit. It comfortably covers every
// concrete consumer ROADMAP.md Milestone 52 names (a ~2 KB CAN XL payload
// split at DefaultMaxSegmentBody is under 2 segments; a large discovery
// register-map read in the tens of kilobytes is still a small fraction of
// this bound).
const DefaultMaxSegments = 4096

// Config configures a Reassembler's segment-count and staleness bounds.
type Config struct {
	// Timeout is the maximum tolerated gap between successive segments of
	// one in-progress sequence before Sweep purges it as abandoned. A
	// non-positive value passed to NewReassembler/NewReassemblerWithClock is
	// replaced with DefaultReassemblyTimeout.
	Timeout time.Duration

	// MaxSegments bounds how many segments one sequence may accumulate. A
	// non-positive value passed to NewReassembler/NewReassemblerWithClock is
	// replaced with DefaultMaxSegments.
	MaxSegments int
}
