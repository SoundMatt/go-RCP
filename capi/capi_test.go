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
	"testing"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/capi"
	"github.com/SoundMatt/go-RCP/mock"
)

// REQ-CAPI-001: NewController returns a valid positive handle.
func TestNewController_Handle(t *testing.T) {
	h := capi.NewController(rcp.ZoneFrontLeft)
	if h <= 0 {
		t.Fatalf("NewController returned handle %d, want > 0", h)
	}
	capi.Close(h)
}

// REQ-CAPI-002: RegisterController registers an external controller by handle.
func TestRegisterController(t *testing.T) {
	inner := mock.NewController(rcp.ZoneFrontLeft, func(cmd *rcp.Command) *rcp.Response {
		return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusOK}
	})
	defer inner.Close()

	h := capi.RegisterController(inner)
	if h <= 0 {
		t.Fatalf("RegisterController returned handle %d, want > 0", h)
	}
	capi.Close(h) // deregisters; inner already has its own Close called by defer
}

// REQ-CAPI-003: Send dispatches a command and returns StatusOK.
func TestSend_OK(t *testing.T) {
	h := capi.NewController(rcp.ZoneFrontLeft)
	defer capi.Close(h)

	status, code := capi.Send(h, rcp.ZoneFrontLeft, rcp.CmdSet, nil)
	if code != capi.ErrCodeOK {
		t.Fatalf("Send code = %d, want ErrCodeOK", code)
	}
	if status != rcp.StatusOK {
		t.Errorf("status = %v, want StatusOK", status)
	}
}

// REQ-CAPI-004: Send returns ErrCodeInvalidHandle for unknown handles.
func TestSend_InvalidHandle(t *testing.T) {
	_, code := capi.Send(99999, rcp.ZoneFrontLeft, rcp.CmdSet, nil)
	if code != capi.ErrCodeInvalidHandle {
		t.Errorf("code = %d, want ErrCodeInvalidHandle", code)
	}
}

// REQ-CAPI-005: Subscribe returns a valid subscription handle.
func TestSubscribe_OK(t *testing.T) {
	h := capi.NewController(rcp.ZoneFrontLeft)
	defer capi.Close(h)

	sub, code := capi.Subscribe(h)
	if code != capi.ErrCodeOK {
		t.Fatalf("Subscribe code = %d, want ErrCodeOK", code)
	}
	if sub <= 0 {
		t.Errorf("sub handle = %d, want > 0", sub)
	}
	capi.CloseSub(sub)
}

// REQ-CAPI-006: PollStatus returns ErrCodeNoData when no event is ready.
func TestPollStatus_NoData(t *testing.T) {
	h := capi.NewController(rcp.ZoneFrontLeft)
	defer capi.Close(h)

	sub, _ := capi.Subscribe(h)
	defer capi.CloseSub(sub)

	_, code := capi.PollStatus(sub)
	if code != capi.ErrCodeNoData {
		t.Errorf("code = %d, want ErrCodeNoData", code)
	}
}

// REQ-CAPI-007: PollStatus returns a Status event after Publish.
func TestPollStatus_EventAvailable(t *testing.T) {
	dispatched := make(chan struct{}, 1)
	inner := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer inner.Close()

	h := capi.RegisterController(inner)
	defer capi.Close(h)

	sub, code := capi.Subscribe(h)
	if code != capi.ErrCodeOK {
		t.Fatalf("Subscribe: code %d", code)
	}
	defer capi.CloseSub(sub)

	go func() {
		time.Sleep(5 * time.Millisecond)
		inner.Publish([]byte("ping"))
		dispatched <- struct{}{}
	}()

	<-dispatched
	time.Sleep(10 * time.Millisecond) // let the event propagate

	st, code := capi.PollStatus(sub)
	if code != capi.ErrCodeOK {
		t.Errorf("PollStatus code = %d after Publish, want ErrCodeOK", code)
	}
	if st != nil && st.Zone != rcp.ZoneFrontLeft {
		t.Errorf("status zone = %v, want ZoneFrontLeft", st.Zone)
	}
}

// REQ-CAPI-008: Close deregisters the handle; subsequent Send returns ErrCodeInvalidHandle.
func TestClose_InvalidatesHandle(t *testing.T) {
	h := capi.NewController(rcp.ZoneFrontLeft)
	capi.Close(h)

	_, code := capi.Send(h, rcp.ZoneFrontLeft, rcp.CmdSet, nil)
	if code != capi.ErrCodeInvalidHandle {
		t.Errorf("code after Close = %d, want ErrCodeInvalidHandle", code)
	}
}
