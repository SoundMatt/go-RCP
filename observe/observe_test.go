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
	"sync"
	"testing"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/observe"
)

func testStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55}, 1)
}

type stubController struct {
	stream avtp.StreamID
	resp   acf.Message
	err    error
	closed bool

	mu       sync.Mutex
	lastAddr avtp.ByteBusID
	calls    int
}

func (s *stubController) StreamID() avtp.StreamID { return s.stream }

func (s *stubController) Request(_ context.Context, addr avtp.ByteBusID, _ acf.ControlFlags, _ []byte) (acf.Message, error) {
	s.mu.Lock()
	s.lastAddr = addr
	s.calls++
	s.mu.Unlock()
	if s.err != nil {
		return acf.Message{}, s.err
	}
	return s.resp, nil
}

func (s *stubController) Close() error {
	s.closed = true
	return nil
}

type fakeMetrics struct {
	mu             sync.Mutex
	latencies      int
	errors         int
	deadlineMisses int
	lastHealthy    bool
	healthSetCount int
}

func (m *fakeMetrics) ObserveRequestLatency(avtp.StreamID, avtp.ByteBusID, float64) {
	m.mu.Lock()
	m.latencies++
	m.mu.Unlock()
}
func (m *fakeMetrics) IncRequestError(avtp.StreamID, avtp.ByteBusID) {
	m.mu.Lock()
	m.errors++
	m.mu.Unlock()
}
func (m *fakeMetrics) SetEndpointHealth(_ avtp.StreamID, _ avtp.ByteBusID, healthy bool) {
	m.mu.Lock()
	m.lastHealthy = healthy
	m.healthSetCount++
	m.mu.Unlock()
}
func (m *fakeMetrics) IncDeadlineMiss(avtp.StreamID, avtp.ByteBusID) {
	m.mu.Lock()
	m.deadlineMisses++
	m.mu.Unlock()
}

// TestController_StreamID_Delegates verifies StreamID passes through to the
// inner Controller (REQ-OB-001).
func TestController_StreamID_Delegates(t *testing.T) {
	stub := &stubController{stream: testStream()}
	c := observe.New(stub, observe.DefaultConfig())
	if c.StreamID() != testStream() {
		t.Errorf("StreamID() = %v, want %v", c.StreamID(), testStream())
	}
}

// TestController_Request_DelegatesAndReturnsResponse verifies Request
// reaches the inner Controller and returns its response (REQ-OB-002).
func TestController_Request_DelegatesAndReturnsResponse(t *testing.T) {
	stub := &stubController{stream: testStream(), resp: acf.Message{Body: []byte("ok")}}
	c := observe.New(stub, observe.DefaultConfig())
	resp, err := c.Request(context.Background(), 3, acf.FlagRead, nil)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if string(resp.Body) != "ok" {
		t.Errorf("resp.Body = %q, want ok", resp.Body)
	}
	if stub.lastAddr != 3 {
		t.Errorf("lastAddr = %d, want 3", stub.lastAddr)
	}
}

// TestController_Request_Success_RecordsLatencyAndHealth verifies a
// successful Request records latency and sets health true when the
// response carries no FlagError (REQ-OB-003).
func TestController_Request_Success_RecordsLatencyAndHealth(t *testing.T) {
	stub := &stubController{stream: testStream(), resp: acf.Message{Control: acf.FlagResponse}}
	m := &fakeMetrics{}
	c := observe.New(stub, observe.Config{Metrics: m})
	if _, err := c.Request(context.Background(), 1, acf.FlagRead, nil); err != nil {
		t.Fatalf("Request: %v", err)
	}
	if m.latencies != 1 {
		t.Errorf("latencies = %d, want 1", m.latencies)
	}
	if m.healthSetCount != 1 || !m.lastHealthy {
		t.Errorf("health not set healthy: count=%d healthy=%v", m.healthSetCount, m.lastHealthy)
	}
}

// TestController_Request_ErrorFlagResponse_SetsUnhealthy verifies a
// successful Request whose response carries FlagError still counts as
// unhealthy (REQ-OB-004).
func TestController_Request_ErrorFlagResponse_SetsUnhealthy(t *testing.T) {
	stub := &stubController{stream: testStream(), resp: acf.Message{Control: acf.FlagResponse | acf.FlagError}}
	m := &fakeMetrics{}
	c := observe.New(stub, observe.Config{Metrics: m})
	if _, err := c.Request(context.Background(), 1, acf.FlagRead, nil); err != nil {
		t.Fatalf("Request: %v", err)
	}
	if m.lastHealthy {
		t.Error("expected unhealthy for a FlagError response")
	}
}

// TestController_Request_Error_IncrementsErrorCounter verifies a failing
// Request increments IncRequestError and does not record latency/health
// (REQ-OB-005).
func TestController_Request_Error_IncrementsErrorCounter(t *testing.T) {
	stub := &stubController{stream: testStream(), err: errors.New("boom")}
	m := &fakeMetrics{}
	c := observe.New(stub, observe.Config{Metrics: m})
	if _, err := c.Request(context.Background(), 1, acf.FlagRead, nil); err == nil {
		t.Fatal("expected error")
	}
	if m.errors != 1 {
		t.Errorf("errors = %d, want 1", m.errors)
	}
	if m.latencies != 0 || m.healthSetCount != 0 {
		t.Errorf("latencies=%d healthSetCount=%d, want 0 and 0", m.latencies, m.healthSetCount)
	}
}

// TestController_Request_TimeoutError_IncrementsDeadlineMiss verifies a
// udp.ErrTimeout-wrapping error also increments IncDeadlineMiss
// (REQ-OB-006).
func TestController_Request_TimeoutError_IncrementsDeadlineMiss(t *testing.T) {
	stub := &stubController{stream: testStream(), err: context.DeadlineExceeded}
	m := &fakeMetrics{}
	c := observe.New(stub, observe.Config{Metrics: m})
	if _, err := c.Request(context.Background(), 1, acf.FlagRead, nil); err == nil {
		t.Fatal("expected error")
	}
	if m.deadlineMisses != 1 {
		t.Errorf("deadlineMisses = %d, want 1", m.deadlineMisses)
	}
}

// TestController_Close_IdempotentAndRejectsRequest verifies Close is safe
// to call multiple times and closes the inner Controller (REQ-OB-007).
func TestController_Close_IdempotentAndRejectsRequest(t *testing.T) {
	stub := &stubController{stream: testStream()}
	c := observe.New(stub, observe.DefaultConfig())
	if err := c.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	if !stub.closed {
		t.Error("inner Controller was not closed")
	}
	if _, err := c.Request(context.Background(), 1, acf.FlagRead, nil); err == nil {
		t.Error("expected error requesting through a closed Controller")
	}
}

// TestController_Request_Concurrent verifies concurrent Request calls are
// data-race free (REQ-OB-008).
func TestController_Request_Concurrent(t *testing.T) {
	stub := &stubController{stream: testStream(), resp: acf.Message{Control: acf.FlagResponse}}
	m := &fakeMetrics{}
	c := observe.New(stub, observe.Config{Metrics: m})
	var wg sync.WaitGroup
	for range 30 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.Request(context.Background(), 1, acf.FlagRead, nil)
		}()
	}
	wg.Wait()
}
