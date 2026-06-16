//fusa:req REQ-DOIP-001
//fusa:req REQ-DOIP-002
//fusa:req REQ-DOIP-003
//fusa:req REQ-DOIP-004
//fusa:req REQ-DOIP-005
//fusa:req REQ-DOIP-006
//fusa:req REQ-DOIP-007
//fusa:req REQ-DOIP-008

// Package doipbr provides a DoIP (Diagnostics over IP, ISO 13400) bridge
// for go-RCP.
//
// DoIP allows diagnostic tools to communicate with automotive ECUs over
// standard IP networks. This package implements an in-process DoIP server
// over TCP that tunnels UDS payloads from a diagnostic client to an
// rcp.Controller via the udsbr layer.
//
// DoIP message header (ISO 13400-2):
//   byte 0-1: Protocol version (0x02) and inverse (0xFD)
//   byte 2-3: Payload type (big-endian uint16)
//   byte 4-7: Payload length (big-endian uint32)
//   byte 8+:  Payload
//
// Payload types used here:
//   0x8001 — Diagnostic message (UDS request from client)
//   0x8002 — Diagnostic message ACK (positive)
//   0x8003 — Diagnostic message NACK
package doipbr

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"

	"github.com/SoundMatt/go-RCP/udsbr"
)

// DoIP protocol constants (ISO 13400-2).
const (
	ProtoVersion        = uint8(0x02)
	ProtoVersionInverse = uint8(0xFD)

	PayloadTypeDiagMessage     = uint16(0x8001)
	PayloadTypeDiagMessageAck  = uint16(0x8002)
	PayloadTypeDiagMessageNack = uint16(0x8003)

	headerLen = 8
)

// ErrInvalidHeader is returned when a DoIP header is malformed.
var ErrInvalidHeader = errors.New("rcp/doipbr: invalid DoIP header")

// ErrUnsupportedPayload is returned for unrecognised payload types.
var ErrUnsupportedPayload = errors.New("rcp/doipbr: unsupported payload type")

// ─── Header encoding ─────────────────────────────────────────────────────────

// BuildHeader serialises a DoIP header.
func BuildHeader(payloadType uint16, payloadLen uint32) []byte {
	h := make([]byte, headerLen)
	h[0] = ProtoVersion
	h[1] = ProtoVersionInverse
	binary.BigEndian.PutUint16(h[2:], payloadType)
	binary.BigEndian.PutUint32(h[4:], payloadLen)
	return h
}

// ParseHeader reads the 8-byte DoIP header from r.
// Returns payloadType and payloadLen on success.
func ParseHeader(r io.Reader) (payloadType uint16, payloadLen uint32, err error) {
	h := make([]byte, headerLen)
	if _, err = io.ReadFull(r, h); err != nil {
		return 0, 0, err
	}
	if h[0] != ProtoVersion || h[1] != ProtoVersionInverse {
		return 0, 0, ErrInvalidHeader
	}
	return binary.BigEndian.Uint16(h[2:]), binary.BigEndian.Uint32(h[4:]), nil
}

// ─── Server ───────────────────────────────────────────────────────────────────

// Server listens for DoIP TCP connections and dispatches diagnostic messages
// to an embedded udsbr.Server.
type Server struct {
	uds      *udsbr.Server
	ln       net.Listener
	closed   atomic.Bool
	wg       sync.WaitGroup
}

// NewServer creates a Server backed by udsServer and listening on ln.
// Call Serve to start accepting connections.
func NewServer(udsServer *udsbr.Server, ln net.Listener) *Server {
	return &Server{uds: udsServer, ln: ln}
}

// Addr returns the listener's network address.
func (s *Server) Addr() net.Addr { return s.ln.Addr() }

// Serve starts accepting TCP connections. It returns when Close is called.
func (s *Server) Serve() {
	s.wg.Add(1)
	defer s.wg.Done()
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConn(conn)
		}()
	}
}

// Close stops the server and closes the listener. Idempotent.
func (s *Server) Close() error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	err := s.ln.Close()
	s.wg.Wait()
	return err
}

// handleConn processes one DoIP TCP connection.
func (s *Server) handleConn(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	ctx := context.Background()
	for {
		payloadType, payloadLen, err := ParseHeader(conn)
		if err != nil {
			return
		}
		payload := make([]byte, payloadLen)
		if _, err = io.ReadFull(conn, payload); err != nil {
			return
		}
		switch payloadType {
		case PayloadTypeDiagMessage:
			resp, _ := s.uds.Handle(ctx, payload) //nolint:errcheck
			ack := append(BuildHeader(PayloadTypeDiagMessageAck, uint32(len(resp))), resp...)
			if _, err = conn.Write(ack); err != nil {
				return
			}
		default:
			nack := BuildHeader(PayloadTypeDiagMessageNack, 0)
			if _, err = conn.Write(nack); err != nil {
				return
			}
		}
	}
}

// ─── Client ───────────────────────────────────────────────────────────────────

// Client connects to a DoIP server and sends diagnostic messages.
type Client struct {
	conn   net.Conn
	mu     sync.Mutex
	closed atomic.Bool
}

// NewClient dials serverAddr and returns a Client.
func NewClient(serverAddr string) (*Client, error) {
	conn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", serverAddr)
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn}, nil
}

// Send transmits udsPDU as a diagnostic message and returns the UDS response payload.
func (c *Client) Send(ctx context.Context, udsPDU []byte) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	msg := append(BuildHeader(PayloadTypeDiagMessage, uint32(len(udsPDU))), udsPDU...)
	if _, err := c.conn.Write(msg); err != nil {
		return nil, err
	}

	payloadType, payloadLen, err := ParseHeader(c.conn)
	if err != nil {
		return nil, err
	}
	switch payloadType {
	case PayloadTypeDiagMessageAck:
		resp := make([]byte, payloadLen)
		if _, err = io.ReadFull(c.conn, resp); err != nil {
			return nil, err
		}
		return resp, nil
	case PayloadTypeDiagMessageNack:
		return nil, ErrUnsupportedPayload
	default:
		return nil, ErrUnsupportedPayload
	}
}

// Close closes the underlying connection. Idempotent.
func (c *Client) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	return c.conn.Close()
}
