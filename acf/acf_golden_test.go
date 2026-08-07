//fusa:test REQ-AVTP-016

package acf_test

import (
	"bytes"
	"testing"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
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
// byte_bus_id 0x10, transaction 1, Read set with the acknowledge bit of EVT
// set, a 4-byte read size, no body.
//
// The 8-byte RCP message breaks down as: byte0 acf_msg_type=0x0E<<1|len-top-
// bit=0x1C, byte1 acf_msg_length(quadlets)=0x02 (8 bytes = 2 quadlets),
// byte2 pad=0/mtv=0/rsv=0/byte_bus_id-top3=0, byte3 byte_bus_id-low8=0x10,
// byte4 evt=0x8(ack)<<4|hs=0|cs=0=0x80, byte5 transaction_num=0x01, byte6
// op=0(read)/rsp=0/err=0/ms=0/read_size-top4=0, byte7 read_size-low8=0x04.
var goldenUntimedShortRead = []byte{
	// AVTPDU NTSCF header, 12 bytes, laid out by hand from TC18 §11.1 p.22
	// Figure 6 ("NTSCF-Header Version 0") — one quadlet then stream_id:
	//
	//   octet 0        subtype                        = 0x82 (per Figure 6's
	//                                                   own "subtype(0x82)")
	//   octet 1 bit 8  sv                             = 1
	//           bits 9-11  version                    = 0
	//           bit 12     r (reserved)               = 0
	//           bits 13-15 ntscf_data_length[10:8]    = 0
	//                      -> 1 000 0 000            = 0x80
	//   octet 2        ntscf_data_length[7:0]         = 8 -> 0x08
	//   octet 3        sequence_num                   = 1 -> 0x01
	//   octets 4-11    stream_id (MAC || suffix)
	0x82, 0x80, 0x08, 0x01,
	0x02, 0x11, 0x22, 0x33, 0x44, 0x55, 0x00, 0x01,
	// RCP message (8 bytes, no body) — see breakdown above.
	0x1C, 0x02, 0x00, 0x10, 0x80, 0x01, 0x00, 0x04,
}

// goldenTimedLongWrite is a presentation-timestamped (TSCF) AVTPDU carrying
// a long-form (ACF_GBB) write request: sequence 9, stream suffix 0x0002,
// AVTPDU timestamp 0x00112233 marked valid, targeting byte_bus_id 0x20,
// transaction 7, Write set, a 64-bit message_timestamp slot (MTV unset —
// this vector exercises the raw Timestamp passthrough, not the
// conditional-request encoding), and a 4-byte body.
//
// The 20-byte RCP message breaks down as: byte0 acf_msg_type=0x0D<<1|
// len-top-bit=0x1A, byte1 acf_msg_length(quadlets)=0x05 (20 bytes = 5
// quadlets), byte2 pad=0/mtv=0/rsv=0/byte_bus_id-top3=0, byte3
// byte_bus_id-low8=0x20, bytes4-11 message_timestamp=0x0102030405060708,
// byte12 evt=0/hs=0/cs=0=0x00, byte13 transaction_num=0x07, byte14
// op=1(write)/rsp=0/err=0/ms=0/read_size-top4=0=0x80, byte15
// read_size-low8=0x00, bytes16-19 body=DE AD BE EF.
var goldenTimedLongWrite = []byte{
	// AVTPDU TSCF header, 24 bytes, laid out by hand from TC18 §11.1 p.22
	// Figure 5 ("TSCF-Header Version 0") — six quadlets:
	//
	//   octet 0        subtype                     = 0x05 (per Figure 5's
	//                                                own "subtype(0x05)")
	//   octet 1 bit 8      sv                      = 1
	//           bits 9-11  version                 = 0
	//           bit 12     mr (media clock restart)= 0
	//           bits 13-14 rsv                     = 0
	//           bit 15     tv (timestamp valid)    = 1
	//                      -> 1 000 0 00 1        = 0x81
	//   octet 2        sequence_num                = 9 -> 0x09
	//   octet 3 bits 24-30 reserved                = 0
	//           bit 31     tu (timestamp uncertain)= 0 -> 0x00
	//   octets 4-11    stream_id (MAC || suffix)
	//   octets 12-15   avtp_timestamp              = 0x00112233
	//   octets 16-19   "Format specific" reserved  = 0
	//   octets 20-21   stream_data_length          = 20 -> 0x0014
	//   octets 22-23   reserved                    = 0
	0x05, 0x81, 0x09, 0x00,
	0x02, 0x11, 0x22, 0x33, 0x44, 0x55, 0x00, 0x02,
	0x00, 0x11, 0x22, 0x33,
	0x00, 0x00, 0x00, 0x00,
	0x00, 0x14, 0x00, 0x00,
	// RCP message (20 bytes) — see breakdown above.
	0x1A, 0x05, 0x00, 0x20,
	0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
	0x00, 0x07, 0x80, 0x00,
	0xDE, 0xAD, 0xBE, 0xEF,
}

func TestGolden_UntimedShortRead(t *testing.T) {
	hdr := avtp.Header{
		Timed:         false,
		StreamIDValid: true,
		SequenceNum:   1,
		StreamID:      avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55}, 0x0001),
	}
	msg := acf.Message{
		Kind:              acf.KindShort,
		ByteBusID:         avtp.ByteBusID(0x10),
		TransactionNum:    avtp.TransactionNum(1),
		Control:           acf.FlagRead,
		EVT:               0x08, // evt[3] request-acknowledge bit
		ReadSizeOrSegment: 4,
	}

	got, err := acf.EncodeFrame(hdr, msg)
	if err != nil {
		t.Fatalf("EncodeFrame: %v", err)
	}
	if !bytes.Equal(got, goldenUntimedShortRead) {
		t.Fatalf("encoded bytes changed:\n got  % X\n want % X", got, goldenUntimedShortRead)
	}

	frame, err := acf.DecodeFrame(goldenUntimedShortRead)
	if err != nil {
		t.Fatalf("DecodeFrame(golden): %v", err)
	}
	if len(frame.Messages) != 1 {
		t.Fatalf("len(Messages) = %d, want 1", len(frame.Messages))
	}
	if frame.Header.SequenceNum != 1 || frame.Messages[0].ByteBusID != 0x10 {
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
	msg := acf.Message{
		Kind:           acf.KindLong,
		ByteBusID:      avtp.ByteBusID(0x20),
		TransactionNum: avtp.TransactionNum(7),
		Control:        acf.FlagWrite,
		Timestamp:      0x0102030405060708,
		Body:           []byte{0xDE, 0xAD, 0xBE, 0xEF},
	}

	got, err := acf.EncodeFrame(hdr, msg)
	if err != nil {
		t.Fatalf("EncodeFrame: %v", err)
	}
	if !bytes.Equal(got, goldenTimedLongWrite) {
		t.Fatalf("encoded bytes changed:\n got  % X\n want % X", got, goldenTimedLongWrite)
	}

	frame, err := acf.DecodeFrame(goldenTimedLongWrite)
	if err != nil {
		t.Fatalf("DecodeFrame(golden): %v", err)
	}
	if frame.Header.Disposition(true) != avtp.DispositionScheduled {
		t.Errorf("Disposition = %v, want DispositionScheduled", frame.Header.Disposition(true))
	}
	if frame.Header.Disposition(false) != avtp.DispositionDrop {
		t.Errorf("Disposition(no time-sync) = %v, want DispositionDrop", frame.Header.Disposition(false))
	}
	if len(frame.Messages) != 1 {
		t.Fatalf("len(Messages) = %d, want 1", len(frame.Messages))
	}
	if !bytes.Equal(frame.Messages[0].Body, []byte{0xDE, 0xAD, 0xBE, 0xEF}) {
		t.Errorf("decoded golden vector body = % X, want DE AD BE EF", frame.Messages[0].Body)
	}
}
