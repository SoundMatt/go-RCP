//go:build linux

package l2

import (
	"fmt"
	"net"
	"syscall"
	"time"
)

// Transport is this package's real, Linux-only L2 (raw Ethernet) transport:
// an AF_PACKET/SOCK_RAW socket bound to one network interface, filtered to
// EtherTypeAVTP frames. Opening it requires CAP_NET_RAW (or root) — see
// doc.go's "Linux only" section, which documents this plainly as a genuine
// runtime requirement rather than a formality.
type Transport struct {
	iface    *net.Interface
	localMAC [MACLen]byte
	fd       int
	closed   bool
}

// htons converts a uint16 from host to network byte order. Restated locally
// (rather than depending on an exported syscall.Htons, which this repo's
// pinned Go toolchain does not export on every Linux GOARCH) — the same
// "depend only on symbols verified present everywhere this repo targets"
// posture tsn/sockprio_linux.go already established for its own use of the
// syscall package.
func htons(v uint16) uint16 {
	return v<<8 | v>>8
}

// NewTransport opens a raw AF_PACKET/SOCK_RAW socket bound to the named
// network interface (e.g. "eth0"), filtered to EtherTypeAVTP frames, and
// reads the interface's own MAC address to serve as this Transport's source
// address on every Send (LocalMAC) — a caller never supplies its own source
// MAC, symmetric with how a caller does supply its own destination MAC on
// every Send call (see doc.go's "Destination addressing is caller-supplied"
// section).
func NewTransport(ifaceName string) (*Transport, error) {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return nil, fmt.Errorf("rcp/l2: interface %s: %w", ifaceName, err)
	}
	var localMAC [MACLen]byte
	copy(localMAC[:], iface.HardwareAddr)

	fd, err := syscall.Socket(syscall.AF_PACKET, syscall.SOCK_RAW, int(htons(EtherTypeAVTP)))
	if err != nil {
		return nil, fmt.Errorf("rcp/l2: socket: %w", err)
	}

	bindAddr := syscall.SockaddrLinklayer{
		Protocol: htons(EtherTypeAVTP),
		Ifindex:  iface.Index,
	}
	if err := syscall.Bind(fd, &bindAddr); err != nil {
		_ = syscall.Close(fd)
		return nil, fmt.Errorf("rcp/l2: bind %s: %w", ifaceName, err)
	}

	return &Transport{iface: iface, localMAC: localMAC, fd: fd}, nil
}

// LocalMAC returns the bound interface's own MAC address, as read at
// NewTransport time.
func (t *Transport) LocalMAC() [MACLen]byte { return t.localMAC }

// Send transmits avtpdu as a raw Ethernet frame (see EncodeFrame) to dst,
// using this Transport's own interface MAC as the source address.
func (t *Transport) Send(dst [MACLen]byte, avtpdu []byte) error {
	if t.closed {
		return ErrClosed
	}
	frame := EncodeFrame(dst, t.localMAC, avtpdu)

	to := syscall.SockaddrLinklayer{
		Protocol: htons(EtherTypeAVTP),
		Ifindex:  t.iface.Index,
		Halen:    MACLen,
	}
	copy(to.Addr[:MACLen], dst[:])

	if err := syscall.Sendto(t.fd, frame, 0, &to); err != nil {
		return fmt.Errorf("rcp/l2: sendto: %w", err)
	}
	return nil
}

// Recv blocks until a frame arrives on the bound interface or timeout
// elapses, and returns its source MAC and AVTPDU payload (see DecodeFrame).
// A timeout of 0 blocks indefinitely.
func (t *Transport) Recv(timeout time.Duration) (src [MACLen]byte, avtpdu []byte, err error) {
	if t.closed {
		return src, nil, ErrClosed
	}
	if timeout > 0 {
		tv := syscall.NsecToTimeval(timeout.Nanoseconds())
		if sockErr := syscall.SetsockoptTimeval(t.fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &tv); sockErr != nil {
			return src, nil, fmt.Errorf("rcp/l2: set recv timeout: %w", sockErr)
		}
	}

	// A full-size Ethernet frame (up to the standard 1500-byte MTU plus the
	// 14-byte header) comfortably fits; oversized/jumbo frames are out of
	// scope for this milestone.
	buf := make([]byte, 1600)
	n, _, err := syscall.Recvfrom(t.fd, buf, 0)
	if err != nil {
		return src, nil, fmt.Errorf("rcp/l2: recvfrom: %w", err)
	}

	_, srcMAC, payload, decErr := DecodeFrame(buf[:n])
	if decErr != nil {
		return src, nil, decErr
	}
	return srcMAC, payload, nil
}

// Close closes the underlying raw socket. Safe to call more than once.
func (t *Transport) Close() error {
	if t.closed {
		return nil
	}
	t.closed = true
	return syscall.Close(t.fd)
}
