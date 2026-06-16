package mdns

import (
	"fmt"
	"net"
	"sync/atomic"
)

// Announcer advertises a single zone controller on the mDNS bus.
// The caller owns the Transport; Announcer borrows it and never closes it.
type Announcer struct {
	transport Transport
	pkt       []byte
	closed    atomic.Bool
}

// NewAnnouncer creates an Announcer for the given zone controller endpoint.
// zone is the numeric Zone value (e.g. 1 = ZoneFrontLeft).
// localAddr is the UDP address of the zone controller's ZoneServer (e.g. "192.168.1.10:7777").
// transport is the mDNS packet bus (caller-owned); pass nil to open real multicast UDP.
func NewAnnouncer(zone uint8, localAddr string, transport Transport) (*Announcer, error) {
	if transport == nil {
		var err error
		transport, err = NewMulticastTransport(nil)
		if err != nil {
			return nil, err
		}
	}

	udpAddr, err := net.ResolveUDPAddr("udp", localAddr)
	if err != nil {
		return nil, fmt.Errorf("mdns: resolve addr %s: %w", localAddr, err)
	}

	ip := udpAddr.IP.To4()
	if ip == nil {
		return nil, fmt.Errorf("mdns: only IPv4 addresses supported, got %s", udpAddr.IP)
	}

	zoneName := fmt.Sprintf("zone-%d", zone)
	instanceName := zoneName + "." + serviceType
	host := zoneName + ".local."
	txtKV := fmt.Sprintf("zone=%d", zone)

	pkt := buildAnnouncement(instanceName, host, ip, uint16(udpAddr.Port), txtKV)
	return &Announcer{transport: transport, pkt: pkt}, nil
}

// Announce sends one proactive mDNS announcement onto the bus.
func (a *Announcer) Announce() error {
	if a.closed.Load() {
		return fmt.Errorf("mdns: announcer closed")
	}
	return a.transport.WritePacket(a.pkt)
}

// Close marks the Announcer as closed. It does NOT close the underlying Transport.
func (a *Announcer) Close() error {
	a.closed.Store(true)
	return nil
}
