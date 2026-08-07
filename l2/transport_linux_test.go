//go:build linux

//fusa:test REQ-L2-006

package l2_test

import (
	"bytes"
	"net"
	"testing"
	"time"

	"github.com/SoundMatt/go-RCP/v9/l2"
)

// TestL2VethRoundTrip verifies two real l2.Transport instances, bound to a
// veth pair, round-trip a real Ethernet frame byte-for-byte over an actual
// AF_PACKET/SOCK_RAW socket pair (REQ-L2-006). This is the one test in this
// package that exercises transport_linux.go's real socket code path rather
// than frame.go's pure byte manipulation (see frame_test.go, which runs
// everywhere with no privileges and no Linux requirement).
//
// It requires: (1) a Linux kernel (this file's own build tag), (2) a
// "veth0"/"veth1" veth pair already present and up, and (3) CAP_NET_RAW (or
// root) to open the raw sockets — none of which the standard cross-platform
// CI "test" job (.github/workflows/ci.yml, matrix over ubuntu/macos/windows)
// provides, so this test skips itself cleanly whenever any of the three is
// missing rather than failing that job. The dedicated "L2 transport (Linux,
// veth)" CI job sets up all three (`sudo ip link add veth0 type veth peer
// name veth1 ...`, then runs this test under sudo) and is the only CI job
// that actually exercises this path.
func TestL2VethRoundTrip(t *testing.T) {
	if _, err := net.InterfaceByName("veth0"); err != nil {
		t.Skipf("veth0 not present, skipping (see this test's doc comment): %v", err)
	}
	if _, err := net.InterfaceByName("veth1"); err != nil {
		t.Skipf("veth1 not present, skipping (see this test's doc comment): %v", err)
	}

	tx, err := l2.NewTransport("veth0")
	if err != nil {
		t.Skipf("NewTransport(veth0) failed (likely missing CAP_NET_RAW — see this test's doc comment): %v", err)
	}
	defer func() { _ = tx.Close() }()

	rx, err := l2.NewTransport("veth1")
	if err != nil {
		t.Fatalf("NewTransport(veth1): %v", err)
	}
	defer func() { _ = rx.Close() }()

	dst := rx.LocalMAC()
	avtpdu := []byte{0x82, 0x00, 0x00, 0x04, 0xDE, 0xAD, 0xBE, 0xEF}

	errCh := make(chan error, 1)
	recvCh := make(chan []byte, 1)
	go func() {
		_, payload, recvErr := rx.Recv(5 * time.Second)
		if recvErr != nil {
			errCh <- recvErr
			return
		}
		recvCh <- payload
	}()

	// Give the receive goroutine a moment to call Recv (and set its socket
	// timeout) before the send — belt-and-braces against a scheduling race,
	// not a hard synchronization requirement.
	time.Sleep(50 * time.Millisecond)

	if err := tx.Send(dst, avtpdu); err != nil {
		t.Fatalf("Send: %v", err)
	}

	select {
	case recvErr := <-errCh:
		t.Fatalf("Recv: %v", recvErr)
	case got := <-recvCh:
		if !bytes.Equal(got, avtpdu) {
			t.Fatalf("received AVTPDU = % X, want % X", got, avtpdu)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("timed out waiting for veth0 -> veth1 frame")
	}
}
