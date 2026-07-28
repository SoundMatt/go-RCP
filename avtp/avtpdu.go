package avtp

import "encoding/binary"

// AVTPDU subtype tags this package assigns to the two RCP-carrying control
// formats it implements: the untimed form (NTSCF) and the presentation-
// timestamped form (TSCF). See the package doc's "note on spec fidelity" —
// these exact byte values are this implementation's own choice, not a
// verified transcription of a published registry.
const (
	SubtypeNTSCF byte = 0x82 // untimed, "execute as soon as possible"
	SubtypeTSCF  byte = 0x83 // presentation-timestamped
)

// ProtocolVersion is the only AVTPDU header version this package accepts.
const ProtocolVersion uint8 = 0

const (
	// untimedHeaderLen is the wire size of an NTSCF header: subtype(1) +
	// flags(1) + sequence_num(1) + data_length(2) + stream_id(8).
	untimedHeaderLen = 13

	// timedHeaderLen is the wire size of a TSCF header: untimedHeaderLen
	// plus a 4-byte presentation timestamp.
	timedHeaderLen = untimedHeaderLen + 4

	// dataLengthMask bounds the 11-bit data-length field.
	dataLengthMask = 0x07FF
)

// TimestampStatus is the validity marker carried by a timestamped (TSCF)
// AVTPDU header. It is meaningless on an untimed (NTSCF) header.
type TimestampStatus uint8

const (
	// TimestampMissing is the safe zero value: no marker has been set.
	// Header.Disposition treats it the same as TimestampInvalid.
	TimestampMissing TimestampStatus = iota

	// TimestampValid indicates the presentation timestamp was captured
	// from a synchronized clock and may be scheduled against.
	TimestampValid

	// TimestampInvalid indicates the sender explicitly could not vouch for
	// the timestamp (e.g. it was never synchronized).
	TimestampInvalid

	// TimestampUncertain indicates the sender's clock sync is degraded or
	// unverified at the moment of capture (e.g. a sync interval was
	// missed), short of being outright invalid.
	TimestampUncertain
)

// timestampStatusMask bounds the 2-bit timestamp-status field.
const timestampStatusMask = 0x03

// Disposition describes how a received AVTPDU should be handled given the
// receiving server's own time-synchronization capability.
type Disposition uint8

const (
	// DispositionBestEffort means: execute as soon as possible: the
	// header is untimed, or its timestamp marker is missing, invalid, or
	// uncertain.
	DispositionBestEffort Disposition = iota

	// DispositionScheduled means: execute at the header's presentation
	// timestamp. Only possible for a timed header with a valid marker,
	// received by a server that supports time synchronization.
	DispositionScheduled

	// DispositionDrop means: reject the AVTPDU outright. Only applies to a
	// timed (TSCF) header received by a server with no time-sync support
	// at all — it has no clock to schedule against or fall back from.
	DispositionDrop
)

// Header is a decoded IEEE 1722 AVTPDU header in either its untimed (NTSCF)
// or presentation-timestamped (TSCF) form.
type Header struct {
	// Timed selects the header variant: false for NTSCF (untimed), true
	// for TSCF (presentation-timestamped).
	Timed bool

	// StreamIDValid mirrors the AVTP "sv" bit: whether StreamID identifies
	// a real stream. This package always encodes/decodes StreamID
	// regardless of this flag's value; callers that need the null-stream
	// convention check it themselves.
	StreamIDValid bool

	// SequenceNum is the per-stream AVTPDU sequence counter.
	SequenceNum uint8

	// DataLength is the length, in bytes, of the RCP message that follows
	// this header. It fits an 11-bit wire field (0-2047).
	DataLength uint16

	// StreamID addresses the sender of this AVTPDU.
	StreamID StreamID

	// Timestamp is the presentation timestamp. Only meaningful when Timed
	// is true.
	Timestamp uint32

	// TimestampStatus is the validity marker for Timestamp. Only
	// meaningful when Timed is true.
	TimestampStatus TimestampStatus
}

// wireLen returns the encoded size of h given its Timed variant.
func (h Header) wireLen() int {
	if h.Timed {
		return timedHeaderLen
	}
	return untimedHeaderLen
}

// EncodeHeader serializes h into its wire representation. The protocol-
// version bits are always encoded as ProtocolVersion; Header has no
// separate version field to keep this milestone's API from carrying a
// setting decode would reject in every other value anyway.
func EncodeHeader(h Header) ([]byte, error) {
	if h.DataLength > dataLengthMask {
		return nil, ErrDataLengthOverflow
	}

	buf := make([]byte, h.wireLen())
	if h.Timed {
		buf[0] = SubtypeTSCF
	} else {
		buf[0] = SubtypeNTSCF
	}

	var flags byte
	if h.StreamIDValid {
		flags |= 0x80
	}
	// bits 6-4: version (always 0 for this milestone, kept for wire
	// forward-compatibility with a future revision).
	if h.Timed {
		flags |= (byte(h.TimestampStatus) & timestampStatusMask) << 2
	}
	buf[1] = flags

	buf[2] = h.SequenceNum
	binary.BigEndian.PutUint16(buf[3:5], h.DataLength&dataLengthMask)
	copy(buf[5:13], h.StreamID[:])

	if h.Timed {
		binary.BigEndian.PutUint32(buf[13:17], h.Timestamp)
	}
	return buf, nil
}

// DecodeHeader parses an AVTPDU header from the front of b and returns the
// decoded Header along with the remaining bytes (the RCP message and any
// trailer). It never panics on malformed input.
func DecodeHeader(b []byte) (Header, []byte, error) {
	if len(b) < 1 {
		return Header{}, nil, ErrShortHeader
	}

	var timed bool
	switch b[0] {
	case SubtypeNTSCF:
		timed = false
	case SubtypeTSCF:
		timed = true
	default:
		return Header{}, nil, ErrUnknownSubtype
	}

	need := untimedHeaderLen
	if timed {
		need = timedHeaderLen
	}
	if len(b) < need {
		return Header{}, nil, ErrShortHeader
	}

	flags := b[1]
	version := (flags >> 4) & 0x07
	if version != ProtocolVersion {
		return Header{}, nil, ErrUnsupportedVersion
	}
	if !timed && (flags&0x0F) != 0 {
		// On an untimed header the low nibble (including the timestamp-
		// status bits, which don't apply) must be zero.
		return Header{}, nil, ErrReservedBitsSet
	}
	if timed && (flags&0x03) != 0 {
		return Header{}, nil, ErrReservedBitsSet
	}

	h := Header{
		Timed:         timed,
		StreamIDValid: flags&0x80 != 0,
		SequenceNum:   b[2],
		DataLength:    binary.BigEndian.Uint16(b[3:5]) & dataLengthMask,
	}
	copy(h.StreamID[:], b[5:13])

	rest := b[untimedHeaderLen:]
	if timed {
		h.Timestamp = binary.BigEndian.Uint32(b[13:17])
		h.TimestampStatus = TimestampStatus((flags >> 2) & timestampStatusMask)
		rest = b[timedHeaderLen:]
	}
	return h, rest, nil
}

// Disposition reports how a server should handle this Header given whether
// it supports time synchronization at all.
//
// Precedence: a timed header received by a server with no time-sync support
// is always dropped, regardless of the timestamp marker — that check comes
// first. Otherwise, an untimed header is always best-effort, and a timed
// header falls back to best-effort execution unless its marker is
// TimestampValid, in which case it is scheduled.
func (h Header) Disposition(timeSyncSupported bool) Disposition {
	if !h.Timed {
		return DispositionBestEffort
	}
	if !timeSyncSupported {
		return DispositionDrop
	}
	if h.TimestampStatus == TimestampValid {
		return DispositionScheduled
	}
	return DispositionBestEffort
}
