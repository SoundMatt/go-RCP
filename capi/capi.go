//fusa:req REQ-CAPI-001
//fusa:req REQ-CAPI-002
//fusa:req REQ-CAPI-003
//fusa:req REQ-CAPI-004
//fusa:req REQ-CAPI-005
//fusa:req REQ-CAPI-006
//fusa:req REQ-CAPI-007
//fusa:req REQ-CAPI-008

// Package capi provides a C-compatible handle-based API for go-RCP controllers.
//
// RTOS firmware and bare-metal C code can interact with go-RCP by building
// this package with "-buildmode=c-shared" (shared library) or
// "-buildmode=c-archive" (static archive), then linking against the generated
// rcp.h header and library.
//
// This package exposes a handle-based API where each rcp.Controller is
// identified by an opaque int32 handle. Subscription channels are similarly
// identified by a separate subscription handle.
//
// Handle-based C API surface (see rcp.h):
//
//	int32_t rcp_new_controller(int32_t zone);
//	int32_t rcp_send(int32_t handle, int32_t zone, int32_t type,
//	                 const uint8_t *payload, int32_t len,
//	                 uint8_t *status_out);
//	int32_t rcp_subscribe(int32_t handle, int32_t *sub_handle_out);
//	int32_t rcp_poll_status(int32_t sub, int32_t *zone, uint32_t *seq,
//	                        uint8_t *payload, int32_t payload_cap,
//	                        int32_t *payload_len);
//	void    rcp_close(int32_t handle);
//	void    rcp_close_sub(int32_t sub);
package capi

import (
	"context"
	"sync"
	"sync/atomic"

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/mock"
)

// ErrCodeOK is returned by C API functions on success.
const ErrCodeOK = int32(0)

// ErrCodeInvalidHandle is returned when an invalid handle is supplied.
const ErrCodeInvalidHandle = int32(-1)

// ErrCodeSendFailed is returned when rcp.Controller.Send fails.
const ErrCodeSendFailed = int32(-2)

// ErrCodeNoData is returned by PollStatus when no event is available.
const ErrCodeNoData = int32(-3)

// ErrCodeSubscribeFailed is returned when Subscribe fails.
const ErrCodeSubscribeFailed = int32(-4)

// ─── Registry ─────────────────────────────────────────────────────────────────

var (
	ctrlMu  sync.Mutex
	ctrls   = map[int32]rcp.Controller{}
	ctrlSeq atomic.Int32

	subMu  sync.Mutex
	subs   = map[int32]<-chan *rcp.Status{}
	subSeq atomic.Int32
)

// newHandle allocates a unique positive int32 handle.
func newCtrlHandle(ctrl rcp.Controller) int32 {
	h := ctrlSeq.Add(1)
	ctrlMu.Lock()
	ctrls[h] = ctrl
	ctrlMu.Unlock()
	return h
}

func getCtrl(h int32) (rcp.Controller, bool) {
	ctrlMu.Lock()
	c, ok := ctrls[h]
	ctrlMu.Unlock()
	return c, ok
}

func newSubHandle(ch <-chan *rcp.Status) int32 {
	h := subSeq.Add(1)
	subMu.Lock()
	subs[h] = ch
	subMu.Unlock()
	return h
}

func getSub(h int32) (<-chan *rcp.Status, bool) {
	subMu.Lock()
	ch, ok := subs[h]
	subMu.Unlock()
	return ch, ok
}

// ─── C API functions (callable from Go tests; exported via //export for C) ───

// NewController registers a mock rcp.Controller for the given zone and returns
// an opaque handle. In production, replace mock.NewController with the real
// implementation.
func NewController(zone rcp.Zone) int32 {
	ctrl := mock.NewController(zone, func(cmd *rcp.Command) *rcp.Response {
		return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusOK}
	})
	return newCtrlHandle(ctrl)
}

// RegisterController registers an existing rcp.Controller and returns its handle.
// Use this when go-RCP controllers are created externally (e.g. by the Go runtime)
// and need to be exposed through the C API.
func RegisterController(ctrl rcp.Controller) int32 {
	return newCtrlHandle(ctrl)
}

// Send dispatches a command to the controller identified by handle.
// Returns ErrCodeOK on success, ErrCodeInvalidHandle if the handle is unknown,
// or ErrCodeSendFailed if the controller returns an error.
func Send(handle int32, zone rcp.Zone, cmdType rcp.CommandType, payload []byte) (rcp.ResponseStatus, int32) {
	ctrl, ok := getCtrl(handle)
	if !ok {
		return 0, ErrCodeInvalidHandle
	}
	cmd := &rcp.Command{Zone: zone, Type: cmdType, Payload: payload}
	resp, err := ctrl.Send(context.Background(), cmd)
	if err != nil {
		return 0, ErrCodeSendFailed
	}
	return resp.Status, ErrCodeOK
}

// Subscribe opens a subscription on the controller identified by handle.
// Returns (subHandle, ErrCodeOK) on success or (-1, error code) on failure.
func Subscribe(handle int32) (int32, int32) {
	ctrl, ok := getCtrl(handle)
	if !ok {
		return -1, ErrCodeInvalidHandle
	}
	ch, err := ctrl.Subscribe(context.Background())
	if err != nil {
		return -1, ErrCodeSubscribeFailed
	}
	return newSubHandle(ch), ErrCodeOK
}

// PollStatus checks for a pending status event on the subscription identified
// by subHandle. Returns (event, ErrCodeOK) if an event is ready, or
// (nil, ErrCodeNoData) if the channel is empty.
func PollStatus(subHandle int32) (*rcp.Status, int32) {
	ch, ok := getSub(subHandle)
	if !ok {
		return nil, ErrCodeInvalidHandle
	}
	select {
	case st, ok := <-ch:
		if !ok {
			return nil, ErrCodeNoData
		}
		return st, ErrCodeOK
	default:
		return nil, ErrCodeNoData
	}
}

// Close deregisters the controller identified by handle and calls Close on it.
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

// CloseSub deregisters the subscription identified by subHandle.
func CloseSub(subHandle int32) {
	subMu.Lock()
	delete(subs, subHandle)
	subMu.Unlock()
}
