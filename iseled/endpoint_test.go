//fusa:test REQ-ISELED-005
//fusa:test REQ-ISELED-006
//fusa:test REQ-ISELED-007
//fusa:test REQ-ISELED-008

package iseled_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/iseled"
	"github.com/SoundMatt/go-RCP/server"
)

func commandReq(cmd iseled.Command) avtp.Message {
	return avtp.Message{
		Kind:      avtp.KindShort,
		ByteBusID: avtp.ByteBusID(1),
		Control:   avtp.FlagWrite,
		Body:      iseled.EncodeCommand(cmd),
	}
}

// recordingTransport is a Transport test double that reverses each device's
// echoed Data, so tests can tell the configured Transport actually ran
// rather than the default loopback.
type recordingTransport struct{ calls int }

func (r *recordingTransport) Exchange(cmd iseled.Command) (iseled.AggregatedResponse, error) {
	r.calls++
	rx := make([]byte, len(cmd.Data))
	for i, b := range cmd.Data {
		rx[len(cmd.Data)-1-i] = b
	}
	return iseled.AggregatedResponse{{Address: cmd.Address, Data: rx}}, nil
}

// TestHandleRequest_RequiresWriteWrongEndpointOrAccess checks a request
// missing the Write flag, one addressed to the wrong endpoint, and one from a
// stream with no access grant are all rejected (REQ-ISELED-005).
func TestHandleRequest_RequiresWriteWrongEndpointOrAccess(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if err := ep.Configure(root, iseled.Config{Enabled: true, DeviceCount: 4}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	noWrite := avtp.Message{Kind: avtp.KindShort, ByteBusID: avtp.ByteBusID(1), Body: iseled.EncodeCommand(iseled.Command{})}
	if _, err := ep.HandleRequest(root, noWrite); !errors.Is(err, iseled.ErrRequestMustWrite) {
		t.Errorf("HandleRequest(no write flag) err = %v, want ErrRequestMustWrite", err)
	}

	wrongAddr := commandReq(iseled.Command{Address: 0})
	wrongAddr.ByteBusID = 2
	if _, err := ep.HandleRequest(root, wrongAddr); !errors.Is(err, iseled.ErrWrongEndpoint) {
		t.Errorf("HandleRequest(wrong addr) err = %v, want ErrWrongEndpoint", err)
	}

	stranger := avtp.NewStreamID([6]byte{0x05, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE}, 9)
	if _, err := ep.HandleRequest(stranger, commandReq(iseled.Command{Address: 0})); !errors.Is(err, server.ErrAccessDenied) {
		t.Errorf("HandleRequest(no grant) err = %v, want server.ErrAccessDenied", err)
	}
}

// TestHandleRequest_RejectsDisabledChainAndOutOfRangeAddress checks a
// command against an unconfigured chain, and one addressed past the
// configured DeviceCount, are both rejected (REQ-ISELED-006).
func TestHandleRequest_RejectsDisabledChainAndOutOfRangeAddress(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if _, err := ep.HandleRequest(root, commandReq(iseled.Command{Address: 0})); !errors.Is(err, iseled.ErrChainNotConfigured) {
		t.Errorf("HandleRequest(disabled chain) err = %v, want ErrChainNotConfigured", err)
	}

	if err := ep.Configure(root, iseled.Config{Enabled: true, DeviceCount: 4}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if _, err := ep.HandleRequest(root, commandReq(iseled.Command{Address: 4})); !errors.Is(err, iseled.ErrDeviceAddressOutOfRange) {
		t.Errorf("HandleRequest(address out of range) err = %v, want ErrDeviceAddressOutOfRange", err)
	}
}

// TestHandleRequest_TargetedLoopbackAndTransport checks a targeted command
// echoes through the default loopback (single device response) or the
// configured Transport, and queues a command-complete trigger
// (REQ-ISELED-007).
func TestHandleRequest_TargetedLoopbackAndTransport(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if err := ep.Configure(root, iseled.Config{Enabled: true, DeviceCount: 4}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	resp, err := ep.HandleRequest(root, commandReq(iseled.Command{Address: 2, Data: []byte{0x01, 0x02}}))
	if err != nil {
		t.Fatalf("HandleRequest(loopback): %v", err)
	}
	got, err := iseled.DecodeAggregatedResponse(resp.Body)
	if err != nil {
		t.Fatalf("DecodeAggregatedResponse: %v", err)
	}
	if len(got) != 1 || got[0].Address != 2 || !bytes.Equal(got[0].Data, []byte{0x01, 0x02}) {
		t.Errorf("HandleRequest(targeted loopback) = %+v, want one entry echoing device 2", got)
	}
	if triggers := ep.DrainTriggers(); len(triggers) != 1 || triggers[0].ResponseCount != 1 {
		t.Errorf("DrainTriggers() after targeted command = %+v, want 1 event with ResponseCount 1", triggers)
	}

	tr := &recordingTransport{}
	ep.SetTransport(tr)
	resp, err = ep.HandleRequest(root, commandReq(iseled.Command{Address: 1, Data: []byte{0x01, 0x02, 0x03}}))
	if err != nil {
		t.Fatalf("HandleRequest(transport): %v", err)
	}
	got, err = iseled.DecodeAggregatedResponse(resp.Body)
	if err != nil {
		t.Fatalf("DecodeAggregatedResponse: %v", err)
	}
	if len(got) != 1 || !bytes.Equal(got[0].Data, []byte{0x03, 0x02, 0x01}) {
		t.Errorf("HandleRequest(transport) = %+v, want reversed 03 02 01", got)
	}
	if tr.calls != 1 {
		t.Errorf("Transport.Exchange calls = %d, want 1", tr.calls)
	}
}

// TestHandleRequest_BroadcastAggregatesEveryDevice checks a
// DeviceBroadcast command's default loopback response carries one entry per
// configured device (REQ-ISELED-008).
func TestHandleRequest_BroadcastAggregatesEveryDevice(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if err := ep.Configure(root, iseled.Config{Enabled: true, DeviceCount: 3}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	resp, err := ep.HandleRequest(root, commandReq(iseled.Command{Address: iseled.DeviceBroadcast, Data: []byte{0x7A}}))
	if err != nil {
		t.Fatalf("HandleRequest(broadcast): %v", err)
	}
	got, err := iseled.DecodeAggregatedResponse(resp.Body)
	if err != nil {
		t.Fatalf("DecodeAggregatedResponse: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("HandleRequest(broadcast) entry count = %d, want 3", len(got))
	}
	for i, r := range got {
		if r.Address != uint8(i) || !bytes.Equal(r.Data, []byte{0x7A}) {
			t.Errorf("entry %d = %+v, want Address %d Data [0x7A]", i, r, i)
		}
	}
}
