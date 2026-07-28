// Package grpcbridge provides a gRPC transport bridge for go-RCP, for the
// OPEN Alliance TC18 Remote Control Protocol (RCP), as described by the
// "OPEN Alliance TC18 Remote Control Protocol Specification v0.5.1_RC".
//
// This is ROADMAP.md Milestone 56 (v0.69.0)'s ADAPT-flagged rebuild: per
// Phase 17's disposition table, remote/cloud RPC access to an RC Server is
// orthogonal to TC18 RCP and stays genuinely necessary, re-pointed at the
// new Controller-equivalent interface — *udp.Controller — the same
// "wrap the concrete transport type directly, since the caller/inner
// interface contract is a Phase 18 root-module concern" precedent
// Milestone 54's udp package established (see e.g. authz.Controller,
// federation.Registry).
//
// Server wraps an upstream *udp.Controller and exposes it over gRPC:
// Request forwards one plain request/response, and PublishTelemetry/
// Subscribe fan out caller-supplied endpoint telemetry to connected gRPC
// clients — TC18 has no native server-push broadcast (Phase 13's "no
// server-side safety net" framing), so this mirrors the same
// caller-driven publication posture ddsbr/mqttbr adopt this same
// milestone (see ddsbr's own package doc comment for the fuller
// rationale). Controller is the reciprocal client-side stub: it presents
// the same Request/Read/Write/Close surface a *udp.Controller does, but
// reaches a remote grpcbridge.Server over a gRPC connection instead of
// dialing an RC Server directly.
//
// The bridge uses a JSON codec (no protoc compilation required). The service
// name is "rcp.Bridge" and the content-type is "application/grpc+json".
package grpcbridge

//fusa:req REQ-GRPC-001
//fusa:req REQ-GRPC-002
//fusa:req REQ-GRPC-003
//fusa:req REQ-GRPC-004
//fusa:req REQ-GRPC-005
//fusa:req REQ-GRPC-006
//fusa:req REQ-GRPC-007
//fusa:req REQ-GRPC-008

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/udp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding"
)

func init() {
	// Register JSON as the "proto" codec so gRPC uses it by default.
	// This means the bridge works without any .proto compilation step.
	encoding.RegisterCodec(jsonCodec{})
}

// jsonCodec serialises gRPC messages as JSON.
type jsonCodec struct{}

func (jsonCodec) Name() string { return "proto" }

func (jsonCodec) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (jsonCodec) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// ErrClosed is returned by Controller methods once Close has been called.
var ErrClosed = errors.New("rcp/grpcbridge: closed")

// ─── wire types ──────────────────────────────────────────────────────────────

// RequestMsg is the inbound message for the Request RPC.
type RequestMsg struct {
	ByteBusID avtp.ByteBusID   `json:"byte_bus_id"`
	Control   acf.ControlFlags `json:"control"`
	Body      []byte           `json:"body,omitempty"`
}

// ResponseMsg is the outbound message from the Request RPC.
type ResponseMsg struct {
	ByteBusID      avtp.ByteBusID      `json:"byte_bus_id"`
	TransactionNum avtp.TransactionNum `json:"transaction_num"`
	Control        acf.ControlFlags    `json:"control"`
	Body           []byte              `json:"body,omitempty"`
}

// TelemetryEvent is streamed by the Subscribe RPC (see Server.PublishTelemetry).
type TelemetryEvent struct {
	ByteBusID avtp.ByteBusID   `json:"byte_bus_id"`
	Control   acf.ControlFlags `json:"control"`
	Body      []byte           `json:"body,omitempty"`
}

// SubscribeRequest starts a Subscribe stream.
type SubscribeRequest struct{}

// ─── service descriptor ───────────────────────────────────────────────────────

// BridgeServer is the gRPC server-side interface.
type BridgeServer interface {
	Request(context.Context, *RequestMsg) (*ResponseMsg, error)
	Subscribe(*SubscribeRequest, grpc.ServerStream) error
}

var bridgeServiceDesc = grpc.ServiceDesc{
	ServiceName: "rcp.Bridge",
	HandlerType: (*BridgeServer)(nil),
	Methods: []grpc.MethodDesc{
		{MethodName: "Request", Handler: requestHandler},
	},
	Streams: []grpc.StreamDesc{
		{StreamName: "Subscribe", Handler: subscribeHandler, ServerStreams: true},
	},
}

func requestHandler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	req := new(RequestMsg)
	if err := dec(req); err != nil {
		return nil, err
	}
	bs := srv.(BridgeServer) //nolint:errcheck
	if interceptor == nil {
		return bs.Request(ctx, req)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/rcp.Bridge/Request"}
	handler := func(ctx context.Context, req any) (any, error) {
		return bs.Request(ctx, req.(*RequestMsg)) //nolint:errcheck
	}
	return interceptor(ctx, req, info, handler)
}

func subscribeHandler(srv any, stream grpc.ServerStream) error {
	req := new(SubscribeRequest)
	if err := stream.RecvMsg(req); err != nil {
		return err
	}
	return srv.(BridgeServer).Subscribe(req, stream) //nolint:errcheck
}

// ─── Server ───────────────────────────────────────────────────────────────────

// Server bridges a gRPC connection to an upstream *udp.Controller.
// Register it with grpc.Server using RegisterServer.
type Server struct {
	upstream *udp.Controller

	mu   sync.Mutex
	subs map[chan *TelemetryEvent]struct{}
}

// NewServer returns a Server forwarding Request calls to upstream.
func NewServer(upstream *udp.Controller) *Server {
	return &Server{upstream: upstream, subs: make(map[chan *TelemetryEvent]struct{})}
}

// RegisterServer registers s on gs so gRPC clients can call it.
func RegisterServer(gs *grpc.Server, s *Server) {
	gs.RegisterService(&bridgeServiceDesc, s)
}

// Request implements BridgeServer — forwards to the upstream controller.
func (s *Server) Request(ctx context.Context, req *RequestMsg) (*ResponseMsg, error) {
	resp, err := s.upstream.Request(ctx, req.ByteBusID, req.Control, req.Body)
	if err != nil {
		return nil, err
	}
	return &ResponseMsg{
		ByteBusID:      resp.ByteBusID,
		TransactionNum: resp.TransactionNum,
		Control:        resp.Control,
		Body:           resp.Body,
	}, nil
}

// PublishTelemetry fans ev out to every currently connected Subscribe
// stream. A caller obtains ev however it likes — a direct Read/Write call
// against upstream, a request.Dispatcher poll result, or any other
// source — mirroring ddsbr.Bridge.PublishResponse's own caller-driven
// posture.
func (s *Server) PublishTelemetry(ev *TelemetryEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for ch := range s.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

// Subscribe implements BridgeServer — streams TelemetryEvents published via
// PublishTelemetry until stream's context is cancelled.
func (s *Server) Subscribe(_ *SubscribeRequest, stream grpc.ServerStream) error {
	ch := make(chan *TelemetryEvent, 16)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.subs, ch)
		s.mu.Unlock()
	}()

	for {
		select {
		case ev := <-ch:
			if err := stream.SendMsg(ev); err != nil {
				return err
			}
		case <-stream.Context().Done():
			return stream.Context().Err()
		}
	}
}

// ─── Client Controller ────────────────────────────────────────────────────────

// Controller reaches a remote grpcbridge.Server over a gRPC connection,
// presenting the same Request/Read/Write surface a *udp.Controller does.
type Controller struct {
	cc     *grpc.ClientConn
	closed atomic.Bool
}

// NewController dials serverAddr. The connection uses insecure credentials;
// wrap with TLS for production.
func NewController(_ context.Context, serverAddr string) (*Controller, error) {
	cc, err := grpc.NewClient(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("rcp/grpcbridge: dial %s: %w", serverAddr, err)
	}
	return &Controller{cc: cc}, nil
}

// Request sends one request to addr and blocks for the response.
func (c *Controller) Request(ctx context.Context, addr avtp.ByteBusID, control acf.ControlFlags, body []byte) (acf.Message, error) {
	if c.closed.Load() {
		return acf.Message{}, fmt.Errorf("rcp/grpcbridge: %w", ErrClosed)
	}
	req := &RequestMsg{ByteBusID: addr, Control: control, Body: body}
	var resp ResponseMsg
	if err := c.cc.Invoke(ctx, "/rcp.Bridge/Request", req, &resp); err != nil {
		return acf.Message{}, fmt.Errorf("rcp/grpcbridge: Request: %w", err)
	}
	return acf.Message{
		ByteBusID:      resp.ByteBusID,
		TransactionNum: resp.TransactionNum,
		Control:        resp.Control,
		Body:           resp.Body,
	}, nil
}

// Read is Request with acf.FlagRead set and no body.
func (c *Controller) Read(ctx context.Context, addr avtp.ByteBusID) (acf.Message, error) {
	return c.Request(ctx, addr, acf.FlagRead, nil)
}

// Write is Request with acf.FlagWrite set and the given body.
func (c *Controller) Write(ctx context.Context, addr avtp.ByteBusID, body []byte) (acf.Message, error) {
	return c.Request(ctx, addr, acf.FlagWrite, body)
}

// Subscribe opens a gRPC Subscribe stream and returns a channel of
// TelemetryEvents published by the remote Server's PublishTelemetry.
func (c *Controller) Subscribe(ctx context.Context) (<-chan *TelemetryEvent, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("rcp/grpcbridge: %w", ErrClosed)
	}
	desc := &grpc.StreamDesc{ServerStreams: true}
	stream, err := c.cc.NewStream(ctx, desc, "/rcp.Bridge/Subscribe")
	if err != nil {
		return nil, fmt.Errorf("rcp/grpcbridge: Subscribe: %w", err)
	}
	if err := stream.SendMsg(&SubscribeRequest{}); err != nil {
		return nil, fmt.Errorf("rcp/grpcbridge: Subscribe send: %w", err)
	}
	if err := stream.CloseSend(); err != nil {
		return nil, fmt.Errorf("rcp/grpcbridge: Subscribe close-send: %w", err)
	}
	ch := make(chan *TelemetryEvent, 16)
	go func() {
		defer close(ch)
		for {
			var ev TelemetryEvent
			if err := stream.RecvMsg(&ev); err != nil {
				return
			}
			select {
			case ch <- &ev:
			default:
			}
		}
	}()
	return ch, nil
}

// Close closes the gRPC connection. Idempotent.
func (c *Controller) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	return c.cc.Close()
}
