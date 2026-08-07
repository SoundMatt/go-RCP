//fusa:test REQ-CRC-001
//fusa:test REQ-CRC-002
//fusa:test REQ-CRC-003
//fusa:test REQ-CRC-004

package e2e_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/e2e"
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

// TestProtectVerify_RoundTrip checks Protect appends realPayload's 0-3 zero
// pad bytes followed by a Len-byte trailing CRC32 (TC18 §13.6 Figures
// 19/20's payload-then-pad-then-CRC wire order) and Verify strips and
// validates it, returning the original message on success (REQ-CRC-002).
func TestProtectVerify_RoundTrip(t *testing.T) {
	stream := testStream()
	m := baseMessage()

	protected := e2e.Protect(stream, m)
	// m.Body is 3 bytes (baseMessage), so Protect must insert 1 zero pad
	// byte before the trailing CRC to round up to a whole quadlet.
	wantPad := 1
	wantLen := len(m.Body) + wantPad + e2e.Len
	if len(protected.Body) != wantLen {
		t.Fatalf("Protect(Body) length = %d, want %d (payload=%d + pad=%d + CRC=%d)", len(protected.Body), wantLen, len(m.Body), wantPad, e2e.Len)
	}
	gotPad := protected.Body[len(m.Body) : len(protected.Body)-e2e.Len]
	for i, b := range gotPad {
		if b != 0 {
			t.Errorf("Protect(Body) pad byte %d = %#02x, want 0x00", i, b)
		}
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

// TestProtect_Figure19ByteOrder reproduces TC18 §13.6 Figure 19's worked
// ACF_ABB example byte-for-byte: a 6-byte payload, 2 zero pad bytes (to
// round up to a whole quadlet), then the 4-byte CRC32 trailer — header,
// payload, pad, CRC, in that order. This is the order a pre-fix Protect got
// backwards: it appended the CRC directly onto Body and left
// acf.EncodeMessage's own padding to run afterward, producing
// header-payload-CRC-pad instead. 20 bytes total (8-byte ACF_ABB header + 6
// payload + 2 pad + 4 CRC) = 5 quadlets, i.e. acf_msg_length 0x05, exactly
// as Figure 19 states (REQ-CRC-002).
func TestProtect_Figure19ByteOrder(t *testing.T) {
	stream := testStream()
	m := acf.Message{
		Kind:      acf.KindShort, // ACF_ABB
		ByteBusID: 1,
		Control:   acf.FlagWrite,
		Body:      []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06}, // 6-byte payload
	}
	protected := e2e.Protect(stream, m)

	wire, err := acf.EncodeMessage(protected)
	if err != nil {
		t.Fatalf("EncodeMessage: %v", err)
	}
	if len(wire) != 20 {
		t.Fatalf("encoded length = %d bytes, want 20 (Figure 19: 8 header + 6 payload + 2 pad + 4 CRC)", len(wire))
	}

	quadlets := (uint16(wire[0]&0x01) << 8) | uint16(wire[1])
	if quadlets != 0x05 {
		t.Errorf("acf_msg_length = %#02x, want 0x05 (Figure 19)", quadlets)
	}
	// EncodeMessage's own wire pad field must be 0: Protect already
	// embedded the 2 real pad bytes inside Body, ahead of the CRC, so
	// EncodeMessage has nothing left of its own to pad.
	if pad := (wire[2] >> 6) & 0x03; pad != 0 {
		t.Errorf("wire pad field = %d, want 0 (the real pad is inside Body, before the CRC)", pad)
	}

	payload := wire[8:14]
	padBytes := wire[14:16]
	crcBytes := wire[16:20]

	if !bytes.Equal(payload, m.Body) {
		t.Errorf("wire payload = % X, want % X", payload, m.Body)
	}
	if !bytes.Equal(padBytes, []byte{0x00, 0x00}) {
		t.Errorf("wire pad bytes (offset 14:16) = % X, want 00 00", padBytes)
	}
	wantCRC := e2e.Compute(stream, m)
	var wantCRCBytes [4]byte
	binary.BigEndian.PutUint32(wantCRCBytes[:], wantCRC)
	if !bytes.Equal(crcBytes, wantCRCBytes[:]) {
		t.Errorf("wire CRC bytes (offset 16:20) = % X, want % X", crcBytes, wantCRCBytes)
	}

	// The full round trip through real wire bytes (not just the in-memory
	// acf.Message Protect returned) must recover the original message.
	decoded, err := acf.DecodeMessage(wire)
	if err != nil {
		t.Fatalf("DecodeMessage: %v", err)
	}
	inner, err := e2e.Verify(stream, decoded)
	if err != nil {
		t.Fatalf("Verify(decoded wire bytes): %v", err)
	}
	if !bytes.Equal(inner.Body, m.Body) {
		t.Errorf("Verify(decoded wire bytes).Body = % X, want % X", inner.Body, m.Body)
	}
}

// TestProtect_Figure20ByteOrder is Figure19's counterpart for TC18 §13.6
// Figure 20's worked ACF_GBB example: a 7-byte payload, 1 zero pad byte,
// then the 4-byte CRC32 trailer. 28 bytes total (16-byte ACF_GBB header,
// including its 8-byte message_timestamp slot, + 7 payload + 1 pad + 4 CRC)
// = 7 quadlets, i.e. acf_msg_length 0x07, exactly as Figure 20 states
// (REQ-CRC-002).
func TestProtect_Figure20ByteOrder(t *testing.T) {
	stream := testStream()
	m := acf.Message{
		Kind:      acf.KindLong, // ACF_GBB
		ByteBusID: 2,
		Control:   acf.FlagWrite,
		Timestamp: 0x0102030405060708,
		Body:      []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77}, // 7-byte payload
	}
	protected := e2e.Protect(stream, m)

	wire, err := acf.EncodeMessage(protected)
	if err != nil {
		t.Fatalf("EncodeMessage: %v", err)
	}
	if len(wire) != 28 {
		t.Fatalf("encoded length = %d bytes, want 28 (Figure 20: 16 header + 7 payload + 1 pad + 4 CRC)", len(wire))
	}

	quadlets := (uint16(wire[0]&0x01) << 8) | uint16(wire[1])
	if quadlets != 0x07 {
		t.Errorf("acf_msg_length = %#02x, want 0x07 (Figure 20)", quadlets)
	}
	if pad := (wire[2] >> 6) & 0x03; pad != 0 {
		t.Errorf("wire pad field = %d, want 0 (the real pad is inside Body, before the CRC)", pad)
	}

	payload := wire[16:23]
	padBytes := wire[23:24]
	crcBytes := wire[24:28]

	if !bytes.Equal(payload, m.Body) {
		t.Errorf("wire payload = % X, want % X", payload, m.Body)
	}
	if !bytes.Equal(padBytes, []byte{0x00}) {
		t.Errorf("wire pad byte (offset 23) = % X, want 00", padBytes)
	}
	wantCRC := e2e.Compute(stream, m)
	var wantCRCBytes [4]byte
	binary.BigEndian.PutUint32(wantCRCBytes[:], wantCRC)
	if !bytes.Equal(crcBytes, wantCRCBytes[:]) {
		t.Errorf("wire CRC bytes (offset 24:28) = % X, want % X", crcBytes, wantCRCBytes)
	}

	decoded, err := acf.DecodeMessage(wire)
	if err != nil {
		t.Fatalf("DecodeMessage: %v", err)
	}
	inner, err := e2e.Verify(stream, decoded)
	if err != nil {
		t.Fatalf("Verify(decoded wire bytes): %v", err)
	}
	if !bytes.Equal(inner.Body, m.Body) {
		t.Errorf("Verify(decoded wire bytes).Body = % X, want % X", inner.Body, m.Body)
	}
}
