package l2

import "errors"

var (
	// ErrL2UnsupportedPlatform is returned by NewTransport (and every
	// Transport method on the !linux build) on any non-Linux platform: see
	// doc.go's "Linux only" section for why no portable fallback exists.
	ErrL2UnsupportedPlatform = errors.New("rcp/l2: raw Ethernet transport requires Linux (AF_PACKET/SOCK_RAW)")

	// ErrShortFrame is returned by DecodeFrame when b is too short to hold
	// a full Ethernet header (HeaderLen bytes: destination MAC + source MAC
	// + EtherType).
	ErrShortFrame = errors.New("rcp/l2: buffer too short for Ethernet header")

	// ErrUnexpectedEtherType is returned by DecodeFrame when b's EtherType
	// field is not EtherTypeAVTP.
	ErrUnexpectedEtherType = errors.New("rcp/l2: unexpected EtherType (want 0x22F0)")

	// ErrClosed is returned by Transport methods after Close.
	ErrClosed = errors.New("rcp/l2: transport closed")
)
