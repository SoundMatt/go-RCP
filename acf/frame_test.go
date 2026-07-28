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
				Control:           acf.FlagWrite | acf.FlagAck,
				ReadSizeOrSegment: 0,
				Timestamp:         0xFEEDFACECAFEBEEF,
				Body:              []byte("payload"),
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
			if !reflect.DeepEqual(frame.Message, wantMsg) {
				t.Errorf("timed=%v kind=%v: message mismatch:\n got  %+v\n want %+v",
					timed, kind, frame.Message, wantMsg)
			}
			if frame.Header.StreamID != hdr.StreamID || frame.Header.Timed != hdr.Timed {
				t.Errorf("timed=%v kind=%v: header mismatch: %+v", timed, kind, frame.Header)
			}
		}
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
