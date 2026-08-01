//go:build !linux

package l2

import "time"

// Transport is this package's L2 (raw Ethernet) transport type. On this
// (non-Linux) platform it is an inert stub: every method returns
// ErrL2UnsupportedPlatform. See doc.go's "Linux only" section for why —
// AF_PACKET/SOCK_RAW has no portable non-Linux equivalent this package
// implements. The stub exists so that code elsewhere in this repo (or a
// downstream caller) can reference l2.Transport unconditionally, without
// its own build tags, and get an explicit, actionable error at
// construction time on an unsupported platform instead of a build failure.
type Transport struct{}

// NewTransport always fails on this platform with ErrL2UnsupportedPlatform.
// ifaceName is accepted (and ignored) only to keep this stub's signature
// identical to transport_linux.go's real constructor.
func NewTransport(ifaceName string) (*Transport, error) {
	return nil, ErrL2UnsupportedPlatform
}

// LocalMAC always returns the zero MAC on this platform.
func (t *Transport) LocalMAC() [MACLen]byte {
	return [MACLen]byte{}
}

// Send always fails on this platform with ErrL2UnsupportedPlatform.
func (t *Transport) Send(dst [MACLen]byte, avtpdu []byte) error {
	return ErrL2UnsupportedPlatform
}

// Recv always fails on this platform with ErrL2UnsupportedPlatform.
func (t *Transport) Recv(timeout time.Duration) (src [MACLen]byte, avtpdu []byte, err error) {
	return src, nil, ErrL2UnsupportedPlatform
}

// Close always fails on this platform with ErrL2UnsupportedPlatform — there
// is never a real socket for it to close, since NewTransport itself always
// failed first.
func (t *Transport) Close() error {
	return ErrL2UnsupportedPlatform
}
