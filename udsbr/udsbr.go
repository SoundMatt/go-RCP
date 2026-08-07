// Package udsbr provides a UDS (Unified Diagnostic Services, ISO 14229)
// bridge for go-RCP, for the OPEN Alliance TC18 Remote Control Protocol
// (RCP), as described by the "OPEN Alliance TC18 Remote Control Protocol
// Specification v0.5.1_RC".
//
// This is ROADMAP.md Milestone 56 (v0.69.0)'s ADAPT-flagged rebuild: per
// Phase 17's disposition table, UDS diagnostics is unrelated to TC18 RCP's
// endpoint model, so this bridge re-points at the new request/response
// types rather than the retired rcp.Controller. A UDS DataIdentifier (DID)
// now addresses an avtp.ByteBusID directly (its low byte — see
// addressFor); WriteDataByIdentifier (0x2E) forwards its payload as a
// write request, and ReadDataByIdentifier (0x22) issues a read request and
// returns the response body. There is no longer a single fixed
// DIDRCPCommand/DIDRCPStatus pair the way the retired package had — TC18's
// addressing model has no single global command/status endpoint to pin a
// well-known DID to, so this package accepts any DID and re-points it at
// whichever endpoint its low byte names, leaving DID range/meaning
// conventions to the caller. This package is also flagged as a candidate
// transport for the firmware package's chunked-transfer needs (ROADMAP.md
// Milestone 57) — a future milestone's concern, not implemented here.
//
// PDU layout (unchanged from the retired package):
//
//	request:  [ServiceID][DID high][DID low][payload...]
//	response: [ServiceID+0x40][DID high][DID low][data...]
//	negative: [0x7F][ServiceID][NRC]
package udsbr

//fusa:req REQ-UDS-001
//fusa:req REQ-UDS-002
//fusa:req REQ-UDS-003
//fusa:req REQ-UDS-004
//fusa:req REQ-UDS-005
//fusa:req REQ-UDS-006
//fusa:req REQ-UDS-007
//fusa:req REQ-UDS-008

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/udp"
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
	NRCSubFunctionNotSupported   = uint8(0x12)
	NRCRequestOutOfRange         = uint8(0x31)
	NRCGeneralProgrammingFailure = uint8(0x72)
	NRCPDUTooShort               = uint8(0x13)
)

// ErrNegativeResponse is returned when the UDS server would send a 0x7F
// response.
var ErrNegativeResponse = errors.New("rcp/udsbr: negative response")

// ErrPDUTooShort is returned when the PDU is too short to be valid.
var ErrPDUTooShort = errors.New("rcp/udsbr: PDU too short")

// ErrClosed is returned by Server.Handle once Close has been called.
var ErrClosed = errors.New("rcp/udsbr: server closed")

// DataIdentifier is the 16-bit UDS data identifier (DID).
type DataIdentifier uint16

// addressFor derives the avtp.ByteBusID a DID addresses: its low byte,
// ByteBusID's own full representable width (see avtp/address.go). Any DID
// is accepted; this package prescribes no fixed DID-to-endpoint convention
// beyond this (see the package doc comment).
func addressFor(did DataIdentifier) avtp.ByteBusID {
	return avtp.ByteBusID(did & 0xFF)
}

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

// Server is an in-process UDS server that maps diagnostic PDUs to requests
// against an upstream *udp.Controller.
type Server struct {
	upstream *udp.Controller
	closed   atomic.Bool
}

// NewServer returns a Server forwarding to upstream.
func NewServer(upstream *udp.Controller) *Server {
	return &Server{upstream: upstream}
}

// Close marks the server as closed. Subsequent Handle calls return errors.
func (s *Server) Close() {
	s.closed.Store(true)
}

// Handle processes a UDS request PDU and returns the response PDU.
// Supports SIDWriteDataByIdentifier (0x2E) and SIDReadDataByIdentifier (0x22).
func (s *Server) Handle(ctx context.Context, pdu []byte) ([]byte, error) {
	if s.closed.Load() {
		return BuildNegativeResponse(0x00, NRCGeneralProgrammingFailure), ErrClosed
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

// handleWrite forwards payload as a write request to the endpoint did
// addresses.
func (s *Server) handleWrite(ctx context.Context, did DataIdentifier, payload []byte) ([]byte, error) {
	resp, err := s.upstream.Write(ctx, addressFor(did), payload)
	if err != nil {
		return BuildNegativeResponse(SIDWriteDataByIdentifier, NRCGeneralProgrammingFailure), err
	}
	if resp.Control.Has(acf.FlagError) {
		return BuildNegativeResponse(SIDWriteDataByIdentifier, NRCRequestOutOfRange),
			fmt.Errorf("%w: DID 0x%04X: %s", ErrNegativeResponse, did, resp.Body)
	}
	return BuildPositiveResponse(SIDWriteDataByIdentifier, did, resp.Body), nil
}

// handleRead issues a read request to the endpoint did addresses and
// returns its response body as a positive ReadDataByIdentifier response.
func (s *Server) handleRead(ctx context.Context, did DataIdentifier) ([]byte, error) {
	resp, err := s.upstream.Read(ctx, addressFor(did))
	if err != nil {
		return BuildNegativeResponse(SIDReadDataByIdentifier, NRCGeneralProgrammingFailure), err
	}
	if resp.Control.Has(acf.FlagError) {
		return BuildNegativeResponse(SIDReadDataByIdentifier, NRCRequestOutOfRange),
			fmt.Errorf("%w: DID 0x%04X: %s", ErrNegativeResponse, did, resp.Body)
	}
	return BuildPositiveResponse(SIDReadDataByIdentifier, did, resp.Body), nil
}
