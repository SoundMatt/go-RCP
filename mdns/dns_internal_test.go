//fusa:test REQ-MDNS-002
//fusa:test REQ-MDNS-003
//fusa:test REQ-MDNS-004

package mdns

import (
	"errors"
	"testing"
)

// ── encodeName / decodeName ────────────────────────────────────────────────────

func TestEncodeName_Empty(t *testing.T) {
	for _, in := range []string{"", "."} {
		got := encodeName(in)
		if len(got) != 1 || got[0] != 0 {
			t.Errorf("encodeName(%q) = %v, want [0]", in, got)
		}
	}
}

func TestDecodeName_RoundTrip(t *testing.T) {
	const name = "zone-front-left._rcp._udp.local."
	msg := encodeName(name)
	got, off, err := decodeName(msg, 0)
	if err != nil {
		t.Fatalf("decodeName: %v", err)
	}
	if got != name {
		t.Errorf("decodeName = %q, want %q", got, name)
	}
	if off != len(msg) {
		t.Errorf("offset = %d, want %d", off, len(msg))
	}
}

func TestDecodeName_Compression(t *testing.T) {
	// Offset 0: name "a." (01 'a' 00). Offset 3: pointer back to offset 0.
	msg := []byte{0x01, 'a', 0x00, 0xC0, 0x00}
	got, off, err := decodeName(msg, 3)
	if err != nil {
		t.Fatalf("decodeName(pointer): %v", err)
	}
	if got != "a." {
		t.Errorf("decodeName = %q, want %q", got, "a.")
	}
	if off != 5 {
		t.Errorf("offset = %d, want 5 (just past the 2-byte pointer)", off)
	}
}

func TestDecodeName_Truncated(t *testing.T) {
	// A length byte promising 5 more bytes that are not present.
	if _, _, err := decodeName([]byte{0x05, 'a'}, 0); !errors.Is(err, errTruncated) {
		t.Errorf("decodeName(truncated label) err = %v, want errTruncated", err)
	}
	// Empty message.
	if _, _, err := decodeName([]byte{}, 0); !errors.Is(err, errTruncated) {
		t.Errorf("decodeName(empty) err = %v, want errTruncated", err)
	}
	// Pointer byte at the very end with no second byte.
	if _, _, err := decodeName([]byte{0xC0}, 0); !errors.Is(err, errTruncated) {
		t.Errorf("decodeName(half pointer) err = %v, want errTruncated", err)
	}
}

func TestDecodeName_BadForwardPointer(t *testing.T) {
	// Pointer at offset 0 referencing offset 2 (not strictly backward).
	if _, _, err := decodeName([]byte{0xC0, 0x02, 0x00}, 0); !errors.Is(err, errBadPointer) {
		t.Errorf("decodeName(forward pointer) err = %v, want errBadPointer", err)
	}
}

// ── decodeHeader ───────────────────────────────────────────────────────────────

func TestDecodeHeader_Short(t *testing.T) {
	if _, err := decodeHeader(make([]byte, 11)); !errors.Is(err, errTruncated) {
		t.Errorf("decodeHeader(11 bytes) err = %v, want errTruncated", err)
	}
}

// ── parseRRs round-trip and robustness ─────────────────────────────────────────

func TestParseRRs_AnnouncementRoundTrip(t *testing.T) {
	msg := buildAnnouncement(
		"zone-front-left._rcp._udp.local.",
		"zone-front-left.local.",
		[]byte{192, 168, 1, 10},
		9000,
		"zone=1",
	)
	rrs, err := parseRRs(msg)
	if err != nil {
		t.Fatalf("parseRRs: %v", err)
	}
	if len(rrs) != 4 { // PTR + SRV + TXT + A
		t.Fatalf("parsed %d RRs, want 4", len(rrs))
	}
	svcs := extractServices(rrs, msg)
	if len(svcs) != 1 {
		t.Fatalf("extractServices returned %d services, want 1", len(svcs))
	}
	if svcs[0].Zone != 1 {
		t.Errorf("Zone = %d, want 1", svcs[0].Zone)
	}
	if svcs[0].Addr != "192.168.1.10:9000" {
		t.Errorf("Addr = %q, want %q", svcs[0].Addr, "192.168.1.10:9000")
	}
}

func TestParseRRs_SkipsQuestionSection(t *testing.T) {
	// A PTR query carries one question and no answers; parseRRs must skip the
	// question cleanly and return an empty RR set.
	rrs, err := parseRRs(buildPTRQuery())
	if err != nil {
		t.Fatalf("parseRRs(query): %v", err)
	}
	if len(rrs) != 0 {
		t.Errorf("parsed %d RRs from a query, want 0", len(rrs))
	}
}

func TestParseRRs_TruncatedHeader(t *testing.T) {
	if _, err := parseRRs([]byte{0x00, 0x01}); !errors.Is(err, errTruncated) {
		t.Errorf("parseRRs(short) err = %v, want errTruncated", err)
	}
}

// FuzzParseRRs asserts the received-packet parsing path never panics on
// arbitrary or hostile input (REQ-MDNS robustness at the network trust boundary).
func FuzzParseRRs(f *testing.F) {
	f.Add(buildPTRQuery())
	f.Add(buildAnnouncement("i._rcp._udp.local.", "h.local.", []byte{1, 2, 3, 4}, 1, "zone=2"))
	f.Add([]byte{})
	f.Add(make([]byte, 12))
	f.Fuzz(func(t *testing.T, msg []byte) {
		rrs, err := parseRRs(msg)
		if err == nil {
			_ = extractServices(rrs, msg) // also exercise extraction on parsed RRs
		}
	})
}
