package udp

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	rcp "github.com/SoundMatt/go-RCP"
)

// ZoneServer simulates a zone controller over UDP for testing and integration.
// It handles Command frames, publishes Status to all registered subscribers,
// and manages Subscribe/Unsubscribe control frames.
type ZoneServer struct {
	zone    rcp.Zone
	conn    *net.UDPConn
	done    chan struct{}
	closed  atomic.Bool
	seq     atomic.Uint32
	healthy atomic.Bool

	mu      sync.Mutex
	handler func(*rcp.Command) *rcp.Response
	subs    map[string]*net.UDPAddr
}

// NewZoneServer listens on addr (e.g. "127.0.0.1:0") and serves the given zone.
func NewZoneServer(zone rcp.Zone, addr string) (*ZoneServer, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("rcp/udp: zone server %s: resolve: %w", zone, err)
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, fmt.Errorf("rcp/udp: zone server %s: listen: %w", zone, err)
	}
	s := &ZoneServer{
		zone: zone,
		conn: conn,
		done: make(chan struct{}),
		subs: make(map[string]*net.UDPAddr),
	}
	s.healthy.Store(true)
	go s.serve()
	return s, nil
}

// Addr returns the local UDP address the server is listening on.
func (s *ZoneServer) Addr() *net.UDPAddr {
	a, ok := s.conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		panic("rcp/udp: ZoneServer.Addr: underlying conn is not UDP")
	}
	return a
}

// SetHandler installs a command handler. If nil, the server returns StatusOK.
func (s *ZoneServer) SetHandler(h func(*rcp.Command) *rcp.Response) {
	s.mu.Lock()
	s.handler = h
	s.mu.Unlock()
}

// SetHealthy controls the Healthy field in published Status frames.
func (s *ZoneServer) SetHealthy(v bool) { s.healthy.Store(v) }

// Publish sends a Status frame to all current subscribers.
func (s *ZoneServer) Publish(payload []byte) {
	seq := s.seq.Add(1)
	var p []byte
	if len(payload) > 0 {
		p = make([]byte, len(payload))
		copy(p, payload)
	}
	st := &rcp.Status{Zone: s.zone, Seq: seq, Healthy: s.healthy.Load(), Payload: p}
	frame := encodeStatus(st)

	s.mu.Lock()
	addrs := make([]*net.UDPAddr, 0, len(s.subs))
	for _, a := range s.subs {
		addrs = append(addrs, a)
	}
	s.mu.Unlock()

	for _, a := range addrs {
		_, _ = s.conn.WriteToUDP(frame, a)
	}
}

// Close shuts down the server.
func (s *ZoneServer) Close() error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	err := s.conn.Close()
	<-s.done
	return err
}

func (s *ZoneServer) serve() {
	defer close(s.done)
	buf := make([]byte, headerLen+MaxPayload)
	for {
		n, clientAddr, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		frame := buf[:n]
		if len(frame) < headerLen {
			continue
		}
		switch frame[3] {
		case typeCommand:
			cmd, err := decodeCommand(frame)
			if err != nil {
				continue
			}
			s.mu.Lock()
			h := s.handler
			s.mu.Unlock()
			var resp *rcp.Response
			if h != nil {
				resp = h(cmd)
			} else {
				resp = &rcp.Response{CommandID: cmd.ID, Zone: s.zone, Status: rcp.StatusOK}
			}
			_, _ = s.conn.WriteToUDP(encodeResponse(resp), clientAddr)

		case typeSubscribe:
			s.mu.Lock()
			s.subs[clientAddr.String()] = clientAddr
			s.mu.Unlock()

		case typeUnsubscribe:
			s.mu.Lock()
			delete(s.subs, clientAddr.String())
			s.mu.Unlock()
		}
	}
}
