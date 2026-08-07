//fusa:test REQ-FRAG-002
//fusa:test REQ-FRAG-007

package fragment_test

import (
	"bytes"
	"testing"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/can"
	"github.com/SoundMatt/go-RCP/v9/fragment"
	"github.com/SoundMatt/go-RCP/v9/gpio"
	"github.com/SoundMatt/go-RCP/v9/regmap"
	"github.com/SoundMatt/go-RCP/v9/server"
	"github.com/SoundMatt/go-RCP/v9/uart"
)

// splitAndReassemble runs msg through fragment.Split at maxBody and back
// through a fresh fragment.Reassembler, failing the test on any error, and
// returns the reassembled Message.
func splitAndReassemble(t *testing.T, stream avtp.StreamID, msg acf.Message, maxBody int) acf.Message {
	t.Helper()
	segs, err := fragment.Split(msg, maxBody)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(segs) < 2 {
		t.Fatalf("test setup: want multiple segments at maxBody=%d, got %d", maxBody, len(segs))
	}
	re := fragment.NewReassembler(fragment.Config{})
	for i, seg := range segs {
		if _, addErr := re.Add(stream, seg); addErr != nil {
			t.Fatalf("Add(segment %d): %v", i, addErr)
		}
	}
	out, err := re.Finish(fragment.KeyOf(stream, msg))
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	return out
}

// TestIntegration_CANXLPayload checks a maximal, XLMaxPayload-sized CAN XL
// Frame (ROADMAP.md Milestone 51's own "cannot fit in a single frame on
// realistic MTUs" motivating example for this milestone) survives
// Split/Reassembler and decodes back to the original Frame via
// can.DecodeFrame — the concrete consumer ROADMAP.md Milestone 52 names
// first (REQ-FRAG-002, REQ-FRAG-007).
func TestIntegration_CANXLPayload(t *testing.T) {
	stream := testStream()
	data := bytes.Repeat([]byte{0x5A}, can.XLMaxPayload)
	frame := can.Frame{
		Format:   can.FormatXL,
		Extended: true,
		ID:       0x1FFFFFFF,
		XL:       can.XLHeader{SDT: 1, VCID: 2, AF: 0xDEADBEEF},
		Data:     data,
	}
	if err := frame.Validate(); err != nil {
		t.Fatalf("test setup: Frame.Validate: %v", err)
	}

	original := acf.Message{
		Kind:      acf.KindShort,
		ByteBusID: 1,
		Control:   acf.FlagWrite,
		Body:      can.EncodeFrame(frame),
	}

	// DefaultMaxSegmentBody comfortably fits an XL-format frame's own
	// EncodeFrame overhead plus XLMaxPayload bytes in two segments; this
	// test still asserts the multi-segment path is exercised.
	out := splitAndReassemble(t, stream, original, fragment.DefaultMaxSegmentBody)

	decoded, err := can.DecodeFrame(out.Body)
	if err != nil {
		t.Fatalf("can.DecodeFrame(reassembled): %v", err)
	}
	if decoded.Format != frame.Format || decoded.ID != frame.ID || decoded.XL != frame.XL || !bytes.Equal(decoded.Data, frame.Data) {
		t.Errorf("reassembled CAN XL frame = %+v, want %+v", decoded, frame)
	}
}

// TestIntegration_UARTFIFODrain checks a large UART RX read-response body
// (uart's own "fragmented delivery of a partial FIFO drain" completion
// path, ROADMAP.md Milestone 48) survives Split/Reassembler and decodes
// back via uart.DecodeReadResponse — the second concrete consumer
// ROADMAP.md Milestone 52 names (REQ-FRAG-002, REQ-FRAG-007).
func TestIntegration_UARTFIFODrain(t *testing.T) {
	stream := testStream()
	drained := bytes.Repeat([]byte{0x42}, 4000)
	respBody := uart.EncodeReadResponse(true, drained)

	original := acf.Message{
		Kind:      acf.KindShort,
		ByteBusID: 2,
		Control:   acf.FlagResponse,
		Body:      respBody,
	}

	out := splitAndReassemble(t, stream, original, fragment.DefaultMaxSegmentBody)

	complete, data, err := uart.DecodeReadResponse(out.Body)
	if err != nil {
		t.Fatalf("uart.DecodeReadResponse(reassembled): %v", err)
	}
	if !complete || !bytes.Equal(data, drained) {
		t.Errorf("reassembled UART read response = (complete=%v, %d bytes), want (true, %d bytes matching)", complete, len(data), len(drained))
	}
}

// TestIntegration_DiscoveryRegisterMap checks a large server register-map
// discovery read (server.Server.ReadDiscovery — ROADMAP.md Milestone 46,
// flagged by Milestone 52 as growing past one frame's payload as a
// deployment's endpoint count grows) survives Split/Reassembler intact —
// the third concrete consumer ROADMAP.md Milestone 52 names
// (REQ-FRAG-002, REQ-FRAG-007).
func TestIntegration_DiscoveryRegisterMap(t *testing.T) {
	stream := testStream()
	s := server.NewServer()
	if err := s.ClaimRoot(stream); err != nil {
		t.Fatalf("ClaimRoot: %v", err)
	}
	// Enough declared endpoints that the encoded register map exceeds a
	// small per-segment budget, modelling Milestone 52's "grows with
	// endpoint count" concern without needing a slow, unrealistically large
	// fixture.
	types := []regmap.EndpointType{gpio.EndpointType, can.EndpointType, uart.EndpointType}
	for i := 0; i < 60; i++ {
		if err := s.AddEndpoint(stream, avtp.ByteBusID(i+1), types[i%len(types)]); err != nil {
			t.Fatalf("AddEndpoint(%d): %v", i, err)
		}
	}

	discovery := s.ReadDiscovery()
	original := acf.Message{Kind: acf.KindShort, Control: acf.FlagResponse, Body: discovery}

	const smallSegmentBudget = 64 // well under a real register map's size, to force multiple segments
	out := splitAndReassemble(t, stream, original, smallSegmentBudget)

	if !bytes.Equal(out.Body, discovery) {
		t.Errorf("reassembled discovery body length = %d, want %d matching the original register-map snapshot", len(out.Body), len(discovery))
	}
}
