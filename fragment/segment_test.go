//fusa:test REQ-FRAG-001
//fusa:test REQ-FRAG-002

package fragment_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/fragment"
)

func testStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55}, 1)
}

// TestSplit_SmallBodyUnchanged checks a Message whose Body already fits
// within maxBody is returned as a single unchanged segment, and that
// invalid input is rejected (REQ-FRAG-001).
func TestSplit_SmallBodyUnchanged(t *testing.T) {
	msg := acf.Message{
		Kind:              acf.KindShort,
		ByteBusID:         avtp.ByteBusID(1),
		TransactionNum:    7,
		Control:           acf.FlagWrite,
		ReadSizeOrSegment: 42,
		Body:              []byte{0x01, 0x02, 0x03},
	}

	segs, err := fragment.Split(msg, 16)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(segs) != 1 || !bytes.Equal(segs[0].Body, msg.Body) || segs[0].Control != msg.Control {
		t.Fatalf("Split(small body) = %+v, want unchanged single segment", segs)
	}

	if _, err := fragment.Split(msg, 0); !errors.Is(err, fragment.ErrInvalidMaxSegmentBody) {
		t.Errorf("Split(maxBody=0) err = %v, want ErrInvalidMaxSegmentBody", err)
	}
	if _, err := fragment.Split(msg, -1); !errors.Is(err, fragment.ErrInvalidMaxSegmentBody) {
		t.Errorf("Split(maxBody=-1) err = %v, want ErrInvalidMaxSegmentBody", err)
	}

	already := msg
	already.Control |= acf.FlagMoreSegments
	if _, err := fragment.Split(already, 1); !errors.Is(err, fragment.ErrAlreadyFragmented) {
		t.Errorf("Split(already fragmented) err = %v, want ErrAlreadyFragmented", err)
	}
}

// TestSplit_MultiSegmentSequencing checks Split's FlagMoreSegments/
// ReadSizeOrSegment sequencing, that shared descriptor fields are preserved
// verbatim across every segment, and that concatenating the segment bodies
// in order losslessly reconstructs the original Body (REQ-FRAG-002).
func TestSplit_MultiSegmentSequencing(t *testing.T) {
	body := make([]byte, 25)
	for i := range body {
		body[i] = byte(i)
	}
	msg := acf.Message{
		Kind:              acf.KindLong,
		ByteBusID:         avtp.ByteBusID(3),
		TransactionNum:    99,
		Control:           acf.FlagRead | acf.FlagResponse,
		ReadSizeOrSegment: 25,
		Timestamp:         0x1122334455667788,
		Body:              body,
	}

	segs, err := fragment.Split(msg, 10)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(segs) != 3 {
		t.Fatalf("Split produced %d segments, want 3", len(segs))
	}

	var recombined []byte
	for i, seg := range segs {
		if seg.Kind != msg.Kind || seg.ByteBusID != msg.ByteBusID || seg.TransactionNum != msg.TransactionNum || seg.Timestamp != msg.Timestamp {
			t.Fatalf("segment %d shared descriptor fields diverged: %+v", i, seg)
		}
		last := i == len(segs)-1
		if got := seg.Control.Has(acf.FlagMoreSegments); got == last {
			t.Fatalf("segment %d FlagMoreSegments = %v, want %v", i, got, !last)
		}
		if !last {
			if num, ok := seg.SegmentNumber(); !ok || int(num) != i {
				t.Fatalf("segment %d SegmentNumber() = (%d, %v), want (%d, true)", i, num, ok, i)
			}
		} else if seg.ReadSizeOrSegment != msg.ReadSizeOrSegment {
			t.Fatalf("final segment ReadSizeOrSegment = %d, want original %d", seg.ReadSizeOrSegment, msg.ReadSizeOrSegment)
		}
		if len(seg.Body) > 10 {
			t.Fatalf("segment %d Body length %d exceeds maxBody", i, len(seg.Body))
		}
		recombined = append(recombined, seg.Body...)
	}
	if !bytes.Equal(recombined, body) {
		t.Fatalf("recombined segment bodies = % X, want % X", recombined, body)
	}
}

// TestSplit_TooManySegments checks a Body that would require more
// non-terminal segments than the wire's 16-bit segment-number field can
// represent is rejected (REQ-FRAG-001).
func TestSplit_TooManySegments(t *testing.T) {
	msg := acf.Message{Body: make([]byte, 3)}
	if _, err := fragment.Split(msg, 1); err != nil {
		t.Fatalf("Split: %v", err)
	}

	// A single-byte-per-segment split of a body one byte over the
	// representable non-terminal segment-count bound.
	huge := acf.Message{Body: make([]byte, 0x10001)}
	if _, err := fragment.Split(huge, 1); !errors.Is(err, fragment.ErrTooManySegments) {
		t.Errorf("Split(huge) err = %v, want ErrTooManySegments", err)
	}
}
