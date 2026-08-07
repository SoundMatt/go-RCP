//fusa:test REQ-UART-004
//fusa:test REQ-UART-005
//fusa:test REQ-UART-006
//fusa:test REQ-UART-007
//fusa:test REQ-UART-008

package uart_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/regmap"
	"github.com/SoundMatt/go-RCP/v9/uart"
)

func writeReq(tx []byte) acf.Message {
	return acf.Message{
		Kind:      acf.KindShort,
		ByteBusID: avtp.ByteBusID(1),
		Control:   acf.FlagWrite,
		Body:      uart.EncodeWriteRequest(tx),
	}
}

func readReq(want uint16) acf.Message {
	return acf.Message{
		Kind:              acf.KindShort,
		ByteBusID:         avtp.ByteBusID(1),
		Control:           acf.FlagRead,
		ReadSizeOrSegment: want,
	}
}

func configured(t *testing.T) (*uart.Endpoint, avtp.StreamID) {
	t.Helper()
	ep, root := newDeclaredEndpoint(t)
	cfg := uart.Config{Enabled: true, BaudRate: 9600, DataBits: 8, Parity: uart.ParityNone, StopBits: uart.StopBitsOne}
	if err := ep.Configure(root, cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	return ep, root
}

// halvingTransport is a Transport test double that accepts only half of what
// it's handed (rounded down), so tests can tell the configured Transport
// actually ran rather than the default full-acceptance loopback.
type halvingTransport struct{ calls int }

func (h *halvingTransport) Write(tx []byte) (int, error) {
	h.calls++
	return len(tx) / 2, nil
}

// TestHandleRequest_RoutesReadWriteRejectsNeither checks HandleRequest
// dispatches Write to TX and Read to RX, and rejects a request with neither
// flag (REQ-UART-004).
func TestHandleRequest_RoutesReadWriteRejectsNeither(t *testing.T) {
	ep, root := configured(t)

	resp, err := ep.HandleRequest(root, writeReq([]byte{0x01, 0x02}))
	if err != nil {
		t.Fatalf("HandleRequest(write): %v", err)
	}
	n, err := uart.DecodeWriteResponse(resp.Body)
	if err != nil || n != 2 {
		t.Errorf("DecodeWriteResponse = (%d, %v), want (2, nil)", n, err)
	}

	resp, err = ep.HandleRequest(root, readReq(2))
	if err != nil {
		t.Fatalf("HandleRequest(read): %v", err)
	}
	complete, data, err := uart.DecodeReadResponse(resp.Body)
	if err != nil {
		t.Fatalf("DecodeReadResponse: %v", err)
	}
	if !complete || !bytes.Equal(data, []byte{0x01, 0x02}) {
		t.Errorf("read = (complete=%v, % X), want (true, 01 02) [TX loopback into RX]", complete, data)
	}

	noFlags := acf.Message{Kind: acf.KindShort, ByteBusID: avtp.ByteBusID(1)}
	if _, err := ep.HandleRequest(root, noFlags); !errors.Is(err, uart.ErrRequestMustReadOrWrite) {
		t.Errorf("HandleRequest(no flags) err = %v, want ErrRequestMustReadOrWrite", err)
	}
}

// TestHandleRequest_ReadMustBePayloadLess checks a read request carrying a
// nonempty body is rejected (REQ-UART-004).
func TestHandleRequest_ReadMustBePayloadLess(t *testing.T) {
	ep, root := configured(t)
	req := readReq(1)
	req.Body = []byte{0x00}
	if _, err := ep.HandleRequest(root, req); !errors.Is(err, uart.ErrReadRequestNotPayloadLess) {
		t.Errorf("HandleRequest(read with body) err = %v, want ErrReadRequestNotPayloadLess", err)
	}
}

// TestHandleRequest_WrongEndpointNoAccessNotConfigured checks a request
// addressed to a different byte_bus_id, one from a stream with no access
// grant, and one against a disabled endpoint are all rejected (REQ-UART-005).
func TestHandleRequest_WrongEndpointNoAccessNotConfigured(t *testing.T) {
	ep, root := configured(t)

	wrongAddr := writeReq(nil)
	wrongAddr.ByteBusID = 2
	if _, err := ep.HandleRequest(root, wrongAddr); !errors.Is(err, uart.ErrWrongEndpoint) {
		t.Errorf("HandleRequest(wrong addr) err = %v, want ErrWrongEndpoint", err)
	}

	stranger := avtp.NewStreamID([6]byte{0x02, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE}, 9)
	if _, err := ep.HandleRequest(stranger, writeReq(nil)); !errors.Is(err, regmap.ErrAccessDenied) {
		t.Errorf("HandleRequest(no grant) err = %v, want regmap.ErrAccessDenied", err)
	}

	unconfigured, root2 := newDeclaredEndpoint(t)
	if _, err := unconfigured.HandleRequest(root2, writeReq([]byte{0x01})); !errors.Is(err, uart.ErrUARTNotConfigured) {
		t.Errorf("HandleRequest(write, disabled) err = %v, want ErrUARTNotConfigured", err)
	}
	if _, err := unconfigured.HandleRequest(root2, readReq(1)); !errors.Is(err, uart.ErrUARTNotConfigured) {
		t.Errorf("HandleRequest(read, disabled) err = %v, want ErrUARTNotConfigured", err)
	}
}

// TestHandleRequest_UsesConfiguredTransport checks a configured Transport
// performs TX, and TX queues a TX-complete trigger with the accepted byte
// count (REQ-UART-006).
func TestHandleRequest_UsesConfiguredTransport(t *testing.T) {
	ep, root := configured(t)
	tr := &halvingTransport{}
	ep.SetTransport(tr)

	resp, err := ep.HandleRequest(root, writeReq([]byte{0x01, 0x02, 0x03, 0x04}))
	if err != nil {
		t.Fatalf("HandleRequest(write): %v", err)
	}
	n, err := uart.DecodeWriteResponse(resp.Body)
	if err != nil || n != 2 {
		t.Errorf("DecodeWriteResponse = (%d, %v), want (2, nil)", n, err)
	}
	if tr.calls != 1 {
		t.Errorf("Transport.Write calls = %d, want 1", tr.calls)
	}

	got := ep.DrainTriggers()
	if len(got) != 1 || got[0].Kind != uart.TriggerTXComplete || got[0].ByteCount != 2 {
		t.Errorf("DrainTriggers() after configured-Transport write = %+v, want 1 TXComplete event with ByteCount 2", got)
	}
}

// TestRead_DrainsFIFOAndReportsPartialOnUnderfill checks a read drains up to
// the requested count, reports complete=false and returns the smaller amount
// actually available when the FIFO can't fill the request, and complete=true
// once enough has arrived (REQ-UART-007).
func TestRead_DrainsFIFOAndReportsPartialOnUnderfill(t *testing.T) {
	ep, root := configured(t)
	ep.Receive([]byte{0xAA, 0xBB})

	resp, err := ep.HandleRequest(root, readReq(5))
	if err != nil {
		t.Fatalf("HandleRequest(read): %v", err)
	}
	complete, data, err := uart.DecodeReadResponse(resp.Body)
	if err != nil {
		t.Fatalf("DecodeReadResponse: %v", err)
	}
	if complete || !bytes.Equal(data, []byte{0xAA, 0xBB}) {
		t.Errorf("underfilled read = (complete=%v, % X), want (false, AA BB)", complete, data)
	}

	ep.Receive([]byte{0xCC, 0xDD, 0xEE})
	resp, err = ep.HandleRequest(root, readReq(3))
	if err != nil {
		t.Fatalf("HandleRequest(read 2): %v", err)
	}
	complete, data, err = uart.DecodeReadResponse(resp.Body)
	if err != nil {
		t.Fatalf("DecodeReadResponse: %v", err)
	}
	if !complete || !bytes.Equal(data, []byte{0xCC, 0xDD, 0xEE}) {
		t.Errorf("follow-up read = (complete=%v, % X), want (true, CC DD EE)", complete, data)
	}
}

// TestReceive_QueuesRXDataAvailableAndDrainClears checks Receive queues a
// TriggerRXDataAvailable event with the arrived byte count, and DrainTriggers
// returns queued events FIFO and clears the queue (REQ-UART-008).
func TestReceive_QueuesRXDataAvailableAndDrainClears(t *testing.T) {
	ep, _ := configured(t)
	ep.Receive([]byte{0x01})
	ep.Receive([]byte{0x02, 0x03})

	got := ep.DrainTriggers()
	if len(got) != 2 {
		t.Fatalf("DrainTriggers() = %+v, want 2 events", got)
	}
	if got[0].Kind != uart.TriggerRXDataAvailable || got[0].ByteCount != 1 {
		t.Errorf("trigger[0] = %+v, want {RXDataAvailable, 1}", got[0])
	}
	if got[1].Kind != uart.TriggerRXDataAvailable || got[1].ByteCount != 2 {
		t.Errorf("trigger[1] = %+v, want {RXDataAvailable, 2}", got[1])
	}
	if again := ep.DrainTriggers(); again != nil {
		t.Errorf("DrainTriggers() after drain = %+v, want nil", again)
	}
}
