// Package proxy provides a transparent zone proxy for multi-hop zonal topologies.
//
// A Proxy wraps an upstream rcp.Controller and presents the same rcp.Controller
// interface to downstream callers. Commands are forwarded verbatim to the
// upstream; responses and Status events are relayed back unchanged.
//
// The proxy adds one configurable behaviour: an optional ForwardTransform that
// can inspect or rewrite a Command before forwarding (e.g. to add a routing
// header or translate a zone alias). A nil transform is a no-op.
//
// Use cases:
//   - HPC-A forwards zone commands to HPC-B's controller over a secondary link
//   - A diagnostic gateway proxies commands between an external tool and a zone
//   - A test harness intercepts and optionally transforms commands in-process
//
// Composable with authz, ratelimit, prioqueue, and faultinject wrappers on
// either the upstream or the proxy itself.
package proxy

//fusa:req REQ-PX-001
//fusa:req REQ-PX-002
//fusa:req REQ-PX-003
//fusa:req REQ-PX-004
//fusa:req REQ-PX-005
//fusa:req REQ-PX-006
//fusa:req REQ-PX-007
//fusa:req REQ-PX-008

import (
	"context"
	"fmt"
	"sync/atomic"

	rcp "github.com/SoundMatt/go-RCP"
)

// TransformFunc may inspect or rewrite a Command before it is forwarded.
// It receives the original command and must return either a (possibly new)
// command to forward, or an error to abort the send.
// A nil TransformFunc is equivalent to the identity transform.
type TransformFunc func(cmd *rcp.Command) (*rcp.Command, error)

// Controller is a transparent proxy in front of an upstream rcp.Controller.
type Controller struct {
	upstream  rcp.Controller
	transform TransformFunc
	closed    atomic.Bool
}

// NewController creates a Proxy in front of upstream.
// If transform is nil the command is forwarded unchanged.
func NewController(upstream rcp.Controller, transform TransformFunc) *Controller {
	return &Controller{upstream: upstream, transform: transform}
}

// Send optionally transforms cmd, then forwards to the upstream controller.
// Returns ErrClosed if the proxy has been closed.
func (c *Controller) Send(ctx context.Context, cmd *rcp.Command) (*rcp.Response, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("rcp/proxy: zone %s: %w", c.upstream.Zone(), rcp.ErrClosed)
	}
	out := cmd
	if c.transform != nil {
		var err error
		out, err = c.transform(cmd)
		if err != nil {
			return nil, fmt.Errorf("rcp/proxy: zone %s: transform: %w", c.upstream.Zone(), err)
		}
	}
	return c.upstream.Send(ctx, out)
}

// Zone delegates to the upstream controller.
func (c *Controller) Zone() rcp.Zone { return c.upstream.Zone() }

// Subscribe delegates to the upstream controller.
func (c *Controller) Subscribe(ctx context.Context) (<-chan *rcp.Status, error) {
	return c.upstream.Subscribe(ctx)
}

// Close closes the upstream controller. Safe to call multiple times.
func (c *Controller) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	return c.upstream.Close()
}
