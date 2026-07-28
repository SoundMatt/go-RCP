package avtp

// Frame is a full AVTPDU carrying exactly one RCP-over-ACF message. Packing
// multiple ACF messages into a single AVTPDU, and fragmenting one logical
// RCP message across multiple AVTPDUs, are both out of scope for this
// milestone (see ROADMAP.md Milestone 52, Fragmentation); Message.Control's
// FlagMoreSegments and ReadSizeOrSegment-as-segment-number exist so that
// later milestone's wire encoding stays backward compatible with what's
// built here.
type Frame struct {
	Header  Header
	Message Message
}

// EncodeFrame serializes hdr and msg into a single AVTPDU. The header's
// DataLength is always recomputed from the encoded message rather than
// trusting hdr.DataLength, the same way this repo's other wire encoders
// (e.g. wire.EncodeCommand, someip's encodeFrame) derive their own length
// fields instead of accepting a caller-supplied value that could drift from
// the truth.
func EncodeFrame(hdr Header, msg Message) ([]byte, error) {
	msgBytes, err := EncodeMessage(msg)
	if err != nil {
		return nil, err
	}
	if len(msgBytes) > dataLengthMask {
		return nil, ErrDataLengthOverflow
	}
	hdr.DataLength = uint16(len(msgBytes))

	hdrBytes, err := EncodeHeader(hdr)
	if err != nil {
		return nil, err
	}

	out := make([]byte, 0, len(hdrBytes)+len(msgBytes))
	out = append(out, hdrBytes...)
	out = append(out, msgBytes...)
	return out, nil
}

// DecodeFrame parses a full AVTPDU from b: the header, then exactly
// hdr.DataLength bytes decoded as the enclosed Message. Any bytes beyond
// that are rejected as a length mismatch rather than silently ignored,
// since a frame that's longer than its own declared length is exactly the
// kind of malformed input a receiver must not paper over. It never panics
// on malformed input.
func DecodeFrame(b []byte) (Frame, error) {
	hdr, rest, err := DecodeHeader(b)
	if err != nil {
		return Frame{}, err
	}
	if uint64(len(rest)) != uint64(hdr.DataLength) {
		return Frame{}, ErrFrameLengthMismatch
	}
	msg, err := DecodeMessage(rest)
	if err != nil {
		return Frame{}, err
	}
	return Frame{Header: hdr, Message: msg}, nil
}
