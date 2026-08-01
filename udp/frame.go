package udp

import "github.com/SoundMatt/go-RCP/avtp"

// maxTimedHeaderLen is the largest AVTPDU header this package's decode path
// needs to budget for: the presentation-timestamped (TSCF) variant. avtp
// does not export its own wire length (Header.wireLen is unexported, an
// internal encode/decode bookkeeping detail, not a public transport
// concern), so this package restates just the one number it actually needs
// as its own transport-level buffer-sizing constant.
//
// Per TC18 §11.1 p.22 Figure 5 a TSCF header is six quadlets: the "subtype
// data" quadlet, stream_id (two quadlets), avtp_timestamp, the "Format
// specific" reserved quadlet, and the "Packet Info" quadlet carrying
// stream_data_length. That is 24 octets, not the 17 this constant claimed
// through v8.0.0, when avtp itself mis-encoded both header variants.
const maxTimedHeaderLen = 6 * 4

// MaxFrameLen is the largest single UDP datagram this package's Controller
// and Server ever need to send or receive: Annex J's 4-byte encapsulation
// sequence number (AnnexJEncapSeqLen — see annexj.go) plus the largest
// AVTPDU header variant plus the largest RCP-over-ACF message
// avtp.Header.DataLength can declare. It is comfortably under the
// 65507-byte practical UDP payload ceiling, so this package does not need
// the old wire package's own MaxPayload-vs-UDP-ceiling accounting.
const MaxFrameLen = AnnexJEncapSeqLen + maxTimedHeaderLen + avtp.MaxDataLength
