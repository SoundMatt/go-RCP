//fusa:test REQ-CANEP-006
//fusa:test REQ-CANEP-007
//fusa:test REQ-CANEP-008
//fusa:test REQ-CANEP-009

package can_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/can"
	"github.com/SoundMatt/go-RCP/regmap"
)

func writeReq(f can.Frame) acf.Message {
	return acf.Message{
		Kind:      acf.KindShort,
		ByteBusID: avtp.ByteBusID(1),
		Control:   acf.FlagWrite,
		Body:      can.EncodeFrame(f),
	}
}

func readReq() acf.Message {
	return acf.Message{
		Kind:      acf.KindShort,
		ByteBusID: avtp.ByteBusID(1),
		Control:   acf.FlagRead,
	}
}

// recordingTransport is a Transport test double that records every
// transmitted Frame.
type recordingTransport struct{ got []can.Frame }

func (r *recordingTransport) Transmit(f can.Frame) error {
	r.got = append(r.got, f)
	return nil
}

// TestHandleRequest_RequiresReadOrWriteWrongEndpointOrAccess checks a
// request with neither Read nor Write set, one addressed to the wrong
// endpoint, and one from a stream with no access grant are all rejected
// (REQ-CANEP-006).
func TestHandleRequest_RequiresReadOrWriteWrongEndpointOrAccess(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if err := ep.Configure(root, can.Config{Enabled: true, NominalBitrateKbps: 500}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	neither := acf.Message{Kind: acf.KindShort, ByteBusID: avtp.ByteBusID(1)}
	if _, err := ep.HandleRequest(root, neither); !errors.Is(err, can.ErrRequestMustReadOrWrite) {
		t.Errorf("HandleRequest(neither flag) err = %v, want ErrRequestMustReadOrWrite", err)
	}

	wrongAddr := readReq()
	wrongAddr.ByteBusID = 2
	if _, err := ep.HandleRequest(root, wrongAddr); !errors.Is(err, can.ErrWrongEndpoint) {
		t.Errorf("HandleRequest(wrong addr) err = %v, want ErrWrongEndpoint", err)
	}

	stranger := avtp.NewStreamID([6]byte{0x04, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE}, 9)
	if _, err := ep.HandleRequest(stranger, readReq()); !errors.Is(err, regmap.ErrAccessDenied) {
		t.Errorf("HandleRequest(no grant) err = %v, want regmap.ErrAccessDenied", err)
	}
}

// TestHandleRequest_RejectsDisabledBus checks a request against an
// unconfigured/disabled bus is rejected (REQ-CANEP-007).
func TestHandleRequest_RejectsDisabledBus(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if _, err := ep.HandleRequest(root, readReq()); !errors.Is(err, can.ErrNotConfigured) {
		t.Errorf("HandleRequest(disabled bus) err = %v, want ErrNotConfigured", err)
	}
}

// TestHandleRequest_WriteTransmitsThroughConfiguredTransport checks a write
// request is validated, echoed back on success, and handed to the
// configured Transport (or silently accepted with none set) (REQ-CANEP-008).
func TestHandleRequest_WriteTransmitsThroughConfiguredTransport(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if err := ep.Configure(root, can.Config{Enabled: true, NominalBitrateKbps: 500}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	f := can.Frame{Format: can.FormatClassical, ID: 0x123, Data: []byte{0xDE, 0xAD}}

	// Default: accepted, no Transport.
	resp, err := ep.HandleRequest(root, writeReq(f))
	if err != nil {
		t.Fatalf("HandleRequest(write, no transport): %v", err)
	}
	got, err := can.DecodeFrame(resp.Body)
	if err != nil {
		t.Fatalf("DecodeFrame(response): %v", err)
	}
	if got.ID != f.ID {
		t.Errorf("HandleRequest(write) echoed ID = %#x, want %#x", got.ID, f.ID)
	}

	// Configured Transport receives the transmitted Frame.
	tr := &recordingTransport{}
	ep.SetTransport(tr)
	if _, err := ep.HandleRequest(root, writeReq(f)); err != nil {
		t.Fatalf("HandleRequest(write, transport): %v", err)
	}
	if len(tr.got) != 1 || tr.got[0].ID != f.ID {
		t.Errorf("Transport.Transmit calls = %+v, want one call with ID %#x", tr.got, f.ID)
	}

	// An invalid frame (payload too large for Classical) is rejected before
	// ever reaching Transport.
	bad := can.Frame{Format: can.FormatClassical, Data: make([]byte, 9)}
	if _, err := ep.HandleRequest(root, writeReq(bad)); !errors.Is(err, can.ErrPayloadTooLarge) {
		t.Errorf("HandleRequest(write, oversized classical) err = %v, want ErrPayloadTooLarge", err)
	}
	if len(tr.got) != 1 {
		t.Errorf("Transport.Transmit calls after rejected write = %d, want still 1 (unchanged)", len(tr.got))
	}
}

// TestHandleRequest_ReadReturnsLastReceivedOrErrNoFrameReceived checks a
// read request returns whatever SetReceivedFrame last recorded, and fails
// explicitly when nothing has been received yet (REQ-CANEP-009).
func TestHandleRequest_ReadReturnsLastReceivedOrErrNoFrameReceived(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if err := ep.Configure(root, can.Config{Enabled: true, NominalBitrateKbps: 500}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	if _, err := ep.HandleRequest(root, readReq()); !errors.Is(err, can.ErrNoFrameReceived) {
		t.Errorf("HandleRequest(read, nothing received) err = %v, want ErrNoFrameReceived", err)
	}

	want := can.Frame{Format: can.FormatFD, ID: 0x7AB, BitRateSwitch: true, Data: []byte{0x01, 0x02}}
	ep.SetReceivedFrame(want)

	resp, err := ep.HandleRequest(root, readReq())
	if err != nil {
		t.Fatalf("HandleRequest(read): %v", err)
	}
	got, err := can.DecodeFrame(resp.Body)
	if err != nil {
		t.Fatalf("DecodeFrame(response): %v", err)
	}
	if got.ID != want.ID || got.Format != want.Format {
		t.Errorf("HandleRequest(read) = %+v, want %+v", got, want)
	}
}
