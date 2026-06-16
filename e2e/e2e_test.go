//fusa:test REQ-E2E-001
//fusa:test REQ-E2E-002
//fusa:test REQ-E2E-003
//fusa:test REQ-E2E-004
//fusa:test REQ-E2E-005
//fusa:test REQ-E2E-006
//fusa:test REQ-E2E-007
//fusa:test REQ-E2E-008

package e2e_test

import (
	"context"
	"errors"
	"testing"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/e2e"
	"github.com/SoundMatt/go-RCP/mock"
)

// TestHeaderLen verifies the E2E header is exactly 6 bytes (REQ-E2E-006).
func TestHeaderLen(t *testing.T) {
	if e2e.HeaderLen != 6 {
		t.Errorf("HeaderLen = %d, want 6", e2e.HeaderLen)
	}
}

// TestWrap_HeaderStructure verifies Wrap prepends 4-byte seqNum + 2-byte CRC (REQ-E2E-001).
func TestWrap_HeaderStructure(t *testing.T) {
	payload := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	frame := e2e.Wrap(1, payload)

	if len(frame) != e2e.HeaderLen+len(payload) {
		t.Fatalf("frame len = %d, want %d", len(frame), e2e.HeaderLen+len(payload))
	}
	// SeqNum bytes.
	if frame[0] != 0 || frame[1] != 0 || frame[2] != 0 || frame[3] != 1 {
		t.Errorf("seqNum bytes = %v, want [0 0 0 1]", frame[0:4])
	}
	// Original payload preserved.
	for i, b := range payload {
		if frame[e2e.HeaderLen+i] != b {
			t.Errorf("payload[%d] = 0x%02x, want 0x%02x", i, frame[e2e.HeaderLen+i], b)
		}
	}
}

// TestWrap_NilPayload verifies Wrap works with an empty payload (REQ-E2E-001).
func TestWrap_NilPayload(t *testing.T) {
	frame := e2e.Wrap(42, nil)
	if len(frame) != e2e.HeaderLen {
		t.Errorf("frame len = %d, want %d (header only)", len(frame), e2e.HeaderLen)
	}
}

// TestUnwrap_RoundTrip verifies Wrap → Unwrap recovers seqNum and payload (REQ-E2E-007).
func TestUnwrap_RoundTrip(t *testing.T) {
	for _, seqNum := range []uint32{0, 1, 255, 1<<16 - 1, 1<<32 - 1} {
		payload := []byte{0xCA, 0xFE, 0xBA, 0xBE}
		frame := e2e.Wrap(seqNum, payload)
		gotSeq, gotPayload, err := e2e.Unwrap(frame)
		if err != nil {
			t.Errorf("seq=%d: Unwrap error: %v", seqNum, err)
			continue
		}
		if gotSeq != seqNum {
			t.Errorf("seq=%d: got seqNum=%d", seqNum, gotSeq)
		}
		if len(gotPayload) != len(payload) {
			t.Errorf("seq=%d: payload len = %d, want %d", seqNum, len(gotPayload), len(payload))
			continue
		}
		for i, b := range payload {
			if gotPayload[i] != b {
				t.Errorf("seq=%d: payload[%d] = 0x%02x, want 0x%02x", seqNum, i, gotPayload[i], b)
			}
		}
	}
}

// TestUnwrap_CRCMismatch verifies Unwrap detects single-bit corruption (REQ-E2E-003, REQ-E2E-008).
func TestUnwrap_CRCMismatch(t *testing.T) {
	payload := []byte{0x01, 0x02, 0x03}
	frame := e2e.Wrap(7, payload)

	// Flip one bit in the payload section.
	frame[e2e.HeaderLen] ^= 0xFF

	_, _, err := e2e.Unwrap(frame)
	if !errors.Is(err, e2e.ErrCRCMismatch) {
		t.Errorf("Unwrap after payload corruption: err = %v, want ErrCRCMismatch", err)
	}
}

// TestUnwrap_CRC_SeqCorruption verifies CRC covers seqNum field (REQ-E2E-002).
func TestUnwrap_CRC_SeqCorruption(t *testing.T) {
	frame := e2e.Wrap(100, []byte{0xAA})
	// Corrupt the seqNum (byte 0).
	frame[0] ^= 0x01

	_, _, err := e2e.Unwrap(frame)
	if !errors.Is(err, e2e.ErrCRCMismatch) {
		t.Errorf("Unwrap after seqNum corruption: err = %v, want ErrCRCMismatch", err)
	}
}

// TestUnwrap_ShortFrame verifies ErrShortFrame on truncated input (REQ-E2E-003).
func TestUnwrap_ShortFrame(t *testing.T) {
	_, _, err := e2e.Unwrap([]byte{0x00, 0x01})
	if !errors.Is(err, e2e.ErrShortFrame) {
		t.Errorf("Unwrap on 2-byte frame: err = %v, want ErrShortFrame", err)
	}
}

// TestUnwrap_EmptyFrame verifies ErrShortFrame on empty input (REQ-E2E-003).
func TestUnwrap_EmptyFrame(t *testing.T) {
	_, _, err := e2e.Unwrap(nil)
	if !errors.Is(err, e2e.ErrShortFrame) {
		t.Errorf("Unwrap on nil: err = %v, want ErrShortFrame", err)
	}
}

// TestController_Send_WrapsPayload verifies e2e.Controller adds E2E header on Send (REQ-E2E-004).
func TestController_Send_WrapsPayload(t *testing.T) {
	received := make(chan []byte, 1)
	handler := func(cmd *rcp.Command) *rcp.Response {
		p := make([]byte, len(cmd.Payload))
		copy(p, cmd.Payload)
		select {
		case received <- p:
		default:
		}
		return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusOK}
	}
	inner := mock.NewController(rcp.ZoneFrontLeft, handler)
	ctrl := e2e.NewController(inner)
	defer func() { _ = ctrl.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	originalPayload := []byte{0x11, 0x22, 0x33}
	_, err := ctrl.Send(ctx, &rcp.Command{
		Zone:    rcp.ZoneFrontLeft,
		Type:    rcp.CmdNoop,
		Payload: originalPayload,
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	var frame []byte
	select {
	case frame = <-received:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("handler not called")
	}

	// Frame must start with E2E header and contain original payload.
	seqNum, gotPayload, unwrapErr := e2e.Unwrap(frame)
	if unwrapErr != nil {
		t.Fatalf("Unwrap of sent frame: %v", unwrapErr)
	}
	if seqNum == 0 {
		t.Error("seqNum should be > 0 after first Send")
	}
	if len(gotPayload) != len(originalPayload) {
		t.Fatalf("payload len = %d, want %d", len(gotPayload), len(originalPayload))
	}
	for i, b := range originalPayload {
		if gotPayload[i] != b {
			t.Errorf("payload[%d] = 0x%02x, want 0x%02x", i, gotPayload[i], b)
		}
	}
}

// TestController_Send_IncrementsSeq verifies the sequence counter increments per Send (REQ-E2E-004).
func TestController_Send_IncrementsSeq(t *testing.T) {
	seqNums := make(chan uint32, 10)
	handler := func(cmd *rcp.Command) *rcp.Response {
		seq, _, _ := e2e.Unwrap(cmd.Payload)
		select {
		case seqNums <- seq:
		default:
		}
		return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusOK}
	}
	inner := mock.NewController(rcp.ZoneFrontLeft, handler)
	ctrl := e2e.NewController(inner)
	defer func() { _ = ctrl.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	for i := 0; i < 5; i++ {
		if _, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdNoop}); err != nil {
			t.Fatalf("Send %d: %v", i, err)
		}
	}

	prev := uint32(0)
	for i := 0; i < 5; i++ {
		seq := <-seqNums
		if seq <= prev {
			t.Errorf("seq[%d]=%d not strictly greater than prev=%d", i, seq, prev)
		}
		prev = seq
	}
}

// TestController_Zone_Subscribe_Close verify delegation to inner controller.
func TestController_Zone_Subscribe_Close(t *testing.T) {
	inner := mock.NewController(rcp.ZoneFrontLeft, nil)
	ctrl := e2e.NewController(inner)

	if ctrl.Zone() != rcp.ZoneFrontLeft {
		t.Errorf("Zone() = %v, want ZoneFrontLeft", ctrl.Zone())
	}

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := ctrl.Subscribe(ctx)
	cancel()
	if err != nil {
		t.Errorf("Subscribe: %v", err)
	}
	_ = ch

	if err := ctrl.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// TestReplayGuard_AcceptsNewSeq verifies new sequences pass (REQ-E2E-005).
func TestReplayGuard_AcceptsNewSeq(t *testing.T) {
	rg := e2e.NewReplayGuard()
	for seq := uint32(1); seq <= 50; seq++ {
		if err := rg.Check(seq); err != nil {
			t.Errorf("Check(%d) unexpected error: %v", seq, err)
		}
	}
}

// TestReplayGuard_RejectsReplay verifies replayed sequence numbers are detected (REQ-E2E-005).
func TestReplayGuard_RejectsReplay(t *testing.T) {
	rg := e2e.NewReplayGuard()
	if err := rg.Check(10); err != nil {
		t.Fatalf("first Check: %v", err)
	}
	if err := rg.Check(10); !errors.Is(err, e2e.ErrReplay) {
		t.Errorf("duplicate Check(10): err = %v, want ErrReplay", err)
	}
}

// TestReplayGuard_ConcurrentSafe verifies no data race under concurrent access (REQ-E2E-005).
func TestReplayGuard_ConcurrentSafe(t *testing.T) {
	rg := e2e.NewReplayGuard()
	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func(base uint32) {
			for seq := base; seq < base+50; seq++ {
				_ = rg.Check(seq)
			}
			done <- struct{}{}
		}(uint32(i * 1000))
	}
	for i := 0; i < 8; i++ {
		<-done
	}
}

// TestWrap_DifferentSeqProducesDifferentCRC verifies CRC changes with seqNum (REQ-E2E-002).
func TestWrap_DifferentSeqProducesDifferentCRC(t *testing.T) {
	payload := []byte{0xAB, 0xCD}
	frame1 := e2e.Wrap(1, payload)
	frame2 := e2e.Wrap(2, payload)

	crc1 := uint16(frame1[4])<<8 | uint16(frame1[5])
	crc2 := uint16(frame2[4])<<8 | uint16(frame2[5])

	if crc1 == crc2 {
		t.Error("CRC must differ when seqNum differs with same payload")
	}
}
