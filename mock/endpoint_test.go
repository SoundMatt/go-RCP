//fusa:test REQ-MEP-001
//fusa:test REQ-MEP-002
//fusa:test REQ-MEP-003
//fusa:test REQ-MEP-004
//fusa:test REQ-MEP-005
//fusa:test REQ-MEP-006

package mock_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/mock"
)

func testStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55}, 1)
}

// TestEndpoint_DefaultOK verifies a nil EndpointFunc answers with
// FlagResponse and the originating Read/Write flag preserved
// (REQ-MEP-001).
func TestEndpoint_DefaultOK(t *testing.T) {
	ep := mock.NewEndpoint(1, nil)
	resp, err := ep.HandleRequest(testStream(), acf.Message{ByteBusID: 1, TransactionNum: 7, Control: acf.FlagRead})
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	if !resp.Control.Has(acf.FlagResponse) {
		t.Error("response missing FlagResponse")
	}
	if !resp.Control.Has(acf.FlagRead) {
		t.Error("response lost the originating FlagRead")
	}
	if resp.TransactionNum != 7 {
		t.Errorf("TransactionNum = %d, want 7", resp.TransactionNum)
	}
}

// TestEndpoint_CustomFunc verifies a configured EndpointFunc is invoked
// with the requester and request, and its response is returned verbatim
// (REQ-MEP-002).
func TestEndpoint_CustomFunc(t *testing.T) {
	var gotFrom avtp.StreamID
	var gotReq acf.Message
	want := acf.Message{Body: []byte("custom")}
	ep := mock.NewEndpoint(3, func(requester avtp.StreamID, req acf.Message) (acf.Message, error) {
		gotFrom = requester
		gotReq = req
		return want, nil
	})

	stream := testStream()
	req := acf.Message{ByteBusID: 3, Control: acf.FlagWrite, Body: []byte("in")}
	resp, err := ep.HandleRequest(stream, req)
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	if gotFrom != stream {
		t.Errorf("requester = %v, want %v", gotFrom, stream)
	}
	if string(gotReq.Body) != "in" {
		t.Errorf("req.Body = %q, want %q", gotReq.Body, "in")
	}
	if string(resp.Body) != "custom" {
		t.Errorf("resp.Body = %q, want %q", resp.Body, "custom")
	}
}

// TestEndpoint_WrongEndpoint verifies a request addressed to a different
// byte_bus_id is rejected (REQ-MEP-003).
func TestEndpoint_WrongEndpoint(t *testing.T) {
	ep := mock.NewEndpoint(1, nil)
	_, err := ep.HandleRequest(testStream(), acf.Message{ByteBusID: 2, Control: acf.FlagRead})
	if !errors.Is(err, mock.ErrWrongEndpoint) {
		t.Errorf("err = %v, want ErrWrongEndpoint", err)
	}
}

// TestEndpoint_ClosedRejectsRequest verifies a request after Close reports
// ErrClosed (REQ-MEP-004).
func TestEndpoint_ClosedRejectsRequest(t *testing.T) {
	ep := mock.NewEndpoint(1, nil)
	if err := ep.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	_, err := ep.HandleRequest(testStream(), acf.Message{ByteBusID: 1, Control: acf.FlagRead})
	if !errors.Is(err, mock.ErrClosed) {
		t.Errorf("err = %v, want ErrClosed", err)
	}
}

// TestEndpoint_Close_Idempotent verifies Close is safe to call more than
// once (REQ-MEP-005).
func TestEndpoint_Close_Idempotent(t *testing.T) {
	ep := mock.NewEndpoint(1, nil)
	if err := ep.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := ep.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

// TestEndpoint_Addr verifies Addr reports the configured address
// (REQ-MEP-006).
func TestEndpoint_Addr(t *testing.T) {
	ep := mock.NewEndpoint(42, nil)
	if ep.Addr() != 42 {
		t.Errorf("Addr() = %d, want 42", ep.Addr())
	}
}
