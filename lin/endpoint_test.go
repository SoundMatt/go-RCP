//fusa:test REQ-LINEP-003
//fusa:test REQ-LINEP-004
//fusa:test REQ-LINEP-005
//fusa:test REQ-LINEP-006

package lin_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/lin"
	"github.com/SoundMatt/go-RCP/regmap"
)

func transferReq(tx []byte) acf.Message {
	return acf.Message{
		Kind:      acf.KindShort,
		ByteBusID: avtp.ByteBusID(1),
		Control:   acf.FlagWrite,
		Body:      lin.EncodeTransferRequest(tx),
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
// stream with no access grant are all rejected (REQ-LINEP-003).
func TestHandleRequest_RequiresWriteWrongEndpointOrAccess(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if err := ep.Configure(root, lin.Config{Enabled: true, BaudRate: 19200}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	noWrite := acf.Message{Kind: acf.KindShort, ByteBusID: avtp.ByteBusID(1), Body: lin.EncodeTransferRequest(nil)}
	if _, err := ep.HandleRequest(root, noWrite); !errors.Is(err, lin.ErrRequestMustWrite) {
		t.Errorf("HandleRequest(no write flag) err = %v, want ErrRequestMustWrite", err)
	}

	wrongAddr := transferReq(nil)
	wrongAddr.ByteBusID = 2
	if _, err := ep.HandleRequest(root, wrongAddr); !errors.Is(err, lin.ErrWrongEndpoint) {
		t.Errorf("HandleRequest(wrong addr) err = %v, want ErrWrongEndpoint", err)
	}

	stranger := avtp.NewStreamID([6]byte{0x03, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE}, 9)
	if _, err := ep.HandleRequest(stranger, transferReq(nil)); !errors.Is(err, regmap.ErrAccessDenied) {
		t.Errorf("HandleRequest(no grant) err = %v, want regmap.ErrAccessDenied", err)
	}
}

// TestHandleRequest_RejectsDisabledBus checks a transfer against an
// unconfigured/disabled bus is rejected (REQ-LINEP-004).
func TestHandleRequest_RejectsDisabledBus(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if _, err := ep.HandleRequest(root, transferReq([]byte{0x01, 0x02})); !errors.Is(err, lin.ErrBusNotConfigured) {
		t.Errorf("HandleRequest(disabled bus) err = %v, want ErrBusNotConfigured", err)
	}
}

// TestHandleRequest_UsesConfiguredTransport checks a configured Transport
// performs the exchange, and defaults to loopback with none set, and queues a
// transaction-complete trigger with the resulting byte count (REQ-LINEP-005,
// REQ-LINEP-006).
func TestHandleRequest_UsesConfiguredTransport(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if err := ep.Configure(root, lin.Config{Enabled: true, BaudRate: 9600}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	// Default loopback.
	resp, err := ep.HandleRequest(root, transferReq([]byte{0x50, 0x01}))
	if err != nil {
		t.Fatalf("HandleRequest(loopback): %v", err)
	}
	if !bytes.Equal(lin.DecodeTransferResponse(resp.Body), []byte{0x50, 0x01}) {
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
	if !bytes.Equal(lin.DecodeTransferResponse(resp.Body), []byte{0x03, 0x02, 0x01}) {
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
	if !bytes.Equal(lin.DecodeTransferResponse(resp.Body), []byte{0xAA}) {
		t.Errorf("HandleRequest(restored loopback) response = % X, want AA", resp.Body)
	}
	ep.DrainTriggers() // clear the restored-loopback transfer's own trigger

	if again := ep.DrainTriggers(); again != nil {
		t.Errorf("DrainTriggers() after full drain = %+v, want nil", again)
	}
}
