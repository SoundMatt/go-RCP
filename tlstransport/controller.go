// Package tlstransport provides a mutual-TLS TCP transport for the RCP protocol.
package tlstransport

//fusa:req REQ-TLS-001
//fusa:req REQ-TLS-002
//fusa:req REQ-TLS-003
//fusa:req REQ-TLS-004
//fusa:req REQ-TLS-005
//fusa:req REQ-TLS-006
//fusa:req REQ-TLS-007
//fusa:req REQ-TLS-008
//fusa:req REQ-TLS-009
//fusa:req REQ-TLS-010

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/wire"
)

type subscription struct {
	ch   chan *rcp.Status
	once sync.Once
}

func (s *subscription) close() { s.once.Do(func() { close(s.ch) }) }

// Controller is an rcp.Controller that communicates over a mutual-TLS TCP stream.
type Controller struct {
	zone     rcp.Zone
	conn     *tls.Conn
	nextID   atomic.Uint32
	closed   atomic.Bool
	readDone chan struct{}

	writeMu sync.Mutex
	mu      sync.Mutex
	pending map[uint32]chan *rcp.Response
	subs    []*subscription
}

// NewController dials the server at addr using tlsCfg and returns a Controller for zone.
// tlsCfg must include client certificates for mutual TLS.
func NewController(zone rcp.Zone, addr string, tlsCfg *tls.Config) (*Controller, error) {
	dialer := &tls.Dialer{NetDialer: &net.Dialer{}, Config: tlsCfg}
	raw, err := dialer.DialContext(context.Background(), "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("rcp/tls: dial zone %s: %w", zone, err)
	}
	conn, ok := raw.(*tls.Conn)
	if !ok {
		_ = raw.Close()
		return nil, fmt.Errorf("rcp/tls: dial zone %s: unexpected conn type", zone)
	}
	c := &Controller{
		zone:     zone,
		conn:     conn,
		readDone: make(chan struct{}),
		pending:  make(map[uint32]chan *rcp.Response),
	}
	go c.readLoop()
	return c, nil
}

// Zone implements rcp.Controller.
func (c *Controller) Zone() rcp.Zone { return c.zone }

// Send implements rcp.Controller.
func (c *Controller) Send(ctx context.Context, cmd *rcp.Command) (*rcp.Response, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("rcp/tls: zone %s: %w", c.zone, rcp.ErrClosed)
	}
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("rcp/tls: zone %s: %w", c.zone, rcp.ErrTimeout)
	default:
	}
	if cmd.Zone != c.zone {
		return nil, fmt.Errorf("rcp/tls: zone %s: %w", c.zone, rcp.ErrZoneMismatch)
	}

	id := c.nextID.Add(1)
	safe := *cmd
	if len(cmd.Payload) > 0 {
		safe.Payload = make([]byte, len(cmd.Payload))
		copy(safe.Payload, cmd.Payload)
	}
	safe.ID = id

	ch := make(chan *rcp.Response, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	frame := wire.EncodeCommand(&safe)
	c.writeMu.Lock()
	_, err := c.conn.Write(frame)
	c.writeMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("rcp/tls: zone %s: write: %w", c.zone, err)
	}

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("rcp/tls: zone %s: %w", c.zone, rcp.ErrTimeout)
	case resp, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("rcp/tls: zone %s: %w", c.zone, rcp.ErrClosed)
		}
		return resp, nil
	case <-c.readDone:
		return nil, fmt.Errorf("rcp/tls: zone %s: %w", c.zone, rcp.ErrClosed)
	}
}

// Subscribe implements rcp.Controller.
func (c *Controller) Subscribe(ctx context.Context) (<-chan *rcp.Status, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("rcp/tls: zone %s: %w", c.zone, rcp.ErrClosed)
	}

	sub := &subscription{ch: make(chan *rcp.Status, 16)}
	c.mu.Lock()
	c.subs = append(c.subs, sub)
	c.mu.Unlock()

	frame := wire.EncodeControlFrame(wire.TypeSubscribe, c.zone)
	c.writeMu.Lock()
	_, err := c.conn.Write(frame)
	c.writeMu.Unlock()
	if err != nil {
		c.removeSub(sub)
		return nil, fmt.Errorf("rcp/tls: zone %s: subscribe: %w", c.zone, err)
	}

	go func() {
		select {
		case <-ctx.Done():
			frame := wire.EncodeControlFrame(wire.TypeUnsubscribe, c.zone)
			if !c.closed.Load() {
				c.writeMu.Lock()
				_, _ = c.conn.Write(frame)
				c.writeMu.Unlock()
			}
		case <-c.readDone:
		}
		c.removeSub(sub)
		sub.close()
	}()

	return sub.ch, nil
}

func (c *Controller) removeSub(sub *subscription) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, s := range c.subs {
		if s == sub {
			c.subs = append(c.subs[:i], c.subs[i+1:]...)
			return
		}
	}
}

// Close implements rcp.Controller.
func (c *Controller) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	err := c.conn.Close()
	<-c.readDone

	c.mu.Lock()
	for id, ch := range c.pending {
		close(ch)
		delete(c.pending, id)
	}
	c.mu.Unlock()

	return err
}

func (c *Controller) readLoop() {
	defer close(c.readDone)
	hdr := make([]byte, wire.HeaderLen)
	for {
		if _, err := io.ReadFull(c.conn, hdr); err != nil {
			return
		}
		if hdr[0] != wire.MagicByte0 || hdr[1] != wire.MagicByte1 || hdr[2] != wire.ProtoVer {
			return
		}
		bodyLen := uint32(hdr[12])<<24 | uint32(hdr[13])<<16 | uint32(hdr[14])<<8 | uint32(hdr[15])
		var body []byte
		if bodyLen > 0 {
			body = make([]byte, bodyLen)
			if _, err := io.ReadFull(c.conn, body); err != nil {
				return
			}
		}
		frame := append(hdr[:wire.HeaderLen:wire.HeaderLen], body...)

		switch hdr[3] {
		case wire.TypeResponse:
			resp, err := wire.DecodeResponse(frame)
			if err != nil {
				continue
			}
			c.mu.Lock()
			ch, ok := c.pending[resp.CommandID]
			c.mu.Unlock()
			if ok {
				select {
				case ch <- resp:
				default:
				}
			}
		case wire.TypeStatus:
			st, err := wire.DecodeStatus(frame)
			if err != nil {
				continue
			}
			c.mu.Lock()
			subs := make([]*subscription, len(c.subs))
			copy(subs, c.subs)
			c.mu.Unlock()
			for _, sub := range subs {
				select {
				case sub.ch <- st:
				default:
				}
			}
		}
	}
}
