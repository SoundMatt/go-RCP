package request

import (
	"encoding/binary"

	"github.com/SoundMatt/go-RCP/acf"
)

// ChainedSegment is one sub-request packed inside a KindChained envelope:
// the Control flags (Read/Write/etc.) and Body an equivalent Plain request
// to the same endpoint would carry.
type ChainedSegment struct {
	Control acf.ControlFlags
	Body    []byte
}

// maxChainedSegments bounds the segment count field to one byte, the same
// small-integer sizing this package's other count fields (see
// ChainedResult.Total) use.
const maxChainedSegments = 255

// EncodeChained serializes a KindChained request envelope: how many
// segments follow, then each segment's Control byte, a 2-byte length, and
// its Body — forced-sequential execution, in this exact order, is
// Dispatcher's job (ROADMAP.md Milestone 49), not this encoding's.
func EncodeChained(segs []ChainedSegment) ([]byte, error) {
	if len(segs) == 0 || len(segs) > maxChainedSegments {
		return nil, ErrInvalidSegmentCount
	}
	size := 1 + 1
	for _, seg := range segs {
		size += 1 + 2 + len(seg.Body)
	}
	buf := make([]byte, size)
	buf[0] = byte(KindChained)
	buf[1] = byte(len(segs))
	off := 2
	for _, seg := range segs {
		buf[off] = byte(seg.Control)
		binary.BigEndian.PutUint16(buf[off+1:off+3], uint16(len(seg.Body)))
		copy(buf[off+3:], seg.Body)
		off += 3 + len(seg.Body)
	}
	return buf, nil
}

// DecodeChained parses a KindChained envelope. It never panics on malformed
// input.
func DecodeChained(body []byte) ([]ChainedSegment, error) {
	if len(body) < 2 {
		return nil, ErrShortBuffer
	}
	if Kind(body[0]) != KindChained {
		return nil, ErrWrongKind
	}
	count := int(body[1])
	if count == 0 {
		return nil, ErrInvalidSegmentCount
	}
	segs := make([]ChainedSegment, 0, count)
	off := 2
	for i := 0; i < count; i++ {
		if len(body) < off+3 {
			return nil, ErrShortBuffer
		}
		control := acf.ControlFlags(body[off])
		segLen := int(binary.BigEndian.Uint16(body[off+1 : off+3]))
		off += 3
		if len(body) < off+segLen {
			return nil, ErrShortBuffer
		}
		segBody := make([]byte, segLen)
		copy(segBody, body[off:off+segLen])
		segs = append(segs, ChainedSegment{Control: control, Body: segBody})
		off += segLen
	}
	if off != len(body) {
		return nil, ErrTrailingBytes
	}
	return segs, nil
}

// ChainedResponse is one segment's outcome within a resolved KindChained
// ticket's response.
type ChainedResponse struct {
	// Body is the inner Handler.HandleRequest response body for this
	// segment. Nil for a segment that was never executed because an
	// earlier one in the same chain failed.
	Body []byte

	// Failed reports whether this segment is the one whose
	// Handler.HandleRequest call returned an error, aborting the chain.
	// At most one segment in a ChainedResult has Failed set.
	Failed bool
}

// ChainedResult is the response payload for a resolved KindChained ticket:
// one ChainedResponse per segment that was attempted, in order. Per
// ROADMAP.md Milestone 49's "forced sequential execution" framing, this
// package's own reasoned choice (see doc.go) is fail-fast: once a segment's
// Handler.HandleRequest call errors, every remaining segment is skipped
// rather than attempted out of order or silently continued, since a chain's
// whole purpose is guaranteeing the segments before a given point actually
// ran — an aborted tail is honest about what did and didn't happen, where
// continuing past a failure would not be.
type ChainedResult struct {
	Responses []ChainedResponse

	// Total is the number of segments the original request declared. len(Responses) <
	// Total exactly when the chain aborted early on a failure.
	Total int
}

// EncodeChainedResponse serializes r.
func EncodeChainedResponse(r ChainedResult) []byte {
	size := 2
	for _, resp := range r.Responses {
		size += 1 + 2 + len(resp.Body)
	}
	buf := make([]byte, size)
	buf[0] = byte(len(r.Responses))
	buf[1] = byte(r.Total)
	off := 2
	for _, resp := range r.Responses {
		if resp.Failed {
			buf[off] = 1
		}
		binary.BigEndian.PutUint16(buf[off+1:off+3], uint16(len(resp.Body)))
		copy(buf[off+3:], resp.Body)
		off += 3 + len(resp.Body)
	}
	return buf
}

// DecodeChainedResponse parses a response body produced by
// EncodeChainedResponse. It never panics on malformed input.
func DecodeChainedResponse(body []byte) (ChainedResult, error) {
	if len(body) < 2 {
		return ChainedResult{}, ErrShortBuffer
	}
	count := int(body[0])
	total := int(body[1])
	responses := make([]ChainedResponse, 0, count)
	off := 2
	for i := 0; i < count; i++ {
		if len(body) < off+3 {
			return ChainedResult{}, ErrShortBuffer
		}
		failed := body[off] != 0
		respLen := int(binary.BigEndian.Uint16(body[off+1 : off+3]))
		off += 3
		if len(body) < off+respLen {
			return ChainedResult{}, ErrShortBuffer
		}
		respBody := make([]byte, respLen)
		copy(respBody, body[off:off+respLen])
		responses = append(responses, ChainedResponse{Body: respBody, Failed: failed})
		off += respLen
	}
	if off != len(body) {
		return ChainedResult{}, ErrTrailingBytes
	}
	return ChainedResult{Responses: responses, Total: total}, nil
}
