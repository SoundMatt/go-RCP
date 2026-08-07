package capi

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/udp"
)

// ErrCodeOK is returned by this package's C-ABI-shaped functions on
// success.
const ErrCodeOK = int32(0)

// ErrCodeInvalidHandle is returned when an unknown handle is supplied.
const ErrCodeInvalidHandle = int32(-1)

// ErrCodeDialFailed is returned when NewController fails to resolve or dial
// serverAddr.
const ErrCodeDialFailed = int32(-2)

// ErrCodeRequestFailed is returned when Request/Read/Write's underlying
// Controller call returns an error.
const ErrCodeRequestFailed = int32(-3)

// Controller is the minimal surface this package's C ABI needs from an
// underlying RCP client — exactly the Request/Close shape *udp.Controller
// (production) and *mock.Client (unit tests) already share. This package
// defines its own narrow local interface, rather than importing mock or
// depending on a shared root-module interface (the root module's own
// Controller interface is a Phase 18 concern this milestone does not
// resolve — see ROADMAP.md Phase 17's disposition table), so a caller of
// RegisterController can pass either concrete type without capi importing
// mock as a non-test dependency.
type Controller interface {
	Request(ctx context.Context, addr avtp.ByteBusID, control acf.ControlFlags, body []byte) (acf.Message, error)
	Close() error
}

var (
	ctrlMu  sync.Mutex
	ctrls   = map[int32]Controller{}
	ctrlSeq atomic.Int32
)

// newHandle allocates a unique positive int32 handle for ctrl.
func newHandle(ctrl Controller) int32 {
	h := ctrlSeq.Add(1)
	ctrlMu.Lock()
	ctrls[h] = ctrl
	ctrlMu.Unlock()
	return h
}

func getCtrl(h int32) (Controller, bool) {
	ctrlMu.Lock()
	c, ok := ctrls[h]
	ctrlMu.Unlock()
	return c, ok
}

// NewController dials a *udp.Controller presenting streamID at serverAddr
// and registers it, returning an opaque handle. This is the production
// path; unit tests instead build any Controller (e.g. a *mock.Client) and
// pass it to RegisterController directly.
func NewController(streamID avtp.StreamID, serverAddr string) (int32, int32) {
	addr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		return -1, ErrCodeDialFailed
	}
	ctrl, err := udp.NewController(streamID, addr)
	if err != nil {
		return -1, ErrCodeDialFailed
	}
	return newHandle(ctrl), ErrCodeOK
}

// RegisterController registers an existing Controller and returns its
// handle, for callers (typically tests) that already have one.
func RegisterController(ctrl Controller) int32 {
	return newHandle(ctrl)
}

// Request issues one request through the Controller identified by handle,
// blocking until a response arrives or timeoutMs elapses (a non-positive
// timeoutMs means no deadline at all — context.Background()). Returns
// ErrCodeInvalidHandle for an unknown handle, ErrCodeRequestFailed if the
// underlying Controller.Request call itself errors, or ErrCodeOK with the
// response otherwise (a wire-level FlagError response is still ErrCodeOK
// here — the caller inspects resp.Control itself, the same posture
// *udp.Controller.Request already takes; only Discover-shaped calls treat
// FlagError as a Go error).
func Request(handle int32, addr avtp.ByteBusID, control acf.ControlFlags, body []byte, timeoutMs int32) (acf.Message, int32) {
	ctrl, ok := getCtrl(handle)
	if !ok {
		return acf.Message{}, ErrCodeInvalidHandle
	}
	ctx := context.Background()
	if timeoutMs > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
		defer cancel()
	}
	resp, err := ctrl.Request(ctx, addr, control, body)
	if err != nil {
		return acf.Message{}, ErrCodeRequestFailed
	}
	return resp, ErrCodeOK
}

// Read is Request with acf.FlagRead set and no body.
func Read(handle int32, addr avtp.ByteBusID, timeoutMs int32) (acf.Message, int32) {
	return Request(handle, addr, acf.FlagRead, nil, timeoutMs)
}

// Write is Request with acf.FlagWrite set and the given body.
func Write(handle int32, addr avtp.ByteBusID, body []byte, timeoutMs int32) (acf.Message, int32) {
	return Request(handle, addr, acf.FlagWrite, body, timeoutMs)
}

// Close deregisters the Controller identified by handle and calls Close on
// it. It is a no-op (returns no error code — matching this package's
// void-returning rcp_close C signature) for an unknown handle.
func Close(handle int32) {
	ctrlMu.Lock()
	ctrl, ok := ctrls[handle]
	if ok {
		delete(ctrls, handle)
	}
	ctrlMu.Unlock()
	if ok {
		_ = ctrl.Close()
	}
}

// HandleCount reports the number of currently registered handles — mainly
// useful for tests verifying Close actually deregisters.
func HandleCount() int {
	ctrlMu.Lock()
	defer ctrlMu.Unlock()
	return len(ctrls)
}
