// Package canbr provides a CAN-bus-shaped client ergonomics layer over the
// native CAN endpoint type, for the OPEN Alliance TC18 Remote Control
// Protocol (RCP), as described by the "OPEN Alliance TC18 Remote Control
// Protocol Specification v0.5.1_RC".
//
// This is ROADMAP.md Milestone 56 (v0.69.0)'s ADAPT-flagged rebuild: per
// Phase 17's disposition table, CAN is now a native RCP endpoint type (the
// can package, ROADMAP.md Milestone 51), so "bridge RCP to CAN" narrows from
// a translation necessity — the retired package's own bespoke 13-byte frame
// format and in-process Bus, invented because nothing in the old protocol
// understood CAN framing at all — to an ergonomics layer: a caller-friendly,
// can.Frame-typed Send/Receive surface on top of a *udp.Controller already
// addressing a declared can.Endpoint, so an existing consumer written
// against "send a CAN frame, get a CAN frame back" doesn't have to hand-roll
// can.EncodeFrame/can.DecodeFrame calls itself. It is a thin wrapper, not a
// second framing implementation: every byte on the wire is exactly what
// can.EncodeFrame/can.DecodeFrame already define (Classical/FD/XL, up to
// XL's 2 KB payload — see can/types.go), and simulating a bus without real
// hardware is the native can.Endpoint's own Transport interface's job
// (can/endpoint.go), not this package's.
package canbr

//fusa:req REQ-CAN-001
//fusa:req REQ-CAN-002
//fusa:req REQ-CAN-003
//fusa:req REQ-CAN-004
//fusa:req REQ-CAN-005
//fusa:req REQ-CAN-006
//fusa:req REQ-CAN-007
//fusa:req REQ-CAN-008

import (
	"context"
	"errors"
	"fmt"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/can"
	"github.com/SoundMatt/go-RCP/v9/udp"
)

// ErrNotAResponse is returned when a request's response body does not
// decode as a can.Frame, or the response carries acf.FlagError.
var ErrNotAResponse = errors.New("rcp/canbr: response is not a valid CAN frame")

// Controller is a can.Frame-typed ergonomics wrapper around a *udp.Controller
// already addressing a declared can.Endpoint (see can.EndpointType and
// server.Server.AddEndpoint). Every method here is a thin call-through to
// the wrapped Controller's own Request/Read/Write, framed and parsed with
// can.EncodeFrame/can.DecodeFrame — this package owns no addressing,
// correlation, or transport logic of its own.
type Controller struct {
	inner *udp.Controller
	addr  avtp.ByteBusID
}

// NewController returns a Controller presenting a CAN-frame-shaped API for
// the declared CAN endpoint addr on inner.
func NewController(inner *udp.Controller, addr avtp.ByteBusID) *Controller {
	return &Controller{inner: inner, addr: addr}
}

// StreamID returns the wrapped Controller's own avtp.StreamID identity.
func (c *Controller) StreamID() avtp.StreamID { return c.inner.StreamID() }

// Send transmits f (validated first — see can.Frame.Validate) as a write
// request to the wrapped CAN endpoint, and decodes the response body as the
// echoed can.Frame (see can.Endpoint.HandleRequest's write-request
// contract).
func (c *Controller) Send(ctx context.Context, f can.Frame) (can.Frame, error) {
	if err := f.Validate(); err != nil {
		return can.Frame{}, fmt.Errorf("rcp/canbr: %w", err)
	}
	resp, err := c.inner.Write(ctx, c.addr, can.EncodeFrame(f))
	if err != nil {
		return can.Frame{}, err
	}
	return decodeResponse(resp)
}

// Receive issues a read request to the wrapped CAN endpoint and decodes the
// response body as the most recently received can.Frame (see
// can.Endpoint.HandleRequest's read-request contract, and
// can.Endpoint.SetReceivedFrame for how that frame is populated).
func (c *Controller) Receive(ctx context.Context) (can.Frame, error) {
	resp, err := c.inner.Read(ctx, c.addr)
	if err != nil {
		return can.Frame{}, err
	}
	return decodeResponse(resp)
}

// decodeResponse parses resp.Body as a can.Frame, reporting ErrNotAResponse
// for a wire-level error response or an undecodable body.
func decodeResponse(resp acf.Message) (can.Frame, error) {
	if resp.Control.Has(acf.FlagError) {
		return can.Frame{}, fmt.Errorf("rcp/canbr: %w: %s", ErrNotAResponse, resp.Body)
	}
	f, err := can.DecodeFrame(resp.Body)
	if err != nil {
		return can.Frame{}, fmt.Errorf("rcp/canbr: %w: %v", ErrNotAResponse, err)
	}
	return f, nil
}

// Close closes the wrapped Controller. Safe to call multiple times (see
// udp.Controller.Close).
func (c *Controller) Close() error { return c.inner.Close() }
