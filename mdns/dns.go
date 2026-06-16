// Package mdns provides zero-configuration zone-controller discovery via mDNS/DNS-SD.
package mdns

//fusa:req REQ-MDNS-001
//fusa:req REQ-MDNS-002
//fusa:req REQ-MDNS-003
//fusa:req REQ-MDNS-004
//fusa:req REQ-MDNS-005
//fusa:req REQ-MDNS-006
//fusa:req REQ-MDNS-007
//fusa:req REQ-MDNS-008

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

// mDNS / DNS-SD constants.
const (
	MDNSAddr = "224.0.0.251"
	MDNSPort = 5353

	serviceType = "_rcp._udp.local."

	dnsTypeA   = 1
	dnsTypePTR = 12
	dnsTypeTXT = 16
	dnsTypeSRV = 33

	dnsClassIN = 1

	flagResponse  = uint16(0x8400) // QR=1, AA=1
	flagQuery     = uint16(0x0000) // standard query
	ttlAnnounce   = uint32(120)    // 2-minute TTL for announced records
)

var errTruncated = errors.New("mdns: truncated DNS message")
var errBadPointer = errors.New("mdns: invalid DNS label pointer")

// encodeName writes a DNS name as a sequence of length-prefixed labels.
func encodeName(name string) []byte {
	name = strings.TrimSuffix(name, ".")
	if name == "" {
		return []byte{0}
	}
	var buf []byte
	for _, label := range strings.Split(name, ".") {
		buf = append(buf, byte(len(label)))
		buf = append(buf, label...)
	}
	buf = append(buf, 0)
	return buf
}

// decodeName reads a DNS name starting at offset, following pointer compression.
// Returns the decoded name and the offset after the name in the original message.
func decodeName(msg []byte, off int) (string, int, error) {
	var parts []string
	visited := 0
	origOff := -1
	for {
		if off >= len(msg) {
			return "", 0, errTruncated
		}
		ln := int(msg[off])
		switch {
		case ln == 0:
			off++
			if origOff != -1 {
				off = origOff
			}
			return strings.Join(parts, ".") + ".", off, nil
		case ln&0xC0 == 0xC0:
			if off+1 >= len(msg) {
				return "", 0, errTruncated
			}
			ptr := int(binary.BigEndian.Uint16(msg[off:off+2]) & 0x3FFF)
			if ptr >= off {
				return "", 0, errBadPointer
			}
			if origOff == -1 {
				origOff = off + 2
			}
			off = ptr
			visited++
			if visited > 64 {
				return "", 0, errBadPointer
			}
		default:
			off++
			if off+ln > len(msg) {
				return "", 0, errTruncated
			}
			parts = append(parts, string(msg[off:off+ln]))
			off += ln
		}
	}
}

// dnsHeader holds a DNS message header.
type dnsHeader struct {
	id      uint16
	flags   uint16
	qdCount uint16
	anCount uint16
	nsCount uint16
	arCount uint16
}

func (h dnsHeader) encode() []byte {
	b := make([]byte, 12)
	binary.BigEndian.PutUint16(b[0:2], h.id)
	binary.BigEndian.PutUint16(b[2:4], h.flags)
	binary.BigEndian.PutUint16(b[4:6], h.qdCount)
	binary.BigEndian.PutUint16(b[6:8], h.anCount)
	binary.BigEndian.PutUint16(b[8:10], h.nsCount)
	binary.BigEndian.PutUint16(b[10:12], h.arCount)
	return b
}

func decodeHeader(b []byte) (dnsHeader, error) {
	if len(b) < 12 {
		return dnsHeader{}, errTruncated
	}
	return dnsHeader{
		id:      binary.BigEndian.Uint16(b[0:2]),
		flags:   binary.BigEndian.Uint16(b[2:4]),
		qdCount: binary.BigEndian.Uint16(b[4:6]),
		anCount: binary.BigEndian.Uint16(b[6:8]),
		nsCount: binary.BigEndian.Uint16(b[8:10]),
		arCount: binary.BigEndian.Uint16(b[10:12]),
	}, nil
}

// dnsRR is a parsed resource record.
type dnsRR struct {
	name  string
	typ   uint16
	class uint16
	ttl   uint32
	rdata []byte
}

// buildPTRQuery encodes a DNS-SD PTR query for _rcp._udp.local.
func buildPTRQuery() []byte {
	var buf []byte
	hdr := dnsHeader{id: 0, flags: flagQuery, qdCount: 1}
	buf = append(buf, hdr.encode()...)
	buf = append(buf, encodeName(serviceType)...)
	buf = append(buf, 0, dnsTypePTR)
	buf = append(buf, 0, dnsClassIN)
	return buf
}

// buildAnnouncement builds a mDNS proactive announcement for a zone controller.
// instanceName: e.g. "zone-front-left._rcp._udp.local."
// host: e.g. "zone-front-left.local."
// ip4: 4-byte IPv4 address
// port: UDP port
// zoneTXT: e.g. "zone=1"
func buildAnnouncement(instanceName, host string, ip4 []byte, port uint16, zoneTXT string) []byte {
	var buf []byte
	hdr := dnsHeader{flags: flagResponse, anCount: 3, arCount: 1}
	buf = append(buf, hdr.encode()...)

	// PTR record: _rcp._udp.local. -> instanceName
	buf = append(buf, encodeRR(serviceType, dnsTypePTR, ttlAnnounce, encodeName(instanceName))...)

	// SRV record
	srv := make([]byte, 6+len(encodeName(host)))
	binary.BigEndian.PutUint16(srv[0:2], 0) // priority
	binary.BigEndian.PutUint16(srv[2:4], 0) // weight
	binary.BigEndian.PutUint16(srv[4:6], port)
	copy(srv[6:], encodeName(host))
	buf = append(buf, encodeRR(instanceName, dnsTypeSRV, ttlAnnounce, srv)...)

	// TXT record
	txt := make([]byte, 1+len(zoneTXT))
	txt[0] = byte(len(zoneTXT))
	copy(txt[1:], zoneTXT)
	buf = append(buf, encodeRR(instanceName, dnsTypeTXT, ttlAnnounce, txt)...)

	// A record (additional)
	buf = append(buf, encodeRR(host, dnsTypeA, ttlAnnounce, ip4[:4])...)
	return buf
}

func encodeRR(name string, typ uint16, ttl uint32, rdata []byte) []byte {
	var buf []byte
	buf = append(buf, encodeName(name)...)
	buf = append(buf, 0, 0) // type
	binary.BigEndian.PutUint16(buf[len(buf)-2:], typ)
	buf = append(buf, 0, dnsClassIN) // class IN
	ttlB := make([]byte, 4)
	binary.BigEndian.PutUint32(ttlB, ttl)
	buf = append(buf, ttlB...)
	rdlen := make([]byte, 2)
	binary.BigEndian.PutUint16(rdlen, uint16(len(rdata)))
	buf = append(buf, rdlen...)
	buf = append(buf, rdata...)
	return buf
}

// parseRRs parses the answer, authority, and additional sections of a DNS message.
func parseRRs(msg []byte) ([]dnsRR, error) {
	hdr, err := decodeHeader(msg)
	if err != nil {
		return nil, err
	}
	off := 12
	// skip questions
	for i := 0; i < int(hdr.qdCount); i++ {
		_, off, err = decodeName(msg, off)
		if err != nil {
			return nil, err
		}
		off += 4 // QTYPE + QCLASS
	}

	total := int(hdr.anCount) + int(hdr.nsCount) + int(hdr.arCount)
	rrs := make([]dnsRR, 0, total)
	for i := 0; i < total; i++ {
		rr, newOff, rErr := parseOneRR(msg, off)
		if rErr != nil {
			return rrs, nil // return what we have
		}
		rrs = append(rrs, rr)
		off = newOff
	}
	return rrs, nil
}

func parseOneRR(msg []byte, off int) (dnsRR, int, error) {
	name, off, err := decodeName(msg, off)
	if err != nil {
		return dnsRR{}, 0, err
	}
	if off+10 > len(msg) {
		return dnsRR{}, 0, errTruncated
	}
	typ := binary.BigEndian.Uint16(msg[off : off+2])
	class := binary.BigEndian.Uint16(msg[off+2 : off+4])
	ttl := binary.BigEndian.Uint32(msg[off+4 : off+8])
	rdLen := int(binary.BigEndian.Uint16(msg[off+8 : off+10]))
	off += 10
	if off+rdLen > len(msg) {
		return dnsRR{}, 0, errTruncated
	}
	rdata := make([]byte, rdLen)
	copy(rdata, msg[off:off+rdLen])
	return dnsRR{name: name, typ: typ, class: class, ttl: ttl, rdata: rdata}, off + rdLen, nil
}

// extractServiceInfo parses PTR/SRV/TXT/A records into a ServiceInfo.
type ServiceInfo struct {
	Instance string
	Zone     uint8
	Addr     string // "ip:port"
}

func extractServices(rrs []dnsRR, msg []byte) []ServiceInfo {
	ptrTargets := map[string]bool{}
	srvPorts := map[string]uint16{}
	srvHosts := map[string]string{}
	aRecords := map[string]string{}
	txtZones := map[string]uint8{}

	for _, rr := range rrs {
		switch rr.typ {
		case dnsTypePTR:
			if strings.HasSuffix(rr.name, serviceType) || rr.name == serviceType {
				name, _, err := decodeName(append(rr.rdata, 0), 0)
				if err == nil {
					ptrTargets[name] = true
				}
			}
		case dnsTypeSRV:
			if len(rr.rdata) >= 7 {
				port := binary.BigEndian.Uint16(rr.rdata[4:6])
				host, _, err := decodeName(append(rr.rdata[6:], 0), 0)
				if err == nil {
					srvPorts[rr.name] = port
					srvHosts[rr.name] = host
				}
			}
		case dnsTypeTXT:
			off := 0
			for off < len(rr.rdata) {
				l := int(rr.rdata[off])
				off++
				if off+l > len(rr.rdata) {
					break
				}
				kv := string(rr.rdata[off : off+l])
				off += l
				if strings.HasPrefix(kv, "zone=") {
					var z uint8
					if _, err := fmt.Sscanf(kv[5:], "%d", &z); err == nil {
						txtZones[rr.name] = z
					}
				}
			}
		case dnsTypeA:
			if len(rr.rdata) == 4 {
				aRecords[rr.name] = fmt.Sprintf("%d.%d.%d.%d", rr.rdata[0], rr.rdata[1], rr.rdata[2], rr.rdata[3])
			}
		}
	}

	var out []ServiceInfo
	for inst := range ptrTargets {
		host, ok := srvHosts[inst]
		if !ok {
			continue
		}
		port, ok := srvPorts[inst]
		if !ok {
			continue
		}
		ip, ok := aRecords[host]
		if !ok {
			continue
		}
		zone, ok := txtZones[inst]
		if !ok {
			continue
		}
		out = append(out, ServiceInfo{
			Instance: inst,
			Zone:     zone,
			Addr:     fmt.Sprintf("%s:%d", ip, port),
		})
	}
	return out
}
