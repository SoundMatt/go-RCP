//fusa:test REQ-CAPI-001
//fusa:test REQ-CAPI-002
//fusa:test REQ-CAPI-003
//fusa:test REQ-CAPI-004
//fusa:test REQ-CAPI-005
//fusa:test REQ-CAPI-006
//fusa:test REQ-CAPI-007
//fusa:test REQ-CAPI-008

package capi_test

import (
	"context"
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/capi"
)

// stubController is a minimal capi.Controller test double, independent of
// mock/udp, exercising capi's own handle bookkeeping in isolation.
type stubController struct {
	lastAddr    avtp.ByteBusID
	lastControl acf.ControlFlags
	lastBody    []byte
	resp        acf.Message
	err         error
	closed      bool
}

func (s *stubController) Request(_ context.Context, addr avtp.ByteBusID, control acf.ControlFlags, body []byte) (acf.Message, error) {
	s.lastAddr, s.lastControl, s.lastBody = addr, control, body
	return s.resp, s.err
}

func (s *stubController) Close() error {
	s.closed = true
	return nil
}

// TestRegisterController_ThenRequest verifies a registered Controller's
// Request is reachable through its handle with the given addr/control/body
// (REQ-CAPI-001).
func TestRegisterController_ThenRequest(t *testing.T) {
	stub := &stubController{resp: acf.Message{Body: []byte("ack")}}
	h := capi.RegisterController(stub)
	defer capi.Close(h)

	resp, code := capi.Request(h, 3, acf.FlagWrite, []byte("hi"), 0)
	if code != capi.ErrCodeOK {
		t.Fatalf("code = %d, want ErrCodeOK", code)
	}
	if string(resp.Body) != "ack" {
		t.Errorf("resp.Body = %q, want %q", resp.Body, "ack")
	}
	if stub.lastAddr != 3 || !stub.lastControl.Has(acf.FlagWrite) || string(stub.lastBody) != "hi" {
		t.Errorf("stub saw addr=%d control=%v body=%q", stub.lastAddr, stub.lastControl, stub.lastBody)
	}
}

// TestRequest_InvalidHandle verifies an unknown handle reports
// ErrCodeInvalidHandle (REQ-CAPI-002).
func TestRequest_InvalidHandle(t *testing.T) {
	_, code := capi.Request(999999, 1, acf.FlagRead, nil, 0)
	if code != capi.ErrCodeInvalidHandle {
		t.Errorf("code = %d, want ErrCodeInvalidHandle", code)
	}
}

// TestRequest_UnderlyingError_ReportsRequestFailed verifies a Controller
// error is surfaced as ErrCodeRequestFailed (REQ-CAPI-003).
func TestRequest_UnderlyingError_ReportsRequestFailed(t *testing.T) {
	stub := &stubController{err: errors.New("boom")}
	h := capi.RegisterController(stub)
	defer capi.Close(h)

	_, code := capi.Request(h, 1, acf.FlagRead, nil, 0)
	if code != capi.ErrCodeRequestFailed {
		t.Errorf("code = %d, want ErrCodeRequestFailed", code)
	}
}

// TestRead_SetsFlagRead verifies Read issues a request with exactly
// FlagRead set and no body (REQ-CAPI-004).
func TestRead_SetsFlagRead(t *testing.T) {
	stub := &stubController{}
	h := capi.RegisterController(stub)
	defer capi.Close(h)

	if _, code := capi.Read(h, 7, 0); code != capi.ErrCodeOK {
		t.Fatalf("code = %d, want ErrCodeOK", code)
	}
	if !stub.lastControl.Has(acf.FlagRead) || stub.lastControl.Has(acf.FlagWrite) {
		t.Errorf("control = %v, want FlagRead only", stub.lastControl)
	}
	if stub.lastAddr != 7 {
		t.Errorf("addr = %d, want 7", stub.lastAddr)
	}
}

// TestWrite_SetsFlagWrite verifies Write issues a request with exactly
// FlagWrite set and the given body (REQ-CAPI-005).
func TestWrite_SetsFlagWrite(t *testing.T) {
	stub := &stubController{}
	h := capi.RegisterController(stub)
	defer capi.Close(h)

	if _, code := capi.Write(h, 7, []byte("payload"), 0); code != capi.ErrCodeOK {
		t.Fatalf("code = %d, want ErrCodeOK", code)
	}
	if !stub.lastControl.Has(acf.FlagWrite) || stub.lastControl.Has(acf.FlagRead) {
		t.Errorf("control = %v, want FlagWrite only", stub.lastControl)
	}
	if string(stub.lastBody) != "payload" {
		t.Errorf("body = %q, want %q", stub.lastBody, "payload")
	}
}

// TestClose_DeregistersAndClosesController verifies Close removes the
// handle (HandleCount drops, and a subsequent Request reports
// ErrCodeInvalidHandle) and calls Close on the underlying Controller
// (REQ-CAPI-006).
func TestClose_DeregistersAndClosesController(t *testing.T) {
	stub := &stubController{}
	h := capi.RegisterController(stub)
	before := capi.HandleCount()

	capi.Close(h)

	if got := capi.HandleCount(); got != before-1 {
		t.Errorf("HandleCount after Close = %d, want %d", got, before-1)
	}
	if !stub.closed {
		t.Error("underlying Controller.Close was not called")
	}
	if _, code := capi.Request(h, 1, acf.FlagRead, nil, 0); code != capi.ErrCodeInvalidHandle {
		t.Errorf("Request after Close: code = %d, want ErrCodeInvalidHandle", code)
	}
}

// TestClose_UnknownHandle_NoPanic verifies Close on a never-registered
// handle is a safe no-op (REQ-CAPI-007).
func TestClose_UnknownHandle_NoPanic(t *testing.T) {
	capi.Close(424242) // must not panic
}

// TestNewController_DialFailure verifies an unparseable server address
// reports ErrCodeDialFailed without panicking (REQ-CAPI-008).
func TestNewController_DialFailure(t *testing.T) {
	_, code := capi.NewController(avtp.StreamID{}, "not a valid address")
	if code != capi.ErrCodeDialFailed {
		t.Errorf("code = %d, want ErrCodeDialFailed", code)
	}
}
