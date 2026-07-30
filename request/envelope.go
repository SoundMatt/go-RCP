package request

import (
	"encoding/binary"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
)

// This file implements this package's own wire encoding for the
// conditional-request envelope carried in a Message's Body whenever
// acf.FlagExtended is set. Every encoder writes the envelope's Kind byte
// first; every decoder validates it against the Kind the caller asked for.
// As with every other package's envelope/register byte layout in this repo,
// these exact field widths and orderings have not yet been independently
// re-verified against the governing specification's own wire format — see
// doc.go.

// PeekKind reads and validates only body's leading Kind byte, without
// attempting to decode the rest — the routing step Dispatcher.Submit uses to
// pick which per-kind decoder to call next.
func PeekKind(body []byte) (Kind, error) {
	if len(body) < 1 {
		return 0, ErrShortBuffer
	}
	k := Kind(body[0])
	if !k.Valid() {
		return 0, ErrInvalidKind
	}
	return k, nil
}

// conditionalLen is SequencerID(2) + Op(1) + Operand(4) + AdvanceOnMatch(4).
const conditionalLen = 2 + 1 + 4 + 4

func encodeConditional(buf []byte, c Conditional) []byte {
	binary.BigEndian.PutUint16(buf[0:2], uint16(c.Sequencer))
	buf[2] = byte(c.Op)
	binary.BigEndian.PutUint32(buf[3:7], c.Operand)
	binary.BigEndian.PutUint32(buf[7:11], uint32(c.AdvanceOnMatch))
	return buf
}

func decodeConditional(b []byte) (Conditional, error) {
	if len(b) < conditionalLen {
		return Conditional{}, ErrShortBuffer
	}
	op := CompareOp(b[2])
	if !op.Valid() {
		return Conditional{}, ErrInvalidCompareOp
	}
	return Conditional{
		Sequencer:      SequencerID(binary.BigEndian.Uint16(b[0:2])),
		Op:             op,
		Operand:        binary.BigEndian.Uint32(b[3:7]),
		AdvanceOnMatch: int32(binary.BigEndian.Uint32(b[7:11])),
	}, nil
}

// EncodeCompound serializes a KindCompound request envelope: the sequencer
// gate condition c, followed by the inner request's own Control flags
// (Read/Write, exactly as a Plain request to the same endpoint would set)
// and Body, carried verbatim so the Dispatcher can hand them to Handler.HandleRequest
// unchanged once the condition matches.
func EncodeCompound(c Conditional, innerControl acf.ControlFlags, innerBody []byte) []byte {
	buf := make([]byte, 1+conditionalLen+1+len(innerBody))
	buf[0] = byte(KindCompound)
	encodeConditional(buf[1:1+conditionalLen], c)
	buf[1+conditionalLen] = byte(innerControl)
	copy(buf[1+conditionalLen+1:], innerBody)
	return buf
}

// EncodeCompoundSafety is EncodeCompound's safety-request ("MSB-set")
// counterpart (ROADMAP.md Milestone 50): same body layout, tagged
// KindCompoundSafety instead of KindCompound so Dispatcher only executes it
// once the addressed endpoint's configured safe state is active, and so it
// survives Dispatcher.PurgeNonSafety.
func EncodeCompoundSafety(c Conditional, innerControl acf.ControlFlags, innerBody []byte) []byte {
	body := EncodeCompound(c, innerControl, innerBody)
	body[0] = byte(KindCompoundSafety)
	return body
}

// DecodeCompound parses a KindCompound or KindCompoundSafety envelope — the
// body layout is identical between the two; only the leading Kind byte
// differs. It never panics on malformed input.
func DecodeCompound(body []byte) (Conditional, acf.ControlFlags, []byte, error) {
	if len(body) < 1+conditionalLen+1 {
		return Conditional{}, 0, nil, ErrShortBuffer
	}
	if Kind(body[0]).Base() != KindCompound {
		return Conditional{}, 0, nil, ErrWrongKind
	}
	c, err := decodeConditional(body[1 : 1+conditionalLen])
	if err != nil {
		return Conditional{}, 0, nil, err
	}
	innerControl := acf.ControlFlags(body[1+conditionalLen])
	var innerBody []byte
	if rest := body[1+conditionalLen+1:]; len(rest) > 0 {
		innerBody = make([]byte, len(rest))
		copy(innerBody, rest)
	}
	return c, innerControl, innerBody, nil
}

// EncodeCompoundWait serializes a KindCompoundWait request envelope: just
// the sequencer gate condition c. Unlike EncodeCompound, there is no inner
// request — CompoundWait never touches endpoint output (ROADMAP.md
// Milestone 49).
func EncodeCompoundWait(c Conditional) []byte {
	buf := make([]byte, 1+conditionalLen)
	buf[0] = byte(KindCompoundWait)
	encodeConditional(buf[1:], c)
	return buf
}

// EncodeCompoundWaitSafety is EncodeCompoundWait's safety-request
// counterpart; see EncodeCompoundSafety's doc comment.
func EncodeCompoundWaitSafety(c Conditional) []byte {
	body := EncodeCompoundWait(c)
	body[0] = byte(KindCompoundWaitSafety)
	return body
}

// DecodeCompoundWait parses a KindCompoundWait or KindCompoundWaitSafety
// envelope. It never panics on malformed input.
func DecodeCompoundWait(body []byte) (Conditional, error) {
	if len(body) < 1+conditionalLen {
		return Conditional{}, ErrShortBuffer
	}
	if Kind(body[0]).Base() != KindCompoundWait {
		return Conditional{}, ErrWrongKind
	}
	if len(body) > 1+conditionalLen {
		return Conditional{}, ErrTrailingBytes
	}
	return decodeConditional(body[1:])
}

// EncodeTriggered serializes a KindTriggered request envelope: which other
// endpoint's trigger signal (source) gates execution, followed by the inner
// request's own Control flags and Body, carried verbatim the same way
// EncodeCompound does.
func EncodeTriggered(source avtp.ByteBusID, innerControl acf.ControlFlags, innerBody []byte) []byte {
	buf := make([]byte, 1+1+1+len(innerBody))
	buf[0] = byte(KindTriggered)
	buf[1] = byte(source)
	buf[2] = byte(innerControl)
	copy(buf[3:], innerBody)
	return buf
}

// EncodeTriggeredSafety is EncodeTriggered's safety-request counterpart; see
// EncodeCompoundSafety's doc comment.
func EncodeTriggeredSafety(source avtp.ByteBusID, innerControl acf.ControlFlags, innerBody []byte) []byte {
	body := EncodeTriggered(source, innerControl, innerBody)
	body[0] = byte(KindTriggeredSafety)
	return body
}

// DecodeTriggered parses a KindTriggered or KindTriggeredSafety envelope. It
// never panics on malformed input.
func DecodeTriggered(body []byte) (avtp.ByteBusID, acf.ControlFlags, []byte, error) {
	if len(body) < 3 {
		return 0, 0, nil, ErrShortBuffer
	}
	if Kind(body[0]).Base() != KindTriggered {
		return 0, 0, nil, ErrWrongKind
	}
	source := avtp.ByteBusID(body[1])
	innerControl := acf.ControlFlags(body[2])
	var innerBody []byte
	if rest := body[3:]; len(rest) > 0 {
		innerBody = make([]byte, len(rest))
		copy(innerBody, rest)
	}
	return source, innerControl, innerBody, nil
}

// EncodeTimed serializes a KindTimed request envelope: the target execution
// time executeAtMicros (in whatever monotonic microsecond clock domain the
// caller and Dispatcher.Pump agree on — see doc.go's spec-fidelity note),
// followed by the inner request's own Control flags and Body.
func EncodeTimed(executeAtMicros uint64, innerControl acf.ControlFlags, innerBody []byte) []byte {
	buf := make([]byte, 1+8+1+len(innerBody))
	buf[0] = byte(KindTimed)
	binary.BigEndian.PutUint64(buf[1:9], executeAtMicros)
	buf[9] = byte(innerControl)
	copy(buf[10:], innerBody)
	return buf
}

// DecodeTimed parses a KindTimed envelope. It never panics on malformed
// input.
func DecodeTimed(body []byte) (executeAtMicros uint64, innerControl acf.ControlFlags, innerBody []byte, err error) {
	if len(body) < 10 {
		return 0, 0, nil, ErrShortBuffer
	}
	if Kind(body[0]) != KindTimed {
		return 0, 0, nil, ErrWrongKind
	}
	executeAtMicros = binary.BigEndian.Uint64(body[1:9])
	innerControl = acf.ControlFlags(body[9])
	if rest := body[10:]; len(rest) > 0 {
		innerBody = make([]byte, len(rest))
		copy(innerBody, rest)
	}
	return executeAtMicros, innerControl, innerBody, nil
}

// EncodeCancelAll serializes the mandatory clear-all cancellation request:
// just the Kind byte, since it carries no further scope — it targets every
// pending ticket a Dispatcher holds.
func EncodeCancelAll() []byte {
	return []byte{byte(KindCancelAll)}
}

// DecodeCancelAll validates body is exactly a KindCancelAll envelope. It
// never panics on malformed input.
func DecodeCancelAll(body []byte) error {
	if len(body) < 1 {
		return ErrShortBuffer
	}
	if Kind(body[0]) != KindCancelAll {
		return ErrWrongKind
	}
	if len(body) > 1 {
		return ErrTrailingBytes
	}
	return nil
}

// EncodeCancelTransaction serializes the first optional narrower
// cancellation variant: clear only the one pending ticket whose original
// request carried txn as its acf.Message.TransactionNum.
func EncodeCancelTransaction(txn avtp.TransactionNum) []byte {
	buf := make([]byte, 3)
	buf[0] = byte(KindCancelTransaction)
	binary.BigEndian.PutUint16(buf[1:3], uint16(txn))
	return buf
}

// DecodeCancelTransaction parses a KindCancelTransaction envelope. It never
// panics on malformed input.
func DecodeCancelTransaction(body []byte) (avtp.TransactionNum, error) {
	if len(body) < 3 {
		return 0, ErrShortBuffer
	}
	if Kind(body[0]) != KindCancelTransaction {
		return 0, ErrWrongKind
	}
	if len(body) > 3 {
		return 0, ErrTrailingBytes
	}
	return avtp.TransactionNum(binary.BigEndian.Uint16(body[1:3])), nil
}

// EncodeCancelSequencer serializes the second optional narrower
// cancellation variant: clear every pending Compound/CompoundWait ticket
// gated on sequencer id, leaving every other pending ticket untouched.
func EncodeCancelSequencer(id SequencerID) []byte {
	buf := make([]byte, 3)
	buf[0] = byte(KindCancelSequencer)
	binary.BigEndian.PutUint16(buf[1:3], uint16(id))
	return buf
}

// DecodeCancelSequencer parses a KindCancelSequencer envelope. It never
// panics on malformed input.
func DecodeCancelSequencer(body []byte) (SequencerID, error) {
	if len(body) < 3 {
		return 0, ErrShortBuffer
	}
	if Kind(body[0]) != KindCancelSequencer {
		return 0, ErrWrongKind
	}
	if len(body) > 3 {
		return 0, ErrTrailingBytes
	}
	return SequencerID(binary.BigEndian.Uint16(body[1:3])), nil
}

// ConditionalResult is the response payload for a resolved KindCompound or
// KindCompoundWait ticket: whether the gate condition matched, and the
// gated sequencer's value immediately after resolution (advanced on a
// match, unchanged otherwise — see Conditional.AdvanceOnMatch).
type ConditionalResult struct {
	Matched        bool
	SequencerValue uint32
}

// EncodeConditionalResponse serializes res, followed by innerBody (the
// wrapped Handler.HandleRequest response body for a matched KindCompound
// ticket; always empty for KindCompoundWait, and for an unmatched
// KindCompound). Unlike the request-side envelopes, a response carries no
// leading Kind byte — the requester already knows which Kind it sent,
// correlated via the enclosing Message's TransactionNum, matching the
// unwrapped-response convention every Phase 14 endpoint type already uses.
func EncodeConditionalResponse(res ConditionalResult, innerBody []byte) []byte {
	buf := make([]byte, 1+4+len(innerBody))
	if res.Matched {
		buf[0] = 1
	}
	binary.BigEndian.PutUint32(buf[1:5], res.SequencerValue)
	copy(buf[5:], innerBody)
	return buf
}

// DecodeConditionalResponse parses a response body produced by
// EncodeConditionalResponse. It never panics on malformed input.
func DecodeConditionalResponse(body []byte) (ConditionalResult, []byte, error) {
	if len(body) < 5 {
		return ConditionalResult{}, nil, ErrShortBuffer
	}
	res := ConditionalResult{
		Matched:        body[0] != 0,
		SequencerValue: binary.BigEndian.Uint32(body[1:5]),
	}
	var inner []byte
	if rest := body[5:]; len(rest) > 0 {
		inner = make([]byte, len(rest))
		copy(inner, rest)
	}
	return res, inner, nil
}

// EncodeCancelResponse serializes the number of tickets a cancellation
// request cleared.
func EncodeCancelResponse(cancelled uint16) []byte {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, cancelled)
	return buf
}

// DecodeCancelResponse parses a response body produced by
// EncodeCancelResponse. It never panics on malformed input.
func DecodeCancelResponse(body []byte) (uint16, error) {
	if len(body) < 2 {
		return 0, ErrShortBuffer
	}
	if len(body) > 2 {
		return 0, ErrTrailingBytes
	}
	return binary.BigEndian.Uint16(body), nil
}
