package l2

import "encoding/binary"

// MACLen is the byte length of an Ethernet MAC address.
const MACLen = 6

// EtherTypeAVTP is the EtherType value TC18 §10.1 assigns to a raw
// Ethernet-carried AVTPDU, quoted directly from the specification text:
// "an AVTPDU is marked by an EtherType value of 0x22F0." See doc.go's
// provenance note.
const EtherTypeAVTP uint16 = 0x22F0

// HeaderLen is the wire size of this package's Ethernet header: destination
// MAC (MACLen) + source MAC (MACLen) + EtherType (2 bytes).
const HeaderLen = MACLen + MACLen + 2

// EncodeFrame builds a raw Ethernet frame carrying avtpdu: dst (MACLen
// bytes) + src (MACLen bytes) + EtherTypeAVTP (2 bytes, big-endian,
// matching this repo's existing AVTPDU byte-order convention — see
// avtp.EncodeHeader) + avtpdu unchanged. There is no trailing frame check
// sequence here — the NIC/driver appends and verifies that itself — and, in
// contrast with the sibling udp package's Annex J framing (see
// udp/annexj.go), no encapsulation sequence number: that field exists only
// on the UDP/IP wire, not at layer 2.
func EncodeFrame(dst, src [MACLen]byte, avtpdu []byte) []byte {
	out := make([]byte, HeaderLen+len(avtpdu))
	copy(out[0:MACLen], dst[:])
	copy(out[MACLen:2*MACLen], src[:])
	binary.BigEndian.PutUint16(out[2*MACLen:HeaderLen], EtherTypeAVTP)
	copy(out[HeaderLen:], avtpdu)
	return out
}

// DecodeFrame parses a raw Ethernet frame into its destination/source MAC
// addresses and AVTPDU payload. It returns ErrShortFrame if b is too short
// to hold a full Ethernet header, and ErrUnexpectedEtherType if b's
// EtherType field is not EtherTypeAVTP — the same "reject, don't panic"
// posture avtp.DecodeHeader/acf.DecodeFrame already use for malformed
// input in this repo's sibling packages.
func DecodeFrame(b []byte) (dst, src [MACLen]byte, avtpdu []byte, err error) {
	if len(b) < HeaderLen {
		return dst, src, nil, ErrShortFrame
	}
	copy(dst[:], b[0:MACLen])
	copy(src[:], b[MACLen:2*MACLen])
	etherType := binary.BigEndian.Uint16(b[2*MACLen : HeaderLen])
	if etherType != EtherTypeAVTP {
		return dst, src, nil, ErrUnexpectedEtherType
	}
	avtpdu = b[HeaderLen:]
	return dst, src, avtpdu, nil
}
