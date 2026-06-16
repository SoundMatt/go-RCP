//fusa:test REQ-OB-001
//fusa:test REQ-OB-002
//fusa:test REQ-OB-003
//fusa:test REQ-OB-004
//fusa:test REQ-OB-005
//fusa:test REQ-OB-006
//fusa:test REQ-OB-007
//fusa:test REQ-OB-008

package observe_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/mock"
	"github.com/SoundMatt/go-RCP/observe"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// recordingMetrics collects calls for assertion.
type recordingMetrics struct {
	mu           sync.Mutex
	latencies    []float64
	errors       int
	healths      map[rcp.Zone]bool
	deadlineMiss int
}

func newRecording() *recordingMetrics {
	return &recordingMetrics{healths: make(map[rcp.Zone]bool)}
}

func (r *recordingMetrics) ObserveSendLatency(_ rcp.Zone, ms float64) {
	r.mu.Lock(); defer r.mu.Unlock()
	r.latencies = append(r.latencies, ms)
}
func (r *recordingMetrics) IncSendError(_ rcp.Zone) {
	r.mu.Lock(); defer r.mu.Unlock()
	r.errors++
}
func (r *recordingMetrics) SetZoneHealth(zone rcp.Zone, healthy bool) {
	r.mu.Lock(); defer r.mu.Unlock()
	r.healths[zone] = healthy
}
func (r *recordingMetrics) IncDeadlineMiss(_ rcp.Zone) {
	r.mu.Lock(); defer r.mu.Unlock()
	r.deadlineMiss++
}

// errController always returns a fixed error from Send.
type errController struct {
	zone rcp.Zone
	err  error
}

func (e *errController) Zone() rcp.Zone { return e.zone }
func (e *errController) Send(_ context.Context, _ *rcp.Command) (*rcp.Response, error) {
	return nil, e.err
}
func (e *errController) Subscribe(_ context.Context) (<-chan *rcp.Status, error) {
	return nil, e.err
}
func (e *errController) Close() error { return nil }

func newSpanRecorder() (*tracetest.SpanRecorder, trace.Tracer) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	return rec, tp.Tracer("test")
}

func sendCmd(t *testing.T, c *observe.Controller, zone rcp.Zone) (*rcp.Response, error) {
	t.Helper()
	return c.Send(context.Background(), &rcp.Command{Zone: zone, Type: rcp.CmdSet})
}

func TestSend_SpanCreated(t *testing.T) {
	rec, tr := newSpanRecorder()
	m := mock.NewController(rcp.ZoneFrontLeft, nil)
	c := observe.New(m, observe.Config{Tracer: tr})

	if _, err := sendCmd(t, c, rcp.ZoneFrontLeft); err != nil {
		t.Fatal(err)
	}

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("want 1 span, got %d", len(spans))
	}
	if spans[0].Name() != "rcp.Send" {
		t.Errorf("span name = %q, want rcp.Send", spans[0].Name())
	}
	if spans[0].Status().Code != codes.Ok {
		t.Errorf("span status = %v, want Ok", spans[0].Status().Code)
	}
}

func TestSend_SpanAttributes(t *testing.T) {
	rec, tr := newSpanRecorder()
	m := mock.NewController(rcp.ZoneFrontLeft, nil)
	c := observe.New(m, observe.Config{Tracer: tr})

	if _, err := sendCmd(t, c, rcp.ZoneFrontLeft); err != nil {
		t.Fatal(err)
	}

	span := rec.Ended()[0]
	attrs := span.Attributes()
	found := make(map[attribute.Key]bool)
	for _, a := range attrs {
		found[a.Key] = true
	}
	for _, want := range []attribute.Key{"rcp.zone", "rcp.cmd_type", "rcp.priority", "rcp.cmd_id"} {
		if !found[want] {
			t.Errorf("missing attribute %q", want)
		}
	}
}

func TestSend_MetricsLatency(t *testing.T) {
	_, tr := newSpanRecorder()
	metrics := newRecording()
	m := mock.NewController(rcp.ZoneFrontLeft, nil)
	c := observe.New(m, observe.Config{Tracer: tr, Metrics: metrics})

	if _, err := sendCmd(t, c, rcp.ZoneFrontLeft); err != nil {
		t.Fatal(err)
	}

	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	if len(metrics.latencies) != 1 {
		t.Fatalf("want 1 latency observation, got %d", len(metrics.latencies))
	}
	if metrics.latencies[0] < 0 {
		t.Errorf("latency %v < 0", metrics.latencies[0])
	}
}

func TestSend_MetricsZoneHealth(t *testing.T) {
	_, tr := newSpanRecorder()
	metrics := newRecording()
	m := mock.NewController(rcp.ZoneFrontLeft, nil)
	c := observe.New(m, observe.Config{Tracer: tr, Metrics: metrics})

	if _, err := sendCmd(t, c, rcp.ZoneFrontLeft); err != nil {
		t.Fatal(err)
	}

	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	if !metrics.healths[rcp.ZoneFrontLeft] {
		t.Error("zone health should be true after OK response")
	}
}

func TestSend_ErrorSpanAndMetrics(t *testing.T) {
	rec, tr := newSpanRecorder()
	metrics := newRecording()
	inner := &errController{zone: rcp.ZoneFrontLeft, err: fmt.Errorf("bus fault")}
	c := observe.New(inner, observe.Config{Tracer: tr, Metrics: metrics})

	_, err := c.Send(context.Background(), &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdSet})
	if err == nil {
		t.Fatal("expected error")
	}

	spans := rec.Ended()
	if spans[0].Status().Code != codes.Error {
		t.Errorf("span status = %v, want Error", spans[0].Status().Code)
	}

	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	if metrics.errors != 1 {
		t.Errorf("want 1 error, got %d", metrics.errors)
	}
	if len(metrics.latencies) != 0 {
		t.Errorf("want no latency on error, got %d", len(metrics.latencies))
	}
}

func TestSend_DeadlineMiss(t *testing.T) {
	_, tr := newSpanRecorder()
	metrics := newRecording()
	inner := &errController{zone: rcp.ZoneFrontLeft, err: context.DeadlineExceeded}
	c := observe.New(inner, observe.Config{Tracer: tr, Metrics: metrics})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdSet}) //nolint:errcheck

	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	if metrics.deadlineMiss == 0 {
		t.Error("want ≥1 deadline miss, got 0")
	}
}

func TestSubscribe_SpanCreated(t *testing.T) {
	rec, tr := newSpanRecorder()
	m := mock.NewController(rcp.ZoneFrontLeft, nil)
	c := observe.New(m, observe.Config{Tracer: tr})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if _, err := c.Subscribe(ctx); err != nil {
		t.Fatal(err)
	}

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("want 1 span, got %d", len(spans))
	}
	if spans[0].Name() != "rcp.Subscribe" {
		t.Errorf("span name = %q, want rcp.Subscribe", spans[0].Name())
	}
}

func TestClose_Idempotent(t *testing.T) {
	_, tr := newSpanRecorder()
	m := mock.NewController(rcp.ZoneFrontLeft, nil)
	c := observe.New(m, observe.Config{Tracer: tr})

	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("second Close returned error: %v", err)
	}
}

func TestClose_RejectsSend(t *testing.T) {
	_, tr := newSpanRecorder()
	m := mock.NewController(rcp.ZoneFrontLeft, nil)
	c := observe.New(m, observe.Config{Tracer: tr})
	_ = c.Close()

	_, err := sendCmd(t, c, rcp.ZoneFrontLeft)
	if !errors.Is(err, rcp.ErrClosed) {
		t.Errorf("want ErrClosed, got %v", err)
	}
}

func TestClose_RejectsSubscribe(t *testing.T) {
	_, tr := newSpanRecorder()
	m := mock.NewController(rcp.ZoneFrontLeft, nil)
	c := observe.New(m, observe.Config{Tracer: tr})
	_ = c.Close()

	_, err := c.Subscribe(context.Background())
	if !errors.Is(err, rcp.ErrClosed) {
		t.Errorf("want ErrClosed, got %v", err)
	}
}

func TestConcurrent(t *testing.T) {
	_, tr := newSpanRecorder()
	metrics := newRecording()
	m := mock.NewController(rcp.ZoneFrontLeft, nil)
	c := observe.New(m, observe.Config{Tracer: tr, Metrics: metrics})

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sendCmd(t, c, rcp.ZoneFrontLeft) //nolint:errcheck
		}()
	}
	wg.Wait()
}

func TestNilMetrics(t *testing.T) {
	_, tr := newSpanRecorder()
	m := mock.NewController(rcp.ZoneFrontLeft, nil)
	c := observe.New(m, observe.Config{Tracer: tr})

	if _, err := sendCmd(t, c, rcp.ZoneFrontLeft); err != nil {
		t.Errorf("nil Metrics should not panic: %v", err)
	}
}
