//fusa:test REQ-UDP-015
//fusa:test REQ-UDP-016
//fusa:test REQ-UDP-017

package udp

import (
	"bytes"
	"encoding/binary"
	"net"
	"strconv"
	"testing"
)

// TestPrependEncapSeq_ByteLayout verifies prependEncapSeq lays out the
// 4-byte big-endian sequence number directly ahead of the AVTPDU bytes,
// with no gap or reordering (REQ-UDP-015).
func TestPrependEncapSeq_ByteLayout(t *testing.T) {
	avtpdu := []byte{0x82, 0x00, 0x01, 0xAA, 0xBB}
	got := prependEncapSeq(0x01020304, avtpdu)

	want := []byte{0x01, 0x02, 0x03, 0x04, 0x82, 0x00, 0x01, 0xAA, 0xBB}
	if !bytes.Equal(got, want) {
		t.Fatalf("prependEncapSeq = % X, want % X", got, want)
	}
}

// TestPrependStripEncapSeq_RoundTrip verifies stripEncapSeq recovers
// exactly the seq value and AVTPDU bytes prependEncapSeq was given, for a
// range of inputs including an empty AVTPDU (REQ-UDP-015).
func TestPrependStripEncapSeq_RoundTrip(t *testing.T) {
	cases := []struct {
		seq    uint32
		avtpdu []byte
	}{
		{0, nil},
		{1, []byte{}},
		{42, []byte{0x82, 0x00}},
		{0xFFFFFFFF, bytes.Repeat([]byte{0xAB}, 300)},
	}
	for _, c := range cases {
		wire := prependEncapSeq(c.seq, c.avtpdu)
		gotSeq, gotRest, err := stripEncapSeq(wire)
		if err != nil {
			t.Fatalf("stripEncapSeq(seq=%d): unexpected error: %v", c.seq, err)
		}
		if gotSeq != c.seq {
			t.Errorf("stripEncapSeq(seq=%d): seq = %d, want %d", c.seq, gotSeq, c.seq)
		}
		if !bytes.Equal(gotRest, c.avtpdu) {
			t.Errorf("stripEncapSeq(seq=%d): rest = % X, want % X", c.seq, gotRest, c.avtpdu)
		}
	}
}

// TestStripEncapSeq_ShortBuffer verifies stripEncapSeq rejects a buffer too
// short to hold the 4-byte field, rather than panicking or silently
// returning a truncated value (REQ-UDP-015).
func TestStripEncapSeq_ShortBuffer(t *testing.T) {
	for n := 0; n < AnnexJEncapSeqLen; n++ {
		buf := make([]byte, n)
		_, _, err := stripEncapSeq(buf)
		if err != ErrShortBuffer {
			t.Errorf("stripEncapSeq(%d bytes): err = %v, want ErrShortBuffer", n, err)
		}
	}
}

// TestEncapSeq_Monotonic verifies successive prependEncapSeq calls driven
// by an incrementing counter (the same pattern Controller.Request and
// Server.serve use via their own atomic.Uint32 fields) produce a strictly
// increasing sequence of wire values (REQ-UDP-016).
func TestEncapSeq_Monotonic(t *testing.T) {
	var seq uint32
	var last uint32
	for i := 0; i < 5; i++ {
		seq++
		wire := prependEncapSeq(seq, []byte{0x01})
		got := binary.BigEndian.Uint32(wire[:4])
		if i > 0 && got != last+1 {
			t.Errorf("iteration %d: encap seq = %d, want %d", i, got, last+1)
		}
		last = got
	}
}

// TestResolveAnnexJAddr_DefaultsControlPort verifies resolveAnnexJAddr
// applies AnnexJControlPort when addr names a host with no port at all
// (REQ-UDP-017).
func TestResolveAnnexJAddr_DefaultsControlPort(t *testing.T) {
	got, err := resolveAnnexJAddr("127.0.0.1")
	if err != nil {
		t.Fatalf("resolveAnnexJAddr: %v", err)
	}
	if got.Port != AnnexJControlPort {
		t.Errorf("Port = %d, want %d (AnnexJControlPort)", got.Port, AnnexJControlPort)
	}
}

// TestResolveAnnexJAddr_ExplicitPortHonored verifies resolveAnnexJAddr
// leaves a caller-specified port — including 0, this package's own
// ephemeral-port test convention — completely unchanged (REQ-UDP-017).
func TestResolveAnnexJAddr_ExplicitPortHonored(t *testing.T) {
	cases := []int{0, 9999, AnnexJContinuousPort}
	for _, port := range cases {
		got, err := resolveAnnexJAddr("127.0.0.1:" + strconv.Itoa(port))
		if err != nil {
			t.Fatalf("resolveAnnexJAddr(port=%d): %v", port, err)
		}
		if got.Port != port {
			t.Errorf("Port = %d, want %d", got.Port, port)
		}
	}
}

// TestDefaultAnnexJPort_ZeroDefaults verifies defaultAnnexJPort substitutes
// AnnexJControlPort for a zero-Port *net.UDPAddr, and leaves IP/Zone intact
// (REQ-UDP-017).
func TestDefaultAnnexJPort_ZeroDefaults(t *testing.T) {
	in := &net.UDPAddr{IP: net.ParseIP("192.0.2.1"), Port: 0, Zone: "eth0"}
	got := defaultAnnexJPort(in)
	if got.Port != AnnexJControlPort {
		t.Errorf("Port = %d, want %d", got.Port, AnnexJControlPort)
	}
	if !got.IP.Equal(in.IP) || got.Zone != in.Zone {
		t.Errorf("defaultAnnexJPort changed IP/Zone: got %+v, from %+v", got, in)
	}
}

// TestDefaultAnnexJPort_NonZeroUnchanged verifies defaultAnnexJPort leaves
// an already-specified nonzero port (and the *net.UDPAddr itself) alone
// (REQ-UDP-017).
func TestDefaultAnnexJPort_NonZeroUnchanged(t *testing.T) {
	in := &net.UDPAddr{IP: net.ParseIP("192.0.2.1"), Port: 4242}
	got := defaultAnnexJPort(in)
	if got != in {
		t.Errorf("defaultAnnexJPort replaced a non-zero-port addr: got %+v, want the same pointer %+v", got, in)
	}
}
