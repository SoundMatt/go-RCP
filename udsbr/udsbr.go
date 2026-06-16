//fusa:req REQ-UDS-001
//fusa:req REQ-UDS-002
//fusa:req REQ-UDS-003
//fusa:req REQ-UDS-004
//fusa:req REQ-UDS-005
//fusa:req REQ-UDS-006
//fusa:req REQ-UDS-007
//fusa:req REQ-UDS-008

// Package udsbr provides a UDS (Unified Diagnostic Services, ISO 14229) bridge
// for go-RCP.
//
// UDS defines a set of diagnostic services used by automotive ECUs for
// reading fault codes, live data, and actuator testing. This package
// implements an in-process UDS server that maps the WriteDataByIdentifier
// (0x2E) service to rcp.Controller.Send, and the ReadDataByIdentifier (0x22)
// service to rcp.Controller.Subscribe status queries.
//
// PDU layout (request): [ServiceID][SubFunction/DID high][DID low][payload...]
// PDU layout (response): [ServiceID+0x40][DID high][DID low][data...]
// Negative response: [0x7F][ServiceID][NRC]
package udsbr

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync/atomic"

	rcp "github.com/SoundMatt/go-RCP"
)

// UDS service IDs used by this bridge.
const (
	SIDWriteDataByIdentifier = uint8(0x2E)
	SIDReadDataByIdentifier  = uint8(0x22)
	SIDPositiveOffset        = uint8(0x40)
	SIDNegativeResponse      = uint8(0x7F)
)

// UDS Negative Response Codes.
const (
	NRCSubFunctionNotSupported = uint8(0x12)
	NRCRequestOutOfRange       = uint8(0x31)
	NRCGeneralProgrammingFailure = uint8(0x72)
)

// ErrNegativeResponse is returned when the UDS server sends a 0x7F response.
var ErrNegativeResponse = errors.New("rcp/udsbr: negative response")

// ErrPDUTooShort is returned when the PDU is too short to be valid.
var ErrPDUTooShort = errors.New("rcp/udsbr: PDU too short")

// DataIdentifier is the 16-bit UDS data identifier (DID).
type DataIdentifier uint16

// Well-known DIDs used by the bridge.
const (
	DIDRCPCommand = DataIdentifier(0xF190) // map to rcp.Command
	DIDRCPStatus  = DataIdentifier(0xF191) // read rcp.Status
)

// ─── PDU encoding ─────────────────────────────────────────────────────────────

// BuildRequest builds a UDS request PDU for the given service and payload.
func BuildRequest(sid uint8, did DataIdentifier, payload []byte) []byte {
	pdu := make([]byte, 3+len(payload))
	pdu[0] = sid
	binary.BigEndian.PutUint16(pdu[1:], uint16(did))
	copy(pdu[3:], payload)
	return pdu
}

// BuildPositiveResponse builds the positive response PDU.
func BuildPositiveResponse(sid uint8, did DataIdentifier, data []byte) []byte {
	pdu := make([]byte, 3+len(data))
	pdu[0] = sid + SIDPositiveOffset
	binary.BigEndian.PutUint16(pdu[1:], uint16(did))
	copy(pdu[3:], data)
	return pdu
}

// BuildNegativeResponse builds a 0x7F negative response PDU.
func BuildNegativeResponse(sid, nrc uint8) []byte {
	return []byte{SIDNegativeResponse, sid, nrc}
}

// ─── Server ───────────────────────────────────────────────────────────────────

// Server is an in-process UDS server that maps diagnostic PDUs to rcp operations.
type Server struct {
	ctrl   rcp.Controller
	closed atomic.Bool
}

// NewServer returns a Server backed by ctrl.
func NewServer(ctrl rcp.Controller) *Server {
	return &Server{ctrl: ctrl}
}

// Close marks the server as closed. Subsequent Handle calls return errors.
func (s *Server) Close() {
	s.closed.Store(true)
}

// Handle processes a UDS request PDU and returns the response PDU.
// Supports SIDWriteDataByIdentifier (0x2E) and SIDReadDataByIdentifier (0x22).
func (s *Server) Handle(ctx context.Context, pdu []byte) ([]byte, error) {
	if s.closed.Load() {
		return BuildNegativeResponse(0x00, NRCGeneralProgrammingFailure), rcp.ErrClosed
	}
	if len(pdu) < 3 {
		return BuildNegativeResponse(0x00, NRCPDUTooShort), ErrPDUTooShort
	}
	sid := pdu[0]
	did := DataIdentifier(binary.BigEndian.Uint16(pdu[1:3]))
	payload := pdu[3:]

	switch sid {
	case SIDWriteDataByIdentifier:
		return s.handleWrite(ctx, did, payload)
	case SIDReadDataByIdentifier:
		return s.handleRead(ctx, did)
	default:
		return BuildNegativeResponse(sid, NRCSubFunctionNotSupported),
			fmt.Errorf("%w: SID 0x%02X", ErrNegativeResponse, sid)
	}
}

// handleWrite maps a WriteDataByIdentifier to rcp.Controller.Send.
// Payload layout: [zone byte][type byte][...data].
func (s *Server) handleWrite(ctx context.Context, did DataIdentifier, payload []byte) ([]byte, error) {
	if did != DIDRCPCommand {
		return BuildNegativeResponse(SIDWriteDataByIdentifier, NRCRequestOutOfRange),
			fmt.Errorf("%w: DID 0x%04X", ErrNegativeResponse, did)
	}
	if len(payload) < 2 {
		return BuildNegativeResponse(SIDWriteDataByIdentifier, NRCPDUTooShort), ErrPDUTooShort
	}
	cmd := &rcp.Command{
		Zone:    rcp.Zone(payload[0]),
		Type:    rcp.CommandType(payload[1]),
		Payload: append([]byte(nil), payload[2:]...),
	}
	resp, err := s.ctrl.Send(ctx, cmd)
	if err != nil {
		return BuildNegativeResponse(SIDWriteDataByIdentifier, NRCGeneralProgrammingFailure), err
	}
	respPayload := []byte{byte(resp.Status)}
	respPayload = append(respPayload, resp.Payload...)
	return BuildPositiveResponse(SIDWriteDataByIdentifier, did, respPayload), nil
}

// handleRead returns the most recent rcp.Status as a ReadDataByIdentifier response.
func (s *Server) handleRead(ctx context.Context, did DataIdentifier) ([]byte, error) {
	if did != DIDRCPStatus {
		return BuildNegativeResponse(SIDReadDataByIdentifier, NRCRequestOutOfRange),
			fmt.Errorf("%w: DID 0x%04X", ErrNegativeResponse, did)
	}
	ch, err := s.ctrl.Subscribe(ctx)
	if err != nil {
		return BuildNegativeResponse(SIDReadDataByIdentifier, NRCGeneralProgrammingFailure), err
	}
	select {
	case st, ok := <-ch:
		if !ok {
			return BuildNegativeResponse(SIDReadDataByIdentifier, NRCGeneralProgrammingFailure),
				errors.New("rcp/udsbr: subscribe channel closed")
		}
		data := []byte{byte(st.Zone), byte(st.Seq & 0xFF)}
		data = append(data, st.Payload...)
		return BuildPositiveResponse(SIDReadDataByIdentifier, did, data), nil
	case <-ctx.Done():
		return BuildNegativeResponse(SIDReadDataByIdentifier, NRCGeneralProgrammingFailure), ctx.Err()
	}
}

// NRCPDUTooShort is the NRC used when the PDU is shorter than expected.
const NRCPDUTooShort = uint8(0x13)
