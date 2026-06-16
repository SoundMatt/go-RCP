// Package e2e provides end-to-end communication protection for go-RCP command payloads.
//
// E2E protection (ISO 26262 Part 7 E2E profile) adds three layers of defence:
//
//  1. Sequence counter — a per-Controller monotonically incrementing uint32 prepended
//     to each command payload, enabling detection of lost or out-of-order frames.
//  2. CRC-16/CCITT-FALSE checksum — computed over the sequence counter and original
//     payload; detects corruption in transit.
//  3. Replay guard — a ReplayGuard tracks recently seen sequence numbers and rejects
//     frames that appear more than once (replay attack / stuck-at-value fault).
//
// On the sender side, wrap with e2e.NewController. On the receiver side, call
// e2e.Unwrap followed by ReplayGuard.Check.
package e2e

//fusa:req REQ-E2E-001
//fusa:req REQ-E2E-002
//fusa:req REQ-E2E-003
//fusa:req REQ-E2E-004
//fusa:req REQ-E2E-005
//fusa:req REQ-E2E-006
//fusa:req REQ-E2E-007
//fusa:req REQ-E2E-008

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	rcp "github.com/SoundMatt/go-RCP"
)

// HeaderLen is the byte length of the E2E protection header prepended by Wrap.
// Layout: [0:4] SeqNum uint32 big-endian, [4:6] CRC-16 uint16 big-endian.
const HeaderLen = 6

// ErrCRCMismatch is returned by Unwrap when the CRC does not match the payload.
var ErrCRCMismatch = errors.New("e2e: CRC-16 mismatch — payload corrupted or truncated")

// ErrShortFrame is returned by Unwrap when the frame is shorter than HeaderLen.
var ErrShortFrame = errors.New("e2e: frame too short to contain E2E header")

// ErrReplay is returned by ReplayGuard.Check when a sequence number has been seen before.
var ErrReplay = errors.New("e2e: replayed sequence number detected")

// Wrap prepends the E2E header (seqNum + CRC-16) to payload and returns the
// protected frame. The CRC-16/CCITT-FALSE is computed over the 4-byte seqNum
// followed by the original payload.
func Wrap(seqNum uint32, payload []byte) []byte {
	frame := make([]byte, HeaderLen+len(payload))
	binary.BigEndian.PutUint32(frame[0:4], seqNum)
	copy(frame[HeaderLen:], payload)
	crc := crc16(frame[0:4], payload)
	binary.BigEndian.PutUint16(frame[4:6], crc)
	return frame
}

// Unwrap validates the CRC-16 in frame and returns the embedded sequence number
// and original payload. Returns ErrShortFrame or ErrCRCMismatch on failure.
func Unwrap(frame []byte) (seqNum uint32, payload []byte, err error) {
	if len(frame) < HeaderLen {
		return 0, nil, ErrShortFrame
	}
	seqNum = binary.BigEndian.Uint32(frame[0:4])
	gotCRC := binary.BigEndian.Uint16(frame[4:6])
	payload = frame[HeaderLen:]
	wantCRC := crc16(frame[0:4], payload)
	if gotCRC != wantCRC {
		return 0, nil, fmt.Errorf("%w: got 0x%04x, want 0x%04x", ErrCRCMismatch, gotCRC, wantCRC)
	}
	return seqNum, payload, nil
}

// replayWindow is the number of sequence numbers retained by ReplayGuard.
// A larger window costs more memory but handles reorder / duplicates farther back.
const replayWindow = 32

// ReplayGuard detects replayed frames using a fixed-size sliding window of recently
// seen sequence numbers. It is safe for concurrent use.
type ReplayGuard struct {
	mu      sync.Mutex
	seen    [replayWindow]uint32
	count   int
	maxSeen uint32
	primed  bool
}

// NewReplayGuard returns a freshly initialised ReplayGuard.
func NewReplayGuard() *ReplayGuard { return &ReplayGuard{} }

// Check returns nil when seqNum is new and acceptable, or ErrReplay when the
// sequence number has already been seen within the sliding window.
func (r *ReplayGuard) Check(seqNum uint32) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.primed {
		for i := range r.seen {
			if r.seen[i] == seqNum {
				return fmt.Errorf("%w: seq=%d", ErrReplay, seqNum)
			}
		}
		if r.count < replayWindow {
			r.seen[r.count] = seqNum
			r.count++
		} else {
			// Evict the entry with the smallest (oldest) sequence number.
			minIdx := 0
			for i := range r.seen {
				if r.seen[i] < r.seen[minIdx] {
					minIdx = i
				}
			}
			r.seen[minIdx] = seqNum
		}
	} else {
		r.seen[0] = seqNum
		r.count = 1
		r.primed = true
	}

	if seqNum > r.maxSeen || !r.primed {
		r.maxSeen = seqNum
	}
	return nil
}

// Controller wraps any rcp.Controller and automatically applies E2E protection
// (sequence counter + CRC-16) to every command payload on Send.
type Controller struct {
	inner rcp.Controller
	seq   atomic.Uint32
}

// NewController returns an E2E-protecting Controller wrapping inner.
func NewController(inner rcp.Controller) *Controller {
	return &Controller{inner: inner}
}

// Send wraps cmd.Payload with an E2E header and delegates to the inner controller.
// The seq counter is incremented atomically on every call regardless of outcome.
func (c *Controller) Send(ctx context.Context, cmd *rcp.Command) (*rcp.Response, error) {
	seqNum := c.seq.Add(1)
	protected := &rcp.Command{
		ID:       cmd.ID,
		Zone:     cmd.Zone,
		Type:     cmd.Type,
		Priority: cmd.Priority,
		Payload:  Wrap(seqNum, cmd.Payload),
	}
	return c.inner.Send(ctx, protected)
}

// Zone delegates to the inner controller.
func (c *Controller) Zone() rcp.Zone { return c.inner.Zone() }

// Subscribe delegates to the inner controller.
func (c *Controller) Subscribe(ctx context.Context) (<-chan *rcp.Status, error) {
	return c.inner.Subscribe(ctx)
}

// Close delegates to the inner controller.
func (c *Controller) Close() error { return c.inner.Close() }

// crc16 computes the CRC-16/CCITT-FALSE checksum over prefix followed by data.
// Initial value: 0xFFFF, polynomial: 0x1021, no reflection.
func crc16(prefix, data []byte) uint16 {
	const poly = uint16(0x1021)
	crc := uint16(0xFFFF)
	for _, b := range prefix {
		crc = crc16Update(crc, b, poly)
	}
	for _, b := range data {
		crc = crc16Update(crc, b, poly)
	}
	return crc
}

func crc16Update(crc uint16, b uint8, poly uint16) uint16 {
	crc ^= uint16(b) << 8
	for i := 0; i < 8; i++ {
		if crc&0x8000 != 0 {
			crc = (crc << 1) ^ poly
		} else {
			crc <<= 1
		}
	}
	return crc
}
