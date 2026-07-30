//fusa:test REQ-MDIO-004

package mdio_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/mdio"
)

// TestReadWriteRequestRoundTrip checks Encode/Decode round-trip for read
// requests, write requests, and responses — at both the 16-bit (MMD) and
// 32-bit (MMS0/MMS1) payload widths — and rejects short/overlong buffers
// (REQ-MDIO-004).
func TestReadWriteRequestRoundTrip(t *testing.T) {
	t.Run("16-bit (MMD)", func(t *testing.T) {
		r := mdio.Request{Mode: mdio.ModeMMDMultiByte, DevAddr: 3, RegAddr: 0x1234}
		if got := r.DataWidth(); got != 2 {
			t.Fatalf("DataWidth() = %d, want 2", got)
		}
		testRoundTrip(t, r, 0xBEEF, 0xCAFE)
	})

	t.Run("32-bit (MMS0)", func(t *testing.T) {
		r := mdio.Request{Mode: mdio.ModeMMSSingleWord, DevAddr: 0, RegAddr: 0x0010}
		if got := r.DataWidth(); got != 4 {
			t.Fatalf("DataWidth() = %d, want 4", got)
		}
		testRoundTrip(t, r, 0xDEADBEEF, 0x12345678)
	})

	t.Run("16-bit (MMS, non-MMS0/1)", func(t *testing.T) {
		r := mdio.Request{Mode: mdio.ModeMMSMultiWord, DevAddr: 5, RegAddr: 0x0008}
		if got := r.DataWidth(); got != 2 {
			t.Fatalf("DataWidth() = %d, want 2", got)
		}
		testRoundTrip(t, r, 0x1357, 0x2468)
	})
}

// testRoundTrip exercises EncodeReadRequest/DecodeReadRequest,
// EncodeWriteRequest/DecodeWriteRequest, and EncodeResponse/DecodeResponse
// for r, and checks short/overlong buffers are rejected for each.
func testRoundTrip(t *testing.T, r mdio.Request, writeData, respData uint32) {
	t.Helper()

	rb := mdio.EncodeReadRequest(r)
	gotR, err := mdio.DecodeReadRequest(rb)
	if err != nil {
		t.Fatalf("DecodeReadRequest: %v", err)
	}
	if gotR != r {
		t.Errorf("DecodeReadRequest round-trip = %+v, want %+v", gotR, r)
	}
	if _, rerr := mdio.DecodeReadRequest(rb[:len(rb)-1]); !errors.Is(rerr, mdio.ErrShortBuffer) {
		t.Errorf("DecodeReadRequest(short) err = %v, want ErrShortBuffer", rerr)
	}
	if _, rerr := mdio.DecodeReadRequest(append(rb, 0x00)); !errors.Is(rerr, mdio.ErrTrailingBytes) {
		t.Errorf("DecodeReadRequest(overlong) err = %v, want ErrTrailingBytes", rerr)
	}

	wb := mdio.EncodeWriteRequest(r, writeData)
	if want := len(rb) + r.DataWidth(); len(wb) != want {
		t.Fatalf("len(EncodeWriteRequest) = %d, want %d", len(wb), want)
	}
	gotR, gotData, err := mdio.DecodeWriteRequest(wb)
	if err != nil {
		t.Fatalf("DecodeWriteRequest: %v", err)
	}
	if gotR != r || gotData != writeData {
		t.Errorf("DecodeWriteRequest round-trip = %+v/%#x, want %+v/%#x", gotR, gotData, r, writeData)
	}
	if _, _, werr := mdio.DecodeWriteRequest(wb[:len(wb)-1]); !errors.Is(werr, mdio.ErrShortBuffer) {
		t.Errorf("DecodeWriteRequest(short) err = %v, want ErrShortBuffer", werr)
	}
	if _, _, werr := mdio.DecodeWriteRequest(append(wb, 0x00)); !errors.Is(werr, mdio.ErrTrailingBytes) {
		t.Errorf("DecodeWriteRequest(overlong) err = %v, want ErrTrailingBytes", werr)
	}

	respB := mdio.EncodeResponse(r, respData)
	if len(respB) != r.DataWidth() {
		t.Fatalf("len(EncodeResponse) = %d, want %d", len(respB), r.DataWidth())
	}
	gotResp, err := mdio.DecodeResponse(r, respB)
	if err != nil {
		t.Fatalf("DecodeResponse: %v", err)
	}
	if gotResp != respData {
		t.Errorf("DecodeResponse round-trip = %#x, want %#x", gotResp, respData)
	}
	if _, err := mdio.DecodeResponse(r, respB[:len(respB)-1]); !errors.Is(err, mdio.ErrShortBuffer) {
		t.Errorf("DecodeResponse(short) err = %v, want ErrShortBuffer", err)
	}
	if _, err := mdio.DecodeResponse(r, append(respB, 0x00)); !errors.Is(err, mdio.ErrTrailingBytes) {
		t.Errorf("DecodeResponse(overlong) err = %v, want ErrTrailingBytes", err)
	}
}
