package udp

import (
	"encoding/binary"
	"net"
	"strconv"
)

//fusa:req REQ-UDP-015
//fusa:req REQ-UDP-016
//fusa:req REQ-UDP-017

// AnnexJEncapSeqLen is the wire size, in bytes, of the encapsulation
// sequence number IEEE 1722-2016 Annex J prepends to every AVTPDU carried
// over UDP/IP. See this file's provenance note below.
const AnnexJEncapSeqLen = 4

// AnnexJControlPort is the standard destination UDP port Annex J assigns to
// "Discrete" (control-plane) AVTP traffic — the traffic class RCP's own
// request/response/acknowledge exchanges fall under (TC18 §10.1: "[IEEE1722]
// can also be used in IP-networks... Encapsulation of 1722 frames in
// IP/UDP and port usage is described in Annex J."). This package's
// Controller and Server both default to this port whenever a caller dials
// or listens without naming one explicitly — see resolveAnnexJAddr and
// defaultAnnexJPort.
//
// # Provenance (Guiding Principle 10)
//
// This package does not have access to the paywalled IEEE 1722-2016
// standard text itself. This port assignment, AnnexJContinuousPort, and the
// encapsulation-sequence-number wire field this file implements
// (prependEncapSeq/stripEncapSeq) are drawn from two independent public
// secondary sources, cross-checked against each other:
//
//   - A Wireshark issue tracker discussion of the real Annex J wire format.
//   - The COVESA Open1722 open-source reference implementation's actual
//     Avtp_Udp_t header struct (include/avtp/Udp.h, BSD-3-Clause,
//     github.com/COVESA/Open1722), which encodes exactly this 4-byte
//     leading sequence-number field ahead of the AVTPDU bytes.
//
// Neither is the primary standard text. This is stated here explicitly,
// rather than presented as if independently verified against the primary
// source, matching this repo's established practice of flagging
// secondary-source-only provenance rather than overclaiming (see
// e2e/doc.go's CRC32P4 provenance note and avtp/doc.go's "note on spec
// fidelity" for precedent).
const AnnexJControlPort = 17221

// AnnexJContinuousPort is Annex J's standard destination UDP port for
// "Continuous" (streaming/periodic) AVTP traffic, per the same two
// secondary sources as AnnexJControlPort. This package's Controller and
// Server only ever originate/serve control-plane (RCP request/response)
// traffic, so nothing in this package binds or dials this port by
// default — it is exported for a caller building a separate,
// streaming-oriented use of this package's framing that specifically needs
// to name it.
const AnnexJContinuousPort = 17220

// resolveAnnexJAddr resolves addr for this package's UDP use (Server's
// listen side), defaulting to AnnexJControlPort when addr names a host with
// no explicit port at all (e.g. "127.0.0.1" or "zone-ecu.example") rather
// than requiring every caller to spell out ":17221" by hand. A caller that
// wants a different port — including port 0 for an OS-assigned ephemeral
// port, which this package's own tests rely on for test isolation (e.g.
// "127.0.0.1:0" throughout udp_test.go and every sibling package's test
// suite that dials a udp.Server) — states it explicitly, and that choice is
// honored completely unchanged: only the totally-absent-port case is
// defaulted.
func resolveAnnexJAddr(addr string) (*net.UDPAddr, error) {
	if _, _, err := net.SplitHostPort(addr); err != nil {
		addr = net.JoinHostPort(addr, strconv.Itoa(AnnexJControlPort))
	}
	return net.ResolveUDPAddr("udp", addr)
}

// defaultAnnexJPort returns addr unchanged if it already names an explicit
// non-zero port, or a copy with Port set to AnnexJControlPort if
// addr.Port == 0 — the net.UDPAddr equivalent of resolveAnnexJAddr's
// string-based defaulting, for Controller, which is handed an
// already-resolved *net.UDPAddr rather than a string. Port 0 has no
// legitimate meaning as an explicit remote dial target (nothing listens on
// UDP port 0), so treating it as "caller didn't specify a port" is
// unambiguous here — unlike on the listen side, where "0" deliberately
// keeps its OS-assigned-ephemeral-port meaning (see resolveAnnexJAddr).
func defaultAnnexJPort(addr *net.UDPAddr) *net.UDPAddr {
	if addr == nil || addr.Port != 0 {
		return addr
	}
	return &net.UDPAddr{IP: addr.IP, Port: AnnexJControlPort, Zone: addr.Zone}
}

// prependEncapSeq returns a new byte slice: seq encoded as a big-endian
// uint32 (matching this repo's existing AVTPDU byte-order convention — see
// avtp.EncodeHeader), followed by avtpdu unchanged. This is Annex J's
// UDP/IP encapsulation wrapper: the 4-byte encapsulation sequence number
// field exists only on the UDP/IP wire, ahead of the AVTPDU itself (which
// starts at its own subtype byte) — it has no equivalent at layer 2 (see
// the sibling l2 package, whose EncodeFrame carries no such field).
func prependEncapSeq(seq uint32, avtpdu []byte) []byte {
	out := make([]byte, AnnexJEncapSeqLen+len(avtpdu))
	binary.BigEndian.PutUint32(out[:AnnexJEncapSeqLen], seq)
	copy(out[AnnexJEncapSeqLen:], avtpdu)
	return out
}

// stripEncapSeq reads the leading 4-byte encapsulation sequence number off
// b and returns it alongside the remaining bytes (the AVTPDU itself, ready
// to hand to acf.DecodeFrame). It returns ErrShortBuffer if b is too short
// to hold the field at all.
//
// Neither Controller nor Server currently surfaces the returned seq to any
// caller-facing API — they only strip it. Annex J's exact receiver-side
// semantics for this field (e.g. whether/how a receiver is meant to use it
// for loss detection across a UDP session) are not stated in either of this
// file's public secondary sources, so this package does not invent one; see
// this file's provenance note. seq is returned rather than silently
// discarded so a future caller-facing use (if the real semantics are ever
// confirmed) does not require re-parsing the wire format again.
func stripEncapSeq(b []byte) (seq uint32, rest []byte, err error) {
	if len(b) < AnnexJEncapSeqLen {
		return 0, nil, ErrShortBuffer
	}
	return binary.BigEndian.Uint32(b[:AnnexJEncapSeqLen]), b[AnnexJEncapSeqLen:], nil
}
