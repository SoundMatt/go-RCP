//fusa:test REQ-L2-001
//fusa:test REQ-L2-002
//fusa:test REQ-L2-003
//fusa:test REQ-L2-004

package l2_test

import (
	"bytes"
	"testing"

	"github.com/SoundMatt/go-RCP/l2"
)

func mac(b0 byte) [6]byte {
	return [6]byte{b0, 0x11, 0x22, 0x33, 0x44, 0x55}
}

// TestEncodeFrame_ByteLayout verifies EncodeFrame lays out dst MAC, src
// MAC, big-endian EtherType, then the AVTPDU bytes unchanged, with no
// encapsulation sequence number and no trailer (REQ-L2-001).
func TestEncodeFrame_ByteLayout(t *testing.T) {
	dst := [6]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06}
	src := [6]byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF}
	avtpdu := []byte{0x82, 0x00, 0x01, 0x02}

	got := l2.EncodeFrame(dst, src, avtpdu)
	want := []byte{
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, // dst
		0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, // src
		0x22, 0xF0, // EtherType, big-endian
		0x82, 0x00, 0x01, 0x02, // AVTPDU
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("EncodeFrame = % X, want % X", got, want)
	}
}

// TestEncodeDecodeFrame_RoundTrip verifies DecodeFrame recovers exactly the
// dst/src/avtpdu EncodeFrame was given, for a range of AVTPDU payload
// lengths including empty (REQ-L2-002).
func TestEncodeDecodeFrame_RoundTrip(t *testing.T) {
	dst := mac(0x01)
	src := mac(0x02)
	payloads := [][]byte{
		nil,
		{},
		{0x82},
		bytes.Repeat([]byte{0x5A}, 512),
	}
	for _, payload := range payloads {
		wire := l2.EncodeFrame(dst, src, payload)
		gotDst, gotSrc, gotPayload, err := l2.DecodeFrame(wire)
		if err != nil {
			t.Fatalf("DecodeFrame: unexpected error: %v", err)
		}
		if gotDst != dst {
			t.Errorf("dst = %v, want %v", gotDst, dst)
		}
		if gotSrc != src {
			t.Errorf("src = %v, want %v", gotSrc, src)
		}
		if !bytes.Equal(gotPayload, payload) {
			t.Errorf("payload = % X, want % X", gotPayload, payload)
		}
	}
}

// TestDecodeFrame_ShortBuffer verifies DecodeFrame rejects a buffer too
// short to hold a full Ethernet header, for every length short of
// l2.HeaderLen (REQ-L2-003).
func TestDecodeFrame_ShortBuffer(t *testing.T) {
	for n := 0; n < l2.HeaderLen; n++ {
		buf := make([]byte, n)
		_, _, _, err := l2.DecodeFrame(buf)
		if err != l2.ErrShortFrame {
			t.Errorf("DecodeFrame(%d bytes): err = %v, want ErrShortFrame", n, err)
		}
	}
}

// TestDecodeFrame_UnexpectedEtherType verifies DecodeFrame rejects a
// well-formed Ethernet header carrying any EtherType other than
// EtherTypeAVTP (REQ-L2-004).
func TestDecodeFrame_UnexpectedEtherType(t *testing.T) {
	wire := l2.EncodeFrame(mac(0x01), mac(0x02), []byte{0x82})
	// Overwrite the EtherType field (bytes 12-13) with something else, e.g.
	// 0x0800 (IPv4).
	wire[12], wire[13] = 0x08, 0x00

	_, _, _, err := l2.DecodeFrame(wire)
	if err != l2.ErrUnexpectedEtherType {
		t.Errorf("err = %v, want ErrUnexpectedEtherType", err)
	}
}
