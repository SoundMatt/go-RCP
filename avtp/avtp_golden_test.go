//fusa:test REQ-AVTP-016

package avtp_test

import (
	"bytes"
	"testing"

	"github.com/SoundMatt/go-RCP/avtp"
)

// ── REQ-AVTP-016 (golden-vector half): frozen server-request byte layouts ──
//
// These fixtures pin the exact wire bytes this package produces for two
// representative server requests, so later milestones (RC Server lifecycle
// v0.58.0, discovery v0.59.0, and every endpoint-type phase after it) can
// regression-test against a frozen encoding rather than re-deriving it from
// EncodeFrame's current behaviour. A change to either byte layout below is a
// deliberate wire-format break, not a refactor — it must be caught here
// first.

// goldenUntimedShortRead is an untimed (NTSCF) AVTPDU carrying a short-form
// (ACF_ABB) read request: sequence 1, stream suffix 0x0001, targeting
// byte_bus_id 0x10, transaction 1, Ack+Read set, a 4-byte read size, no body.
var goldenUntimedShortRead = []byte{
	// AVTPDU header (13 bytes): subtype=NTSCF, flags(sv=1), seq=1,
	// data_length=10, stream_id=02:11:22:33:44:55/0001
	0x82, 0x80, 0x01, 0x00, 0x0A, 0x02, 0x11, 0x22, 0x33, 0x44, 0x55, 0x00, 0x01,
	// RCP message (10 bytes, no body): kind=ACF_ABB, pad=0, length=10,
	// byte_bus_id=0x10, transaction_num=1, control=Ack|Read, read_size=4
	0x01, 0x00, 0x00, 0x0A, 0x10, 0x00, 0x01, 0xC0, 0x00, 0x04,
}

// goldenTimedLongWrite is a presentation-timestamped (TSCF) AVTPDU carrying
// a long-form (ACF_GBB) write request: sequence 9, stream suffix 0x0002,
// AVTPDU timestamp 0x00112233 marked valid, targeting byte_bus_id 0x20,
// transaction 7, Write set, a 64-bit message timestamp, and a 4-byte body.
var goldenTimedLongWrite = []byte{
	// AVTPDU header (17 bytes): subtype=TSCF, flags(sv=1, ts_status=Valid),
	// seq=9, data_length=22, stream_id=02:11:22:33:44:55/0002,
	// timestamp=0x00112233
	0x83, 0x84, 0x09, 0x00, 0x16, 0x02, 0x11, 0x22, 0x33, 0x44, 0x55, 0x00, 0x02,
	0x00, 0x11, 0x22, 0x33,
	// RCP message (22 bytes): kind=ACF_GBB, pad=0, length=22,
	// byte_bus_id=0x20, transaction_num=7, control=Write, read/segment=0,
	// message timestamp=0x0102030405060708, body=DE AD BE EF
	0x02, 0x00, 0x00, 0x16, 0x20, 0x00, 0x07, 0x20, 0x00, 0x00,
	0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
	0xDE, 0xAD, 0xBE, 0xEF,
}

func TestGolden_UntimedShortRead(t *testing.T) {
	hdr := avtp.Header{
		Timed:         false,
		StreamIDValid: true,
		SequenceNum:   1,
		StreamID:      avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55}, 0x0001),
	}
	msg := avtp.Message{
		Kind:              avtp.KindShort,
		ByteBusID:         avtp.ByteBusID(0x10),
		TransactionNum:    avtp.TransactionNum(1),
		Control:           avtp.FlagRead | avtp.FlagAck,
		ReadSizeOrSegment: 4,
	}

	got, err := avtp.EncodeFrame(hdr, msg)
	if err != nil {
		t.Fatalf("EncodeFrame: %v", err)
	}
	if !bytes.Equal(got, goldenUntimedShortRead) {
		t.Fatalf("encoded bytes changed:\n got  % X\n want % X", got, goldenUntimedShortRead)
	}

	frame, err := avtp.DecodeFrame(goldenUntimedShortRead)
	if err != nil {
		t.Fatalf("DecodeFrame(golden): %v", err)
	}
	if frame.Header.SequenceNum != 1 || frame.Message.ByteBusID != 0x10 {
		t.Errorf("decoded golden vector mismatch: %+v", frame)
	}
}

func TestGolden_TimedLongWrite(t *testing.T) {
	hdr := avtp.Header{
		Timed:           true,
		StreamIDValid:   true,
		SequenceNum:     9,
		StreamID:        avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55}, 0x0002),
		Timestamp:       0x00112233,
		TimestampStatus: avtp.TimestampValid,
	}
	msg := avtp.Message{
		Kind:           avtp.KindLong,
		ByteBusID:      avtp.ByteBusID(0x20),
		TransactionNum: avtp.TransactionNum(7),
		Control:        avtp.FlagWrite,
		Timestamp:      0x0102030405060708,
		Body:           []byte{0xDE, 0xAD, 0xBE, 0xEF},
	}

	got, err := avtp.EncodeFrame(hdr, msg)
	if err != nil {
		t.Fatalf("EncodeFrame: %v", err)
	}
	if !bytes.Equal(got, goldenTimedLongWrite) {
		t.Fatalf("encoded bytes changed:\n got  % X\n want % X", got, goldenTimedLongWrite)
	}

	frame, err := avtp.DecodeFrame(goldenTimedLongWrite)
	if err != nil {
		t.Fatalf("DecodeFrame(golden): %v", err)
	}
	if frame.Header.Disposition(true) != avtp.DispositionScheduled {
		t.Errorf("Disposition = %v, want DispositionScheduled", frame.Header.Disposition(true))
	}
	if frame.Header.Disposition(false) != avtp.DispositionDrop {
		t.Errorf("Disposition(no time-sync) = %v, want DispositionDrop", frame.Header.Disposition(false))
	}
	if !bytes.Equal(frame.Message.Body, []byte{0xDE, 0xAD, 0xBE, 0xEF}) {
		t.Errorf("decoded golden vector body = % X, want DE AD BE EF", frame.Message.Body)
	}
}
