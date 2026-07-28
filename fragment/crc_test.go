//fusa:test REQ-FRAG-009

package fragment_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/crcsafe"
	"github.com/SoundMatt/go-RCP/fragment"
)

// TestReassembler_FinishProtected checks FinishProtected verifies a
// fragmented message's trailing CRC32 safe point via
// crcsafe.ComputeFragmented, matching what crcsafe.Protect/Verify would
// compute for the same message sent as one unfragmented segment, and fails
// closed on a corrupted segment or a missing safe point (REQ-FRAG-009).
func TestReassembler_FinishProtected(t *testing.T) {
	stream := testStream()
	original := avtp.Message{
		Kind:           avtp.KindShort,
		ByteBusID:      4,
		TransactionNum: 21,
		Control:        avtp.FlagWrite,
		Body:           bytes.Repeat([]byte{0x11}, 21),
	}
	protected := crcsafe.Protect(stream, original)

	t.Run("matches unfragmented Verify by construction", func(t *testing.T) {
		segs, err := fragment.Split(protected, 10)
		if err != nil {
			t.Fatalf("Split: %v", err)
		}
		if len(segs) < 2 {
			t.Fatalf("test setup: want multiple segments, got %d", len(segs))
		}

		re := fragment.NewReassembler(fragment.Config{})
		key := fragment.KeyOf(stream, protected)
		for i, seg := range segs {
			if _, addErr := re.Add(stream, seg); addErr != nil {
				t.Fatalf("Add(segment %d): %v", i, addErr)
			}
		}

		got, err := re.FinishProtected(stream, key)
		if err != nil {
			t.Fatalf("FinishProtected: %v", err)
		}

		want, err := crcsafe.Verify(stream, protected)
		if err != nil {
			t.Fatalf("crcsafe.Verify(unfragmented reference): %v", err)
		}
		if !bytes.Equal(got.Body, want.Body) {
			t.Errorf("FinishProtected body = % X, want %X (matching unfragmented Verify)", got.Body, want.Body)
		}
	})

	t.Run("corrupted final segment fails closed", func(t *testing.T) {
		segs, err := fragment.Split(protected, 10)
		if err != nil {
			t.Fatalf("Split: %v", err)
		}
		segs[len(segs)-1].Body = append([]byte(nil), segs[len(segs)-1].Body...)
		segs[len(segs)-1].Body[0] ^= 0xFF

		re := fragment.NewReassembler(fragment.Config{})
		key := fragment.KeyOf(stream, protected)
		for _, seg := range segs {
			if _, err := re.Add(stream, seg); err != nil {
				t.Fatalf("Add: %v", err)
			}
		}
		if _, err := re.FinishProtected(stream, key); !errors.Is(err, crcsafe.ErrCRCMismatch) {
			t.Errorf("FinishProtected(corrupted) err = %v, want crcsafe.ErrCRCMismatch", err)
		}
		// The failed sequence must not linger.
		if re.Pending() != 0 {
			t.Errorf("Pending() after failed FinishProtected = %d, want 0", re.Pending())
		}
	})

	t.Run("short final segment", func(t *testing.T) {
		re := fragment.NewReassembler(fragment.Config{})
		msg := avtp.Message{ByteBusID: 5, TransactionNum: 1, Body: []byte{0x01, 0x02}}
		if _, err := re.Add(stream, msg); err != nil {
			t.Fatalf("Add: %v", err)
		}
		if _, err := re.FinishProtected(stream, fragment.KeyOf(stream, msg)); !errors.Is(err, crcsafe.ErrShortSafePoint) {
			t.Errorf("FinishProtected(short body) err = %v, want crcsafe.ErrShortSafePoint", err)
		}
	})
}
