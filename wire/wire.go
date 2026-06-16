// Package wire defines the shared RCP binary frame format used by both UDP and TLS transports.
package wire

import (
	"encoding/binary"
	"errors"

	rcp "github.com/SoundMatt/go-RCP"
)

// Wire format constants.
const (
	MagicByte0 = 0x52 // 'R'
	MagicByte1 = 0x43 // 'C'
	ProtoVer   = 0x01

	TypeCommand     = byte(0x01)
	TypeResponse    = byte(0x02)
	TypeStatus      = byte(0x03)
	TypeSubscribe   = byte(0x04)
	TypeUnsubscribe = byte(0x05)

	// HeaderLen is the fixed header size for all frame types.
	HeaderLen = 16

	// MaxPayload is the maximum payload bytes per frame.
	MaxPayload = 65507 - HeaderLen
)

var ErrShortFrame = errors.New("rcp/wire: frame too short")
var ErrBadMagic = errors.New("rcp/wire: bad magic bytes")
var ErrBadVersion = errors.New("rcp/wire: unsupported protocol version")

func ValidateHeader(b []byte) error {
	if len(b) < HeaderLen {
		return ErrShortFrame
	}
	if b[0] != MagicByte0 || b[1] != MagicByte1 {
		return ErrBadMagic
	}
	if b[2] != ProtoVer {
		return ErrBadVersion
	}
	return nil
}

func EncodeCommand(cmd *rcp.Command) []byte {
	pl := cmd.Payload
	buf := make([]byte, HeaderLen+len(pl))
	buf[0] = MagicByte0
	buf[1] = MagicByte1
	buf[2] = ProtoVer
	buf[3] = TypeCommand
	buf[4] = byte(cmd.Zone)
	binary.BigEndian.PutUint16(buf[5:7], uint16(cmd.Type))
	buf[7] = byte(cmd.Priority)
	binary.BigEndian.PutUint32(buf[8:12], cmd.ID)
	binary.BigEndian.PutUint32(buf[12:16], uint32(len(pl)))
	copy(buf[HeaderLen:], pl)
	return buf
}

func DecodeCommand(b []byte) (*rcp.Command, error) {
	if err := ValidateHeader(b); err != nil {
		return nil, err
	}
	bodyLen := binary.BigEndian.Uint32(b[12:16])
	if uint32(len(b)) < uint32(HeaderLen)+bodyLen {
		return nil, ErrShortFrame
	}
	cmd := &rcp.Command{
		Zone:     rcp.Zone(b[4]),
		Type:     rcp.CommandType(binary.BigEndian.Uint16(b[5:7])),
		Priority: rcp.Priority(b[7]),
		ID:       binary.BigEndian.Uint32(b[8:12]),
	}
	if bodyLen > 0 {
		cmd.Payload = make([]byte, bodyLen)
		copy(cmd.Payload, b[HeaderLen:HeaderLen+bodyLen])
	}
	return cmd, nil
}

func EncodeResponse(resp *rcp.Response) []byte {
	pl := resp.Payload
	buf := make([]byte, HeaderLen+len(pl))
	buf[0] = MagicByte0
	buf[1] = MagicByte1
	buf[2] = ProtoVer
	buf[3] = TypeResponse
	buf[4] = byte(resp.Zone)
	binary.BigEndian.PutUint16(buf[5:7], 0)
	buf[7] = byte(resp.Status)
	binary.BigEndian.PutUint32(buf[8:12], resp.CommandID)
	binary.BigEndian.PutUint32(buf[12:16], uint32(len(pl)))
	copy(buf[HeaderLen:], pl)
	return buf
}

func DecodeResponse(b []byte) (*rcp.Response, error) {
	if err := ValidateHeader(b); err != nil {
		return nil, err
	}
	bodyLen := binary.BigEndian.Uint32(b[12:16])
	if uint32(len(b)) < uint32(HeaderLen)+bodyLen {
		return nil, ErrShortFrame
	}
	resp := &rcp.Response{
		Zone:      rcp.Zone(b[4]),
		Status:    rcp.ResponseStatus(b[7]),
		CommandID: binary.BigEndian.Uint32(b[8:12]),
	}
	if bodyLen > 0 {
		resp.Payload = make([]byte, bodyLen)
		copy(resp.Payload, b[HeaderLen:HeaderLen+bodyLen])
	}
	return resp, nil
}

func EncodeStatus(st *rcp.Status) []byte {
	pl := st.Payload
	buf := make([]byte, HeaderLen+len(pl))
	buf[0] = MagicByte0
	buf[1] = MagicByte1
	buf[2] = ProtoVer
	buf[3] = TypeStatus
	buf[4] = byte(st.Zone)
	binary.BigEndian.PutUint16(buf[5:7], 0)
	if st.Healthy {
		buf[7] = 1
	}
	binary.BigEndian.PutUint32(buf[8:12], st.Seq)
	binary.BigEndian.PutUint32(buf[12:16], uint32(len(pl)))
	copy(buf[HeaderLen:], pl)
	return buf
}

func DecodeStatus(b []byte) (*rcp.Status, error) {
	if err := ValidateHeader(b); err != nil {
		return nil, err
	}
	bodyLen := binary.BigEndian.Uint32(b[12:16])
	if uint32(len(b)) < uint32(HeaderLen)+bodyLen {
		return nil, ErrShortFrame
	}
	st := &rcp.Status{
		Zone:    rcp.Zone(b[4]),
		Healthy: b[7] == 1,
		Seq:     binary.BigEndian.Uint32(b[8:12]),
	}
	if bodyLen > 0 {
		st.Payload = make([]byte, bodyLen)
		copy(st.Payload, b[HeaderLen:HeaderLen+bodyLen])
	}
	return st, nil
}

func EncodeControlFrame(msgType byte, zone rcp.Zone) []byte {
	buf := make([]byte, HeaderLen)
	buf[0] = MagicByte0
	buf[1] = MagicByte1
	buf[2] = ProtoVer
	buf[3] = msgType
	buf[4] = byte(zone)
	return buf
}
