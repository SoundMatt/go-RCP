//fusa:test REQ-UDS-001
//fusa:test REQ-UDS-002
//fusa:test REQ-UDS-003
//fusa:test REQ-UDS-004
//fusa:test REQ-UDS-005
//fusa:test REQ-UDS-006
//fusa:test REQ-UDS-007
//fusa:test REQ-UDS-008

package udsbr_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/server"
	"github.com/SoundMatt/go-RCP/udp"
	"github.com/SoundMatt/go-RCP/udsbr"
)

func clientStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55}, 1)
}

func serverStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE}, 1)
}

const testAddr = avtp.ByteBusID(1) // matches udsbr.DataIdentifier(0x01) via its low byte

type echoHandler struct{}

func (echoHandler) HandleRequest(_ avtp.StreamID, req acf.Message) (acf.Message, error) {
	return acf.Message{
		Kind:           req.Kind,
		ByteBusID:      req.ByteBusID,
		TransactionNum: req.TransactionNum,
		Control:        acf.FlagResponse | (req.Control & (acf.FlagRead | acf.FlagWrite)),
		Body:           req.Body,
	}, nil
}

func newHarness(t *testing.T) *udsbr.Server {
	t.Helper()
	router := udp.NewRouter(udp.NewEP0Handler(server.NewServer()), false)
	if err := router.Register(testAddr, echoHandler{}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	srv, err := udp.NewServer(serverStream(), "127.0.0.1:0", router)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	ctrl, err := udp.NewController(clientStream(), srv.Addr())
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	t.Cleanup(func() { _ = ctrl.Close() })
	return udsbr.NewServer(ctrl)
}

// TestBuildRequest_Layout BuildRequest produces a PDU with the correct SID,
// DID, and payload layout (REQ-UDS-001).
func TestBuildRequest_Layout(t *testing.T) {
	pdu := udsbr.BuildRequest(udsbr.SIDWriteDataByIdentifier, 0x1234, []byte{0xAA})
	want := []byte{udsbr.SIDWriteDataByIdentifier, 0x12, 0x34, 0xAA}
	if !bytes.Equal(pdu, want) {
		t.Errorf("BuildRequest = % X, want % X", pdu, want)
	}
}

// TestBuildPositiveResponse_Layout BuildPositiveResponse prefixes SID with
// +0x40 (REQ-UDS-002).
func TestBuildPositiveResponse_Layout(t *testing.T) {
	pdu := udsbr.BuildPositiveResponse(udsbr.SIDReadDataByIdentifier, 0x0001, []byte{0x05})
	want := []byte{udsbr.SIDReadDataByIdentifier + udsbr.SIDPositiveOffset, 0x00, 0x01, 0x05}
	if !bytes.Equal(pdu, want) {
		t.Errorf("BuildPositiveResponse = % X, want % X", pdu, want)
	}
}

// TestBuildNegativeResponse_Layout BuildNegativeResponse produces a 0x7F
// PDU with the echoed SID and NRC byte (REQ-UDS-003).
func TestBuildNegativeResponse_Layout(t *testing.T) {
	pdu := udsbr.BuildNegativeResponse(udsbr.SIDWriteDataByIdentifier, udsbr.NRCRequestOutOfRange)
	want := []byte{udsbr.SIDNegativeResponse, udsbr.SIDWriteDataByIdentifier, udsbr.NRCRequestOutOfRange}
	if !bytes.Equal(pdu, want) {
		t.Errorf("BuildNegativeResponse = % X, want % X", pdu, want)
	}
}

// TestHandle_WriteDispatchesToUpstream Handle forwards a
// WriteDataByIdentifier PDU's payload to the upstream controller as a write
// request, echoed back in the positive response (REQ-UDS-004).
func TestHandle_WriteDispatchesToUpstream(t *testing.T) {
	s := newHarness(t)
	pdu := udsbr.BuildRequest(udsbr.SIDWriteDataByIdentifier, udsbr.DataIdentifier(testAddr), []byte{0xDE, 0xAD})
	resp, err := s.Handle(context.Background(), pdu)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if resp[0] != udsbr.SIDWriteDataByIdentifier+udsbr.SIDPositiveOffset {
		t.Errorf("resp[0] = %#x, want positive WriteDataByIdentifier", resp[0])
	}
	if !bytes.Equal(resp[3:], []byte{0xDE, 0xAD}) {
		t.Errorf("resp data = % X, want % X", resp[3:], []byte{0xDE, 0xAD})
	}
}

// TestHandle_UnsupportedSID Handle returns ErrNegativeResponse for an
// unsupported service ID (REQ-UDS-005).
func TestHandle_UnsupportedSID(t *testing.T) {
	s := newHarness(t)
	pdu := udsbr.BuildRequest(0x99, udsbr.DataIdentifier(testAddr), nil)
	_, err := s.Handle(context.Background(), pdu)
	if !errors.Is(err, udsbr.ErrNegativeResponse) {
		t.Errorf("err = %v, want ErrNegativeResponse", err)
	}
}

// TestHandle_ReadDispatchesToUpstream Handle forwards a ReadDataByIdentifier
// PDU as a read request and returns the endpoint's response body
// (REQ-UDS-006).
func TestHandle_ReadDispatchesToUpstream(t *testing.T) {
	s := newHarness(t)
	pdu := udsbr.BuildRequest(udsbr.SIDReadDataByIdentifier, udsbr.DataIdentifier(testAddr), nil)
	resp, err := s.Handle(context.Background(), pdu)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if resp[0] != udsbr.SIDReadDataByIdentifier+udsbr.SIDPositiveOffset {
		t.Errorf("resp[0] = %#x, want positive ReadDataByIdentifier", resp[0])
	}
}

// TestHandle_PDUTooShort Handle returns ErrPDUTooShort for a PDU shorter
// than 3 bytes (REQ-UDS-007).
func TestHandle_PDUTooShort(t *testing.T) {
	s := newHarness(t)
	_, err := s.Handle(context.Background(), []byte{0x01, 0x02})
	if !errors.Is(err, udsbr.ErrPDUTooShort) {
		t.Errorf("err = %v, want ErrPDUTooShort", err)
	}
}

// TestServer_Close_RejectsHandle Close prevents further Handle calls and is
// idempotent (REQ-UDS-008).
func TestServer_Close_RejectsHandle(t *testing.T) {
	s := newHarness(t)
	s.Close()
	s.Close() // must not panic

	pdu := udsbr.BuildRequest(udsbr.SIDReadDataByIdentifier, udsbr.DataIdentifier(testAddr), nil)
	if _, err := s.Handle(context.Background(), pdu); !errors.Is(err, udsbr.ErrClosed) {
		t.Errorf("err = %v, want ErrClosed", err)
	}
}
