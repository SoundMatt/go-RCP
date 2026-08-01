package avtp

import "encoding/binary"

// AVTPDU subtype tags for the two RCP-carrying control formats this package
// implements: the untimed form (NTSCF) and the presentation-timestamped form
// (TSCF).
//
// Both values are read directly off the governing specification's own
// figures rather than assumed: "OPEN Alliance TC18 Remote Control Protocol
// Specification v0.5.1_RC" §11.1 p.22 Figure 6 labels the NTSCF header's
// first octet "subtype(0x82)", and §11.1 p.22 Figure 5 labels the TSCF
// header's first octet "subtype(0x05)". The worked CRC32 examples on p.79
// (Figure 19 for TSCF, Figure 20 for NTSCF) repeat both values
// independently. SubtypeTSCF was 0x83 here through v8.0.0 — a fabricated
// "one past NTSCF" value that no conformant peer would ever have
// recognised; see the package doc.
const (
	SubtypeNTSCF byte = 0x82 // untimed, "execute as soon as possible"
	SubtypeTSCF  byte = 0x05 // presentation-timestamped
)

// ProtocolVersion is the only AVTPDU header version this package accepts.
// Both TC18 Figures 5 and 6 fix the field's value as "version(0x0)".
const ProtocolVersion uint8 = 0

// MaxDataLength is the largest payload length this package will encode into
// an AVTPDU header. It is the width of the narrower of the two header
// variants' length fields: NTSCF's ntscf_data_length is 11 bits wide (TC18
// Figure 6, bits 13-23), where TSCF's stream_data_length is a full 16
// (Figure 5, "Packet Info" row, bits 0-15). EncodeHeader applies this single
// conservative cap to both variants deliberately, so that one encoded
// payload can be re-framed under either header without silently truncating;
// DecodeHeader, by contrast, reads TSCF's stream_data_length at its true
// full 16-bit width, so a conformant peer's larger frame is reported
// accurately rather than misparsed.
//
// Exported so a caller building a Frame from a separately-encoded acf
// message (see the acf package) can guard against an oversized message the
// same way EncodeHeader itself would reject — checked against the message's
// true pre-truncation byte count, not against a uint16 already wrapped
// modulo 65536.
const MaxDataLength = dataLengthMask

const (
	// untimedHeaderLen is the wire size of an NTSCF header, per TC18 §11.1
	// p.22 Figure 6: one 32-bit "subtype data" quadlet — subtype(8) |
	// sv(1) | version(3) | r(1) | ntscf_data_length(11) | sequence_num(8)
	// — immediately followed by the 64-bit stream_id. There is no reserved
	// gap and no separate 16-bit length field.
	untimedHeaderLen = 4 + 8 // 12

	// timedHeaderLen is the wire size of a TSCF header, per TC18 §11.1 p.22
	// Figure 5: the "subtype data" quadlet — subtype(8) | sv(1) |
	// version(3) | mr(1) | rsv(2) | tv(1) | sequence_num(8) |
	// reserved(7) | tu(1) — then stream_id(64), avtp_timestamp(32), the
	// "Format specific" reserved quadlet(32), and the "Packet Info" quadlet
	// carrying stream_data_length(16) | reserved(16).
	timedHeaderLen = 4 + 8 + 4 + 4 + 4 // 24

	// dataLengthMask bounds NTSCF's 11-bit ntscf_data_length field.
	dataLengthMask = 0x07FF

	// Bit masks within the first ("subtype data") quadlet, shared by both
	// variants unless noted. Bit numbering follows the figures: bit 0 is
	// the most significant bit of octet 0.
	svMask       = 0x80 // octet 1, bit 8 of the quadlet
	versionShift = 4    // octet 1, bits 9-11 hold version
	versionWidth = 0x07

	// untimedReservedMask is NTSCF's single "r" bit (Figure 6, bit 12).
	untimedReservedMask = 0x08
	// untimedLenHighMask is ntscf_data_length's top 3 bits (bits 13-15),
	// which share octet 1 with the fields above; bits 16-23 are octet 2.
	untimedLenHighMask = 0x07

	// TSCF's "mr" media-clock-restart bit (Figure 5, bit 12, octet 1 mask
	// 0x08) has no constant here on purpose: RCP has no media clock, so
	// this package encodes it as zero and neither reads nor rejects it on
	// decode — it is a defined IEEE 1722 field, not a reserved one.
	//
	// timedRsvMask is TSCF's 2-bit "rsv" field (Figure 5, bits 13-14).
	timedRsvMask = 0x06
	// timedTVMask is TSCF's "tv" avtp_timestamp-valid bit (bit 15).
	timedTVMask = 0x01
	// timedTUMask is TSCF's "tu" timestamp-uncertain bit (bit 31, the low
	// bit of octet 3); bits 24-30 of that octet are reserved.
	timedTUMask = 0x01
	// timedOctet3ReservedMask covers Figure 5's reserved bits 24-30.
	timedOctet3ReservedMask = 0xFE
)

// TimestampStatus is the validity marker carried by a timestamped (TSCF)
// AVTPDU header. It is meaningless on an untimed (NTSCF) header.
//
// # Wire mapping
//
// TC18 Figure 5 gives the TSCF header two separate single-bit markers for
// this, not one two-bit field: "tv" (bit 15, avtp_timestamp valid) and "tu"
// (bit 31, timestamp uncertain). This type's four values map onto that pair
// as follows:
//
//	TimestampValid     tv=1 tu=0
//	TimestampUncertain tv=1 tu=1
//	TimestampMissing   tv=0 tu=0
//	TimestampInvalid   tv=0 tu=0
//
// The wire has no way to distinguish "no marker was set" from "the sender
// explicitly could not vouch for the timestamp" — both are simply tv=0 — so
// EncodeHeader encodes TimestampInvalid and TimestampMissing identically and
// DecodeHeader reports tv=0 as TimestampMissing (the zero value, so a
// default-constructed timed Header round-trips exactly). Header.Disposition
// already treats the two identically, so nothing downstream observes the
// collapse. tv=0 with tu=1 is likewise reported as TimestampMissing: a
// timestamp that is not valid at all cannot be meaningfully "uncertain".
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
	// this header: NTSCF's ntscf_data_length or TSCF's stream_data_length.
	// EncodeHeader caps it at MaxDataLength (NTSCF's narrower 11-bit field,
	// 0-2047) for both variants; DecodeHeader reads TSCF's full 16-bit
	// field, so a decoded timed Header may legitimately exceed
	// MaxDataLength.
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

// EncodeHeader serializes h into its wire representation, exactly as laid
// out by TC18 §11.1 p.22 Figure 6 (NTSCF, 12 octets) or Figure 5 (TSCF, 24
// octets). The protocol-version bits are always encoded as ProtocolVersion;
// Header has no separate version field, to keep this API from carrying a
// setting decode would reject in every other value anyway.
//
// Every field the figures mark reserved — NTSCF's "r" bit, TSCF's "rsv"
// bits, TSCF's reserved bits 24-30, its "Format specific" reserved quadlet,
// and the 16 reserved bits trailing stream_data_length — is written as zero.
// TSCF's "mr" (media clock restart) bit is likewise always zero: RCP carries
// no media clock.
func EncodeHeader(h Header) ([]byte, error) {
	if h.DataLength > MaxDataLength {
		return nil, ErrDataLengthOverflow
	}

	buf := make([]byte, h.wireLen())

	// Octet 1 is shared in shape by both variants up to the version field:
	// sv(1) then version(3).
	var b1 byte
	if h.StreamIDValid {
		b1 |= svMask
	}
	b1 |= (ProtocolVersion & versionWidth) << versionShift

	if h.Timed {
		// Figure 5: subtype | sv | version | mr | rsv | tv |
		// sequence_num | reserved | tu.
		buf[0] = SubtypeTSCF
		tv, tu := encodeTimestampMarkers(h.TimestampStatus)
		if tv {
			b1 |= timedTVMask
		}
		buf[1] = b1
		buf[2] = h.SequenceNum
		if tu {
			buf[3] = timedTUMask
		}
		copy(buf[4:12], h.StreamID[:])
		binary.BigEndian.PutUint32(buf[12:16], h.Timestamp)
		// buf[16:20] is the "Format specific" reserved quadlet: zero.
		binary.BigEndian.PutUint16(buf[20:22], h.DataLength)
		// buf[22:24] is stream_data_length's trailing reserved half: zero.
		return buf, nil
	}

	// Figure 6: subtype | sv | version | r | ntscf_data_length(11) |
	// sequence_num, then stream_id. The length field straddles octets 1
	// and 2 — its top 3 bits share octet 1 with the flags above.
	buf[0] = SubtypeNTSCF
	b1 |= byte(h.DataLength>>8) & untimedLenHighMask
	buf[1] = b1
	buf[2] = byte(h.DataLength)
	buf[3] = h.SequenceNum
	copy(buf[4:12], h.StreamID[:])
	return buf, nil
}

// encodeTimestampMarkers maps a TimestampStatus onto TSCF's separate tv and
// tu bits; see the TimestampStatus doc comment for the full table.
func encodeTimestampMarkers(s TimestampStatus) (tv, tu bool) {
	switch s {
	case TimestampValid:
		return true, false
	case TimestampUncertain:
		return true, true
	default: // TimestampMissing, TimestampInvalid
		return false, false
	}
}

// decodeTimestampMarkers is encodeTimestampMarkers' inverse. tv=0 collapses
// to TimestampMissing regardless of tu, since a timestamp that is not valid
// at all cannot be meaningfully "uncertain".
func decodeTimestampMarkers(tv, tu bool) TimestampStatus {
	switch {
	case !tv:
		return TimestampMissing
	case tu:
		return TimestampUncertain
	default:
		return TimestampValid
	}
}

// DecodeHeader parses an AVTPDU header from the front of b and returns the
// decoded Header along with the remaining bytes (the RCP message and any
// trailer). It never panics on malformed input.
//
// It rejects a nonzero value in any bit the governing figure marks reserved
// within the first ("subtype data") quadlet — NTSCF's "r" bit, TSCF's "rsv"
// field and its reserved bits 24-30 — since those are the bits a peer
// disagreeing with us about the header layout would most visibly disturb.
// The two whole reserved words further down the TSCF header (the "Format
// specific" quadlet and stream_data_length's trailing 16 bits) are read and
// ignored rather than rejected: IEEE 1722 leaves the former available to
// format-specific use, and refusing an otherwise well-formed frame over it
// would be gratuitously unfriendly. TSCF's "mr" bit is likewise ignored —
// it is a defined field RCP has no use for, not a reserved one.
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

	b1 := b[1]
	if version := (b1 >> versionShift) & versionWidth; version != ProtocolVersion {
		return Header{}, nil, ErrUnsupportedVersion
	}

	h := Header{
		Timed:         timed,
		StreamIDValid: b1&svMask != 0,
	}

	if timed {
		if b1&timedRsvMask != 0 || b[3]&timedOctet3ReservedMask != 0 {
			return Header{}, nil, ErrReservedBitsSet
		}
		h.SequenceNum = b[2]
		h.TimestampStatus = decodeTimestampMarkers(
			b1&timedTVMask != 0, b[3]&timedTUMask != 0)
		copy(h.StreamID[:], b[4:12])
		h.Timestamp = binary.BigEndian.Uint32(b[12:16])
		// b[16:20] is the "Format specific" reserved quadlet: ignored.
		// stream_data_length is a full 16 bits wide, unlike NTSCF's 11.
		h.DataLength = binary.BigEndian.Uint16(b[20:22])
		return h, b[timedHeaderLen:], nil
	}

	if b1&untimedReservedMask != 0 {
		return Header{}, nil, ErrReservedBitsSet
	}
	h.DataLength = uint16(b1&untimedLenHighMask)<<8 | uint16(b[2])
	h.SequenceNum = b[3]
	copy(h.StreamID[:], b[4:12])
	return h, b[untimedHeaderLen:], nil
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
