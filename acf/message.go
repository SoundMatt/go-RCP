package acf

import (
	"encoding/binary"

	"github.com/SoundMatt/go-RCP/avtp"
)

// MessageKind selects which of the two RCP-over-ACF message encodings a
// Message is. Both share the same two-word request-descriptor header; they
// differ only in whether an 8-byte message_timestamp slot is inserted
// between the two words.
type MessageKind uint8

const (
	// KindShort (ACF_ABB, wire acf_msg_type 0x0E) carries no
	// message_timestamp field at all.
	KindShort MessageKind = 1

	// KindLong (ACF_GBB, wire acf_msg_type 0x0D) carries an additional
	// 8-byte message_timestamp slot between the descriptor's two words.
	// When MTV is false, that slot is not a valid timestamp — the
	// specification reuses it to carry conditional/cancel-request
	// metadata (§11.2.2/§11.2.3) instead; this package still exposes it
	// only as the raw Timestamp field (see its doc comment).
	KindLong MessageKind = 2
)

const (
	wireMsgTypeShort = 0x0E
	wireMsgTypeLong  = 0x0D
)

// ControlFlags are Go-side convenience bits describing a Message's
// request/response semantics. They are not a literal copy of any single
// wire byte: EncodeMessage/DecodeMessage translate between this
// representation and the specification's actual op/rsp/err/ms bits (spread
// across the descriptor's second word alongside evt/hs/cs/transaction_num
// and read_size/segment_num — see EVT, HS, CS and the field explanation in
// this package's doc comment). Keeping the exported flag names stable while
// only correcting their wire translation internally is what let this fix
// land without rewriting every one of this repo's ~50 endpoint/bridge
// packages that construct or inspect a Message's Control.
type ControlFlags uint8

const (
	// FlagRead marks a request as expecting a response with data (wire
	// op=0b), or a response as answering one. Exactly one of FlagRead/
	// FlagWrite is meaningful per message; if neither is set, encoding
	// treats the message as op=0 (Read) by default.
	FlagRead ControlFlags = 1 << 7

	// FlagWrite marks a request as not expecting a response with data
	// (wire op=1b), or a response as answering one.
	FlagWrite ControlFlags = 1 << 6

	// FlagResponse marks this ABB/GBB message as a response rather than a
	// request (wire rsp=1b).
	FlagResponse ControlFlags = 1 << 5

	// FlagError marks a response as reporting a non-successful execution
	// (wire err=1b).
	FlagError ControlFlags = 1 << 4

	// FlagMoreSegments marks that more data follows in one or more
	// subsequent ABB/GBB messages (wire ms=1b), and that
	// ReadSizeOrSegment carries a segment number rather than a requested
	// read size.
	FlagMoreSegments ControlFlags = 1 << 3
)

// Has reports whether all bits of want are set in f.
func (f ControlFlags) Has(want ControlFlags) bool {
	return f&want == want
}

const (
	// row1Len is the wire size of the descriptor's first word: acf_msg_type
	// (7 bits) + acf_msg_length (9 bits, in quadlets) + pad (2 bits) + mtv
	// (1 bit) + rsv (2 bits, must be 0) + byte_bus_id (11 bits).
	row1Len = 4

	// row2Len is the wire size of the descriptor's second word: evt (4
	// bits) + rsv (2 bits, must be 0) + hs (1 bit) + cs (1 bit) +
	// transaction_num (8 bits) + op/rsp/err/ms (1 bit each) +
	// read_size/segment_num (12 bits).
	row2Len = 4

	// descriptorLen is the shared request-descriptor header size for
	// KindShort: row1Len + row2Len, with no message_timestamp slot.
	descriptorLen = row1Len + row2Len

	// timestampFieldLen is the size of KindLong's extra message_timestamp
	// slot, inserted between row1 and row2 on the wire.
	timestampFieldLen = 8

	// padMask bounds the 2-bit pad-byte-count field.
	padMask = 0x03

	// maxQuadlets bounds the 9-bit acf_msg_length field (in quadlets).
	maxQuadlets = 0x1FF

	// maxByteBusID bounds the 11-bit byte_bus_id field (0-2047).
	maxByteBusID = 0x7FF

	// maxTransactionNum bounds the 8-bit transaction_num field.
	// avtp.TransactionNum is 16 bits wide for headroom elsewhere in this
	// repo, so EncodeMessage rejects values that do not fit the wire
	// field rather than silently truncating them.
	maxTransactionNum = 0xFF

	// maxReadSizeOrSegment bounds the 12-bit read_size/segment_num field.
	maxReadSizeOrSegment = 0x0FFF

	// maxEVT bounds the 4-bit evt field.
	maxEVT = 0x0F
)

// Message is a decoded RCP-over-ACF message: the shared two-word
// request-descriptor header, plus (for KindLong) an 8-byte
// message_timestamp slot, plus the request/response body.
type Message struct {
	// Kind selects the short (ACF_ABB) or long (ACF_GBB) encoding.
	Kind MessageKind

	// Pad is the number of zero padding bytes EncodeMessage appended after
	// Body to bring the encoded message's total length to a whole number
	// of quadlets (the unit acf_msg_length is expressed in). EncodeMessage
	// computes and overwrites this field itself — there is exactly one
	// correct pad count for any given Body length (0-3 bytes), so callers
	// do not need to (and cannot usefully) set it; DecodeMessage populates
	// it from the wire so a caller can see how many trailing bytes of a
	// decoded Body-adjacent region were padding.
	Pad uint8

	// ByteBusID addresses the endpoint this message targets, scoped to the
	// stream_id of the enclosing AVTPDU. This is the avtp package's typed
	// addressing unit (see avtp/doc.go) — this package carries it, but does
	// not itself implement (stream_id, byte_bus_id) addressing.
	ByteBusID avtp.ByteBusID

	// TransactionNum correlates this message with its counterpart
	// request/response, scoped to the enclosing stream. The wire field is
	// only 8 bits wide; EncodeMessage rejects a value above 255 rather
	// than truncating it.
	TransactionNum avtp.TransactionNum

	// EVT is the 4-bit evt field: bit 3 is the request-acknowledge flag
	// (see Message.EVTAckRequested), bits 2:0 carry endpoint-specific usage
	// (SPI channel select, GPIO/PWM_OUT write-arithmetic semantics, the
	// config-vs-data discriminator, the compound-wait comparison selector,
	// ...) or, for responses, the multi-response counter / acknowledge
	// coding.
	//
	// Decoding evt[2:0] into per-endpoint behaviour is not left to each
	// endpoint package to reinvent: TC18 §13.5 Table 30's three
	// endpoint-type rows are modelled once in evt.go as EVTClass, and every
	// endpoint-type package routes its requests through
	// Message.EVTDisposition with its own class. See evt.go.
	EVT uint8

	// HS is the wire hs bit. Every currently-defined request/response
	// shape requires it to be zero; this package still round-trips
	// whatever value a caller sets so future message shapes that give it
	// meaning do not require another struct change.
	HS bool

	// CS is the wire cs bit. Its meaning is request-shape-specific (e.g.
	// compound-wait's immediate-vs-after-update check, or a chained
	// request's abort-on-predecessor-error flag); this package only
	// carries it at its correct wire position.
	CS bool

	// MTV is the wire mtv bit: whether the message_timestamp slot (only
	// present for KindLong) holds a genuinely valid timestamp. It is
	// meaningless for KindShort, which never carries that slot at all.
	MTV bool

	// Control carries the Read/Write/Response/Error/MoreSegments bits; see
	// ControlFlags's doc comment for how these map onto the actual wire
	// bits.
	Control ControlFlags

	// ReadSizeOrSegment is dual-purpose: when Control has FlagMoreSegments
	// set, it is the segment number of this fragment; otherwise, for a
	// read request, it is the requested read size. See SegmentNumber and
	// ReadSize. The wire field is only 12 bits wide; EncodeMessage rejects
	// a value above 4095 rather than truncating it.
	ReadSizeOrSegment uint16

	// Timestamp is the raw 8-byte message_timestamp slot, meaningful only
	// when Kind is KindLong. When MTV is true it is a genuine timestamp;
	// when MTV is false the specification reuses these bytes for
	// conditional/cancel-request metadata (§11.2.2/§11.2.3) instead, which
	// this package does not itself decode structurally — see the request
	// package for that layer.
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

// EncodeMessage serializes m into its wire representation. It computes and
// overwrites m.Pad itself — see Pad's doc comment — rather than trusting a
// caller-supplied value, so the encoded length is always a whole number of
// quadlets regardless of Body's length.
func EncodeMessage(m Message) ([]byte, error) {
	if m.Kind != KindShort && m.Kind != KindLong {
		return nil, ErrUnknownMessageKind
	}
	const knownFlags = FlagRead | FlagWrite | FlagResponse | FlagError | FlagMoreSegments
	if m.Control&^knownFlags != 0 {
		return nil, avtp.ErrReservedBitsSet
	}
	if m.EVT > maxEVT {
		return nil, ErrEVTOverflow
	}
	if uint32(m.TransactionNum) > maxTransactionNum {
		return nil, ErrTransactionNumOverflow
	}
	if uint32(m.ByteBusID) > maxByteBusID {
		return nil, ErrByteBusIDOverflow
	}
	if m.ReadSizeOrSegment > maxReadSizeOrSegment {
		return nil, ErrReadSizeOverflow
	}

	unpadded := m.unpaddedLen()
	pad := uint8((4 - unpadded%4) % 4)
	total := unpadded + int(pad)
	quadlets := total / 4
	if quadlets > maxQuadlets {
		return nil, ErrLengthOverflow
	}

	buf := make([]byte, total)

	msgType := byte(wireMsgTypeShort)
	if m.Kind == KindLong {
		msgType = wireMsgTypeLong
	}
	buf[0] = (msgType << 1) | byte((quadlets>>8)&0x01)
	buf[1] = byte(quadlets & 0xFF)

	var b2 byte
	b2 |= (pad & padMask) << 6
	if m.MTV {
		b2 |= 1 << 5
	}
	// bits 4:3 are rsv, left at 0.
	b2 |= byte((uint16(m.ByteBusID) >> 8) & 0x07)
	buf[2] = b2
	buf[3] = byte(m.ByteBusID)

	off := row1Len
	if m.Kind == KindLong {
		binary.BigEndian.PutUint64(buf[off:off+timestampFieldLen], m.Timestamp)
		off += timestampFieldLen
	}

	row2 := buf[off : off+row2Len]
	row2[0] = m.EVT << 4
	if m.HS {
		row2[0] |= 1 << 1
	}
	if m.CS {
		row2[0] |= 1
	}
	row2[1] = byte(m.TransactionNum)
	var b6 byte
	if m.Control.Has(FlagWrite) {
		b6 |= 1 << 7 // op=1: no response with data expected
	}
	if m.Control.Has(FlagResponse) {
		b6 |= 1 << 6
	}
	if m.Control.Has(FlagError) {
		b6 |= 1 << 5
	}
	if m.Control.Has(FlagMoreSegments) {
		b6 |= 1 << 4
	}
	b6 |= byte((m.ReadSizeOrSegment >> 8) & 0x0F)
	row2[2] = b6
	row2[3] = byte(m.ReadSizeOrSegment & 0xFF)
	off += row2Len

	copy(buf[off:], m.Body)
	// Trailing Pad bytes are already zero from make([]byte, ...).
	return buf, nil
}

// DecodeMessage parses a Message from b. b must contain exactly the encoded
// message (including its trailing pad bytes) — callers that receive a
// larger buffer (e.g. the AVTPDU frame layer) must slice it down to the
// message's declared length first, or use DecodeMessagePrefix to decode one
// message off the front of a buffer that may hold more (see TC18 §12.9.1.1
// and acf.DecodeFrame, which walks a frame's payload as a sequence of
// zero-or-more independently-addressed ACF messages). It never panics on
// malformed input.
func DecodeMessage(b []byte) (Message, error) {
	m, n, err := DecodeMessagePrefix(b)
	if err != nil {
		return Message{}, err
	}
	if n != len(b) {
		return Message{}, ErrTrailingBytes
	}
	return m, nil
}

// DecodeMessagePrefix parses the single leading Message off the front of b,
// which may contain further bytes belonging to a subsequent message (per
// TC18 §12.9.1.1, "An RCP frame may include multiple ACF-types
// (requests)"). It returns the decoded Message and n, the number of bytes
// it consumed from b (always a whole number of quadlets, per
// acf_msg_length) — a caller decoding a multi-message frame slices those n
// bytes off and repeats on the remainder. DecodeMessage is DecodeMessagePrefix
// plus a check that b held exactly one message and nothing more. It never
// panics on malformed input.
func DecodeMessagePrefix(b []byte) (m Message, n int, err error) {
	if len(b) < row1Len {
		return Message{}, 0, ErrShortMessage
	}

	msgType := b[0] >> 1
	var kind MessageKind
	switch msgType {
	case wireMsgTypeShort:
		kind = KindShort
	case wireMsgTypeLong:
		kind = KindLong
	default:
		return Message{}, 0, ErrUnknownMessageKind
	}

	quadlets := (uint16(b[0]&0x01) << 8) | uint16(b[1])

	b2 := b[2]
	pad := (b2 >> 6) & padMask
	mtv := b2&(1<<5) != 0
	if b2&(0x03<<3) != 0 { // rsv bits 4:3
		return Message{}, 0, avtp.ErrReservedBitsSet
	}
	busIDTop := uint16(b2 & 0x07)

	headerLen := descriptorLen
	if kind == KindLong {
		headerLen += timestampFieldLen
	}
	if len(b) < headerLen {
		return Message{}, 0, ErrShortMessage
	}

	// busIDTop is at most 3 bits (masked by 0x07 above) and b[3] is a full
	// byte, so busID is always in [0, 0x7FF] — the entire legal 11-bit
	// range is representable by avtp.ByteBusID (uint16); no overflow check
	// is needed or possible here.
	busID := (busIDTop << 8) | uint16(b[3])

	off := row1Len
	var timestamp uint64
	if kind == KindLong {
		timestamp = binary.BigEndian.Uint64(b[off : off+timestampFieldLen])
		off += timestampFieldLen
	}

	r0 := b[off]
	evt := r0 >> 4
	if r0&(0x03<<2) != 0 { // rsv bits 3:2
		return Message{}, 0, avtp.ErrReservedBitsSet
	}
	hs := r0&(1<<1) != 0
	cs := r0&1 != 0
	txn := b[off+1]
	r2 := b[off+2]
	op := r2&(1<<7) != 0
	rsp := r2&(1<<6) != 0
	errBit := r2&(1<<5) != 0
	ms := r2&(1<<4) != 0
	readSize := (uint16(r2&0x0F) << 8) | uint16(b[off+3])
	off += row2Len

	var control ControlFlags
	if op {
		control |= FlagWrite
	} else {
		control |= FlagRead
	}
	if rsp {
		control |= FlagResponse
	}
	if errBit {
		control |= FlagError
	}
	if ms {
		control |= FlagMoreSegments
	}

	m = Message{
		Kind:              kind,
		Pad:               pad,
		ByteBusID:         avtp.ByteBusID(busID),
		TransactionNum:    avtp.TransactionNum(txn),
		EVT:               evt,
		HS:                hs,
		CS:                cs,
		MTV:               mtv,
		Control:           control,
		ReadSizeOrSegment: readSize,
		Timestamp:         timestamp,
	}

	total := int(quadlets) * 4
	// uint64 arithmetic guards against any future widening of this
	// comparison the way wire.DecodeCommand does for its own body-length
	// check — quadlets is only 9 bits today so overflow isn't reachable
	// yet, but the pattern is kept consistent across the repo's decoders.
	if uint64(len(b)) < uint64(total) {
		return Message{}, 0, ErrShortMessage
	}

	bodyEnd := total - int(pad)
	if bodyEnd < off || bodyEnd > total {
		return Message{}, 0, ErrShortMessage
	}
	if bodyEnd > off {
		m.Body = make([]byte, bodyEnd-off)
		copy(m.Body, b[off:bodyEnd])
	}
	return m, total, nil
}
