//fusa:req REQ-GRPC-001
//fusa:req REQ-GRPC-002
//fusa:req REQ-GRPC-003
//fusa:req REQ-GRPC-004
//fusa:req REQ-GRPC-005
//fusa:req REQ-GRPC-006
//fusa:req REQ-GRPC-007
//fusa:req REQ-GRPC-008

// Package grpcbridge provides a gRPC transport bridge for go-RCP.
//
// Server exposes an rcp.Controller over gRPC so cloud tooling can send
// commands and subscribe to status updates from a remote zone controller.
// Controller implements rcp.Controller over a gRPC client connection,
// allowing the HPC to reach zone controllers across a WAN link.
//
// The bridge uses a JSON codec (no protoc compilation required). The service
// name is "rcp.Bridge" and the content-type is "application/grpc+json".
package grpcbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	rcp "github.com/SoundMatt/go-RCP"
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

// ─── wire types ──────────────────────────────────────────────────────────────

// SendRequest is the inbound message for the Send RPC.
type SendRequest struct {
	Zone     rcp.Zone        `json:"zone"`
	Type     rcp.CommandType `json:"type"`
	Priority rcp.Priority    `json:"priority"`
	Payload  []byte          `json:"payload,omitempty"`
}

// SendResponse is the outbound message from the Send RPC.
type SendResponse struct {
	CommandID uint32             `json:"command_id"`
	Zone      rcp.Zone           `json:"zone"`
	Status    rcp.ResponseStatus `json:"status"`
	Payload   []byte             `json:"payload,omitempty"`
}

// StatusEvent is streamed by the Subscribe RPC.
type StatusEvent struct {
	Zone    rcp.Zone `json:"zone"`
	Seq     uint32   `json:"seq"`
	Payload []byte   `json:"payload,omitempty"`
}

// SubscribeRequest starts a Subscribe stream.
type SubscribeRequest struct{}

// ─── service descriptor ───────────────────────────────────────────────────────

// BridgeServer is the gRPC server-side interface.
type BridgeServer interface {
	Send(context.Context, *SendRequest) (*SendResponse, error)
	Subscribe(*SubscribeRequest, grpc.ServerStream) error
}

var bridgeServiceDesc = grpc.ServiceDesc{
	ServiceName: "rcp.Bridge",
	HandlerType: (*BridgeServer)(nil),
	Methods: []grpc.MethodDesc{
		{MethodName: "Send", Handler: sendHandler},
	},
	Streams: []grpc.StreamDesc{
		{StreamName: "Subscribe", Handler: subscribeHandler, ServerStreams: true},
	},
}

func sendHandler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	req := new(SendRequest)
	if err := dec(req); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(BridgeServer).Send(ctx, req)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/rcp.Bridge/Send"}
	handler := func(ctx context.Context, req any) (any, error) {
		return srv.(BridgeServer).Send(ctx, req.(*SendRequest))
	}
	return interceptor(ctx, req, info, handler)
}

func subscribeHandler(srv any, stream grpc.ServerStream) error {
	req := new(SubscribeRequest)
	if err := stream.RecvMsg(req); err != nil {
		return err
	}
	return srv.(BridgeServer).Subscribe(req, stream)
}

// ─── Server ───────────────────────────────────────────────────────────────────

// Server bridges a gRPC connection to an rcp.Controller.
// Register it with grpc.Server using RegisterServer.
type Server struct {
	ctrl rcp.Controller
}

// NewServer returns a Server wrapping ctrl.
func NewServer(ctrl rcp.Controller) *Server { return &Server{ctrl: ctrl} }

// RegisterServer registers s on gs so gRPC clients can call it.
func RegisterServer(gs *grpc.Server, s *Server) {
	gs.RegisterService(&bridgeServiceDesc, s)
}

// Send implements BridgeServer — delegates to the inner rcp.Controller.
func (s *Server) Send(ctx context.Context, req *SendRequest) (*SendResponse, error) {
	cmd := &rcp.Command{
		Zone:     req.Zone,
		Type:     req.Type,
		Priority: req.Priority,
		Payload:  req.Payload,
	}
	resp, err := s.ctrl.Send(ctx, cmd)
	if err != nil {
		return nil, err
	}
	return &SendResponse{
		CommandID: resp.CommandID,
		Zone:      resp.Zone,
		Status:    resp.Status,
		Payload:   resp.Payload,
	}, nil
}

// Subscribe implements BridgeServer — streams Status updates from the inner controller.
func (s *Server) Subscribe(req *SubscribeRequest, stream grpc.ServerStream) error {
	ch, err := s.ctrl.Subscribe(stream.Context())
	if err != nil {
		return err
	}
	for {
		select {
		case st, ok := <-ch:
			if !ok {
				return nil
			}
			if err := stream.SendMsg(&StatusEvent{
				Zone:    st.Zone,
				Seq:     st.Seq,
				Payload: st.Payload,
			}); err != nil {
				return err
			}
		case <-stream.Context().Done():
			return stream.Context().Err()
		}
	}
}

// ─── Client Controller ────────────────────────────────────────────────────────

// Controller implements rcp.Controller over a gRPC client connection.
type Controller struct {
	zone   rcp.Zone
	cc     *grpc.ClientConn
	nextID atomic.Uint32
	closed atomic.Bool

	mu   sync.Mutex
	subs []chan *rcp.Status
}

// NewController dials serverAddr and returns an rcp.Controller for zone.
// The connection uses insecure credentials; wrap with TLS for production.
func NewController(ctx context.Context, zone rcp.Zone, serverAddr string) (*Controller, error) {
	cc, err := grpc.NewClient(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("rcp/grpcbridge: dial %s: %w", serverAddr, err)
	}
	return &Controller{zone: zone, cc: cc}, nil
}

// Zone implements rcp.Controller.
func (c *Controller) Zone() rcp.Zone { return c.zone }

// Send implements rcp.Controller — encodes cmd as a gRPC Send call.
func (c *Controller) Send(ctx context.Context, cmd *rcp.Command) (*rcp.Response, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("rcp/grpcbridge: zone %s: %w", c.zone, rcp.ErrClosed)
	}
	if cmd.Zone != c.zone {
		return nil, fmt.Errorf("rcp/grpcbridge: zone %s: %w", c.zone, rcp.ErrZoneMismatch)
	}
	req := &SendRequest{
		Zone:     cmd.Zone,
		Type:     cmd.Type,
		Priority: cmd.Priority,
		Payload:  cmd.Payload,
	}
	var resp SendResponse
	err := c.cc.Invoke(ctx, "/rcp.Bridge/Send", req, &resp)
	if err != nil {
		return nil, fmt.Errorf("rcp/grpcbridge: Send: %w", err)
	}
	return &rcp.Response{
		CommandID: resp.CommandID,
		Zone:      resp.Zone,
		Status:    resp.Status,
		Payload:   resp.Payload,
	}, nil
}

// Subscribe implements rcp.Controller — opens a gRPC Subscribe stream.
func (c *Controller) Subscribe(ctx context.Context) (<-chan *rcp.Status, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("rcp/grpcbridge: zone %s: %w", c.zone, rcp.ErrClosed)
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
	ch := make(chan *rcp.Status, 16)
	go func() {
		defer close(ch)
		for {
			var ev StatusEvent
			if err := stream.RecvMsg(&ev); err != nil {
				return
			}
			select {
			case ch <- &rcp.Status{Zone: ev.Zone, Seq: ev.Seq, Payload: ev.Payload}:
			default:
			}
		}
	}()
	return ch, nil
}

// Close implements rcp.Controller — idempotent.
func (c *Controller) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	return c.cc.Close()
}
