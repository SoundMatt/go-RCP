//fusa:test REQ-MDIO-004

package mdio_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/mdio"
)

// TestReadWriteRequestRoundTrip checks Encode/Decode round-trip for read
// requests, write requests, and responses, and rejects short/overlong
// buffers (REQ-MDIO-004).
func TestReadWriteRequestRoundTrip(t *testing.T) {
	r := mdio.Request{Mode: mdio.ModeClause45, PhyAddr: 5, DevAddr: 3, RegAddr: 0x1234}

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

	wb := mdio.EncodeWriteRequest(r, 0xBEEF)
	gotR, gotData, err := mdio.DecodeWriteRequest(wb)
	if err != nil {
		t.Fatalf("DecodeWriteRequest: %v", err)
	}
	if gotR != r || gotData != 0xBEEF {
		t.Errorf("DecodeWriteRequest round-trip = %+v/%#x, want %+v/0xBEEF", gotR, gotData, r)
	}
	if _, _, werr := mdio.DecodeWriteRequest(wb[:len(wb)-1]); !errors.Is(werr, mdio.ErrShortBuffer) {
		t.Errorf("DecodeWriteRequest(short) err = %v, want ErrShortBuffer", werr)
	}
	if _, _, werr := mdio.DecodeWriteRequest(append(wb, 0x00)); !errors.Is(werr, mdio.ErrTrailingBytes) {
		t.Errorf("DecodeWriteRequest(overlong) err = %v, want ErrTrailingBytes", werr)
	}

	respB := mdio.EncodeResponse(0xCAFE)
	gotResp, err := mdio.DecodeResponse(respB)
	if err != nil {
		t.Fatalf("DecodeResponse: %v", err)
	}
	if gotResp != 0xCAFE {
		t.Errorf("DecodeResponse round-trip = %#x, want 0xCAFE", gotResp)
	}
	if _, err := mdio.DecodeResponse(respB[:len(respB)-1]); !errors.Is(err, mdio.ErrShortBuffer) {
		t.Errorf("DecodeResponse(short) err = %v, want ErrShortBuffer", err)
	}
	if _, err := mdio.DecodeResponse(append(respB, 0x00)); !errors.Is(err, mdio.ErrTrailingBytes) {
		t.Errorf("DecodeResponse(overlong) err = %v, want ErrTrailingBytes", err)
	}
}
