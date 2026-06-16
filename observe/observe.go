//fusa:req REQ-OB-001
//fusa:req REQ-OB-002
//fusa:req REQ-OB-003
//fusa:req REQ-OB-004
//fusa:req REQ-OB-005
//fusa:req REQ-OB-006
//fusa:req REQ-OB-007
//fusa:req REQ-OB-008

// Package observe wraps an rcp.Controller with OpenTelemetry tracing and a
// pluggable metrics hook for Prometheus-compatible instrumentation.
package observe

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Metrics is the Prometheus-compatible hook point.
// Implementations should be safe for concurrent use.
type Metrics interface {
	// ObserveSendLatency records the round-trip time in milliseconds.
	ObserveSendLatency(zone rcp.Zone, ms float64)
	// IncSendError increments the error counter for the zone.
	IncSendError(zone rcp.Zone)
	// SetZoneHealth updates the zone health gauge (true → 1, false → 0).
	SetZoneHealth(zone rcp.Zone, healthy bool)
	// IncDeadlineMiss increments the deadline-miss counter for the zone.
	IncDeadlineMiss(zone rcp.Zone)
}

// Config configures the observing controller.
type Config struct {
	// Tracer is the OTel tracer to use. Defaults to the global tracer.
	Tracer trace.Tracer
	// Metrics hook for Prometheus-style instrumentation. May be nil.
	Metrics Metrics
}

// DefaultConfig returns a Config that uses the global OTel tracer and no metrics hook.
func DefaultConfig() Config {
	return Config{}
}

// Controller wraps an rcp.Controller to emit OTel spans and metrics.
type Controller struct {
	inner   rcp.Controller
	tracer  trace.Tracer
	metrics Metrics
	closed  atomic.Bool
}

// New wraps inner with observability. cfg.Tracer may be nil (falls back to global).
func New(inner rcp.Controller, cfg Config) *Controller {
	t := cfg.Tracer
	if t == nil {
		t = otel.Tracer("github.com/SoundMatt/go-RCP/observe")
	}
	return &Controller{inner: inner, tracer: t, metrics: cfg.Metrics}
}

// Zone implements rcp.Controller.
func (c *Controller) Zone() rcp.Zone { return c.inner.Zone() }

// Send dispatches the command via the inner controller, recording an OTel span
// and updating Prometheus metrics on every call.
func (c *Controller) Send(ctx context.Context, cmd *rcp.Command) (*rcp.Response, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("rcp/observe: %w", rcp.ErrClosed)
	}

	ctx, span := c.tracer.Start(ctx, "rcp.Send",
		trace.WithAttributes(
			attribute.String("rcp.zone", cmd.Zone.String()),
			attribute.Int("rcp.cmd_type", int(cmd.Type)),
			attribute.Int("rcp.priority", int(cmd.Priority)),
			attribute.Int("rcp.cmd_id", int(cmd.ID)),
		))
	defer span.End()

	start := time.Now()
	resp, err := c.inner.Send(ctx, cmd)
	ms := float64(time.Since(start).Microseconds()) / 1000.0

	if err != nil {
		span.SetStatus(otelcodes.Error, err.Error())
		if c.metrics != nil {
			c.metrics.IncSendError(cmd.Zone)
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, rcp.ErrTimeout) {
				c.metrics.IncDeadlineMiss(cmd.Zone)
			}
		}
		return nil, err
	}

	span.SetStatus(otelcodes.Ok, "")
	if c.metrics != nil {
		c.metrics.ObserveSendLatency(cmd.Zone, ms)
		c.metrics.SetZoneHealth(cmd.Zone, resp.Status == rcp.StatusOK)
	}
	return resp, nil
}

// Subscribe passes through to the inner controller with an OTel span covering
// the subscription setup phase.
func (c *Controller) Subscribe(ctx context.Context) (<-chan *rcp.Status, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("rcp/observe: %w", rcp.ErrClosed)
	}

	ctx, span := c.tracer.Start(ctx, "rcp.Subscribe",
		trace.WithAttributes(
			attribute.String("rcp.zone", c.inner.Zone().String()),
		))
	defer span.End()

	ch, err := c.inner.Subscribe(ctx)
	if err != nil {
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	span.SetStatus(otelcodes.Ok, "")
	return ch, nil
}

// Close closes the inner controller. Safe to call multiple times.
func (c *Controller) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	return c.inner.Close()
}
