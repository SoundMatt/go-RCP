//fusa:test REQ-REQ-008
//fusa:test REQ-REQ-009

package request_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/request"
)

// TestChained_EncodeDecodeRoundTrip checks EncodeChained/DecodeChained
// round-trip multiple segments in order, and reject zero segments or a
// truncated buffer (REQ-REQ-008).
func TestChained_EncodeDecodeRoundTrip(t *testing.T) {
	segs := []request.ChainedSegment{
		{Control: acf.FlagWrite, Body: []byte{0x01, 0x02}},
		{Control: acf.FlagRead, Body: nil},
		{Control: acf.FlagWrite, Body: []byte{0x03}},
	}
	body, err := request.EncodeChained(segs)
	if err != nil {
		t.Fatalf("EncodeChained: %v", err)
	}

	got, err := request.DecodeChained(body)
	if err != nil {
		t.Fatalf("DecodeChained: %v", err)
	}
	if len(got) != len(segs) {
		t.Fatalf("DecodeChained returned %d segments, want %d", len(got), len(segs))
	}
	for i := range segs {
		if got[i].Control != segs[i].Control || !bytes.Equal(got[i].Body, segs[i].Body) {
			t.Errorf("segment %d = %+v, want %+v", i, got[i], segs[i])
		}
	}

	if _, err := request.EncodeChained(nil); !errors.Is(err, request.ErrInvalidSegmentCount) {
		t.Errorf("EncodeChained(empty) err = %v, want ErrInvalidSegmentCount", err)
	}
	if _, err := request.DecodeChained(body[:3]); !errors.Is(err, request.ErrShortBuffer) {
		t.Errorf("DecodeChained(truncated) err = %v, want ErrShortBuffer", err)
	}
}

// TestChainedResponse_RoundTrip checks EncodeChainedResponse/
// DecodeChainedResponse round-trip a partial (aborted) result, preserving
// which segment is marked Failed and that Total still reflects the
// original, larger segment count (REQ-REQ-009).
func TestChainedResponse_RoundTrip(t *testing.T) {
	result := request.ChainedResult{
		Responses: []request.ChainedResponse{
			{Body: []byte{0xAA}},
			{Failed: true},
		},
		Total: 5,
	}
	got, err := request.DecodeChainedResponse(request.EncodeChainedResponse(result))
	if err != nil {
		t.Fatalf("DecodeChainedResponse: %v", err)
	}
	if got.Total != 5 {
		t.Errorf("Total = %d, want 5 (original segment count, not len(Responses))", got.Total)
	}
	if len(got.Responses) != 2 || !got.Responses[1].Failed || got.Responses[0].Failed {
		t.Errorf("Responses = %+v, want [{Body:AA Failed:false} {Failed:true}]", got.Responses)
	}
}
