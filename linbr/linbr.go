// Package linbr provides a commander-side LIN frame layer — protected
// identifier parity, classic/enhanced checksum, and schedule-table slot
// sequencing — over the native LIN endpoint type, for the OPEN Alliance
// TC18 Remote Control Protocol (RCP), as described by the "OPEN Alliance
// TC18 Remote Control Protocol Specification v0.5.1_RC".
//
// This is ROADMAP.md Milestone 56 (v0.69.0)'s ADAPT-flagged rebuild: per
// Phase 17's disposition table, this package narrows the same way canbr
// does — LIN is reachable through a native RCP endpoint type now (the lin
// package, ROADMAP.md Milestone 51) — except the native lin.Endpoint is raw
// byte pass-through only, by Milestone 51's own explicit instruction: no
// protected-identifier parity, no classic/enhanced checksum, no
// schedule-table scheduling (see lin/doc.go's "Scope" section). Whatever
// frame-level logic the retired package used to own, this package must
// still own entirely itself — it does not shrink into a thin ergonomics
// wrapper the way canbr does, because there is no native layer underneath
// it that already speaks LIN framing.
//
// Frame is this package's own domain type — a LIN commander frame (ID, data,
// checksum kind) — encoded to and decoded from the opaque byte string a
// lin.Endpoint transfer request/response body carries verbatim (see
// lin.EncodeTransferRequest/lin.DecodeTransferResponse). ScheduleTable
// tracks a commander's slot sequence; it does not itself drive a timer —
// like request.Dispatcher.Pump's own caller-supplied clock, a caller
// integrates ScheduleTable.Next with its own timing source.
package linbr

//fusa:req REQ-LIN-001
//fusa:req REQ-LIN-002
//fusa:req REQ-LIN-003
//fusa:req REQ-LIN-004
//fusa:req REQ-LIN-005
//fusa:req REQ-LIN-006
//fusa:req REQ-LIN-007
//fusa:req REQ-LIN-008

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/lin"
	"github.com/SoundMatt/go-RCP/udp"
)

// ErrChecksumMismatch is returned when a decoded frame's trailing checksum
// byte does not match the checksum computed over its PID/data.
var ErrChecksumMismatch = errors.New("rcp/linbr: checksum mismatch")

// ErrFrameTooShort is returned when a buffer is too short to contain a PID
// byte and a checksum byte.
var ErrFrameTooShort = errors.New("rcp/linbr: frame too short")

// ErrDataTooLong is returned when a Frame's Data exceeds the 8-byte LIN
// classic/enhanced frame payload limit.
var ErrDataTooLong = errors.New("rcp/linbr: data exceeds 8-byte LIN frame payload limit")

// ChecksumKind selects LIN 1.x classic checksum (data bytes only) or LIN 2.x
// enhanced checksum (protected ID included).
type ChecksumKind uint8

const (
	// ChecksumClassic sums only the data bytes.
	ChecksumClassic ChecksumKind = iota

	// ChecksumEnhanced sums the protected ID byte in addition to the data
	// bytes — the LIN 2.x default for every frame ID other than the two
	// reserved diagnostic IDs (0x3C/0x3D), which this package leaves to its
	// caller to avoid guessing at, since diagnostic-frame handling is
	// outside this package's own scope.
	ChecksumEnhanced
)

// protectedID computes the LIN protected identifier: the 6-bit frame ID
// plus its two parity bits, per the standard LIN PID parity equations.
func protectedID(id uint8) uint8 {
	id &= 0x3F
	p0 := (id>>0 ^ id>>1 ^ id>>2 ^ id>>4) & 1
	p1 := ^(id>>1 ^ id>>3 ^ id>>4 ^ id>>5) & 1
	return id | (p0 << 6) | (p1 << 7)
}

// checksum computes the LIN classic or enhanced checksum: the ones'
// complement of the 8-bit end-around-carry sum of data (plus pid, for
// ChecksumEnhanced).
func checksum(kind ChecksumKind, pid uint8, data []byte) uint8 {
	sum := 0
	if kind == ChecksumEnhanced {
		sum = int(pid)
	}
	for _, b := range data {
		sum += int(b)
		if sum > 0xFF {
			sum -= 0xFF
		}
	}
	return uint8(0xFF - sum)
}

// Frame is one LIN commander frame: an unprotected 6-bit ID, up to 8 data
// bytes, and the checksum kind used to validate it.
type Frame struct {
	ID       uint8
	Data     []byte
	Checksum ChecksumKind
}

// EncodeFrame serializes f into the wire bytes this package places into a
// lin.Endpoint transfer request's opaque body: [protected ID][data...][checksum].
func EncodeFrame(f Frame) ([]byte, error) {
	if len(f.Data) > 8 {
		return nil, ErrDataTooLong
	}
	pid := protectedID(f.ID)
	out := make([]byte, 0, 2+len(f.Data))
	out = append(out, pid)
	out = append(out, f.Data...)
	out = append(out, checksum(f.Checksum, pid, f.Data))
	return out, nil
}

// DecodeFrame parses b (as returned by a lin.Endpoint transfer) into a
// Frame, verifying its checksum as kind. It reports ErrFrameTooShort for a
// buffer under 2 bytes and ErrChecksumMismatch for a computed/stored
// checksum mismatch.
func DecodeFrame(b []byte, kind ChecksumKind) (Frame, error) {
	if len(b) < 2 {
		return Frame{}, ErrFrameTooShort
	}
	pid := b[0]
	data := b[1 : len(b)-1]
	want := checksum(kind, pid, data)
	got := b[len(b)-1]
	if want != got {
		return Frame{}, fmt.Errorf("%w: want 0x%02X got 0x%02X", ErrChecksumMismatch, want, got)
	}
	return Frame{
		ID:       pid & 0x3F,
		Data:     append([]byte(nil), data...),
		Checksum: kind,
	}, nil
}

// ScheduleEntry pairs a frame ID (and the checksum kind its slot uses) with
// how long a commander should wait before advancing to the next slot.
type ScheduleEntry struct {
	ID         uint8
	Checksum   ChecksumKind
	DelaySlots uint32 // slot-time units; this package assigns no wall-clock meaning to the unit (see doc.go)
}

// ScheduleTable is a commander's fixed, ordered LIN schedule: a round-robin
// sequence of ScheduleEntry slots. It tracks position only — it does not
// itself send anything or own a timer (see doc.go).
type ScheduleTable struct {
	mu      sync.Mutex
	entries []ScheduleEntry
	next    int
}

// NewScheduleTable returns a ScheduleTable cycling through entries in
// order, starting at index 0. entries must be non-empty for Next to ever
// return ok == true.
func NewScheduleTable(entries []ScheduleEntry) *ScheduleTable {
	cp := make([]ScheduleEntry, len(entries))
	copy(cp, entries)
	return &ScheduleTable{entries: cp}
}

// Next returns the current slot and advances the table to the next one,
// wrapping back to index 0 after the last entry. ok is false only when the
// table has no entries at all.
func (s *ScheduleTable) Next() (entry ScheduleEntry, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.entries) == 0 {
		return ScheduleEntry{}, false
	}
	e := s.entries[s.next]
	s.next = (s.next + 1) % len(s.entries)
	return e, true
}

// Reset returns the table to its first slot.
func (s *ScheduleTable) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next = 0
}

// Controller performs one commander-issued LIN transfer at a time against a
// declared lin.Endpoint reached through a *udp.Controller: it owns
// PID/checksum framing (see Frame) around the native endpoint's raw
// byte-pass-through transfer request/response (lin.EncodeTransferRequest/
// lin.DecodeTransferResponse).
type Controller struct {
	inner *udp.Controller
	addr  avtp.ByteBusID
}

// NewController returns a Controller performing LIN transfers against the
// declared LIN endpoint addr on inner.
func NewController(inner *udp.Controller, addr avtp.ByteBusID) *Controller {
	return &Controller{inner: inner, addr: addr}
}

// StreamID returns the wrapped Controller's own avtp.StreamID identity.
func (c *Controller) StreamID() avtp.StreamID { return c.inner.StreamID() }

// Transfer encodes f (see EncodeFrame) as a lin.Endpoint transfer request
// body, sends it, and decodes the response body as a Frame using f's own
// ChecksumKind (a LIN transfer's response frame answers with the same PID
// slot as the request, per the LIN commander/responder model).
func (c *Controller) Transfer(ctx context.Context, f Frame) (Frame, error) {
	tx, err := EncodeFrame(f)
	if err != nil {
		return Frame{}, err
	}
	resp, err := c.inner.Write(ctx, c.addr, lin.EncodeTransferRequest(tx))
	if err != nil {
		return Frame{}, err
	}
	if resp.Control.Has(acf.FlagError) {
		return Frame{}, fmt.Errorf("rcp/linbr: transfer: %s", resp.Body)
	}
	return DecodeFrame(lin.DecodeTransferResponse(resp.Body), f.Checksum)
}

// Close closes the wrapped Controller. Safe to call multiple times (see
// udp.Controller.Close).
func (c *Controller) Close() error { return c.inner.Close() }
