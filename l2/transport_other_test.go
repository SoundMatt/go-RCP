//go:build !linux

//fusa:test REQ-L2-005

package l2_test

import (
	"testing"
	"time"

	"github.com/SoundMatt/go-RCP/l2"
)

// TestNewTransport_UnsupportedPlatform verifies NewTransport fails
// explicitly with ErrL2UnsupportedPlatform on a non-Linux build, rather
// than silently no-opping or panicking (REQ-L2-005). This test file only
// builds on !linux — the real Linux implementation is exercised by the
// CI-only veth-pair test instead (see .github/workflows/ci.yml's
// l2-transport-linux job).
func TestNewTransport_UnsupportedPlatform(t *testing.T) {
	tr, err := l2.NewTransport("eth0")
	if err != l2.ErrL2UnsupportedPlatform {
		t.Fatalf("NewTransport: err = %v, want ErrL2UnsupportedPlatform", err)
	}
	if tr != nil {
		t.Fatalf("NewTransport: transport = %v, want nil", tr)
	}
}

// TestTransport_ZeroValue_UnsupportedPlatform verifies every Transport
// method on the !linux stub also reports ErrL2UnsupportedPlatform, even
// called on a zero-value Transport (since NewTransport never successfully
// returns one to call them on) — REQ-L2-005.
func TestTransport_ZeroValue_UnsupportedPlatform(t *testing.T) {
	var tr l2.Transport

	if err := tr.Send([6]byte{}, nil); err != l2.ErrL2UnsupportedPlatform {
		t.Errorf("Send: err = %v, want ErrL2UnsupportedPlatform", err)
	}
	if _, _, err := tr.Recv(time.Second); err != l2.ErrL2UnsupportedPlatform {
		t.Errorf("Recv: err = %v, want ErrL2UnsupportedPlatform", err)
	}
	if err := tr.Close(); err != l2.ErrL2UnsupportedPlatform {
		t.Errorf("Close: err = %v, want ErrL2UnsupportedPlatform", err)
	}
	if mac := tr.LocalMAC(); mac != [6]byte{} {
		t.Errorf("LocalMAC = %v, want zero value", mac)
	}
}
