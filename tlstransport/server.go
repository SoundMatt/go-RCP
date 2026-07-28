package tlstransport

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"

	rcp "github.com/SoundMatt/go-RCP"
)

// ZoneServer is a mutual-TLS TCP server simulating a zone controller.
type ZoneServer struct {
	zone     rcp.Zone
	listener net.Listener
	done     chan struct{}
	closed   atomic.Bool
	seq      atomic.Uint32
	healthy  atomic.Bool

	mu      sync.Mutex
	handler func(*rcp.Command) *rcp.Response
	conns   map[net.Conn]bool // subscribed connections
}

// NewZoneServer listens on addr with the given TLS config.
// tlsCfg must include server certificate and require client certificates for mTLS.
func NewZoneServer(zone rcp.Zone, addr string, tlsCfg *tls.Config) (*ZoneServer, error) {
	l, err := tls.Listen("tcp", addr, tlsCfg)
	if err != nil {
		return nil, fmt.Errorf("rcp/tls: zone server %s: listen: %w", zone, err)
	}
	s := &ZoneServer{
		zone:     zone,
		listener: l,
		done:     make(chan struct{}),
		conns:    make(map[net.Conn]bool),
	}
	s.healthy.Store(true)
	go s.serve()
	return s, nil
}

// Addr returns the server's listening address.
func (s *ZoneServer) Addr() net.Addr { return s.listener.Addr() }

// SetHandler installs a command handler. nil returns StatusOK.
func (s *ZoneServer) SetHandler(h func(*rcp.Command) *rcp.Response) {
	s.mu.Lock()
	s.handler = h
	s.mu.Unlock()
}

// SetHealthy controls the Healthy flag in published Status frames.
func (s *ZoneServer) SetHealthy(v bool) { s.healthy.Store(v) }

// Publish sends a Status frame to all subscribed connections.
func (s *ZoneServer) Publish(payload []byte) {
	seq := s.seq.Add(1)
	var p []byte
	if len(payload) > 0 {
		p = make([]byte, len(payload))
		copy(p, payload)
	}
	st := &rcp.Status{Zone: s.zone, Seq: seq, Healthy: s.healthy.Load(), Payload: p}
	frame := legacyEncodeStatus(st)

	s.mu.Lock()
	for conn := range s.conns {
		_, _ = conn.Write(frame)
	}
	s.mu.Unlock()
}

// Close shuts down the server.
func (s *ZoneServer) Close() error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	err := s.listener.Close()
	<-s.done
	return err
}

func (s *ZoneServer) serve() {
	defer close(s.done)
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handleConn(conn)
	}
}

func (s *ZoneServer) handleConn(conn net.Conn) {
	defer func() {
		s.mu.Lock()
		delete(s.conns, conn)
		s.mu.Unlock()
		_ = conn.Close()
	}()

	hdr := make([]byte, legacyHeaderLen)
	for {
		if _, err := io.ReadFull(conn, hdr); err != nil {
			return
		}
		if hdr[0] != legacyMagicByte0 || hdr[1] != legacyMagicByte1 || hdr[2] != legacyProtoVer {
			return
		}
		bodyLen := uint32(hdr[12])<<24 | uint32(hdr[13])<<16 | uint32(hdr[14])<<8 | uint32(hdr[15])
		var body []byte
		if bodyLen > 0 {
			body = make([]byte, bodyLen)
			if _, err := io.ReadFull(conn, body); err != nil {
				return
			}
		}
		frame := append(hdr[:legacyHeaderLen:legacyHeaderLen], body...)

		switch hdr[3] {
		case legacyTypeCommand:
			cmd, err := legacyDecodeCommand(frame)
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
			_, _ = conn.Write(legacyEncodeResponse(resp))

		case legacyTypeSubscribe:
			s.mu.Lock()
			s.conns[conn] = true
			s.mu.Unlock()

		case legacyTypeUnsubscribe:
			s.mu.Lock()
			delete(s.conns, conn)
			s.mu.Unlock()
		}
	}
}
