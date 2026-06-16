package mdns

import (
	"fmt"
	"net"
	"time"
)

// Transport is the packet I/O abstraction for mDNS.
// The default implementation uses real multicast UDP.
// Tests may inject a unicast loopback pair instead.
//
// Callers own the Transport lifetime; Announcer and Browser borrow it.
// Use SetReadDeadline to interrupt a blocking ReadPacket (e.g. for context cancellation).
type Transport interface {
	// ReadPacket reads one datagram, returning the payload.
	ReadPacket(buf []byte) (n int, err error)
	// WritePacket sends a datagram.
	WritePacket(b []byte) error
	// SetReadDeadline sets the deadline for the next ReadPacket call.
	SetReadDeadline(t time.Time) error
	// Close releases the transport.
	Close() error
}

// mdnsTransport is the real multicast UDP transport.
type mdnsTransport struct {
	recvConn *net.UDPConn // joined to multicast group (receive)
	sendConn *net.UDPConn // unicast sender to multicast addr
	mcast    *net.UDPAddr
}

// NewMulticastTransport opens the mDNS multicast socket on iface (nil = any).
func NewMulticastTransport(iface *net.Interface) (Transport, error) {
	mcast, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", MDNSAddr, MDNSPort))
	if err != nil {
		return nil, err
	}
	recvConn, err := net.ListenMulticastUDP("udp4", iface, mcast)
	if err != nil {
		return nil, fmt.Errorf("mdns: listen multicast: %w", err)
	}
	sendConn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		_ = recvConn.Close()
		return nil, fmt.Errorf("mdns: listen send: %w", err)
	}
	return &mdnsTransport{recvConn: recvConn, sendConn: sendConn, mcast: mcast}, nil
}

func (t *mdnsTransport) ReadPacket(buf []byte) (int, error) {
	n, _, err := t.recvConn.ReadFromUDP(buf)
	return n, err
}

func (t *mdnsTransport) WritePacket(b []byte) error {
	_, err := t.sendConn.WriteToUDP(b, t.mcast)
	return err
}

func (t *mdnsTransport) SetReadDeadline(dl time.Time) error {
	return t.recvConn.SetReadDeadline(dl)
}

func (t *mdnsTransport) Close() error {
	err1 := t.recvConn.Close()
	err2 := t.sendConn.Close()
	if err1 != nil {
		return err1
	}
	return err2
}
