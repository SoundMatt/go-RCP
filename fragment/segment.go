package fragment

import "github.com/SoundMatt/go-RCP/avtp"

// maxSegmentIndex is the largest segment number a non-terminal segment can
// carry in the wire's 16-bit ReadSizeOrSegment field (see
// avtp.Message.SegmentNumber). A sequence needing more non-terminal
// segments than this cannot be represented and is rejected outright.
const maxSegmentIndex = 0xFFFF

// Split divides msg into one or more avtp.Message segments whose Body never
// exceeds maxBody bytes, per this package's send-side half of ROADMAP.md
// Milestone 52. Every returned segment shares msg's Kind, ByteBusID,
// TransactionNum, and Timestamp verbatim — the fields
// crcsafe.ComputeFragmented's own doc comment names as a fragmented
// message's shared descriptor fields — and msg's Control bits apart from
// avtp.FlagMoreSegments, which Split manages itself: every segment but the
// last carries avtp.FlagMoreSegments set and ReadSizeOrSegment holding its
// own 0-based segment number (see avtp.Message.SegmentNumber); the last
// segment carries avtp.FlagMoreSegments clear and ReadSizeOrSegment
// restored to msg's own original value, so a request whose ReadSizeOrSegment
// carried a real read-size (see avtp.Message.ReadSize) keeps that meaning on
// arrival, exactly as an unfragmented single-segment message would. Split
// never sets Pad on a returned segment — each segment is a fresh, unencoded
// logical message, so any wire-alignment padding a caller wants for a given
// segment's own encoded length is that caller's own choice to make at
// avtp.EncodeMessage time (see doc.go for why this package leaves that
// entirely to the caller rather than guessing at it here).
//
// When len(msg.Body) already fits within maxBody, Split returns
// []avtp.Message{msg} unchanged — msg is returned as-is, not copied, so a
// caller that mutates the single returned element also mutates msg.
//
// Split returns ErrInvalidMaxSegmentBody for a non-positive maxBody,
// ErrAlreadyFragmented when msg.Control already has avtp.FlagMoreSegments
// set (Split fragments a complete logical message; it does not
// re-fragment an existing segment), and ErrTooManySegments when
// msg.Body would require more non-terminal segments than fit the wire's
// 16-bit segment-number field.
func Split(msg avtp.Message, maxBody int) ([]avtp.Message, error) {
	if maxBody <= 0 {
		return nil, ErrInvalidMaxSegmentBody
	}
	if msg.Control.Has(avtp.FlagMoreSegments) {
		return nil, ErrAlreadyFragmented
	}
	if len(msg.Body) <= maxBody {
		return []avtp.Message{msg}, nil
	}

	n := (len(msg.Body) + maxBody - 1) / maxBody
	if n-1 > maxSegmentIndex {
		return nil, ErrTooManySegments
	}

	segs := make([]avtp.Message, 0, n)
	for i := 0; i < n; i++ {
		start := i * maxBody
		end := start + maxBody
		if end > len(msg.Body) {
			end = len(msg.Body)
		}

		seg := msg
		seg.Pad = 0
		seg.Body = msg.Body[start:end]
		if i < n-1 {
			seg.Control = msg.Control | avtp.FlagMoreSegments
			seg.ReadSizeOrSegment = uint16(i)
		} else {
			seg.Control = msg.Control &^ avtp.FlagMoreSegments
			seg.ReadSizeOrSegment = msg.ReadSizeOrSegment
		}
		segs = append(segs, seg)
	}
	return segs, nil
}
