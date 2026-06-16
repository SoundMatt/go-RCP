package udp

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	rcp "github.com/SoundMatt/go-RCP"
)

type subscription struct {
	ch   chan *rcp.Status
	once sync.Once
}

func (s *subscription) close() {
	s.once.Do(func() { close(s.ch) })
}

// Controller is an rcp.Controller that sends commands to a ZoneServer over UDP.
type Controller struct {
	zone     rcp.Zone
	conn     *net.UDPConn
	nextID   atomic.Uint32
	closed   atomic.Bool
	readDone chan struct{}

	mu      sync.Mutex
	pending map[uint32]chan *rcp.Response
	subs    []*subscription
}

// NewController dials a UDP ZoneServer at serverAddr and returns a Controller for zone.
func NewController(zone rcp.Zone, serverAddr *net.UDPAddr) (*Controller, error) {
	conn, err := net.DialUDP("udp", nil, serverAddr)
	if err != nil {
		return nil, fmt.Errorf("rcp/udp: dial zone %s: %w", zone, err)
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
		return nil, fmt.Errorf("rcp/udp: zone %s: %w", c.zone, rcp.ErrClosed)
	}
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("rcp/udp: zone %s: %w", c.zone, rcp.ErrTimeout)
	default:
	}
	if cmd.Zone != c.zone {
		return nil, fmt.Errorf("rcp/udp: zone %s: %w", c.zone, rcp.ErrZoneMismatch)
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

	if _, err := c.conn.Write(encodeCommand(&safe)); err != nil {
		return nil, fmt.Errorf("rcp/udp: zone %s: write: %w", c.zone, err)
	}

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("rcp/udp: zone %s: %w", c.zone, rcp.ErrTimeout)
	case resp, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("rcp/udp: zone %s: %w", c.zone, rcp.ErrClosed)
		}
		return resp, nil
	case <-c.readDone:
		return nil, fmt.Errorf("rcp/udp: zone %s: %w", c.zone, rcp.ErrClosed)
	}
}

// Subscribe implements rcp.Controller.
func (c *Controller) Subscribe(ctx context.Context) (<-chan *rcp.Status, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("rcp/udp: zone %s: %w", c.zone, rcp.ErrClosed)
	}

	sub := &subscription{ch: make(chan *rcp.Status, 16)}

	c.mu.Lock()
	c.subs = append(c.subs, sub)
	c.mu.Unlock()

	if _, err := c.conn.Write(encodeControlFrame(typeSubscribe, c.zone)); err != nil {
		c.removeSub(sub)
		return nil, fmt.Errorf("rcp/udp: zone %s: subscribe: %w", c.zone, err)
	}

	go func() {
		select {
		case <-ctx.Done():
			if !c.closed.Load() {
				_, _ = c.conn.Write(encodeControlFrame(typeUnsubscribe, c.zone))
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
	<-c.readDone // waits for readLoop to exit, which closes readDone; Subscribe goroutines then clean up their channels

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
	buf := make([]byte, headerLen+MaxPayload)
	for {
		n, err := c.conn.Read(buf)
		if err != nil {
			return
		}
		frame := buf[:n]
		if len(frame) < headerLen {
			continue
		}
		switch frame[3] {
		case typeResponse:
			resp, err := decodeResponse(frame)
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
		case typeStatus:
			st, err := decodeStatus(frame)
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
