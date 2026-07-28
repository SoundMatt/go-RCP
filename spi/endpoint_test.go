//fusa:test REQ-SPI-004
//fusa:test REQ-SPI-005
//fusa:test REQ-SPI-006
//fusa:test REQ-SPI-007
//fusa:test REQ-SPI-008

package spi_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/server"
	"github.com/SoundMatt/go-RCP/spi"
)

func transferReq(ch spi.Channel, tx []byte) avtp.Message {
	return avtp.Message{
		Kind:      avtp.KindShort,
		ByteBusID: avtp.ByteBusID(1),
		Control:   avtp.FlagWrite,
		Body:      spi.EncodeTransferRequest(ch, tx),
	}
}

// echoTransport is a Transport test double that returns tx reversed, so
// tests can tell the configured Transport actually ran rather than the
// default loopback.
type echoTransport struct{ calls int }

func (e *echoTransport) Transfer(tx []byte) ([]byte, error) {
	e.calls++
	rx := make([]byte, len(tx))
	for i, b := range tx {
		rx[len(tx)-1-i] = b
	}
	return rx, nil
}

// TestHandleRequest_DispatchesToConfiguredChannel checks a transfer request's
// sub-opcode channel byte reaches the correct configured channel, and a
// disabled/unconfigured channel is rejected (REQ-SPI-004).
func TestHandleRequest_DispatchesToConfiguredChannel(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if err := ep.SetChannelConfig(root, spi.Channel1, spi.ChannelConfig{Enabled: true, ClockHz: 1_000_000, Mode: spi.Mode0}); err != nil {
		t.Fatalf("SetChannelConfig: %v", err)
	}

	resp, err := ep.HandleRequest(root, transferReq(spi.Channel1, []byte{0xAA, 0xBB}))
	if err != nil {
		t.Fatalf("HandleRequest(configured channel): %v", err)
	}
	ch, rx, err := spi.DecodeTransferResponse(resp.Body)
	if err != nil {
		t.Fatalf("DecodeTransferResponse: %v", err)
	}
	if ch != spi.Channel1 || !bytes.Equal(rx, []byte{0xAA, 0xBB}) {
		t.Errorf("response = (channel %v, % X), want (Channel1, AA BB) [default loopback]", ch, rx)
	}

	if _, err := ep.HandleRequest(root, transferReq(spi.Channel0, []byte{0x01})); !errors.Is(err, spi.ErrChannelNotConfigured) {
		t.Errorf("HandleRequest(unconfigured channel) err = %v, want ErrChannelNotConfigured", err)
	}
}

// TestHandleRequest_RequiresWriteWrongEndpointOrAccess checks a request
// missing the Write flag, one addressed to the wrong endpoint, and one from
// a stream with no access grant are all rejected (REQ-SPI-005).
func TestHandleRequest_RequiresWriteWrongEndpointOrAccess(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if err := ep.SetChannelConfig(root, spi.Channel0, spi.ChannelConfig{Enabled: true, ClockHz: 1000, Mode: spi.Mode0}); err != nil {
		t.Fatalf("SetChannelConfig: %v", err)
	}

	noWrite := avtp.Message{Kind: avtp.KindShort, ByteBusID: avtp.ByteBusID(1), Body: spi.EncodeTransferRequest(spi.Channel0, nil)}
	if _, err := ep.HandleRequest(root, noWrite); !errors.Is(err, spi.ErrRequestMustWrite) {
		t.Errorf("HandleRequest(no write flag) err = %v, want ErrRequestMustWrite", err)
	}

	wrongAddr := transferReq(spi.Channel0, nil)
	wrongAddr.ByteBusID = 2
	if _, err := ep.HandleRequest(root, wrongAddr); !errors.Is(err, spi.ErrWrongEndpoint) {
		t.Errorf("HandleRequest(wrong addr) err = %v, want ErrWrongEndpoint", err)
	}

	stranger := avtp.NewStreamID([6]byte{0x02, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE}, 9)
	if _, err := ep.HandleRequest(stranger, transferReq(spi.Channel0, nil)); !errors.Is(err, server.ErrAccessDenied) {
		t.Errorf("HandleRequest(no grant) err = %v, want server.ErrAccessDenied", err)
	}
}

// TestHandleRequest_UsesConfiguredTransport checks a configured Transport
// performs the exchange, and defaults to loopback with none set
// (REQ-SPI-006).
func TestHandleRequest_UsesConfiguredTransport(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if err := ep.SetChannelConfig(root, spi.Channel3, spi.ChannelConfig{Enabled: true, ClockHz: 1000, Mode: spi.Mode0}); err != nil {
		t.Fatalf("SetChannelConfig: %v", err)
	}
	tr := &echoTransport{}
	if err := ep.SetTransport(spi.Channel3, tr); err != nil {
		t.Fatalf("SetTransport: %v", err)
	}

	resp, err := ep.HandleRequest(root, transferReq(spi.Channel3, []byte{0x01, 0x02, 0x03}))
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	_, rx, err := spi.DecodeTransferResponse(resp.Body)
	if err != nil {
		t.Fatalf("DecodeTransferResponse: %v", err)
	}
	if !bytes.Equal(rx, []byte{0x03, 0x02, 0x01}) {
		t.Errorf("rx = % X, want reversed 03 02 01 (custom Transport)", rx)
	}
	if tr.calls != 1 {
		t.Errorf("Transport.Transfer calls = %d, want 1", tr.calls)
	}
}

// TestHandleRequest_QueuesTriggersInOrder checks every transfer queues a
// chip-select-assert, transfer-complete, chip-select-deassert trigger
// sequence with the correct byte count (REQ-SPI-007, REQ-SPI-008).
func TestHandleRequest_QueuesTriggersInOrder(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if err := ep.SetChannelConfig(root, spi.Channel4, spi.ChannelConfig{Enabled: true, ClockHz: 1000, Mode: spi.Mode0}); err != nil {
		t.Fatalf("SetChannelConfig: %v", err)
	}

	if _, err := ep.HandleRequest(root, transferReq(spi.Channel4, []byte{0x01, 0x02, 0x03})); err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}

	got := ep.DrainTriggers()
	want := []spi.TriggerEvent{
		{Kind: spi.TriggerChipSelectEdge, Channel: spi.Channel4, Asserted: true},
		{Kind: spi.TriggerChipSelectEdge, Channel: spi.Channel4, Asserted: false},
		{Kind: spi.TriggerTransferComplete, Channel: spi.Channel4, ByteCount: 3},
	}
	if len(got) != len(want) {
		t.Fatalf("DrainTriggers() = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("trigger[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}

	if again := ep.DrainTriggers(); again != nil {
		t.Errorf("DrainTriggers() after drain = %+v, want nil", again)
	}
}
