//fusa:test REQ-REQ-010

package request_test

import (
	"testing"

	"github.com/SoundMatt/go-RCP/request"
)

// ── REQ-REQ-010 (fuzz half): decoders never panic on arbitrary input ──────

func FuzzPeekKind(f *testing.F) {
	f.Add([]byte{})
	f.Add(goldenCompound)
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = request.PeekKind(b) // must not panic
	})
}

func FuzzDecodeCompound(f *testing.F) {
	f.Add([]byte{})
	f.Add(goldenCompound)
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _, _, _ = request.DecodeCompound(b) // must not panic
	})
}

func FuzzDecodeCompoundWait(f *testing.F) {
	f.Add([]byte{})
	f.Add(request.EncodeCompoundWait(request.Conditional{}))
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = request.DecodeCompoundWait(b) // must not panic
	})
}

func FuzzDecodeTriggered(f *testing.F) {
	f.Add([]byte{})
	f.Add(request.EncodeTriggered(0, 0, nil))
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _, _, _ = request.DecodeTriggered(b) // must not panic
	})
}

func FuzzDecodeTimed(f *testing.F) {
	f.Add([]byte{})
	f.Add(request.EncodeTimed(0, 0, nil))
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _, _, _ = request.DecodeTimed(b) // must not panic
	})
}

func FuzzDecodeChained(f *testing.F) {
	f.Add([]byte{})
	seed, _ := request.EncodeChained([]request.ChainedSegment{{Body: []byte{0x01}}})
	f.Add(seed)
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = request.DecodeChained(b) // must not panic
	})
}

func FuzzDecodeChainedResponse(f *testing.F) {
	f.Add([]byte{})
	f.Add(request.EncodeChainedResponse(request.ChainedResult{}))
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = request.DecodeChainedResponse(b) // must not panic
	})
}

func FuzzDecodeConditionalResponse(f *testing.F) {
	f.Add([]byte{})
	f.Add(request.EncodeConditionalResponse(request.ConditionalResult{}, nil))
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _, _ = request.DecodeConditionalResponse(b) // must not panic
	})
}

func FuzzDecodeCancellation(f *testing.F) {
	f.Add([]byte{})
	f.Add(request.EncodeCancelAll())
	f.Add(request.EncodeCancelTransaction(0))
	f.Add(request.EncodeCancelSequencer(0))
	f.Fuzz(func(t *testing.T, b []byte) {
		_ = request.DecodeCancelAll(b)            // must not panic
		_, _ = request.DecodeCancelTransaction(b) // must not panic
		_, _ = request.DecodeCancelSequencer(b)   // must not panic
		_, _ = request.DecodeCancelResponse(b)    // must not panic
	})
}
