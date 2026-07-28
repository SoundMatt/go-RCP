package e2e

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
)

// Len is the byte length of the trailing CRC32 safe-point field Protect
// appends to (and Verify strips from) a Message's Body.
const Len = 4

// Compute returns the CRC32 (IEEE polynomial 0xEDB88320, the standard
// library's crc32.IEEE — a deliberately different polynomial from the
// retired legacy `e2e` package's CRC-16/CCITT-FALSE, per ROADMAP.md
// Milestone 50) safe-point checksum for m as carried by the AVTPDU addressed
// to/from stream. (That legacy package — the pre-TC18 bespoke Zone/Command
// protocol's own CRC mechanism — was retired outright at Milestone 53,
// v0.66.0; this package, originally built under the name `crcsafe`, later
// took over the now-vacant `e2e` name per RELAY spec v1.14 §13.7.2's
// cross-language module-naming registry — the two are otherwise unrelated.)
// Unlike that legacy package's payload-only coverage, this spans:
//
//   - stream (the enclosing AVTPDU's addressing — the frame-level field the
//     old scheme never covered at all, since the legacy package operated
//     purely on the old bespoke Command.Payload);
//   - m.ByteBusID and m.TransactionNum (the message's own addressing/
//     correlation fields);
//   - m.Timestamp (always included, even when m.Kind is KindShort and the
//     field is consequently its zero value — see the "fragmentation
//     interaction" note in doc.go for why this function does not try to
//     omit fields based on which message variant it's covering); and
//   - every remaining field of m — Kind, Control, ReadSizeOrSegment, and
//     Body — i.e. "the whole message", not just its payload.
//
// Compute never mutates m.
func Compute(stream avtp.StreamID, m acf.Message) uint32 {
	h := crc32.NewIEEE()
	writeCovered(h, stream, m)
	return h.Sum32()
}

// writeCovered writes Compute's exact covered-field byte layout to w, in a
// fixed field order so two calls with equal (stream, m) always hash
// identical bytes: stream, then Kind, ByteBusID, TransactionNum, Control,
// ReadSizeOrSegment, Timestamp, and finally Body.
func writeCovered(w io.Writer, stream avtp.StreamID, m acf.Message) {
	var hdr [8 + 1 + 1 + 2 + 1 + 2 + 8]byte
	n := copy(hdr[:], stream[:])
	hdr[n] = byte(m.Kind)
	n++
	hdr[n] = byte(m.ByteBusID)
	n++
	binary.BigEndian.PutUint16(hdr[n:], uint16(m.TransactionNum))
	n += 2
	hdr[n] = byte(m.Control)
	n++
	binary.BigEndian.PutUint16(hdr[n:], m.ReadSizeOrSegment)
	n += 2
	binary.BigEndian.PutUint64(hdr[n:], m.Timestamp)
	n += 8
	// hash.Hash's Write never returns an error (see the hash package's own
	// doc comment); crc32.NewIEEE's returned Hash32 is no exception.
	_, _ = w.Write(hdr[:n])
	_, _ = w.Write(m.Body)
}

// ComputeFragmented is Compute's forward-compatible counterpart for a
// logical message reassembled from multiple physically-transmitted AVTPDU
// segments (ROADMAP.md Milestone 52, Fragmentation — not yet implemented by
// this repo; see doc.go). It honors the rule Milestone 50's own text calls
// out explicitly even though fragmentation itself lands later: only the
// final segment carries a CRC, and that CRC is computed over all segments
// combined, not over the final segment alone. header supplies the shared
// addressing/timestamp/descriptor fields (taken from the reassembled
// message as a whole — the field values every segment of one fragmented
// message shares, per this repo's own reasoned design once Milestone 52
// defines the actual segment framing); bodies is each segment's own Body
// slice in ascending segment-number order, concatenated here rather than by
// the caller so this function — not call-site code repeated at every future
// fragmentation-aware endpoint — owns the "combined" definition.
//
// ComputeFragmented(stream, header, [][]byte{combined}) always equals
// Compute(stream, header-with-Body-combined) for a single-segment
// (unfragmented) message, by construction — see crc_test.go.
func ComputeFragmented(stream avtp.StreamID, header acf.Message, bodies [][]byte) uint32 {
	total := 0
	for _, b := range bodies {
		total += len(b)
	}
	combined := make([]byte, 0, total)
	for _, b := range bodies {
		combined = append(combined, b...)
	}
	reassembled := header
	reassembled.Body = combined
	return Compute(stream, reassembled)
}

// Protect returns a copy of m with a trailing Len-byte big-endian CRC32
// safe point appended to Body, computed by Compute over m as given (i.e.
// before that trailing field exists) — the explicit per-endpoint opt-in
// wire encoding ROADMAP.md Milestone 50 calls for. Protect never mutates m
// or m.Body's backing array.
func Protect(stream avtp.StreamID, m acf.Message) acf.Message {
	crc := Compute(stream, m)
	out := m
	out.Body = make([]byte, len(m.Body)+Len)
	copy(out.Body, m.Body)
	binary.BigEndian.PutUint32(out.Body[len(m.Body):], crc)
	return out
}

// Verify strips and validates the trailing CRC32 safe point Protect
// appended to m.Body, returning a copy of m with Body restored to its
// original (pre-Protect) contents on success. It reports ErrShortSafePoint
// when m.Body is too short to contain a safe point at all, and
// ErrCRCMismatch (wrapped with the observed/expected values) when the
// trailing field does not match the CRC recomputed over the stripped
// message — the caller (typically Guard) is expected to skip execution and
// surface this as a dedicated, distinguishable error rather than attempt
// any recovery, per Milestone 50's explicit failure-handling rule. Verify
// never mutates m or m.Body's backing array.
func Verify(stream avtp.StreamID, m acf.Message) (acf.Message, error) {
	if len(m.Body) < Len {
		return acf.Message{}, ErrShortSafePoint
	}
	n := len(m.Body) - Len
	inner := m
	inner.Body = append([]byte(nil), m.Body[:n]...)
	got := binary.BigEndian.Uint32(m.Body[n:])
	want := Compute(stream, inner)
	if got != want {
		return acf.Message{}, fmt.Errorf("%w: got 0x%08x, want 0x%08x", ErrCRCMismatch, got, want)
	}
	return inner, nil
}
