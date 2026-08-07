package e2e

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
)

// Len is the byte length of the trailing CRC32 safe-point field Protect
// appends to (and Verify strips from) a Message's Body.
const Len = 4

// crc32P4NormalPoly is TC18 §13.6's "CRC32P4" polynomial (Table 31), in its
// normal (non-reflected) form. It is a deliberately different polynomial
// from the standard IEEE 802.3 CRC-32 (0x04C11DB7 normal / 0xEDB88320
// reflected) that Go's hash/crc32 package's own IEEE table implements —
// interoperating with a real TC18-conformant peer requires this exact
// polynomial, not crc32.IEEE.
const crc32P4NormalPoly uint32 = 0xF4ACFB13

// reflect32 reverses the 32 bits of v — used below to derive
// hash/crc32.MakeTable's expected reflected-polynomial input from
// crc32P4NormalPoly's documented normal form (the same "reflected" form the
// standard library's own IEEETable/Castagnoli tables are built from for
// their own, unrelated polynomials). reflect32(crc32P4NormalPoly) is
// 0xC8DF352F — independently checkable by inspection (reverse
// 0xF4ACFB13's 32 bits) and cross-checked against c-RCP's
// RCP_E2E_CRC32_RPOLY (src/e2e.c) and cpp-RCP's
// reflect32(kCrc32PolyNormal) (include/rcp/e2e.hpp), which both already
// use this exact value for the same polynomial — see crc32p4_test.go.
func reflect32(v uint32) uint32 {
	var r uint32
	for i := 0; i < 32; i++ {
		r = (r << 1) | (v & 1)
		v >>= 1
	}
	return r
}

// crc32P4Table is the table hash/crc32.New below drives. A crc32.Table
// built from a reflected polynomial already implements a fully
// reflected-input/reflected-output CRC (refin=true/refout=true), and
// crc32.New's digest already starts from 0xFFFFFFFF and XORs the final sum
// by 0xFFFFFFFF in Sum32 — i.e. crc32.New(crc32P4Table) already matches
// every one of Table 31's parameters (poly, init=0xFFFFFFFF,
// refin=refout=true, xorout=0xFFFFFFFF) with no extra init/final-XOR
// handling needed here. This is *not* the same as the package-level
// crc32.Checksum(data, tab) helper, which starts from 0 and applies no
// final XOR — that helper would silently implement a different, wrong
// checksum despite using the same table. See crc32p4_test.go for
// cross-verification against a from-scratch, non-table bit-level
// implementation of CRC32P4 (including a known-answer vector, "CRC-32/
// AUTOSAR"'s own published check value for this identical parameter set)
// over several inputs, including empty, a single byte, and a multi-byte
// buffer.
var crc32P4Table = crc32.MakeTable(reflect32(crc32P4NormalPoly))

// Compute returns the CRC32P4 (TC18 §13.6 Table 31: polynomial 0xF4ACFB13
// non-reflected, initial value 0xFFFFFFFF, both input and output reflected,
// final XOR 0xFFFFFFFF — a deliberately different polynomial from both the
// standard IEEE 802.3 CRC-32 and the retired legacy `e2e` package's
// CRC-16/CCITT-FALSE, per ROADMAP.md Milestone 50) safe-point checksum for m
// as carried by the AVTPDU addressed to/from stream. (That legacy package —
// the pre-TC18 bespoke Zone/Command protocol's own CRC mechanism — was
// retired outright at Milestone 53, v0.66.0; this package, originally built
// under the name `crcsafe`, later took over the now-vacant `e2e` name per
// RELAY spec v1.14 §13.7.2's cross-language module-naming registry — the two
// are otherwise unrelated.) Unlike that legacy package's payload-only
// coverage, this spans:
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
	h := crc32.New(crc32P4Table)
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
	// doc comment); crc32.New's returned Hash32 is no exception.
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

// padLenFor returns the number of zero pad bytes TC18's own worked examples
// (§13.6 Figures 19/20: ACF_ABB/ACF_GBB, respectively) place between a
// safe-point-protected message's real payload and its trailing CRC32
// trailer — the same "round the payload up to a whole quadlet" rule
// acf.EncodeMessage's own pad computation already applies to an ordinary
// (non-CRC) message's Body, applied here to payloadLen alone so the result
// is identical regardless of whether a trailing CRC follows.
func padLenFor(payloadLen int) int {
	return (4 - payloadLen%4) % 4
}

// Protect returns a copy of m with realPayload (m.Body as given), 0-3 zero
// pad bytes, and a trailing Len-byte big-endian CRC32 safe point — in that
// order — assembled into Body, per TC18 §13.6 Figures 19/20's worked wire
// layout ("header, payload, pad, CRC32 trailer"; pad rounds the payload up
// to a whole quadlet and comes *before* the CRC, not after). The CRC itself
// is computed by Compute over m as given (i.e. over the unpadded original
// Body, before either the pad or the CRC field exist). Because
// len(realPayload)+padLenFor(len(realPayload)) is always a whole multiple
// of 4, and a CRC32 trailer is always exactly one quadlet, the returned
// Body is already a whole number of quadlets — so a caller that hands the
// result to acf.EncodeMessage gets pad=0 from EncodeMessage's own
// computation (its padding logic runs unmodified; it just has nothing left
// to add), and the wire byte order comes out payload-then-pad-then-CRC as
// required, not payload-then-CRC-then-pad. Protect never mutates m or
// m.Body's backing array.
func Protect(stream avtp.StreamID, m acf.Message) acf.Message {
	crc := Compute(stream, m)
	pad := padLenFor(len(m.Body))
	out := m
	out.Body = make([]byte, len(m.Body)+pad+Len)
	n := copy(out.Body, m.Body)
	n += pad // the pad region is already zero-valued, from make([]byte, ...)
	binary.BigEndian.PutUint32(out.Body[n:], crc)
	return out
}

// recoverProtected locates Protect's trailing safe point within
// withoutCRC+got (withoutCRC being m.Body with the trailing Len-byte CRC32
// field already split off into got): withoutCRC is realPayload with 0-3
// zero pad bytes appended, and — because that pad region is
// indistinguishable, by byte value alone, from real payload bytes that
// happen to be zero — there is no way to recover realPayload's exact
// length without the CRC itself. This tries every pad length a conformant
// Protect could have produced (0-3, per padLenFor), shortest pad first,
// skipping any candidate whose supposed pad bytes are not all zero, and
// calls tryCRC once per remaining candidate; the first candidate whose
// tryCRC result equals got is accepted (a genuine Protect'd message has
// exactly one such candidate; two different candidates both matching by
// accident would require an unintended 32-bit CRC collision, probability
// on the order of 2^-32 per wrong candidate). It returns the matching
// candidate and true, or nil and false if none matches.
func recoverProtected(withoutCRC []byte, got uint32, tryCRC func(candidate []byte) uint32) ([]byte, bool) {
	maxPad := 3
	if maxPad > len(withoutCRC) {
		maxPad = len(withoutCRC)
	}
	for pad := 0; pad <= maxPad; pad++ {
		realLen := len(withoutCRC) - pad
		if pad > 0 {
			allZero := true
			for _, b := range withoutCRC[realLen:] {
				if b != 0 {
					allZero = false
					break
				}
			}
			if !allZero {
				continue
			}
		}
		candidate := withoutCRC[:realLen]
		if tryCRC(candidate) == got {
			return candidate, true
		}
	}
	return nil, false
}

// Verify strips and validates the trailing CRC32 safe point Protect
// appended to m.Body (payload, then 0-3 zero pad bytes, then the CRC32
// trailer — see Protect), returning a copy of m with Body restored to its
// original (pre-Protect) contents on success. It reports ErrShortSafePoint
// when m.Body is too short to contain a safe point at all, and
// ErrCRCMismatch (wrapped with the observed value) when no candidate pad
// length's recomputed CRC matches the trailing field — the caller
// (typically Guard) is expected to skip execution and surface this as a
// dedicated, distinguishable error rather than attempt any recovery, per
// Milestone 50's explicit failure-handling rule. Verify never mutates m or
// m.Body's backing array.
func Verify(stream avtp.StreamID, m acf.Message) (acf.Message, error) {
	if len(m.Body) < Len {
		return acf.Message{}, ErrShortSafePoint
	}
	withoutCRC := m.Body[:len(m.Body)-Len]
	got := binary.BigEndian.Uint32(m.Body[len(m.Body)-Len:])

	real, ok := recoverProtected(withoutCRC, got, func(candidate []byte) uint32 {
		inner := m
		inner.Body = candidate
		return Compute(stream, inner)
	})
	if !ok {
		return acf.Message{}, fmt.Errorf("%w: got 0x%08x", ErrCRCMismatch, got)
	}
	inner := m
	inner.Body = append([]byte(nil), real...)
	return inner, nil
}

// VerifyFragmented is ComputeFragmented's Verify counterpart for a message
// reassembled from multiple physically-transmitted segments (see
// fragment.Reassembler.FinishProtected, its intended caller): only the
// final element of bodies carries Protect's trailing safe point (0-3 zero
// pad bytes immediately followed by the Len-byte CRC32 trailer, per
// Protect); every earlier element is untouched. header supplies the shared
// addressing/timestamp/descriptor fields exactly as ComputeFragmented
// itself documents. It reports ErrShortSafePoint when bodies is empty or
// its final element is too short to contain a safe point at all, and
// (wrapped) ErrCRCMismatch when no candidate pad length's recomputed CRC
// matches the received trailer.
func VerifyFragmented(stream avtp.StreamID, header acf.Message, bodies [][]byte) (acf.Message, error) {
	if len(bodies) == 0 {
		return acf.Message{}, ErrShortSafePoint
	}
	last := bodies[len(bodies)-1]
	if len(last) < Len {
		return acf.Message{}, ErrShortSafePoint
	}
	withoutCRC := last[:len(last)-Len]
	got := binary.BigEndian.Uint32(last[len(last)-Len:])

	real, ok := recoverProtected(withoutCRC, got, func(candidate []byte) uint32 {
		trimmed := make([][]byte, len(bodies))
		copy(trimmed, bodies)
		trimmed[len(trimmed)-1] = candidate
		return ComputeFragmented(stream, header, trimmed)
	})
	if !ok {
		return acf.Message{}, fmt.Errorf("%w: got 0x%08x", ErrCRCMismatch, got)
	}

	trimmed := make([][]byte, len(bodies))
	copy(trimmed, bodies)
	trimmed[len(trimmed)-1] = real

	total := 0
	for _, b := range trimmed {
		total += len(b)
	}
	combined := make([]byte, 0, total)
	for _, b := range trimmed {
		combined = append(combined, b...)
	}

	out := header
	out.Body = combined
	return out, nil
}
