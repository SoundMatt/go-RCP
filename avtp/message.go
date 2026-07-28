package avtp

import "encoding/binary"

// MessageKind selects which of the two RCP-over-ACF message encodings a
// Message is. Both share one request-descriptor header; they differ only in
// whether a 64-bit timestamp slot follows it.
type MessageKind uint8

const (
	// KindShort (ACF_ABB) carries no timestamp field at all.
	KindShort MessageKind = 1

	// KindLong (ACF_GBB) carries an additional 64-bit timestamp slot
	// immediately after the shared request-descriptor header.
	KindLong MessageKind = 2
)

// ControlFlags are the request-descriptor header's control bits.
type ControlFlags uint8

const (
	FlagAck          ControlFlags = 1 << 7
	FlagRead         ControlFlags = 1 << 6
	FlagWrite        ControlFlags = 1 << 5
	FlagResponse     ControlFlags = 1 << 4
	FlagError        ControlFlags = 1 << 3
	FlagMoreSegments ControlFlags = 1 << 2

	controlReservedMask ControlFlags = 0x03
)

// Has reports whether all bits of want are set in f.
func (f ControlFlags) Has(want ControlFlags) bool {
	return f&want == want
}

const (
	// descriptorLen is the shared request-descriptor header size: kind(1)
	// + pad/reserved(1) + length(2) + byte_bus_id(1) + transaction_num(2)
	// + control(1) + read-size-or-segment(2).
	descriptorLen = 10

	// timestampFieldLen is the size of the long encoding's extra 64-bit
	// timestamp slot.
	timestampFieldLen = 8

	// padMask bounds the 2-bit pad-byte-count field.
	padMask = 0x03
)

// Message is a decoded RCP-over-ACF message: the shared request-descriptor
// header, plus (for KindLong) a 64-bit timestamp, plus the request/response
// body.
type Message struct {
	// Kind selects the short (ACF_ABB) or long (ACF_GBB) encoding.
	Kind MessageKind

	// Pad is the number of zero padding bytes appended after Body to keep
	// the encoded message's total length aligned. It fits a 2-bit wire
	// field (0-3).
	Pad uint8

	// ByteBusID addresses the endpoint this message targets, scoped to the
	// stream_id of the enclosing AVTPDU.
	ByteBusID ByteBusID

	// TransactionNum correlates this message with its counterpart
	// request/response, scoped to the enclosing stream.
	TransactionNum TransactionNum

	// Control carries the Ack/Read/Write/Response/Error/MoreSegments bits.
	Control ControlFlags

	// ReadSizeOrSegment is dual-purpose: when Control has FlagMoreSegments
	// set, it is the segment number of this fragment; otherwise, for a
	// read request, it is the requested read size. See SegmentNumber and
	// ReadSize.
	ReadSizeOrSegment uint16

	// Timestamp is meaningful only when Kind is KindLong.
	Timestamp uint64

	// Body is the request/response payload proper (excluding Pad).
	Body []byte
}

// SegmentNumber returns ReadSizeOrSegment and true when Control has
// FlagMoreSegments set (i.e. the field holds a segment number).
func (m Message) SegmentNumber() (uint16, bool) {
	if m.Control.Has(FlagMoreSegments) {
		return m.ReadSizeOrSegment, true
	}
	return 0, false
}

// ReadSize returns ReadSizeOrSegment and true when Control has FlagRead set
// and FlagMoreSegments is not set (i.e. the field holds a requested read
// size rather than a segment number).
func (m Message) ReadSize() (uint16, bool) {
	if m.Control.Has(FlagRead) && !m.Control.Has(FlagMoreSegments) {
		return m.ReadSizeOrSegment, true
	}
	return 0, false
}

// unpaddedLen returns the total encoded length of m, excluding Pad.
func (m Message) unpaddedLen() int {
	n := descriptorLen + len(m.Body)
	if m.Kind == KindLong {
		n += timestampFieldLen
	}
	return n
}

// EncodeMessage serializes m into its wire representation.
func EncodeMessage(m Message) ([]byte, error) {
	if m.Kind != KindShort && m.Kind != KindLong {
		return nil, ErrUnknownMessageKind
	}
	if m.Pad > padMask {
		return nil, ErrPadOverflow
	}
	const knownFlags = FlagAck | FlagRead | FlagWrite | FlagResponse | FlagError | FlagMoreSegments
	if m.Control&^knownFlags != 0 {
		return nil, ErrReservedBitsSet
	}

	total := m.unpaddedLen() + int(m.Pad)
	if total > 0xFFFF {
		return nil, ErrLengthOverflow
	}

	buf := make([]byte, total)
	buf[0] = byte(m.Kind)
	buf[1] = (m.Pad & padMask) << 6
	binary.BigEndian.PutUint16(buf[2:4], uint16(total))
	buf[4] = byte(m.ByteBusID)
	binary.BigEndian.PutUint16(buf[5:7], uint16(m.TransactionNum))
	buf[7] = byte(m.Control)
	binary.BigEndian.PutUint16(buf[8:10], m.ReadSizeOrSegment)

	off := descriptorLen
	if m.Kind == KindLong {
		binary.BigEndian.PutUint64(buf[off:off+timestampFieldLen], m.Timestamp)
		off += timestampFieldLen
	}
	copy(buf[off:], m.Body)
	// Trailing Pad bytes are already zero from make([]byte, ...).
	return buf, nil
}

// DecodeMessage parses a Message from b. b must contain exactly the encoded
// message (including its trailing pad bytes) — callers that receive a
// larger buffer (e.g. the AVTPDU frame layer) must slice it down to the
// message's declared length first. It never panics on malformed input.
func DecodeMessage(b []byte) (Message, error) {
	if len(b) < descriptorLen {
		return Message{}, ErrShortMessage
	}

	kind := MessageKind(b[0])
	if kind != KindShort && kind != KindLong {
		return Message{}, ErrUnknownMessageKind
	}

	if b[1]&0x3F != 0 {
		return Message{}, ErrReservedBitsSet
	}
	pad := (b[1] >> 6) & padMask

	length := binary.BigEndian.Uint16(b[2:4])
	// uint64 arithmetic guards against any future widening of this
	// comparison the way wire.DecodeCommand does for its own body-length
	// check — length is only 16 bits today so overflow isn't reachable yet,
	// but the pattern is kept consistent across the repo's decoders.
	if uint64(len(b)) < uint64(length) {
		return Message{}, ErrShortMessage
	}

	control := ControlFlags(b[7])
	if control&controlReservedMask != 0 {
		return Message{}, ErrReservedBitsSet
	}

	m := Message{
		Kind:              kind,
		Pad:               pad,
		ByteBusID:         ByteBusID(b[4]),
		TransactionNum:    TransactionNum(binary.BigEndian.Uint16(b[5:7])),
		Control:           control,
		ReadSizeOrSegment: binary.BigEndian.Uint16(b[8:10]),
	}

	off := descriptorLen
	if kind == KindLong {
		if len(b) < off+timestampFieldLen {
			return Message{}, ErrShortMessage
		}
		m.Timestamp = binary.BigEndian.Uint64(b[off : off+timestampFieldLen])
		off += timestampFieldLen
	}

	bodyEnd := int(length) - int(pad)
	if bodyEnd < off || uint64(bodyEnd) > uint64(len(b)) {
		return Message{}, ErrShortMessage
	}
	if bodyEnd > off {
		m.Body = make([]byte, bodyEnd-off)
		copy(m.Body, b[off:bodyEnd])
	}
	return m, nil
}
