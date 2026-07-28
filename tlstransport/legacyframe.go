package tlstransport

// This file is this package's own frozen copy of the retired wire
// package's bespoke 16-byte frame header encode/decode logic (see
// ROADMAP.md Milestone 54, v0.67.0). Every other satellite package that
// depended on wire (udp, tsn, shmem) migrated to the real IEEE 1722
// AVTPDU/ACF wire format wire's retirement made way for; this package
// deliberately does not, because — per Phase 17's disposition table —
// tlstransport itself is DEPRECATE-flagged, not adapted: mutual TLS over a
// TCP byte stream framed by a bespoke Command/Response/Status header has no
// place in the TC18 model at all (the specification's link-security story
// is MACsec at layer 2, and its addressing model is stream_id/byte_bus_id,
// not a Zone-keyed Command/Response exchange this transport still
// carries). Rebuilding this framing against AVTPDU/ACF would suggest this
// package is a real spec-conformant transport option, which it explicitly
// is not and never will be — see doc.go. This inlined copy exists solely so
// the package keeps compiling and behaving exactly as it did before wire's
// retirement, for whatever non-spec, bespoke use a caller still has for it.

import (
	"encoding/binary"
	"errors"

	rcp "github.com/SoundMatt/go-RCP"
)

const (
	legacyMagicByte0 = 0x52 // 'R'
	legacyMagicByte1 = 0x43 // 'C'
	legacyProtoVer   = 0x01

	legacyTypeCommand     = byte(0x01)
	legacyTypeResponse    = byte(0x02)
	legacyTypeStatus      = byte(0x03)
	legacyTypeSubscribe   = byte(0x04)
	legacyTypeUnsubscribe = byte(0x05)

	// legacyHeaderLen is the fixed header size for all frame types.
	legacyHeaderLen = 16
)

var errLegacyShortFrame = errors.New("rcp/tlstransport: legacy frame too short")
var errLegacyBadMagic = errors.New("rcp/tlstransport: legacy frame bad magic bytes")
var errLegacyBadVersion = errors.New("rcp/tlstransport: legacy frame unsupported protocol version")

func legacyValidateHeader(b []byte) error {
	if len(b) < legacyHeaderLen {
		return errLegacyShortFrame
	}
	if b[0] != legacyMagicByte0 || b[1] != legacyMagicByte1 {
		return errLegacyBadMagic
	}
	if b[2] != legacyProtoVer {
		return errLegacyBadVersion
	}
	return nil
}

func legacyEncodeCommand(cmd *rcp.Command) []byte {
	pl := cmd.Payload
	buf := make([]byte, legacyHeaderLen+len(pl))
	buf[0] = legacyMagicByte0
	buf[1] = legacyMagicByte1
	buf[2] = legacyProtoVer
	buf[3] = legacyTypeCommand
	buf[4] = byte(cmd.Zone)
	binary.BigEndian.PutUint16(buf[5:7], uint16(cmd.Type))
	buf[7] = byte(cmd.Priority)
	binary.BigEndian.PutUint32(buf[8:12], cmd.ID)
	binary.BigEndian.PutUint32(buf[12:16], uint32(len(pl)))
	copy(buf[legacyHeaderLen:], pl)
	return buf
}

func legacyDecodeCommand(b []byte) (*rcp.Command, error) {
	if err := legacyValidateHeader(b); err != nil {
		return nil, err
	}
	bodyLen := binary.BigEndian.Uint32(b[12:16])
	if uint64(len(b)) < uint64(legacyHeaderLen)+uint64(bodyLen) {
		return nil, errLegacyShortFrame
	}
	cmd := &rcp.Command{
		Zone:     rcp.Zone(b[4]),
		Type:     rcp.CommandType(binary.BigEndian.Uint16(b[5:7])),
		Priority: rcp.Priority(b[7]),
		ID:       binary.BigEndian.Uint32(b[8:12]),
	}
	if bodyLen > 0 {
		cmd.Payload = make([]byte, bodyLen)
		copy(cmd.Payload, b[legacyHeaderLen:legacyHeaderLen+bodyLen])
	}
	return cmd, nil
}

func legacyEncodeResponse(resp *rcp.Response) []byte {
	pl := resp.Payload
	buf := make([]byte, legacyHeaderLen+len(pl))
	buf[0] = legacyMagicByte0
	buf[1] = legacyMagicByte1
	buf[2] = legacyProtoVer
	buf[3] = legacyTypeResponse
	buf[4] = byte(resp.Zone)
	binary.BigEndian.PutUint16(buf[5:7], 0)
	buf[7] = byte(resp.Status)
	binary.BigEndian.PutUint32(buf[8:12], resp.CommandID)
	binary.BigEndian.PutUint32(buf[12:16], uint32(len(pl)))
	copy(buf[legacyHeaderLen:], pl)
	return buf
}

func legacyDecodeResponse(b []byte) (*rcp.Response, error) {
	if err := legacyValidateHeader(b); err != nil {
		return nil, err
	}
	bodyLen := binary.BigEndian.Uint32(b[12:16])
	if uint64(len(b)) < uint64(legacyHeaderLen)+uint64(bodyLen) {
		return nil, errLegacyShortFrame
	}
	resp := &rcp.Response{
		Zone:      rcp.Zone(b[4]),
		Status:    rcp.ResponseStatus(b[7]),
		CommandID: binary.BigEndian.Uint32(b[8:12]),
	}
	if bodyLen > 0 {
		resp.Payload = make([]byte, bodyLen)
		copy(resp.Payload, b[legacyHeaderLen:legacyHeaderLen+bodyLen])
	}
	return resp, nil
}

func legacyDecodeStatus(b []byte) (*rcp.Status, error) {
	if err := legacyValidateHeader(b); err != nil {
		return nil, err
	}
	bodyLen := binary.BigEndian.Uint32(b[12:16])
	if uint64(len(b)) < uint64(legacyHeaderLen)+uint64(bodyLen) {
		return nil, errLegacyShortFrame
	}
	st := &rcp.Status{
		Zone:    rcp.Zone(b[4]),
		Healthy: b[7] == 1,
		Seq:     binary.BigEndian.Uint32(b[8:12]),
	}
	if bodyLen > 0 {
		st.Payload = make([]byte, bodyLen)
		copy(st.Payload, b[legacyHeaderLen:legacyHeaderLen+bodyLen])
	}
	return st, nil
}

func legacyEncodeStatus(st *rcp.Status) []byte {
	pl := st.Payload
	buf := make([]byte, legacyHeaderLen+len(pl))
	buf[0] = legacyMagicByte0
	buf[1] = legacyMagicByte1
	buf[2] = legacyProtoVer
	buf[3] = legacyTypeStatus
	buf[4] = byte(st.Zone)
	binary.BigEndian.PutUint16(buf[5:7], 0)
	if st.Healthy {
		buf[7] = 1
	}
	binary.BigEndian.PutUint32(buf[8:12], st.Seq)
	binary.BigEndian.PutUint32(buf[12:16], uint32(len(pl)))
	copy(buf[legacyHeaderLen:], pl)
	return buf
}

func legacyEncodeControlFrame(msgType byte, zone rcp.Zone) []byte {
	buf := make([]byte, legacyHeaderLen)
	buf[0] = legacyMagicByte0
	buf[1] = legacyMagicByte1
	buf[2] = legacyProtoVer
	buf[3] = msgType
	buf[4] = byte(zone)
	return buf
}
