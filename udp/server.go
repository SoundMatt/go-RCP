package udp

import (
	"fmt"
	"net"
	"sync/atomic"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
)

// Server is the listening side of this package's AVTPDU/ACF-over-UDP/IP
// transport: it decodes each inbound datagram into an avtp.Header +
// acf.Message, hands the pair to a Router, and — unless Router.Route
// reports the request should be dropped outright (see Router.Route) —
// replies with the Router's response, framed in an untimed (NTSCF) header
// presenting streamID as this Server's own identity.
type Server struct {
	streamID avtp.StreamID
	conn     *net.UDPConn
	router   *Router
	seq      atomic.Uint32
	encapSeq atomic.Uint32
	closed   atomic.Bool
	done     chan struct{}
}

// NewServer listens on addr (e.g. "127.0.0.1:0") and serves router, replying
// with streamID as its own AVTPDU identity. If addr names a host with no
// explicit port at all (e.g. "127.0.0.1"), it defaults to AnnexJControlPort
// (see resolveAnnexJAddr) — a caller that wants a specific port, including
// "0" for an OS-assigned ephemeral port, states it explicitly in addr,
// exactly as before.
func NewServer(streamID avtp.StreamID, addr string, router *Router) (*Server, error) {
	udpAddr, err := resolveAnnexJAddr(addr)
	if err != nil {
		return nil, fmt.Errorf("rcp/udp: server stream %s: resolve: %w", streamID, err)
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, fmt.Errorf("rcp/udp: server stream %s: listen: %w", streamID, err)
	}
	s := &Server{
		streamID: streamID,
		conn:     conn,
		router:   router,
		done:     make(chan struct{}),
	}
	go s.serve()
	return s, nil
}

// Addr returns the local UDP address the server is listening on.
func (s *Server) Addr() *net.UDPAddr {
	a, ok := s.conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		panic("rcp/udp: Server.Addr: underlying conn is not UDP")
	}
	return a
}

// StreamID returns this Server's own avtp.StreamID identity.
func (s *Server) StreamID() avtp.StreamID { return s.streamID }

// Close shuts down the server.
func (s *Server) Close() error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	err := s.conn.Close()
	<-s.done
	return err
}

func (s *Server) serve() {
	defer close(s.done)
	buf := make([]byte, MaxFrameLen)
	for {
		n, clientAddr, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		// Strip Annex J's leading encapsulation sequence number before
		// handing the remaining bytes to acf.DecodeFrame — see annexj.go.
		_, rest, err := stripEncapSeq(buf[:n])
		if err != nil {
			continue
		}
		frame, err := acf.DecodeFrame(rest)
		if err != nil {
			continue
		}

		// TC18 §12.9.1.1: a frame may carry multiple independently-
		// addressed ACF messages, each checked and routed individually —
		// route every message this frame decoded to, and reply with every
		// response the Router produced (in the same order), rather than
		// assuming there was exactly one.
		var responses []acf.Message
		for _, msg := range frame.Messages {
			resp, shouldReply := s.router.Route(frame.Header, msg)
			if !shouldReply {
				continue
			}
			responses = append(responses, resp)
		}
		if len(responses) == 0 {
			continue
		}

		respHdr := avtp.Header{
			Timed:         false,
			StreamIDValid: true,
			SequenceNum:   uint8(s.seq.Add(1)),
			StreamID:      s.streamID,
		}
		out, err := acf.EncodeFrame(respHdr, responses...)
		if err != nil {
			continue
		}
		// Annex J UDP/IP framing on the reply too — see annexj.go.
		payload := prependEncapSeq(s.encapSeq.Add(1), out)
		_, _ = s.conn.WriteToUDP(payload, clientAddr)
	}
}
