package observe

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/udp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Controller is the minimal surface this package needs from an underlying
// RCP client to instrument its calls — the same Request/StreamID/Close
// shape *udp.Controller (production) and *mock.Client (unit tests) already
// share (see capi.Controller for the identical local-interface pattern;
// each package independently defines this narrow shape rather than
// depending on a shared root-module interface, since the root module's own
// Controller interface is a Phase 18 concern this milestone does not
// resolve).
type Controller interface {
	StreamID() avtp.StreamID
	Request(ctx context.Context, addr avtp.ByteBusID, control acf.ControlFlags, body []byte) (acf.Message, error)
	Close() error
}

// Metrics is the Prometheus-compatible hook point.
// Implementations should be safe for concurrent use.
type Metrics interface {
	// ObserveRequestLatency records the round-trip time in milliseconds for
	// a request addressed to (stream, addr).
	ObserveRequestLatency(stream avtp.StreamID, addr avtp.ByteBusID, ms float64)
	// IncRequestError increments the error counter for (stream, addr).
	IncRequestError(stream avtp.StreamID, addr avtp.ByteBusID)
	// SetEndpointHealth updates the endpoint health gauge for (stream, addr)
	// (true -> 1, false -> 0).
	SetEndpointHealth(stream avtp.StreamID, addr avtp.ByteBusID, healthy bool)
	// IncDeadlineMiss increments the deadline-miss counter for (stream, addr).
	IncDeadlineMiss(stream avtp.StreamID, addr avtp.ByteBusID)
}

// Config configures the observing Controller.
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

// Controller wraps an underlying Controller to emit OTel spans and metrics
// on every Request call. It implements Controller itself, so it composes
// with any other wrapper sharing this package's local interface shape
// (e.g. record.Handler wraps request.Handler on the server side; this
// wraps the client side).
type observingController struct {
	inner   Controller
	tracer  trace.Tracer
	metrics Metrics
	closed  atomic.Bool
}

// New wraps inner with observability. cfg.Tracer may be nil (falls back to
// the global tracer).
func New(inner Controller, cfg Config) Controller {
	t := cfg.Tracer
	if t == nil {
		t = otel.Tracer("github.com/SoundMatt/go-RCP/observe")
	}
	return &observingController{inner: inner, tracer: t, metrics: cfg.Metrics}
}

// StreamID delegates to the inner Controller.
func (c *observingController) StreamID() avtp.StreamID { return c.inner.StreamID() }

// Request dispatches through the inner Controller, recording an OTel span
// and updating Prometheus metrics on every call.
func (c *observingController) Request(ctx context.Context, addr avtp.ByteBusID, control acf.ControlFlags, body []byte) (acf.Message, error) {
	if c.closed.Load() {
		return acf.Message{}, fmt.Errorf("rcp/observe: %w", udp.ErrClosed)
	}

	stream := c.inner.StreamID()
	ctx, span := c.tracer.Start(ctx, "rcp.Request",
		trace.WithAttributes(
			attribute.String("rcp.stream_id", stream.String()),
			attribute.Int("rcp.byte_bus_id", int(addr)),
			attribute.Int("rcp.control", int(control)),
		))
	defer span.End()

	start := time.Now()
	resp, err := c.inner.Request(ctx, addr, control, body)
	ms := float64(time.Since(start).Microseconds()) / 1000.0

	if err != nil {
		span.SetStatus(otelcodes.Error, err.Error())
		if c.metrics != nil {
			c.metrics.IncRequestError(stream, addr)
			if errors.Is(err, udp.ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
				c.metrics.IncDeadlineMiss(stream, addr)
			}
		}
		return acf.Message{}, err
	}

	span.SetStatus(otelcodes.Ok, "")
	if c.metrics != nil {
		c.metrics.ObserveRequestLatency(stream, addr, ms)
		c.metrics.SetEndpointHealth(stream, addr, !resp.Control.Has(acf.FlagError))
	}
	return resp, nil
}

// Close closes the inner Controller. Safe to call multiple times.
func (c *observingController) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	return c.inner.Close()
}
