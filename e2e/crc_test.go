//fusa:test REQ-CRC-001
//fusa:test REQ-CRC-002
//fusa:test REQ-CRC-003
//fusa:test REQ-CRC-004

package e2e_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/e2e"
)

func testStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55}, 7)
}

func baseMessage() acf.Message {
	return acf.Message{
		Kind:              acf.KindLong,
		ByteBusID:         avtp.ByteBusID(3),
		TransactionNum:    avtp.TransactionNum(9),
		Control:           acf.FlagWrite,
		ReadSizeOrSegment: 0,
		Timestamp:         0x1122334455667788,
		Body:              []byte{0xAA, 0xBB, 0xCC},
	}
}

// TestCompute_CoversEveryDeclaredField checks that changing any one of the
// fields Compute's own doc comment declares as covered — the stream
// addressing, ByteBusID, TransactionNum, Control, ReadSizeOrSegment,
// Timestamp, and Body — changes the resulting CRC32, and that two identical
// (stream, Message) pairs always produce the same CRC32 (REQ-CRC-001).
func TestCompute_CoversEveryDeclaredField(t *testing.T) {
	stream := testStream()
	base := baseMessage()
	baseCRC := e2e.Compute(stream, base)

	if got := e2e.Compute(stream, base); got != baseCRC {
		t.Fatalf("Compute is not deterministic: got %#08x and %#08x for identical input", got, baseCRC)
	}

	otherStream := avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x56}, 7)
	mutations := map[string]func() (avtp.StreamID, acf.Message){
		"stream":            func() (avtp.StreamID, acf.Message) { return otherStream, base },
		"ByteBusID":         func() (avtp.StreamID, acf.Message) { m := base; m.ByteBusID++; return stream, m },
		"TransactionNum":    func() (avtp.StreamID, acf.Message) { m := base; m.TransactionNum++; return stream, m },
		"Control":           func() (avtp.StreamID, acf.Message) { m := base; m.Control |= acf.FlagRead; return stream, m },
		"ReadSizeOrSegment": func() (avtp.StreamID, acf.Message) { m := base; m.ReadSizeOrSegment = 42; return stream, m },
		"Timestamp":         func() (avtp.StreamID, acf.Message) { m := base; m.Timestamp++; return stream, m },
		"Body": func() (avtp.StreamID, acf.Message) {
			m := base
			m.Body = append([]byte(nil), base.Body...)
			m.Body[0]++
			return stream, m
		},
	}
	for name, mutate := range mutations {
		s, m := mutate()
		if got := e2e.Compute(s, m); got == baseCRC {
			t.Errorf("mutating %s did not change Compute's CRC32 (still %#08x): field not covered", name, got)
		}
	}
}

// TestProtectVerify_RoundTrip checks Protect appends a Len-byte trailing
// CRC32 and Verify strips and validates it, returning the original message
// on success (REQ-CRC-002).
func TestProtectVerify_RoundTrip(t *testing.T) {
	stream := testStream()
	m := baseMessage()

	protected := e2e.Protect(stream, m)
	if len(protected.Body) != len(m.Body)+e2e.Len {
		t.Fatalf("Protect(Body) length = %d, want %d", len(protected.Body), len(m.Body)+e2e.Len)
	}

	inner, err := e2e.Verify(stream, protected)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if string(inner.Body) != string(m.Body) {
		t.Errorf("Verify(Body) = % X, want % X", inner.Body, m.Body)
	}
	if inner.Kind != m.Kind || inner.ByteBusID != m.ByteBusID || inner.TransactionNum != m.TransactionNum ||
		inner.Control != m.Control || inner.Timestamp != m.Timestamp {
		t.Errorf("Verify restored message = %+v, want %+v", inner, m)
	}
}

// TestVerify_RejectsShortAndMismatched checks Verify reports
// ErrShortSafePoint for a body too short to hold a CRC field at all, and
// ErrCRCMismatch (never a usable message) for a corrupted trailing CRC or a
// tampered covered field (REQ-CRC-003).
func TestVerify_RejectsShortAndMismatched(t *testing.T) {
	stream := testStream()
	m := baseMessage()
	protected := e2e.Protect(stream, m)

	if _, err := e2e.Verify(stream, acf.Message{Body: []byte{0x01, 0x02}}); !errors.Is(err, e2e.ErrShortSafePoint) {
		t.Errorf("Verify(short body) err = %v, want ErrShortSafePoint", err)
	}

	corrupted := protected
	corrupted.Body = append([]byte(nil), protected.Body...)
	corrupted.Body[len(corrupted.Body)-1] ^= 0xFF
	got, err := e2e.Verify(stream, corrupted)
	if !errors.Is(err, e2e.ErrCRCMismatch) {
		t.Errorf("Verify(corrupted CRC) err = %v, want ErrCRCMismatch", err)
	}
	if got.Body != nil {
		t.Errorf("Verify(corrupted CRC) returned a non-empty message on failure: %+v", got)
	}

	tampered := protected
	tampered.Body = append([]byte(nil), protected.Body...)
	tampered.Body[0] ^= 0xFF // flip a covered payload byte, leave the trailing CRC as-is
	if _, err := e2e.Verify(stream, tampered); !errors.Is(err, e2e.ErrCRCMismatch) {
		t.Errorf("Verify(tampered body) err = %v, want ErrCRCMismatch", err)
	}
}

// TestComputeFragmented_MatchesSingleMessage checks ComputeFragmented over
// several segments whose Bodies concatenate to some combined byte slice
// produces exactly the same CRC32 as calling Compute once against a single,
// already-combined message — the fragmentation-CRC interaction rule
// ROADMAP.md Milestone 50's own text calls out even though fragmentation
// itself (Milestone 52) is not yet implemented (REQ-CRC-004).
func TestComputeFragmented_MatchesSingleMessage(t *testing.T) {
	stream := testStream()
	header := baseMessage()
	header.Body = nil // ComputeFragmented supplies Body via segments, not header

	segments := [][]byte{{0x01, 0x02}, {0x03}, {0x04, 0x05, 0x06}}
	var combined []byte
	for _, seg := range segments {
		combined = append(combined, seg...)
	}

	got := e2e.ComputeFragmented(stream, header, segments)
	single := header
	single.Body = combined
	want := e2e.Compute(stream, single)

	if got != want {
		t.Errorf("ComputeFragmented = %#08x, want %#08x (must equal Compute over the reassembled single message)", got, want)
	}

	// A no-op single-segment "fragmentation" must be a true no-op relative
	// to calling Compute directly.
	if got := e2e.ComputeFragmented(stream, header, [][]byte{combined}); got != want {
		t.Errorf("ComputeFragmented(single segment) = %#08x, want %#08x", got, want)
	}
}
