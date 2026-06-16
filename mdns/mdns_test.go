//fusa:test REQ-MDNS-001
//fusa:test REQ-MDNS-002
//fusa:test REQ-MDNS-003
//fusa:test REQ-MDNS-004
//fusa:test REQ-MDNS-005
//fusa:test REQ-MDNS-006
//fusa:test REQ-MDNS-007
//fusa:test REQ-MDNS-008

package mdns_test

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	rcpmdns "github.com/SoundMatt/go-RCP/mdns"
)

// pipeTransport is a paired in-memory transport for tests using real loopback UDP.
// Write on TX -> read on RX side.
type pipeTransport struct {
	sendConn *net.UDPConn
	recvConn *net.UDPConn
	peer     *net.UDPAddr
}

// newTransportPair returns two transports that exchange packets via loopback UDP.
// TX side writes to the RX side of the other transport, and vice versa.
func newTransportPair(t *testing.T) (tx rcpmdns.Transport, rx rcpmdns.Transport) {
	t.Helper()

	aRecv, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	bRecv, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		_ = aRecv.Close()
		t.Fatal(err)
	}
	aSend, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		_ = aRecv.Close()
		_ = bRecv.Close()
		t.Fatal(err)
	}
	bSend, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		_ = aRecv.Close()
		_ = bRecv.Close()
		_ = aSend.Close()
		t.Fatal(err)
	}

	aAddr, _ := aRecv.LocalAddr().(*net.UDPAddr)
	bAddr, _ := bRecv.LocalAddr().(*net.UDPAddr)

	// a writes to b's recv; b writes to a's recv
	a := &pipeTransport{sendConn: aSend, recvConn: aRecv, peer: bAddr}
	b := &pipeTransport{sendConn: bSend, recvConn: bRecv, peer: aAddr}

	t.Cleanup(func() {
		_ = aRecv.Close()
		_ = bRecv.Close()
		_ = aSend.Close()
		_ = bSend.Close()
	})

	return a, b
}

func (p *pipeTransport) ReadPacket(buf []byte) (int, error) {
	n, _, err := p.recvConn.ReadFromUDP(buf)
	return n, err
}

func (p *pipeTransport) WritePacket(b []byte) error {
	_, err := p.sendConn.WriteToUDP(b, p.peer)
	return err
}

func (p *pipeTransport) SetReadDeadline(t time.Time) error {
	return p.recvConn.SetReadDeadline(t)
}

func (p *pipeTransport) Close() error {
	err1 := p.recvConn.Close()
	err2 := p.sendConn.Close()
	if err1 != nil {
		return err1
	}
	return err2
}

// TestTransport_Loopback verifies the test transport delivers packets (REQ-MDNS-001).
func TestTransport_Loopback(t *testing.T) {
	at, bt := newTransportPair(t)

	done := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 512)
		n, err := bt.ReadPacket(buf)
		if err == nil {
			pkt := make([]byte, n)
			copy(pkt, buf[:n])
			done <- pkt
		}
		close(done)
	}()

	if err := at.WritePacket([]byte("ping")); err != nil {
		t.Fatalf("write: %v", err)
	}
	select {
	case pkt := <-done:
		if string(pkt) != "ping" {
			t.Errorf("payload = %q, want ping", string(pkt))
		}
	case <-time.After(time.Second):
		t.Fatal("no packet received within 1s")
	}
}

// TestAnnouncer_Announce verifies that an Announcer emits a packet (REQ-MDNS-002).
func TestAnnouncer_Announce(t *testing.T) {
	at, bt := newTransportPair(t)

	a, err := rcpmdns.NewAnnouncer(1, "127.0.0.1:7000", at)
	if err != nil {
		t.Fatalf("NewAnnouncer: %v", err)
	}

	recv := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 4096)
		n, err := bt.ReadPacket(buf)
		if err == nil {
			pkt := make([]byte, n)
			copy(pkt, buf[:n])
			recv <- pkt
		}
	}()

	if err := a.Announce(); err != nil {
		t.Fatalf("Announce: %v", err)
	}

	select {
	case pkt := <-recv:
		if len(pkt) < 12 {
			t.Errorf("packet too short: %d bytes", len(pkt))
		}
	case <-time.After(time.Second):
		t.Fatal("no announcement packet received within 1s")
	}
}

// TestBrowser_DiscoverAnnounced verifies that a Browser discovers an Announcer (REQ-MDNS-003..006).
func TestBrowser_DiscoverAnnounced(t *testing.T) {
	// announcerTx -> browserRx: announcements flow from announcer to browser
	announcerTx, browserRx := newTransportPair(t)

	ann, err := rcpmdns.NewAnnouncer(2, "127.0.0.1:8000", announcerTx)
	if err != nil {
		t.Fatalf("NewAnnouncer: %v", err)
	}

	bro, err := rcpmdns.NewBrowser(browserRx)
	if err != nil {
		t.Fatalf("NewBrowser: %v", err)
	}

	type result struct {
		zone uint8
		addr string
	}
	discovered := make(chan result, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = bro.Query(ctx, func(zone uint8, addr string) {
			select {
			case discovered <- result{zone, addr}:
			default:
			}
		})
	}()

	time.Sleep(20 * time.Millisecond)
	if err := ann.Announce(); err != nil {
		t.Fatalf("Announce: %v", err)
	}

	select {
	case d := <-discovered:
		if d.zone != 2 {
			t.Errorf("zone = %d, want 2", d.zone)
		}
		if d.addr != "127.0.0.1:8000" {
			t.Errorf("addr = %s, want 127.0.0.1:8000", d.addr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("zone controller not discovered within 2s")
	}

	cancel()
	wg.Wait()
}

// TestBrowser_MultipleZones verifies that multiple zone controllers are discovered (REQ-MDNS-007).
func TestBrowser_MultipleZones(t *testing.T) {
	announcerTx, browserRx := newTransportPair(t)

	zones := []struct {
		zone uint8
		addr string
	}{
		{1, "127.0.0.1:7001"},
		{2, "127.0.0.1:7002"},
		{3, "127.0.0.1:7003"},
	}

	bro, err := rcpmdns.NewBrowser(browserRx)
	if err != nil {
		t.Fatalf("NewBrowser: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	found := make(map[uint8]string)
	var mu sync.Mutex
	allFound := make(chan struct{}, 1)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = bro.Query(ctx, func(zone uint8, addr string) {
			mu.Lock()
			found[zone] = addr
			if len(found) == len(zones) {
				select {
				case allFound <- struct{}{}:
				default:
				}
			}
			mu.Unlock()
		})
	}()

	time.Sleep(20 * time.Millisecond)
	for _, z := range zones {
		ann, err := rcpmdns.NewAnnouncer(z.zone, z.addr, announcerTx)
		if err != nil {
			t.Fatalf("NewAnnouncer zone %d: %v", z.zone, err)
		}
		if err := ann.Announce(); err != nil {
			t.Fatalf("Announce zone %d: %v", z.zone, err)
		}
		time.Sleep(5 * time.Millisecond)
	}

	select {
	case <-allFound:
	case <-time.After(3 * time.Second):
		mu.Lock()
		t.Errorf("only discovered %d/%d zones: %v", len(found), len(zones), found)
		mu.Unlock()
	}

	cancel()
	wg.Wait()
}

// TestAnnouncer_Close verifies idempotent Close (REQ-MDNS-008).
func TestAnnouncer_Close(t *testing.T) {
	at, _ := newTransportPair(t)
	a, err := rcpmdns.NewAnnouncer(1, "127.0.0.1:7000", at)
	if err != nil {
		t.Fatalf("NewAnnouncer: %v", err)
	}
	if err := a.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := a.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}
