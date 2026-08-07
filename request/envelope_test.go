//fusa:test REQ-REQ-005
//fusa:test REQ-REQ-006
//fusa:test REQ-REQ-007

package request_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/request"
)

// TestEnvelope_CompoundRoundTrip checks EncodeCompound/DecodeCompound
// round-trip the condition and inner request, and reject a short buffer or
// a mismatched Kind tag (REQ-REQ-005).
func TestEnvelope_CompoundRoundTrip(t *testing.T) {
	c := request.Conditional{Sequencer: 3, Op: request.CompareGreaterOrEqual, Operand: 42, AdvanceOnMatch: -7}
	body := request.EncodeCompound(c, acf.FlagWrite, []byte{0xAA, 0xBB})

	gotC, gotControl, gotBody, err := request.DecodeCompound(body)
	if err != nil {
		t.Fatalf("DecodeCompound: %v", err)
	}
	if gotC != c || gotControl != acf.FlagWrite || string(gotBody) != "\xAA\xBB" {
		t.Errorf("DecodeCompound = (%+v, %v, % X), want (%+v, %v, AA BB)", gotC, gotControl, gotBody, c, acf.FlagWrite)
	}

	if _, _, _, err := request.DecodeCompound(body[:2]); !errors.Is(err, request.ErrShortBuffer) {
		t.Errorf("DecodeCompound(short) err = %v, want ErrShortBuffer", err)
	}

	wrongKind := append([]byte(nil), body...)
	wrongKind[0] = byte(request.KindTriggered)
	if _, _, _, err := request.DecodeCompound(wrongKind); !errors.Is(err, request.ErrWrongKind) {
		t.Errorf("DecodeCompound(wrong kind) err = %v, want ErrWrongKind", err)
	}
}

// TestEnvelope_CompoundWaitRoundTrip checks EncodeCompoundWait/
// DecodeCompoundWait round-trip and reject trailing bytes (REQ-REQ-005).
func TestEnvelope_CompoundWaitRoundTrip(t *testing.T) {
	c := request.Conditional{Sequencer: 1, Op: request.CompareLess, Operand: 100, AdvanceOnMatch: 1}
	body := request.EncodeCompoundWait(c)

	got, err := request.DecodeCompoundWait(body)
	if err != nil {
		t.Fatalf("DecodeCompoundWait: %v", err)
	}
	if got != c {
		t.Errorf("DecodeCompoundWait = %+v, want %+v", got, c)
	}

	if _, err := request.DecodeCompoundWait(append(body, 0x00)); !errors.Is(err, request.ErrTrailingBytes) {
		t.Errorf("DecodeCompoundWait(overlong) err = %v, want ErrTrailingBytes", err)
	}
}

// TestEnvelope_TriggeredRoundTrip checks EncodeTriggered/DecodeTriggered
// round-trip the trigger source and inner request (REQ-REQ-006).
func TestEnvelope_TriggeredRoundTrip(t *testing.T) {
	body := request.EncodeTriggered(avtp.ByteBusID(9), acf.FlagRead, nil)
	source, control, inner, err := request.DecodeTriggered(body)
	if err != nil {
		t.Fatalf("DecodeTriggered: %v", err)
	}
	if source != avtp.ByteBusID(9) || control != acf.FlagRead || len(inner) != 0 {
		t.Errorf("DecodeTriggered = (%v, %v, %v), want (9, FlagRead, empty)", source, control, inner)
	}
}

// TestEnvelope_TimedRoundTrip checks EncodeTimed/DecodeTimed round-trip the
// target execution time and inner request (REQ-REQ-006).
func TestEnvelope_TimedRoundTrip(t *testing.T) {
	body := request.EncodeTimed(0x0102030405060708, acf.FlagWrite, []byte{0x01})
	at, control, inner, err := request.DecodeTimed(body)
	if err != nil {
		t.Fatalf("DecodeTimed: %v", err)
	}
	if at != 0x0102030405060708 || control != acf.FlagWrite || string(inner) != "\x01" {
		t.Errorf("DecodeTimed = (%#x, %v, % X), want (0x102030405060708, FlagWrite, 01)", at, control, inner)
	}
}

// TestEnvelope_CancellationRoundTrip checks all three cancellation envelope
// shapes (mandatory clear-all, and the two optional narrower variants)
// round-trip through their Encode/Decode pair (REQ-REQ-007).
func TestEnvelope_CancellationRoundTrip(t *testing.T) {
	if err := request.DecodeCancelAll(request.EncodeCancelAll()); err != nil {
		t.Errorf("DecodeCancelAll: %v", err)
	}

	txn, err := request.DecodeCancelTransaction(request.EncodeCancelTransaction(avtp.TransactionNum(0x1234)))
	if err != nil || txn != avtp.TransactionNum(0x1234) {
		t.Errorf("DecodeCancelTransaction = (%v, %v), want (0x1234, nil)", txn, err)
	}

	id, err := request.DecodeCancelSequencer(request.EncodeCancelSequencer(request.SequencerID(0xABCD)))
	if err != nil || id != request.SequencerID(0xABCD) {
		t.Errorf("DecodeCancelSequencer = (%v, %v), want (0xABCD, nil)", id, err)
	}
}

// TestEnvelope_ResponseRoundTrip checks the conditional-result and
// cancellation-count response bodies round-trip (REQ-REQ-007).
func TestEnvelope_ResponseRoundTrip(t *testing.T) {
	res := request.ConditionalResult{Matched: true, SequencerValue: 99}
	gotRes, gotInner, err := request.DecodeConditionalResponse(request.EncodeConditionalResponse(res, []byte{0x01, 0x02}))
	if err != nil {
		t.Fatalf("DecodeConditionalResponse: %v", err)
	}
	if gotRes != res || string(gotInner) != "\x01\x02" {
		t.Errorf("DecodeConditionalResponse = (%+v, % X), want (%+v, 01 02)", gotRes, gotInner, res)
	}

	n, err := request.DecodeCancelResponse(request.EncodeCancelResponse(7))
	if err != nil || n != 7 {
		t.Errorf("DecodeCancelResponse = (%d, %v), want (7, nil)", n, err)
	}
}

// TestPeekKind checks PeekKind reads the leading byte without consuming the
// rest of body, and rejects a short buffer or an invalid tag (REQ-REQ-005).
func TestPeekKind(t *testing.T) {
	k, err := request.PeekKind(request.EncodeCancelAll())
	if err != nil || k != request.KindCancelAll {
		t.Errorf("PeekKind(CancelAll) = (%v, %v), want (KindCancelAll, nil)", k, err)
	}
	if _, err := request.PeekKind(nil); !errors.Is(err, request.ErrShortBuffer) {
		t.Errorf("PeekKind(nil) err = %v, want ErrShortBuffer", err)
	}
	if _, err := request.PeekKind([]byte{0xFF}); !errors.Is(err, request.ErrInvalidKind) {
		t.Errorf("PeekKind(invalid) err = %v, want ErrInvalidKind", err)
	}
	if _, err := request.PeekKind([]byte{byte(request.KindPlain)}); !errors.Is(err, request.ErrInvalidKind) {
		t.Errorf("PeekKind(KindPlain byte) err = %v, want ErrInvalidKind (KindPlain never appears on the wire)", err)
	}
}
