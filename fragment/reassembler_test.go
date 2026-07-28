//fusa:test REQ-FRAG-003
//fusa:test REQ-FRAG-004
//fusa:test REQ-FRAG-005
//fusa:test REQ-FRAG-006
//fusa:test REQ-FRAG-007
//fusa:test REQ-FRAG-008

package fragment_test

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/fragment"
)

// splitBody returns the 3-segment split (segment bodies only) of msg at
// maxBody, failing the test on any error.
func splitOrFatal(t *testing.T, msg acf.Message, maxBody int) []acf.Message {
	t.Helper()
	segs, err := fragment.Split(msg, maxBody)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	return segs
}

// TestReassembler_UnfragmentedMessageIsImmediatelyComplete checks Add
// treats an ordinary Message with no FlagMoreSegments as its own complete,
// one-segment sequence (REQ-FRAG-003).
func TestReassembler_UnfragmentedMessageIsImmediatelyComplete(t *testing.T) {
	re := fragment.NewReassembler(fragment.Config{})
	stream := testStream()
	msg := acf.Message{ByteBusID: 1, TransactionNum: 5, Control: acf.FlagWrite, Body: []byte{0xAA, 0xBB}}

	complete, err := re.Add(stream, msg)
	if err != nil || !complete {
		t.Fatalf("Add(unfragmented) = (%v, %v), want (true, nil)", complete, err)
	}

	out, err := re.Finish(fragment.KeyOf(stream, msg))
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if !bytes.Equal(out.Body, msg.Body) || out.Control != msg.Control {
		t.Errorf("Finish(unfragmented) = %+v, want body/control matching original", out)
	}
	if re.Pending() != 0 {
		t.Errorf("Pending() after Finish = %d, want 0", re.Pending())
	}
}

// TestReassembler_HappyPath drives a full 3-segment sequence through
// Add/Finish and checks the reassembled Message exactly reconstructs the
// original logical Message Split was given (REQ-FRAG-007).
func TestReassembler_HappyPath(t *testing.T) {
	body := bytes.Repeat([]byte{0x5A}, 25)
	original := acf.Message{
		Kind:              acf.KindLong,
		ByteBusID:         2,
		TransactionNum:    11,
		Control:           acf.FlagResponse,
		ReadSizeOrSegment: 0,
		Timestamp:         0xCAFEBABE,
		Body:              body,
	}
	segs := splitOrFatal(t, original, 10)
	if len(segs) < 2 {
		t.Fatalf("test setup: expected multiple segments, got %d", len(segs))
	}

	re := fragment.NewReassembler(fragment.Config{})
	stream := testStream()
	key := fragment.KeyOf(stream, original)

	for i, seg := range segs {
		complete, err := re.Add(stream, seg)
		if err != nil {
			t.Fatalf("Add(segment %d): %v", i, err)
		}
		last := i == len(segs)-1
		if complete != last {
			t.Fatalf("Add(segment %d) complete = %v, want %v", i, complete, last)
		}
		if !last && re.Pending() != 1 {
			t.Fatalf("Pending() mid-sequence = %d, want 1", re.Pending())
		}
	}

	out, err := re.Finish(key)
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if !bytes.Equal(out.Body, body) {
		t.Errorf("Finish body = % X, want % X", out.Body, body)
	}
	if out.Kind != original.Kind || out.ByteBusID != original.ByteBusID || out.TransactionNum != original.TransactionNum ||
		out.Timestamp != original.Timestamp || out.Control != original.Control {
		t.Errorf("Finish descriptor fields = %+v, want matching %+v", out, original)
	}

	if _, err := re.Finish(key); !errors.Is(err, fragment.ErrUnknownSequence) {
		t.Errorf("second Finish err = %v, want ErrUnknownSequence", err)
	}
}

// TestReassembler_OutOfOrderAndDuplicateSegments checks a gapped/reordered
// segment abandons the sequence, while an exact retransmission of the most
// recently accepted segment (and of a completed sequence's terminal
// segment) is tolerated (REQ-FRAG-004).
func TestReassembler_OutOfOrderAndDuplicateSegments(t *testing.T) {
	original := acf.Message{ByteBusID: 1, TransactionNum: 1, Body: bytes.Repeat([]byte{0x01}, 25)}
	segs := splitOrFatal(t, original, 10)
	if len(segs) != 3 {
		t.Fatalf("test setup: want 3 segments, got %d", len(segs))
	}
	stream := testStream()
	key := fragment.KeyOf(stream, original)

	// gapOriginal needs at least two non-terminal segments (i.e. at least
	// three segments total) so a skipped non-terminal segment number is
	// actually detectable: per Reassembler's own doc comment, the terminal
	// segment carries no segment number of its own on the wire once
	// acf.FlagMoreSegments is clear (acf.Message.ReadSizeOrSegment
	// reverts to its ordinary, non-fragmentation meaning), so a gap
	// immediately before the terminal segment is not distinguishable from
	// a legitimately shorter sequence — only a gap among non-terminal
	// segments is.
	gapOriginal := acf.Message{ByteBusID: 1, TransactionNum: 1, Body: bytes.Repeat([]byte{0x02}, 35)}
	gapSegs := splitOrFatal(t, gapOriginal, 10)
	if len(gapSegs) != 4 {
		t.Fatalf("test setup: want 4 segments, got %d", len(gapSegs))
	}

	t.Run("gap among non-terminal segments", func(t *testing.T) {
		re := fragment.NewReassembler(fragment.Config{})
		if _, err := re.Add(stream, gapSegs[0]); err != nil {
			t.Fatalf("Add(0): %v", err)
		}
		if _, err := re.Add(stream, gapSegs[2]); !errors.Is(err, fragment.ErrOutOfOrderSegment) {
			t.Errorf("Add(segment 2 after 0) err = %v, want ErrOutOfOrderSegment", err)
		}
		if re.Pending() != 0 {
			t.Errorf("Pending() after abandoned sequence = %d, want 0", re.Pending())
		}
	})

	t.Run("first segment must start at zero", func(t *testing.T) {
		re := fragment.NewReassembler(fragment.Config{})
		if _, err := re.Add(stream, segs[1]); !errors.Is(err, fragment.ErrOutOfOrderSegment) {
			t.Errorf("Add(segment 1 first) err = %v, want ErrOutOfOrderSegment", err)
		}
	})

	t.Run("duplicate mid-sequence segment is a no-op", func(t *testing.T) {
		re := fragment.NewReassembler(fragment.Config{})
		if _, err := re.Add(stream, segs[0]); err != nil {
			t.Fatalf("Add(0): %v", err)
		}
		if complete, err := re.Add(stream, segs[0]); err != nil || complete {
			t.Errorf("Add(duplicate 0) = (%v, %v), want (false, nil)", complete, err)
		}
		// Sequence must still be able to proceed normally afterward.
		for _, seg := range segs[1:] {
			if _, err := re.Add(stream, seg); err != nil {
				t.Fatalf("Add after duplicate: %v", err)
			}
		}
		if _, err := re.Finish(key); err != nil {
			t.Fatalf("Finish after duplicate: %v", err)
		}
	})

	t.Run("mismatched duplicate is rejected", func(t *testing.T) {
		re := fragment.NewReassembler(fragment.Config{})
		if _, err := re.Add(stream, segs[0]); err != nil {
			t.Fatalf("Add(0): %v", err)
		}
		corrupted := segs[0]
		corrupted.Body = append([]byte(nil), corrupted.Body...)
		corrupted.Body[0] ^= 0xFF
		if _, err := re.Add(stream, corrupted); !errors.Is(err, fragment.ErrDuplicateSegment) {
			t.Errorf("Add(corrupted duplicate) err = %v, want ErrDuplicateSegment", err)
		}
	})

	t.Run("duplicate terminal segment after completion is a no-op", func(t *testing.T) {
		re := fragment.NewReassembler(fragment.Config{})
		for _, seg := range segs {
			if _, err := re.Add(stream, seg); err != nil {
				t.Fatalf("Add: %v", err)
			}
		}
		complete, err := re.Add(stream, segs[len(segs)-1])
		if err != nil || !complete {
			t.Errorf("Add(duplicate terminal) = (%v, %v), want (true, nil)", complete, err)
		}
		if _, err := re.Add(stream, segs[0]); !errors.Is(err, fragment.ErrSequenceComplete) {
			t.Errorf("Add(new segment after completion) err = %v, want ErrSequenceComplete", err)
		}
	})
}

// TestReassembler_HeaderMismatch checks a later segment whose shared
// descriptor fields disagree with the sequence's first segment abandons the
// sequence (REQ-FRAG-005).
func TestReassembler_HeaderMismatch(t *testing.T) {
	original := acf.Message{ByteBusID: 1, TransactionNum: 1, Timestamp: 100, Body: bytes.Repeat([]byte{0x01}, 25)}
	segs := splitOrFatal(t, original, 10)
	stream := testStream()

	re := fragment.NewReassembler(fragment.Config{})
	if _, err := re.Add(stream, segs[0]); err != nil {
		t.Fatalf("Add(0): %v", err)
	}
	mismatched := segs[1]
	mismatched.Timestamp = 999
	if _, err := re.Add(stream, mismatched); !errors.Is(err, fragment.ErrHeaderMismatch) {
		t.Errorf("Add(mismatched timestamp) err = %v, want ErrHeaderMismatch", err)
	}
	if re.Pending() != 0 {
		t.Errorf("Pending() after header mismatch = %d, want 0", re.Pending())
	}
}

// TestReassembler_MaxSegments checks a sequence exceeding Config.MaxSegments
// before completion is abandoned (REQ-FRAG-006).
func TestReassembler_MaxSegments(t *testing.T) {
	original := acf.Message{ByteBusID: 1, TransactionNum: 1, Body: bytes.Repeat([]byte{0x01}, 50)}
	segs := splitOrFatal(t, original, 10)
	if len(segs) < 3 {
		t.Fatalf("test setup: want at least 3 segments, got %d", len(segs))
	}
	stream := testStream()

	re := fragment.NewReassembler(fragment.Config{MaxSegments: 2})
	if _, err := re.Add(stream, segs[0]); err != nil {
		t.Fatalf("Add(0): %v", err)
	}
	if _, err := re.Add(stream, segs[1]); err != nil {
		t.Fatalf("Add(1, still within MaxSegments=2): %v", err)
	}
	if _, err := re.Add(stream, segs[2]); !errors.Is(err, fragment.ErrTooManySegments) {
		t.Errorf("Add(segment 2, MaxSegments=2) err = %v, want ErrTooManySegments", err)
	}
}

// TestReassembler_Sweep checks Sweep purges only stale, incomplete
// sequences, leaving a completed-but-unclaimed sequence untouched
// (REQ-FRAG-008).
func TestReassembler_Sweep(t *testing.T) {
	now := time.Unix(1000, 0)
	clock := func() time.Time { return now }
	re := fragment.NewReassemblerWithClock(fragment.Config{Timeout: time.Second}, clock)
	stream := testStream()

	incomplete := acf.Message{ByteBusID: 1, TransactionNum: 1, Body: bytes.Repeat([]byte{0x01}, 25)}
	incompleteSegs := splitOrFatal(t, incomplete, 10)
	if _, err := re.Add(stream, incompleteSegs[0]); err != nil {
		t.Fatalf("Add(incomplete): %v", err)
	}

	complete := acf.Message{ByteBusID: 2, TransactionNum: 2, Body: []byte{0x01}}
	if _, err := re.Add(stream, complete); err != nil {
		t.Fatalf("Add(complete): %v", err)
	}

	if purged := re.Sweep(); len(purged) != 0 {
		t.Fatalf("Sweep before timeout purged %v, want none", purged)
	}

	now = now.Add(2 * time.Second)
	purged := re.Sweep()
	if len(purged) != 1 || purged[0] != fragment.KeyOf(stream, incompleteSegs[0]) {
		t.Fatalf("Sweep after timeout purged %v, want only the incomplete sequence's key", purged)
	}
	if re.Pending() != 1 {
		t.Fatalf("Pending() after Sweep = %d, want 1 (completed sequence retained)", re.Pending())
	}

	if _, err := re.Finish(fragment.KeyOf(stream, complete)); err != nil {
		t.Fatalf("Finish(completed, still present after Sweep): %v", err)
	}
}
