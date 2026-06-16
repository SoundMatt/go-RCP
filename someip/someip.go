//fusa:req REQ-SIPC-001
//fusa:req REQ-SIPC-002
//fusa:req REQ-SIPC-003
//fusa:req REQ-SIPC-004
//fusa:req REQ-SIPC-005
//fusa:req REQ-SIPC-006
//fusa:req REQ-SIPC-007
//fusa:req REQ-SIPC-008

// Package someip provides a SOME/IP bridge for go-RCP.
//
// The SOME/IP (Scalable service-Oriented MiddlewarE over IP) wire format uses
// a 16-byte header followed by a payload. This package maps rcp.Commands to
// SOME/IP service method invocations and vice versa.
//
// Server listens for incoming SOME/IP request datagrams and dispatches them
// to an rcp.Controller. Controller implements rcp.Controller by sending
// SOME/IP request datagrams to a remote Server.
//
// SOME/IP header layout (16 bytes):
//
//	[0:2]  Service ID  (uint16 big-endian)
//	[2:4]  Method  ID  (uint16 big-endian)
//	[4:8]  Length      (uint32 big-endian, counts from ClientID to end)
//	[8:10] Client  ID  (uint16 big-endian)
//	[10:12]Session ID  (uint16 big-endian)
//	[12]   Proto   Ver (0x01)
//	[13]   Iface   Ver (0x01)
//	[14]   Msg Type    (0x00 REQUEST, 0x80 RESPONSE)
//	[15]   Return Code (0x00 OK, 0x01 NOT_OK)
package someip

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
)

const (
	headerLen = 16

	msgTypeRequest  uint8 = 0x00
	msgTypeResponse uint8 = 0x80

	retCodeOK    uint8 = 0x00
	retCodeNotOK uint8 = 0x01

	// DefaultServiceID is the SOME/IP service ID used for the RCP bridge.
	DefaultServiceID uint16 = 0x0E00
)

// ErrMalformedFrame is returned when a SOME/IP frame is too short or invalid.
var ErrMalformedFrame = errors.New("rcp/someip: malformed SOME/IP frame")

// Header is a decoded SOME/IP message header.
type Header struct {
	ServiceID  uint16
	MethodID   uint16
	Length     uint32
	ClientID   uint16
	SessionID  uint16
	ProtoVer   uint8
	IfaceVer   uint8
	MsgType    uint8
	ReturnCode uint8
}

// encodeFrame serialises hdr and payload into a SOME/IP datagram.
func encodeFrame(hdr Header, payload []byte) []byte {
	out := make([]byte, headerLen+len(payload))
	binary.BigEndian.PutUint16(out[0:], hdr.ServiceID)
	binary.BigEndian.PutUint16(out[2:], hdr.MethodID)
	// Length field covers ClientID..end of payload (headerLen - 8 + len(payload))
	binary.BigEndian.PutUint32(out[4:], uint32(8+len(payload)))
	binary.BigEndian.PutUint16(out[8:], hdr.ClientID)
	binary.BigEndian.PutUint16(out[10:], hdr.SessionID)
	out[12] = hdr.ProtoVer
	out[13] = hdr.IfaceVer
	out[14] = hdr.MsgType
	out[15] = hdr.ReturnCode
	copy(out[headerLen:], payload)
	return out
}

// decodeFrame splits a SOME/IP datagram into header and payload.
func decodeFrame(b []byte) (Header, []byte, error) {
	if len(b) < headerLen {
		return Header{}, nil, ErrMalformedFrame
	}
	hdr := Header{
		ServiceID:  binary.BigEndian.Uint16(b[0:]),
		MethodID:   binary.BigEndian.Uint16(b[2:]),
		Length:     binary.BigEndian.Uint32(b[4:]),
		ClientID:   binary.BigEndian.Uint16(b[8:]),
		SessionID:  binary.BigEndian.Uint16(b[10:]),
		ProtoVer:   b[12],
		IfaceVer:   b[13],
		MsgType:    b[14],
		ReturnCode: b[15],
	}
	return hdr, b[headerLen:], nil
}

// ─── Server ───────────────────────────────────────────────────────────────────

// Server listens for SOME/IP REQUEST datagrams and dispatches them to an
// rcp.Controller. Responses are sent back as SOME/IP RESPONSE datagrams.
type Server struct {
	ctrl      rcp.Controller
	zone      rcp.Zone
	serviceID uint16
	conn      *net.UDPConn
	done      chan struct{}
}

// NewServer creates a Server backed by ctrl, listening on addr.
// serviceID is the SOME/IP service identifier to accept; use DefaultServiceID.
func NewServer(ctrl rcp.Controller, addr *net.UDPAddr, serviceID uint16) (*Server, error) {
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("rcp/someip: listen %s: %w", addr, err)
	}
	s := &Server{
		ctrl:      ctrl,
		zone:      ctrl.Zone(),
		serviceID: serviceID,
		conn:      conn,
		done:      make(chan struct{}),
	}
	go s.readLoop()
	return s, nil
}

// Addr returns the local UDP address the server is listening on.
func (s *Server) Addr() *net.UDPAddr { return s.conn.LocalAddr().(*net.UDPAddr) } //nolint:errcheck

// Close shuts down the server.
func (s *Server) Close() error {
	err := s.conn.Close()
	<-s.done
	return err
}

func (s *Server) readLoop() {
	defer close(s.done)
	buf := make([]byte, 65535)
	for {
		n, remote, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		frame := make([]byte, n)
		copy(frame, buf[:n])
		go s.handle(frame, remote)
	}
}

func (s *Server) handle(frame []byte, remote *net.UDPAddr) {
	hdr, payload, err := decodeFrame(frame)
	if err != nil || hdr.MsgType != msgTypeRequest || hdr.ServiceID != s.serviceID {
		return
	}

	cmd := &rcp.Command{
		Zone:     s.zone,
		Type:     rcp.CommandType(hdr.MethodID),
		Priority: rcp.PriorityNormal,
		Payload:  payload,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := s.ctrl.Send(ctx, cmd)
	retCode := retCodeOK
	var respPayload []byte
	if err != nil {
		retCode = retCodeNotOK
	} else {
		if resp.Status != rcp.StatusOK {
			retCode = retCodeNotOK
		}
		respPayload = resp.Payload
	}

	respHdr := Header{
		ServiceID:  hdr.ServiceID,
		MethodID:   hdr.MethodID,
		ClientID:   hdr.ClientID,
		SessionID:  hdr.SessionID,
		ProtoVer:   0x01,
		IfaceVer:   0x01,
		MsgType:    msgTypeResponse,
		ReturnCode: retCode,
	}
	_, _ = s.conn.WriteToUDP(encodeFrame(respHdr, respPayload), remote)
}

// ─── Controller ───────────────────────────────────────────────────────────────

// Controller implements rcp.Controller by sending SOME/IP REQUEST datagrams
// to a remote Server and correlating RESPONSE datagrams by session ID.
type Controller struct {
	zone      rcp.Zone
	serviceID uint16
	server    *net.UDPAddr
	conn      *net.UDPConn
	nextSess  atomic.Uint32
	closed    atomic.Bool
	readDone  chan struct{}

	mu      sync.Mutex
	pending map[uint16]chan Header
}

// NewController dials a SOME/IP Server at serverAddr and returns an
// rcp.Controller for zone. serviceID must match the server's service ID.
func NewController(zone rcp.Zone, serverAddr *net.UDPAddr, serviceID uint16) (*Controller, error) {
	conn, err := net.DialUDP("udp", nil, serverAddr)
	if err != nil {
		return nil, fmt.Errorf("rcp/someip: dial %s: %w", serverAddr, err)
	}
	c := &Controller{
		zone:      zone,
		serviceID: serviceID,
		server:    serverAddr,
		conn:      conn,
		readDone:  make(chan struct{}),
		pending:   make(map[uint16]chan Header),
	}
	go c.readLoop()
	return c, nil
}

// Zone implements rcp.Controller.
func (c *Controller) Zone() rcp.Zone { return c.zone }

// Send implements rcp.Controller — encodes cmd as a SOME/IP REQUEST.
func (c *Controller) Send(ctx context.Context, cmd *rcp.Command) (*rcp.Response, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("rcp/someip: zone %s: %w", c.zone, rcp.ErrClosed)
	}
	if cmd.Zone != c.zone {
		return nil, fmt.Errorf("rcp/someip: zone %s: %w", c.zone, rcp.ErrZoneMismatch)
	}

	sessID := uint16(c.nextSess.Add(1))
	ch := make(chan Header, 1)
	c.mu.Lock()
	c.pending[sessID] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, sessID)
		c.mu.Unlock()
	}()

	reqHdr := Header{
		ServiceID: c.serviceID,
		MethodID:  uint16(cmd.Type),
		ClientID:  0x0001,
		SessionID: sessID,
		ProtoVer:  0x01,
		IfaceVer:  0x01,
		MsgType:   msgTypeRequest,
	}
	if _, err := c.conn.Write(encodeFrame(reqHdr, cmd.Payload)); err != nil {
		return nil, fmt.Errorf("rcp/someip: Send write: %w", err)
	}

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("rcp/someip: zone %s: %w", c.zone, rcp.ErrTimeout)
	case respHdr, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("rcp/someip: zone %s: %w", c.zone, rcp.ErrClosed)
		}
		status := rcp.StatusOK
		if respHdr.ReturnCode != retCodeOK {
			status = rcp.StatusError
		}
		return &rcp.Response{Zone: c.zone, Status: status}, nil
	case <-c.readDone:
		return nil, fmt.Errorf("rcp/someip: zone %s: %w", c.zone, rcp.ErrClosed)
	}
}

// Subscribe implements rcp.Controller — SOME/IP events require a separate
// event group subscription; this stub returns an empty channel.
func (c *Controller) Subscribe(_ context.Context) (<-chan *rcp.Status, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("rcp/someip: zone %s: %w", c.zone, rcp.ErrClosed)
	}
	ch := make(chan *rcp.Status)
	go func() { <-c.readDone; close(ch) }()
	return ch, nil
}

// Close implements rcp.Controller — idempotent.
func (c *Controller) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	return c.conn.Close()
}

func (c *Controller) readLoop() {
	defer close(c.readDone)
	buf := make([]byte, 65535)
	for {
		n, err := c.conn.Read(buf)
		if err != nil {
			return
		}
		hdr, _, err := decodeFrame(buf[:n])
		if err != nil || hdr.MsgType != msgTypeResponse {
			continue
		}
		c.mu.Lock()
		ch, ok := c.pending[hdr.SessionID]
		c.mu.Unlock()
		if ok {
			select {
			case ch <- hdr:
			default:
			}
		}
	}
}

