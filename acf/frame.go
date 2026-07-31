package acf

import "github.com/SoundMatt/go-RCP/avtp"

// Frame is a full AVTPDU carrying zero or more RCP-over-ACF messages: the
// avtp package's IEEE 1722 header framing wrapped around a sequence of this
// package's ACF_ABB/ACF_GBB messages, each self-describing its own length
// via acf_msg_length. It lives here, in acf, rather than in avtp, because it
// depends on both packages — avtp intentionally does not import acf (see
// avtp/doc.go) — and "the composed unit carrying ACF messages inside an
// AVTPDU" is squarely this package's own concern.
//
// Per TC18 §12.9.1.1 ("Handling multiple requests in incoming messages"):
// "An RC Server shall support to handle multiple requests in one frame and
// check each of them individually if to be processed or not... An RCP frame
// may include multiple ACF-types (requests)." Messages holds every message
// DecodeFrame walked out of the AVTPDU payload, in wire order; a caller that
// only ever expects one message (still the common case for a simple
// request/response exchange) reads Messages[0].
//
// Fragmenting one logical RCP message's payload across multiple AVTPDUs is
// a separate, orthogonal mechanism (see ROADMAP.md Milestone 52,
// Fragmentation, and the fragment package) — Message.Control's
// FlagMoreSegments and ReadSizeOrSegment-as-segment-number exist for that,
// not for this.
type Frame struct {
	Header   avtp.Header
	Messages []Message
}

// EncodeFrame serializes hdr and msgs into a single AVTPDU: each message
// encoded and concatenated in order, per TC18 §12.9.1.1. Passing a single
// Message still works exactly as before (msgs is variadic). The header's
// DataLength is always recomputed from the encoded messages rather than
// trusting hdr.DataLength, the same way this repo's other wire encoders
// (e.g. wire.EncodeCommand, someip's encodeFrame) derive their own length
// fields instead of accepting a caller-supplied value that could drift from
// the truth.
func EncodeFrame(hdr avtp.Header, msgs ...Message) ([]byte, error) {
	var payload []byte
	for _, msg := range msgs {
		msgBytes, err := EncodeMessage(msg)
		if err != nil {
			return nil, err
		}
		payload = append(payload, msgBytes...)
	}
	if len(payload) > avtp.MaxDataLength {
		return nil, avtp.ErrDataLengthOverflow
	}
	hdr.DataLength = uint16(len(payload))

	hdrBytes, err := avtp.EncodeHeader(hdr)
	if err != nil {
		return nil, err
	}

	out := make([]byte, 0, len(hdrBytes)+len(payload))
	out = append(out, hdrBytes...)
	out = append(out, payload...)
	return out, nil
}

// DecodeFrame parses a full AVTPDU from b: the header, then its
// hdr.DataLength-byte payload walked as a sequence of zero or more
// independently-addressed ACF messages, per TC18 §12.9.1.1 — each message's
// own acf_msg_length says how many bytes it occupies, so DecodeFrame slices
// one message off the front of the remaining payload at a time until
// nothing is left, rather than assuming the payload is exactly one message.
// Any bytes that don't form a whole message at the point decoding stops are
// rejected (a malformed/truncated trailing message), and a header whose
// declared DataLength does not match the actual payload length is rejected
// outright — a frame that's longer or shorter than its own declared length
// is exactly the kind of malformed input a receiver must not paper over. It
// never panics on malformed input.
func DecodeFrame(b []byte) (Frame, error) {
	hdr, rest, err := avtp.DecodeHeader(b)
	if err != nil {
		return Frame{}, err
	}
	if uint64(len(rest)) != uint64(hdr.DataLength) {
		return Frame{}, ErrFrameLengthMismatch
	}

	var msgs []Message
	for len(rest) > 0 {
		msg, n, err := DecodeMessagePrefix(rest)
		if err != nil {
			return Frame{}, err
		}
		msgs = append(msgs, msg)
		rest = rest[n:]
	}
	return Frame{Header: hdr, Messages: msgs}, nil
}
