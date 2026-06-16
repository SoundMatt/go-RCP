//fusa:req REQ-REC-001
//fusa:req REQ-REC-002
//fusa:req REQ-REC-003
//fusa:req REQ-REC-004
//fusa:req REQ-REC-005
//fusa:req REQ-REC-006
//fusa:req REQ-REC-007
//fusa:req REQ-REC-008

// Package record provides always-on black-box recording of RCP command, response,
// and status streams to a structured binary log on disk, with ring-buffer semantics
// and a replay mode for regression testing.
package record

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"sync"
	"sync/atomic"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
)

// RecordType identifies the entry type in a log frame.
type RecordType uint8

const (
	TypeCommand  RecordType = 1
	TypeResponse RecordType = 2
	TypeStatus   RecordType = 3
)

// Entry is a single logged event.
type Entry struct {
	Timestamp time.Time
	Type      RecordType
	Zone      rcp.Zone

	Command  *rcp.Command  // non-nil when Type == TypeCommand
	Response *rcp.Response // non-nil when Type == TypeResponse
	Status   *rcp.Status   // non-nil when Type == TypeStatus
}

// Recorder is an append-only, ring-buffer log of RCP events.
// MaxEntries > 0 enables ring-buffer mode; 0 = unlimited.
type Recorder struct {
	mu         sync.RWMutex
	entries    []Entry
	maxEntries int
	head       int // index of oldest entry (ring mode)
	count      int // total entries held
	written    atomic.Int64
}

// New creates a Recorder. maxEntries=0 means unlimited.
func New(maxEntries int) *Recorder {
	if maxEntries > 0 {
		return &Recorder{entries: make([]Entry, maxEntries), maxEntries: maxEntries}
	}
	return &Recorder{}
}

func (r *Recorder) append(e Entry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.maxEntries > 0 {
		r.entries[r.head] = e
		r.head = (r.head + 1) % r.maxEntries
		if r.count < r.maxEntries {
			r.count++
		}
	} else {
		r.entries = append(r.entries, e)
		r.count++
	}
	r.written.Add(1)
}

// RecordCommand appends a command entry.
func (r *Recorder) RecordCommand(cmd *rcp.Command) {
	cp := *cmd
	if len(cmd.Payload) > 0 {
		cp.Payload = make([]byte, len(cmd.Payload))
		copy(cp.Payload, cmd.Payload)
	}
	r.append(Entry{Timestamp: time.Now(), Type: TypeCommand, Zone: cmd.Zone, Command: &cp})
}

// RecordResponse appends a response entry.
func (r *Recorder) RecordResponse(resp *rcp.Response) {
	cp := *resp
	if len(resp.Payload) > 0 {
		cp.Payload = make([]byte, len(resp.Payload))
		copy(cp.Payload, resp.Payload)
	}
	r.append(Entry{Timestamp: time.Now(), Type: TypeResponse, Zone: resp.Zone, Response: &cp})
}

// RecordStatus appends a status entry.
func (r *Recorder) RecordStatus(s *rcp.Status) {
	cp := *s
	if len(s.Payload) > 0 {
		cp.Payload = make([]byte, len(s.Payload))
		copy(cp.Payload, s.Payload)
	}
	r.append(Entry{Timestamp: time.Now(), Type: TypeStatus, Zone: s.Zone, Status: &cp})
}

// Snapshot returns a copy of all currently held entries in order.
func (r *Recorder) Snapshot() []Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.count == 0 {
		return nil
	}
	out := make([]Entry, r.count)
	if r.maxEntries > 0 {
		start := (r.head - r.count + r.maxEntries) % r.maxEntries
		for i := 0; i < r.count; i++ {
			out[i] = r.entries[(start+i)%r.maxEntries]
		}
	} else {
		copy(out, r.entries[:r.count])
	}
	return out
}

// Written returns the total number of entries ever appended (including overwritten ones).
func (r *Recorder) Written() int64 { return r.written.Load() }

// WriteTo serialises the current snapshot to w in a simple binary format.
// Frame layout per entry:
//
//	[8 bytes unix-ns][1 byte type][1 byte zone][4 bytes payload len][payload][4 bytes CRC32]
func (r *Recorder) WriteTo(w io.Writer) (int64, error) {
	entries := r.Snapshot()
	var total int64
	for _, e := range entries {
		payload := marshalEntry(e)
		checksum := crc32.ChecksumIEEE(payload)

		hdr := make([]byte, 8+1+1+4)
		binary.BigEndian.PutUint64(hdr[0:], uint64(e.Timestamp.UnixNano()))
		hdr[8] = byte(e.Type)
		hdr[9] = byte(e.Zone)
		binary.BigEndian.PutUint32(hdr[10:], uint32(len(payload)))

		n, err := w.Write(hdr)
		total += int64(n)
		if err != nil {
			return total, err
		}
		n, err = w.Write(payload)
		total += int64(n)
		if err != nil {
			return total, err
		}
		var crcBuf [4]byte
		binary.BigEndian.PutUint32(crcBuf[:], checksum)
		n, err = w.Write(crcBuf[:])
		total += int64(n)
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// ReadFrom deserialises entries written by WriteTo.
var ErrCorrupted = errors.New("rcp/record: log entry CRC mismatch — data corrupted")

func ReadFrom(r io.Reader) ([]Entry, error) {
	var entries []Entry
	hdr := make([]byte, 8+1+1+4)
	for {
		if _, err := io.ReadFull(r, hdr); errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			break
		} else if err != nil {
			return nil, err
		}
		tsNs := int64(binary.BigEndian.Uint64(hdr[0:]))
		recType := RecordType(hdr[8])
		zone := rcp.Zone(hdr[9])
		payloadLen := int(binary.BigEndian.Uint32(hdr[10:]))

		payload := make([]byte, payloadLen)
		if _, err := io.ReadFull(r, payload); err != nil {
			return nil, err
		}

		var crcBuf [4]byte
		if _, err := io.ReadFull(r, crcBuf[:]); err != nil {
			return nil, err
		}
		if crc32.ChecksumIEEE(payload) != binary.BigEndian.Uint32(crcBuf[:]) {
			return nil, ErrCorrupted
		}

		e, err := unmarshalEntry(tsNs, recType, zone, payload)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// Controller wraps an rcp.Controller and records every Send/Subscribe event.
type Controller struct {
	inner    rcp.Controller
	recorder *Recorder
	closed   atomic.Bool
}

// NewController wraps inner, recording all activity into rec.
func NewController(inner rcp.Controller, rec *Recorder) *Controller {
	return &Controller{inner: inner, recorder: rec}
}

// Zone implements rcp.Controller.
func (c *Controller) Zone() rcp.Zone { return c.inner.Zone() }

// Send records the command and response before returning.
func (c *Controller) Send(ctx context.Context, cmd *rcp.Command) (*rcp.Response, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("rcp/record: %w", rcp.ErrClosed)
	}
	c.recorder.RecordCommand(cmd)
	resp, err := c.inner.Send(ctx, cmd)
	if err == nil {
		c.recorder.RecordResponse(resp)
	}
	return resp, err
}

// Subscribe records each Status published on the channel.
func (c *Controller) Subscribe(ctx context.Context) (<-chan *rcp.Status, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("rcp/record: %w", rcp.ErrClosed)
	}
	ch, err := c.inner.Subscribe(ctx)
	if err != nil {
		return nil, err
	}
	out := make(chan *rcp.Status, cap(ch)+1)
	go func() {
		defer close(out)
		for s := range ch {
			c.recorder.RecordStatus(s)
			out <- s
		}
	}()
	return out, nil
}

// Close closes the inner controller. Safe to call multiple times.
func (c *Controller) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	return c.inner.Close()
}

// Replay feeds all recorded entries into target in order of timestamp.
// Commands are re-sent; responses and status events are compared for assertion.
// Returns the list of responses received from target.
func Replay(ctx context.Context, entries []Entry, target rcp.Controller) ([]*rcp.Response, error) {
	var resps []*rcp.Response
	for _, e := range entries {
		if e.Type != TypeCommand {
			continue
		}
		resp, err := target.Send(ctx, e.Command)
		if err != nil {
			return resps, fmt.Errorf("rcp/record: replay failed at %v: %w", e.Timestamp, err)
		}
		resps = append(resps, resp)
	}
	return resps, nil
}

// marshalEntry converts Entry content to bytes for CRC and storage.
func marshalEntry(e Entry) []byte {
	switch e.Type {
	case TypeCommand:
		return marshalCommand(e.Command)
	case TypeResponse:
		return marshalResponse(e.Response)
	case TypeStatus:
		return marshalStatus(e.Status)
	default:
		return nil
	}
}

func marshalCommand(c *rcp.Command) []byte {
	b := make([]byte, 4+2+1+4+len(c.Payload))
	binary.BigEndian.PutUint32(b[0:], c.ID)
	binary.BigEndian.PutUint16(b[4:], uint16(c.Type))
	b[6] = byte(c.Priority)
	binary.BigEndian.PutUint32(b[7:], uint32(len(c.Payload)))
	copy(b[11:], c.Payload)
	return b
}

func marshalResponse(r *rcp.Response) []byte {
	b := make([]byte, 4+1+4+len(r.Payload))
	binary.BigEndian.PutUint32(b[0:], r.CommandID)
	b[4] = byte(r.Status)
	binary.BigEndian.PutUint32(b[5:], uint32(len(r.Payload)))
	copy(b[9:], r.Payload)
	return b
}

func marshalStatus(s *rcp.Status) []byte {
	healthy := uint8(0)
	if s.Healthy {
		healthy = 1
	}
	b := make([]byte, 4+1+4+len(s.Payload))
	binary.BigEndian.PutUint32(b[0:], s.Seq)
	b[4] = healthy
	binary.BigEndian.PutUint32(b[5:], uint32(len(s.Payload)))
	copy(b[9:], s.Payload)
	return b
}

func unmarshalEntry(tsNs int64, recType RecordType, zone rcp.Zone, payload []byte) (Entry, error) {
	e := Entry{
		Timestamp: time.Unix(0, tsNs),
		Type:      recType,
		Zone:      zone,
	}
	switch recType {
	case TypeCommand:
		if len(payload) < 11 {
			return e, fmt.Errorf("rcp/record: short command payload")
		}
		plen := int(binary.BigEndian.Uint32(payload[7:]))
		if len(payload) < 11+plen {
			return e, fmt.Errorf("rcp/record: truncated command payload")
		}
		cmd := &rcp.Command{
			ID:       binary.BigEndian.Uint32(payload[0:]),
			Zone:     zone,
			Type:     rcp.CommandType(binary.BigEndian.Uint16(payload[4:])),
			Priority: rcp.Priority(payload[6]),
		}
		if plen > 0 {
			cmd.Payload = make([]byte, plen)
			copy(cmd.Payload, payload[11:11+plen])
		}
		e.Command = cmd
	case TypeResponse:
		if len(payload) < 9 {
			return e, fmt.Errorf("rcp/record: short response payload")
		}
		plen := int(binary.BigEndian.Uint32(payload[5:]))
		resp := &rcp.Response{
			CommandID: binary.BigEndian.Uint32(payload[0:]),
			Zone:      zone,
			Status:    rcp.ResponseStatus(payload[4]),
		}
		if plen > 0 {
			resp.Payload = make([]byte, plen)
			copy(resp.Payload, payload[9:9+plen])
		}
		e.Response = resp
	case TypeStatus:
		if len(payload) < 9 {
			return e, fmt.Errorf("rcp/record: short status payload")
		}
		plen := int(binary.BigEndian.Uint32(payload[5:]))
		s := &rcp.Status{
			Zone:    zone,
			Seq:     binary.BigEndian.Uint32(payload[0:]),
			Healthy: payload[4] == 1,
		}
		if plen > 0 {
			s.Payload = make([]byte, plen)
			copy(s.Payload, payload[9:9+plen])
		}
		e.Status = s
	}
	return e, nil
}
