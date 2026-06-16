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
	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/wire"
)

// Re-export wire constants for use within this package.
const (
	headerLen    = wire.HeaderLen
	MaxPayload   = wire.MaxPayload
	typeCommand     = wire.TypeCommand
	typeResponse    = wire.TypeResponse
	typeStatus      = wire.TypeStatus
	typeSubscribe   = wire.TypeSubscribe
	typeUnsubscribe = wire.TypeUnsubscribe
)

func encodeCommand(cmd *rcp.Command) []byte    { return wire.EncodeCommand(cmd) }
func decodeCommand(b []byte) (*rcp.Command, error) { return wire.DecodeCommand(b) }
func encodeResponse(resp *rcp.Response) []byte { return wire.EncodeResponse(resp) }
func decodeResponse(b []byte) (*rcp.Response, error) { return wire.DecodeResponse(b) }
func encodeStatus(st *rcp.Status) []byte       { return wire.EncodeStatus(st) }
func decodeStatus(b []byte) (*rcp.Status, error)  { return wire.DecodeStatus(b) }
func encodeControlFrame(msgType byte, zone rcp.Zone) []byte {
	return wire.EncodeControlFrame(msgType, zone)
}
