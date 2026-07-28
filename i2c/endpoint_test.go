//fusa:test REQ-I2C-004
//fusa:test REQ-I2C-005
//fusa:test REQ-I2C-006
//fusa:test REQ-I2C-007

package i2c_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/i2c"
	"github.com/SoundMatt/go-RCP/server"
)

func transferReq(tx []byte) avtp.Message {
	return avtp.Message{
		Kind:      avtp.KindShort,
		ByteBusID: avtp.ByteBusID(1),
		Control:   avtp.FlagWrite,
		Body:      i2c.EncodeTransferRequest(tx),
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

// TestHandleRequest_RequiresWriteWrongEndpointOrAccess checks a request
// missing the Write flag, one addressed to the wrong endpoint, and one from a
// stream with no access grant are all rejected (REQ-I2C-004).
func TestHandleRequest_RequiresWriteWrongEndpointOrAccess(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if err := ep.Configure(root, i2c.Config{Enabled: true, Speed: i2c.SpeedStandard}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	noWrite := avtp.Message{Kind: avtp.KindShort, ByteBusID: avtp.ByteBusID(1), Body: i2c.EncodeTransferRequest(nil)}
	if _, err := ep.HandleRequest(root, noWrite); !errors.Is(err, i2c.ErrRequestMustWrite) {
		t.Errorf("HandleRequest(no write flag) err = %v, want ErrRequestMustWrite", err)
	}

	wrongAddr := transferReq(nil)
	wrongAddr.ByteBusID = 2
	if _, err := ep.HandleRequest(root, wrongAddr); !errors.Is(err, i2c.ErrWrongEndpoint) {
		t.Errorf("HandleRequest(wrong addr) err = %v, want ErrWrongEndpoint", err)
	}

	stranger := avtp.NewStreamID([6]byte{0x02, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE}, 9)
	if _, err := ep.HandleRequest(stranger, transferReq(nil)); !errors.Is(err, server.ErrAccessDenied) {
		t.Errorf("HandleRequest(no grant) err = %v, want server.ErrAccessDenied", err)
	}
}

// TestHandleRequest_RejectsDisabledBus checks a transfer against an
// unconfigured/disabled bus is rejected (REQ-I2C-005).
func TestHandleRequest_RejectsDisabledBus(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if _, err := ep.HandleRequest(root, transferReq([]byte{0x50, 0x01})); !errors.Is(err, i2c.ErrBusNotConfigured) {
		t.Errorf("HandleRequest(disabled bus) err = %v, want ErrBusNotConfigured", err)
	}
}

// TestHandleRequest_UsesConfiguredTransport checks a configured Transport
// performs the exchange, and defaults to loopback with none set, and queues a
// transaction-complete trigger with the resulting byte count (REQ-I2C-006,
// REQ-I2C-007).
func TestHandleRequest_UsesConfiguredTransport(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if err := ep.Configure(root, i2c.Config{Enabled: true, Speed: i2c.SpeedFast}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	// Default loopback.
	resp, err := ep.HandleRequest(root, transferReq([]byte{0x50, 0x01}))
	if err != nil {
		t.Fatalf("HandleRequest(loopback): %v", err)
	}
	if !bytes.Equal(i2c.DecodeTransferResponse(resp.Body), []byte{0x50, 0x01}) {
		t.Errorf("HandleRequest(loopback) response = % X, want 50 01", resp.Body)
	}
	got := ep.DrainTriggers()
	if len(got) != 1 || got[0].ByteCount != 2 {
		t.Errorf("DrainTriggers() after loopback transfer = %+v, want 1 event with ByteCount 2", got)
	}

	// Configured Transport.
	tr := &echoTransport{}
	ep.SetTransport(tr)
	resp, err = ep.HandleRequest(root, transferReq([]byte{0x01, 0x02, 0x03}))
	if err != nil {
		t.Fatalf("HandleRequest(transport): %v", err)
	}
	if !bytes.Equal(i2c.DecodeTransferResponse(resp.Body), []byte{0x03, 0x02, 0x01}) {
		t.Errorf("HandleRequest(transport) response = % X, want reversed 03 02 01", resp.Body)
	}
	if tr.calls != 1 {
		t.Errorf("Transport.Transfer calls = %d, want 1", tr.calls)
	}
	got = ep.DrainTriggers()
	if len(got) != 1 || got[0].ByteCount != 3 {
		t.Errorf("DrainTriggers() after transport transfer = %+v, want 1 event with ByteCount 3", got)
	}

	// Restoring nil Transport reverts to loopback.
	ep.SetTransport(nil)
	resp, err = ep.HandleRequest(root, transferReq([]byte{0xAA}))
	if err != nil {
		t.Fatalf("HandleRequest(restored loopback): %v", err)
	}
	if !bytes.Equal(i2c.DecodeTransferResponse(resp.Body), []byte{0xAA}) {
		t.Errorf("HandleRequest(restored loopback) response = % X, want AA", resp.Body)
	}
	ep.DrainTriggers() // clear the restored-loopback transfer's own trigger

	if again := ep.DrainTriggers(); again != nil {
		t.Errorf("DrainTriggers() after full drain = %+v, want nil", again)
	}
}
