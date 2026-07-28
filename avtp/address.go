package avtp

import "fmt"

// StreamID is an AVTP stream_id: a sender's 6-byte MAC address followed by a
// 2-byte suffix the sender assigns locally to distinguish multiple streams
// it originates. It addresses the enclosing AVTPDU as a whole; byte_bus_id
// and transaction_num are only meaningful relative to the StreamID of the
// AVTPDU that carried them.
type StreamID [8]byte

// NewStreamID builds a StreamID from a sender MAC address and a
// locally-assigned suffix.
func NewStreamID(mac [6]byte, suffix uint16) StreamID {
	var id StreamID
	copy(id[0:6], mac[:])
	id[6] = byte(suffix >> 8)
	id[7] = byte(suffix)
	return id
}

// MAC returns the sender MAC address portion of the StreamID.
func (s StreamID) MAC() [6]byte {
	var mac [6]byte
	copy(mac[:], s[0:6])
	return mac
}

// Suffix returns the locally-assigned suffix portion of the StreamID.
func (s StreamID) Suffix() uint16 {
	return uint16(s[6])<<8 | uint16(s[7])
}

// String renders the StreamID as MAC/suffix for logs and diagnostics.
func (s StreamID) String() string {
	mac := s.MAC()
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x/%04x",
		mac[0], mac[1], mac[2], mac[3], mac[4], mac[5], s.Suffix())
}

// ByteBusID addresses a single endpoint on an RC Server. It is unique only
// within the stream_id of the AVTPDU that carries it — the same ByteBusID
// value on two different streams may refer to two different endpoints.
type ByteBusID uint8

// TransactionNum correlates an RCP request with its eventual response. Like
// ByteBusID, it is scoped to the enclosing stream: two different streams may
// reuse the same TransactionNum value for unrelated exchanges. This package
// only carries the field — matching a response to its request is a concern
// for the RC Server/client lifecycle layered on top of this wire format.
type TransactionNum uint16
