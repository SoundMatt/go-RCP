// Package udp provides a pure-Go UDP transport for the RCP protocol.
package udp

//fusa:req REQ-UDP-001
//fusa:req REQ-UDP-002
//fusa:req REQ-UDP-003
//fusa:req REQ-UDP-004
//fusa:req REQ-UDP-005
//fusa:req REQ-UDP-006
//fusa:req REQ-UDP-007
//fusa:req REQ-UDP-008
//fusa:req REQ-UDP-009
//fusa:req REQ-UDP-010
//fusa:req REQ-UDP-011
//fusa:req REQ-UDP-012

import (
	"encoding/binary"
	"errors"

	rcp "github.com/SoundMatt/go-RCP"
)

// Wire format constants.
const (
	magicByte0 = 0x52 // 'R'
	magicByte1 = 0x43 // 'C'
	protoVer   = 0x01

	typeCommand     = byte(0x01)
	typeResponse    = byte(0x02)
	typeStatus      = byte(0x03)
	typeSubscribe   = byte(0x04)
	typeUnsubscribe = byte(0x05)

	// headerLen is the fixed header size for all frame types.
	// Layout:
	//   [0]     magic0 = 'R'
	//   [1]     magic1 = 'C'
	//   [2]     version
	//   [3]     msgType
	//   [4]     zone
	//   [5:7]   field16 (cmdType for Cmd; 0 for others) (uint16 BE)
	//   [7]     field8  (priority for Cmd; respStatus for Resp; healthy for Status; 0 for others)
	//   [8:12]  id      (Command.ID for Cmd/Resp; Status.Seq for Status) (uint32 BE)
	//   [12:16] bodyLen (uint32 BE)
	//   [16..]  payload
	headerLen = 16

	// MaxPayload is the maximum payload size in a single UDP frame.
	MaxPayload = 65507 - headerLen
)

var errShortFrame = errors.New("rcp/udp: frame too short")
var errBadMagic = errors.New("rcp/udp: bad magic bytes")
var errBadVersion = errors.New("rcp/udp: unsupported protocol version")

func validateHeader(b []byte) error {
	if len(b) < headerLen {
		return errShortFrame
	}
	if b[0] != magicByte0 || b[1] != magicByte1 {
		return errBadMagic
	}
	if b[2] != protoVer {
		return errBadVersion
	}
	return nil
}

func encodeCommand(cmd *rcp.Command) []byte {
	pl := cmd.Payload
	buf := make([]byte, headerLen+len(pl))
	buf[0] = magicByte0
	buf[1] = magicByte1
	buf[2] = protoVer
	buf[3] = typeCommand
	buf[4] = byte(cmd.Zone)
	binary.BigEndian.PutUint16(buf[5:7], uint16(cmd.Type))
	buf[7] = byte(cmd.Priority)
	binary.BigEndian.PutUint32(buf[8:12], cmd.ID)
	binary.BigEndian.PutUint32(buf[12:16], uint32(len(pl)))
	copy(buf[headerLen:], pl)
	return buf
}

func decodeCommand(b []byte) (*rcp.Command, error) {
	if err := validateHeader(b); err != nil {
		return nil, err
	}
	bodyLen := binary.BigEndian.Uint32(b[12:16])
	if uint32(len(b)) < uint32(headerLen)+bodyLen {
		return nil, errShortFrame
	}
	cmd := &rcp.Command{
		Zone:     rcp.Zone(b[4]),
		Type:     rcp.CommandType(binary.BigEndian.Uint16(b[5:7])),
		Priority: rcp.Priority(b[7]),
		ID:       binary.BigEndian.Uint32(b[8:12]),
	}
	if bodyLen > 0 {
		cmd.Payload = make([]byte, bodyLen)
		copy(cmd.Payload, b[headerLen:headerLen+bodyLen])
	}
	return cmd, nil
}

func encodeResponse(resp *rcp.Response) []byte {
	pl := resp.Payload
	buf := make([]byte, headerLen+len(pl))
	buf[0] = magicByte0
	buf[1] = magicByte1
	buf[2] = protoVer
	buf[3] = typeResponse
	buf[4] = byte(resp.Zone)
	binary.BigEndian.PutUint16(buf[5:7], 0)
	buf[7] = byte(resp.Status)
	binary.BigEndian.PutUint32(buf[8:12], resp.CommandID)
	binary.BigEndian.PutUint32(buf[12:16], uint32(len(pl)))
	copy(buf[headerLen:], pl)
	return buf
}

func decodeResponse(b []byte) (*rcp.Response, error) {
	if err := validateHeader(b); err != nil {
		return nil, err
	}
	bodyLen := binary.BigEndian.Uint32(b[12:16])
	if uint32(len(b)) < uint32(headerLen)+bodyLen {
		return nil, errShortFrame
	}
	resp := &rcp.Response{
		Zone:      rcp.Zone(b[4]),
		Status:    rcp.ResponseStatus(b[7]),
		CommandID: binary.BigEndian.Uint32(b[8:12]),
	}
	if bodyLen > 0 {
		resp.Payload = make([]byte, bodyLen)
		copy(resp.Payload, b[headerLen:headerLen+bodyLen])
	}
	return resp, nil
}

func encodeStatus(st *rcp.Status) []byte {
	pl := st.Payload
	buf := make([]byte, headerLen+len(pl))
	buf[0] = magicByte0
	buf[1] = magicByte1
	buf[2] = protoVer
	buf[3] = typeStatus
	buf[4] = byte(st.Zone)
	binary.BigEndian.PutUint16(buf[5:7], 0)
	if st.Healthy {
		buf[7] = 1
	}
	binary.BigEndian.PutUint32(buf[8:12], st.Seq)
	binary.BigEndian.PutUint32(buf[12:16], uint32(len(pl)))
	copy(buf[headerLen:], pl)
	return buf
}

func decodeStatus(b []byte) (*rcp.Status, error) {
	if err := validateHeader(b); err != nil {
		return nil, err
	}
	bodyLen := binary.BigEndian.Uint32(b[12:16])
	if uint32(len(b)) < uint32(headerLen)+bodyLen {
		return nil, errShortFrame
	}
	st := &rcp.Status{
		Zone:    rcp.Zone(b[4]),
		Healthy: b[7] == 1,
		Seq:     binary.BigEndian.Uint32(b[8:12]),
	}
	if bodyLen > 0 {
		st.Payload = make([]byte, bodyLen)
		copy(st.Payload, b[headerLen:headerLen+bodyLen])
	}
	return st, nil
}

func encodeControlFrame(msgType byte, zone rcp.Zone) []byte {
	buf := make([]byte, headerLen)
	buf[0] = magicByte0
	buf[1] = magicByte1
	buf[2] = protoVer
	buf[3] = msgType
	buf[4] = byte(zone)
	return buf
}
