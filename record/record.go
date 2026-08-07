package record

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/request"
)

// Entry is a single logged request/response event. Response is only
// meaningful when Err == "" (see marshalEntry/unmarshalEntry).
type Entry struct {
	Timestamp time.Time
	Requester avtp.StreamID
	Request   acf.Message
	Response  acf.Message
	Err       string // non-empty when the wrapped Handler returned an error instead of a response
}

// Recorder is an append-only, ring-buffer log of Entry events.
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
//	[8 bytes unix-ns][4 bytes payload len][payload][4 bytes CRC32(payload)]
func (r *Recorder) WriteTo(w io.Writer) (int64, error) {
	entries := r.Snapshot()
	var total int64
	for _, e := range entries {
		payload, err := marshalEntry(e)
		if err != nil {
			return total, err
		}
		checksum := crc32.ChecksumIEEE(payload)

		hdr := make([]byte, 8+4)
		binary.BigEndian.PutUint64(hdr[0:], uint64(e.Timestamp.UnixNano()))
		binary.BigEndian.PutUint32(hdr[8:], uint32(len(payload)))

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

// ErrCorrupted is returned by ReadFrom when a logged entry's CRC32 does not
// match its payload.
var ErrCorrupted = errors.New("rcp/record: log entry CRC mismatch — data corrupted")

// ReadFrom deserialises entries written by WriteTo.
func ReadFrom(r io.Reader) ([]Entry, error) {
	var entries []Entry
	hdr := make([]byte, 8+4)
	for {
		if _, err := io.ReadFull(r, hdr); errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			break
		} else if err != nil {
			return nil, err
		}
		tsNs := int64(binary.BigEndian.Uint64(hdr[0:]))
		payloadLen := int(binary.BigEndian.Uint32(hdr[8:]))

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

		e, err := unmarshalEntry(payload)
		if err != nil {
			return nil, err
		}
		e.Timestamp = time.Unix(0, tsNs)
		entries = append(entries, e)
	}
	return entries, nil
}

// Handler wraps a request.Handler, recording every request/response pair
// (or request/error pair) to a Recorder before returning. It implements
// request.Handler itself, so it is directly registrable into a *udp.Router
// in place of the endpoint it wraps — the same "wrap, don't require
// call-site changes" posture e2e.Guard and proxy.Handler already establish.
type Handler struct {
	inner    request.Handler
	recorder *Recorder
}

// NewHandler wraps inner, recording all activity into rec.
func NewHandler(inner request.Handler, rec *Recorder) *Handler {
	return &Handler{inner: inner, recorder: rec}
}

// HandleRequest implements request.Handler: it delegates to inner, then
// records the outcome (whichever of Response/Err applies) before returning
// it unchanged to the caller.
func (h *Handler) HandleRequest(requester avtp.StreamID, req acf.Message) (acf.Message, error) {
	resp, err := h.inner.HandleRequest(requester, req)
	if err != nil {
		h.recorder.append(Entry{Timestamp: time.Now(), Requester: requester, Request: cloneMessage(req), Err: err.Error()})
		return resp, err
	}
	h.recorder.append(Entry{Timestamp: time.Now(), Requester: requester, Request: cloneMessage(req), Response: cloneMessage(resp)})
	return resp, nil
}

// Replay feeds every recorded request in entries into target, in order,
// via target.HandleRequest. It returns the responses received from target
// for entries whose own outcome was a response (an originally-erroring
// entry is still replayed — target may behave differently the second time
// — but its own recorded Err is not itself replayed as an error).
func Replay(target request.Handler, entries []Entry) ([]acf.Message, error) {
	var resps []acf.Message
	for _, e := range entries {
		resp, err := target.HandleRequest(e.Requester, e.Request)
		if err != nil {
			return resps, fmt.Errorf("rcp/record: replay failed at %v: %w", e.Timestamp, err)
		}
		resps = append(resps, resp)
	}
	return resps, nil
}

// cloneMessage returns a copy of m whose Body has its own backing array, so
// the recorded Entry cannot be mutated by a caller reusing m's buffer.
func cloneMessage(m acf.Message) acf.Message {
	out := m
	if len(m.Body) > 0 {
		out.Body = make([]byte, len(m.Body))
		copy(out.Body, m.Body)
	}
	return out
}

// marshalEntry encodes e's Requester/Request/Response/Err into bytes for
// CRC and storage:
//
//	[8]  Requester
//	[1]  hasErr (1 = Err carries the outcome, Response is not encoded)
//	[4]  request length + request (acf.EncodeMessage)
//	[4]  response length + response (acf.EncodeMessage; 0 length if hasErr)
//	[4]  error length + error bytes (0 length if !hasErr)
func marshalEntry(e Entry) ([]byte, error) {
	reqBytes, err := acf.EncodeMessage(e.Request)
	if err != nil {
		return nil, fmt.Errorf("rcp/record: encode request: %w", err)
	}

	hasErr := e.Err != ""
	var respBytes []byte
	if !hasErr {
		respBytes, err = acf.EncodeMessage(e.Response)
		if err != nil {
			return nil, fmt.Errorf("rcp/record: encode response: %w", err)
		}
	}
	errBytes := []byte(e.Err)

	buf := make([]byte, 0, 8+1+4+len(reqBytes)+4+len(respBytes)+4+len(errBytes))
	buf = append(buf, e.Requester[:]...)
	if hasErr {
		buf = append(buf, 1)
	} else {
		buf = append(buf, 0)
	}
	buf = appendLenPrefixed(buf, reqBytes)
	buf = appendLenPrefixed(buf, respBytes)
	buf = appendLenPrefixed(buf, errBytes)
	return buf, nil
}

func appendLenPrefixed(buf, data []byte) []byte {
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(data)))
	buf = append(buf, lenBuf[:]...)
	return append(buf, data...)
}

// unmarshalEntry decodes a payload produced by marshalEntry.
func unmarshalEntry(payload []byte) (Entry, error) {
	if len(payload) < 8+1+4 {
		return Entry{}, fmt.Errorf("rcp/record: entry payload too short")
	}
	var e Entry
	copy(e.Requester[:], payload[0:8])
	hasErr := payload[8] == 1
	off := 9

	reqBytes, off, err := readLenPrefixed(payload, off)
	if err != nil {
		return Entry{}, err
	}
	req, err := acf.DecodeMessage(reqBytes)
	if err != nil {
		return Entry{}, fmt.Errorf("rcp/record: decode request: %w", err)
	}
	e.Request = req

	respBytes, off, err := readLenPrefixed(payload, off)
	if err != nil {
		return Entry{}, err
	}
	if !hasErr {
		resp, decErr := acf.DecodeMessage(respBytes)
		if decErr != nil {
			return Entry{}, fmt.Errorf("rcp/record: decode response: %w", decErr)
		}
		e.Response = resp
	}

	errBytes, _, err := readLenPrefixed(payload, off)
	if err != nil {
		return Entry{}, err
	}
	if hasErr {
		e.Err = string(errBytes)
	}

	return e, nil
}

func readLenPrefixed(payload []byte, off int) ([]byte, int, error) {
	if len(payload) < off+4 {
		return nil, 0, fmt.Errorf("rcp/record: truncated length prefix")
	}
	n := int(binary.BigEndian.Uint32(payload[off:]))
	off += 4
	if len(payload) < off+n {
		return nil, 0, fmt.Errorf("rcp/record: truncated field")
	}
	return payload[off : off+n], off + n, nil
}
