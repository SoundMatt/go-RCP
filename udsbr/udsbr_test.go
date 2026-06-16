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
	"context"
	"errors"
	"testing"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/mock"
	"github.com/SoundMatt/go-RCP/udsbr"
)

// REQ-UDS-001: BuildRequest produces a PDU with the correct SID and DID.
func TestBuildRequest(t *testing.T) {
	pdu := udsbr.BuildRequest(udsbr.SIDWriteDataByIdentifier, udsbr.DIDRCPCommand, []byte{0x01, 0x02})
	if len(pdu) != 5 {
		t.Fatalf("len = %d, want 5", len(pdu))
	}
	if pdu[0] != udsbr.SIDWriteDataByIdentifier {
		t.Errorf("SID = 0x%02X, want 0x%02X", pdu[0], udsbr.SIDWriteDataByIdentifier)
	}
}

// REQ-UDS-002: BuildPositiveResponse produces the correct 0x6E/0x62 response.
func TestBuildPositiveResponse(t *testing.T) {
	resp := udsbr.BuildPositiveResponse(udsbr.SIDWriteDataByIdentifier, udsbr.DIDRCPCommand, []byte{0x00})
	want := udsbr.SIDWriteDataByIdentifier + udsbr.SIDPositiveOffset
	if resp[0] != want {
		t.Errorf("SID = 0x%02X, want 0x%02X", resp[0], want)
	}
}

// REQ-UDS-003: BuildNegativeResponse produces a 0x7F PDU.
func TestBuildNegativeResponse(t *testing.T) {
	resp := udsbr.BuildNegativeResponse(udsbr.SIDWriteDataByIdentifier, udsbr.NRCRequestOutOfRange)
	if resp[0] != udsbr.SIDNegativeResponse {
		t.Errorf("byte[0] = 0x%02X, want 0x7F", resp[0])
	}
	if resp[1] != udsbr.SIDWriteDataByIdentifier {
		t.Errorf("byte[1] = 0x%02X, want SID", resp[1])
	}
}

// REQ-UDS-004: Server.Handle dispatches WriteDataByIdentifier to rcp.Controller.Send.
func TestServer_Handle_WriteDataByIdentifier(t *testing.T) {
	dispatched := make(chan rcp.CommandType, 1)
	inner := mock.NewController(rcp.ZoneFrontLeft, func(cmd *rcp.Command) *rcp.Response {
		dispatched <- cmd.Type
		return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusOK}
	})
	defer inner.Close()

	srv := udsbr.NewServer(inner)
	defer srv.Close()

	pdu := udsbr.BuildRequest(udsbr.SIDWriteDataByIdentifier, udsbr.DIDRCPCommand,
		[]byte{byte(rcp.ZoneFrontLeft), byte(rcp.CmdSet)})
	resp, err := srv.Handle(context.Background(), pdu)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if resp[0] != udsbr.SIDWriteDataByIdentifier+udsbr.SIDPositiveOffset {
		t.Errorf("response SID = 0x%02X, want positive", resp[0])
	}
	select {
	case got := <-dispatched:
		if got != rcp.CmdSet {
			t.Errorf("dispatched %v, want CmdSet", got)
		}
	default:
		t.Error("controller not dispatched")
	}
}

// REQ-UDS-005: Server.Handle returns ErrNegativeResponse for unknown SID.
func TestServer_Handle_UnknownSID(t *testing.T) {
	inner := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer inner.Close()

	srv := udsbr.NewServer(inner)
	defer srv.Close()

	pdu := []byte{0xFF, 0xF1, 0x90}
	resp, err := srv.Handle(context.Background(), pdu)
	if !errors.Is(err, udsbr.ErrNegativeResponse) {
		t.Errorf("want ErrNegativeResponse, got %v", err)
	}
	if resp[0] != udsbr.SIDNegativeResponse {
		t.Errorf("response[0] = 0x%02X, want 0x7F", resp[0])
	}
}

// REQ-UDS-006: Server.Handle(ReadDataByIdentifier) returns rcp.Status data.
func TestServer_Handle_ReadDataByIdentifier(t *testing.T) {
	inner := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer inner.Close()

	srv := udsbr.NewServer(inner)
	defer srv.Close()

	// Publish a status first so Subscribe returns quickly
	go func() {
		time.Sleep(5 * time.Millisecond)
		inner.Publish([]byte("status-data"))
	}()

	pdu := udsbr.BuildRequest(udsbr.SIDReadDataByIdentifier, udsbr.DIDRCPStatus, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	resp, err := srv.Handle(ctx, pdu)
	if err != nil {
		t.Fatalf("Handle ReadDataByIdentifier: %v", err)
	}
	if resp[0] != udsbr.SIDReadDataByIdentifier+udsbr.SIDPositiveOffset {
		t.Errorf("response SID = 0x%02X, want positive", resp[0])
	}
}

// REQ-UDS-007: Server.Handle returns ErrPDUTooShort for short PDUs.
func TestServer_Handle_PDUTooShort(t *testing.T) {
	inner := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer inner.Close()

	srv := udsbr.NewServer(inner)
	defer srv.Close()

	_, err := srv.Handle(context.Background(), []byte{0x2E})
	if !errors.Is(err, udsbr.ErrPDUTooShort) {
		t.Errorf("want ErrPDUTooShort, got %v", err)
	}
}

// REQ-UDS-008: Server.Close prevents further Handle calls.
func TestServer_Close(t *testing.T) {
	inner := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer inner.Close()

	srv := udsbr.NewServer(inner)
	srv.Close()
	srv.Close() // idempotent

	pdu := udsbr.BuildRequest(udsbr.SIDWriteDataByIdentifier, udsbr.DIDRCPCommand,
		[]byte{byte(rcp.ZoneFrontLeft), byte(rcp.CmdSet)})
	_, err := srv.Handle(context.Background(), pdu)
	if !errors.Is(err, rcp.ErrClosed) {
		t.Errorf("want ErrClosed, got %v", err)
	}
}
