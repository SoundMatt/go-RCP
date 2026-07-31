//fusa:test REQ-AVTP-015

package acf_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
)

func testStreamID() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55}, 0x1234)
}

// ── REQ-AVTP-015: full Frame round-trip across header × message-kind ───────

func TestFrame_RoundTrip(t *testing.T) {
	for _, timed := range []bool{false, true} {
		for _, kind := range []acf.MessageKind{acf.KindShort, acf.KindLong} {
			hdr := avtp.Header{
				Timed:           timed,
				StreamIDValid:   true,
				SequenceNum:     3,
				StreamID:        testStreamID(),
				Timestamp:       0x11223344,
				TimestampStatus: avtp.TimestampValid,
			}
			msg := acf.Message{
				Kind:              kind,
				ByteBusID:         avtp.ByteBusID(2),
				TransactionNum:    avtp.TransactionNum(55),
				Control:           acf.FlagWrite,
				EVT:               0x08, // evt[3] request-acknowledge bit
				HS:                true,
				CS:                true,
				ReadSizeOrSegment: 0,
				Timestamp:         0xFEEDFACECAFEBEEF,
				Body:              []byte("payload!"), // already quadlet-aligned with either header length
			}
			b, err := acf.EncodeFrame(hdr, msg)
			if err != nil {
				t.Fatalf("timed=%v kind=%v: EncodeFrame: %v", timed, kind, err)
			}
			frame, err := acf.DecodeFrame(b)
			if err != nil {
				t.Fatalf("timed=%v kind=%v: DecodeFrame: %v", timed, kind, err)
			}
			wantMsg := msg
			if kind == acf.KindShort {
				wantMsg.Timestamp = 0 // short encoding carries no timestamp field
			}
			if len(frame.Messages) != 1 {
				t.Fatalf("timed=%v kind=%v: len(Messages) = %d, want 1", timed, kind, len(frame.Messages))
			}
			if !reflect.DeepEqual(frame.Messages[0], wantMsg) {
				t.Errorf("timed=%v kind=%v: message mismatch:\n got  %+v\n want %+v",
					timed, kind, frame.Messages[0], wantMsg)
			}
			if frame.Header.StreamID != hdr.StreamID || frame.Header.Timed != hdr.Timed {
				t.Errorf("timed=%v kind=%v: header mismatch: %+v", timed, kind, frame.Header)
			}
		}
	}
}

// TestFrame_MultipleMessagesPerFrame verifies EncodeFrame/DecodeFrame walk
// an AVTPDU payload as a sequence of independently-addressed ACF messages,
// per TC18 §12.9.1.1 ("An RCP frame may include multiple ACF-types
// (requests)."), rather than assuming exactly one.
func TestFrame_MultipleMessagesPerFrame(t *testing.T) {
	hdr := avtp.Header{StreamIDValid: true, StreamID: testStreamID()}
	msgs := []acf.Message{
		{Kind: acf.KindShort, ByteBusID: 1, TransactionNum: 10, Control: acf.FlagRead, Body: []byte("aa")},
		{Kind: acf.KindLong, ByteBusID: 2, TransactionNum: 11, Control: acf.FlagWrite, MTV: true, Timestamp: 0x1122334455667788, Body: []byte("bbbbbbb")},
		{Kind: acf.KindShort, ByteBusID: 3, TransactionNum: 12, Control: acf.FlagRead},
	}

	b, err := acf.EncodeFrame(hdr, msgs...)
	if err != nil {
		t.Fatalf("EncodeFrame: %v", err)
	}
	frame, err := acf.DecodeFrame(b)
	if err != nil {
		t.Fatalf("DecodeFrame: %v", err)
	}
	if len(frame.Messages) != len(msgs) {
		t.Fatalf("len(Messages) = %d, want %d", len(frame.Messages), len(msgs))
	}
	for i, want := range msgs {
		got := frame.Messages[i]
		if got.ByteBusID != want.ByteBusID || got.TransactionNum != want.TransactionNum ||
			got.Kind != want.Kind || !reflect.DeepEqual(got.Body, want.Body) {
			t.Errorf("message %d = %+v, want %+v", i, got, want)
		}
	}
}

// TestDecodeFrame_EmptyPayload verifies a zero-length AVTPDU payload decodes
// to a Frame with zero Messages rather than erroring — TC18 §12.9.1.1 frames
// a multi-message payload as "zero or more" ACF messages.
func TestDecodeFrame_EmptyPayload(t *testing.T) {
	hdrBytes, err := avtp.EncodeHeader(avtp.Header{StreamIDValid: true, StreamID: testStreamID()})
	if err != nil {
		t.Fatalf("EncodeHeader: %v", err)
	}
	frame, err := acf.DecodeFrame(hdrBytes)
	if err != nil {
		t.Fatalf("DecodeFrame(empty payload): %v", err)
	}
	if len(frame.Messages) != 0 {
		t.Errorf("len(Messages) = %d, want 0", len(frame.Messages))
	}
}

func TestDecodeFrame_LengthMismatch(t *testing.T) {
	b, err := acf.EncodeFrame(avtp.Header{StreamID: testStreamID()}, acf.Message{Kind: acf.KindShort, Body: []byte("x")})
	if err != nil {
		t.Fatalf("EncodeFrame: %v", err)
	}
	// Append a stray trailing byte the header's DataLength doesn't account for.
	corrupt := append(b, 0x00)
	if _, err := acf.DecodeFrame(corrupt); !errors.Is(err, acf.ErrFrameLengthMismatch) {
		t.Errorf("DecodeFrame(extra byte) = %v, want ErrFrameLengthMismatch", err)
	}
}
