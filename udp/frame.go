package udp

import "github.com/SoundMatt/go-RCP/avtp"

// maxTimedHeaderLen is the largest AVTPDU header this package's decode path
// needs to budget for: the presentation-timestamped (TSCF) variant. avtp
// does not export its own wire length (Header.wireLen is unexported, an
// internal encode/decode bookkeeping detail, not a public transport
// concern), so this package restates just the one number it actually needs
// — the untimed header (13 bytes) plus the 4-byte timestamp field TSCF
// headers append — as its own transport-level buffer-sizing constant.
const maxTimedHeaderLen = 13 + 4

// MaxFrameLen is the largest single UDP datagram this package's Controller
// and Server ever need to send or receive: the largest AVTPDU header
// variant plus the largest RCP-over-ACF message avtp.Header.DataLength can
// declare. It is comfortably under the 65507-byte practical UDP payload
// ceiling, so this package does not need the old wire package's own
// MaxPayload-vs-UDP-ceiling accounting.
const MaxFrameLen = maxTimedHeaderLen + avtp.MaxDataLength
