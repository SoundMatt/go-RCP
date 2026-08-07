//fusa:test REQ-PWM-004
//fusa:test REQ-PWM-005
//fusa:test REQ-PWM-006
//fusa:test REQ-PWM-007
//fusa:test REQ-PWM-008

package pwm_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/pwm"
	"github.com/SoundMatt/go-RCP/v9/regmap"
)

func writeReq(active, period uint16) acf.Message {
	return acf.Message{
		Kind:      acf.KindShort,
		ByteBusID: avtp.ByteBusID(1),
		Control:   acf.FlagWrite,
		Body:      pwm.EncodeWaveform(active, period),
	}
}

func readReq() acf.Message {
	return acf.Message{Kind: acf.KindShort, ByteBusID: avtp.ByteBusID(1), Control: acf.FlagRead}
}

// recordingOutputTransport is an OutputTransport test double that records
// every applied waveform, so tests can tell the configured Transport
// actually ran rather than the default store-for-readback behavior.
type recordingOutputTransport struct {
	active, period uint16
	calls          int
}

func (r *recordingOutputTransport) SetOutput(active, period uint16) error {
	r.active, r.period = active, period
	r.calls++
	return nil
}

// fixedInputTransport is an InputTransport test double returning a fixed
// waveform or a fixed error.
type fixedInputTransport struct {
	active, period uint16
	err            error
}

func (f *fixedInputTransport) Capture() (uint16, uint16, error) {
	return f.active, f.period, f.err
}

// TestHandleRequest_RoutesReadWriteRejectsNeither checks HandleRequest
// dispatches Write/Read for a RoleOutput endpoint, and rejects a request with
// neither flag (REQ-PWM-004).
func TestHandleRequest_RoutesReadWriteRejectsNeither(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	cfg := pwm.Config{Enabled: true, Role: pwm.RoleOutput, DefaultActiveTicks: 250, DefaultPeriodTicks: 1000}
	if err := ep.Configure(root, cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	resp, err := ep.HandleRequest(root, writeReq(1000, 2000))
	if err != nil {
		t.Fatalf("HandleRequest(write): %v", err)
	}
	active, period, err := pwm.DecodeWaveform(resp.Body)
	if err != nil || active != 1000 || period != 2000 {
		t.Errorf("DecodeWaveform(write response) = (%d, %d, %v), want (1000, 2000, nil)", active, period, err)
	}

	resp, err = ep.HandleRequest(root, readReq())
	if err != nil {
		t.Fatalf("HandleRequest(read): %v", err)
	}
	active, period, err = pwm.DecodeWaveform(resp.Body)
	if err != nil || active != 1000 || period != 2000 {
		t.Errorf("DecodeWaveform(read response) = (%d, %d, %v), want (1000, 2000, nil)", active, period, err)
	}

	noFlags := acf.Message{Kind: acf.KindShort, ByteBusID: avtp.ByteBusID(1)}
	if _, err := ep.HandleRequest(root, noFlags); !errors.Is(err, pwm.ErrRequestMustReadOrWrite) {
		t.Errorf("HandleRequest(no flags) err = %v, want ErrRequestMustReadOrWrite", err)
	}
}

// TestHandleRequest_WrongEndpointNoAccessNotConfigured checks a request
// addressed to a different byte_bus_id, one from a stream with no access
// grant, and one against a disabled endpoint are all rejected (REQ-PWM-004).
func TestHandleRequest_WrongEndpointNoAccessNotConfigured(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	cfg := pwm.Config{Enabled: true, Role: pwm.RoleOutput, DefaultActiveTicks: 250, DefaultPeriodTicks: 1000}
	if err := ep.Configure(root, cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	wrongAddr := writeReq(50, 100)
	wrongAddr.ByteBusID = 2
	if _, err := ep.HandleRequest(root, wrongAddr); !errors.Is(err, pwm.ErrWrongEndpoint) {
		t.Errorf("HandleRequest(wrong addr) err = %v, want ErrWrongEndpoint", err)
	}

	stranger := avtp.NewStreamID([6]byte{0x02, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE}, 9)
	if _, err := ep.HandleRequest(stranger, readReq()); !errors.Is(err, regmap.ErrAccessDenied) {
		t.Errorf("HandleRequest(no grant) err = %v, want regmap.ErrAccessDenied", err)
	}

	unconfigured, root2 := newDeclaredEndpoint(t)
	if _, err := unconfigured.HandleRequest(root2, readReq()); !errors.Is(err, pwm.ErrNotConfigured) {
		t.Errorf("HandleRequest(disabled) err = %v, want ErrNotConfigured", err)
	}
}

// TestHandleRequest_OutputAppliesDefaultUsesTransportRejectsBadWaveform
// checks Configure applies a RoleOutput endpoint's default waveform
// immediately, a configured OutputTransport performs subsequent writes, and
// a write with active ticks exceeding period ticks is rejected (REQ-PWM-005,
// REQ-PWM-006).
func TestHandleRequest_OutputAppliesDefaultUsesTransportRejectsBadWaveform(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	cfg := pwm.Config{Enabled: true, Role: pwm.RoleOutput, DefaultActiveTicks: 400, DefaultPeriodTicks: 1000}
	if err := ep.Configure(root, cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	// The default waveform is applied immediately, readable before any
	// write request.
	resp, err := ep.HandleRequest(root, readReq())
	if err != nil {
		t.Fatalf("HandleRequest(read, pre-write): %v", err)
	}
	active, period, _ := pwm.DecodeWaveform(resp.Body)
	if active != 400 || period != 1000 {
		t.Errorf("read (pre-write, default applied) = (%d, %d), want (400, 1000)", active, period)
	}

	tr := &recordingOutputTransport{}
	ep.SetOutputTransport(tr)
	if _, err := ep.HandleRequest(root, writeReq(2500, 5000)); err != nil {
		t.Fatalf("HandleRequest(write via transport): %v", err)
	}
	if tr.calls != 1 || tr.active != 2500 || tr.period != 5000 {
		t.Errorf("OutputTransport recorded = %+v, want {active:2500 period:5000 calls:1}", tr)
	}

	if _, err := ep.HandleRequest(root, writeReq(200, 100)); !errors.Is(err, pwm.ErrActiveExceedsPeriod) {
		t.Errorf("HandleRequest(active>period) err = %v, want ErrActiveExceedsPeriod", err)
	}
}

// TestHandleRequest_InputCapturesFailsOnSignalLossRejectsWrite checks a
// RoleInput endpoint rejects write requests, returns a captured waveform
// (through a configured InputTransport, or the default
// SetCapturedWaveform-fed behavior), and fails explicitly with
// ErrSignalLost rather than returning stale data (REQ-PWM-007).
func TestHandleRequest_InputCapturesFailsOnSignalLossRejectsWrite(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	cfg := pwm.Config{Enabled: true, Role: pwm.RoleInput}
	if err := ep.Configure(root, cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	if _, err := ep.HandleRequest(root, writeReq(50, 100)); !errors.Is(err, pwm.ErrWriteNotSupportedForInput) {
		t.Errorf("HandleRequest(write, RoleInput) err = %v, want ErrWriteNotSupportedForInput", err)
	}

	// No capture yet: signal loss.
	if _, err := ep.HandleRequest(root, readReq()); !errors.Is(err, pwm.ErrSignalLost) {
		t.Errorf("HandleRequest(read, never captured) err = %v, want ErrSignalLost", err)
	}

	ep.SetCapturedWaveform(1200, 3000)
	resp, err := ep.HandleRequest(root, readReq())
	if err != nil {
		t.Fatalf("HandleRequest(read, captured): %v", err)
	}
	active, period, _ := pwm.DecodeWaveform(resp.Body)
	if active != 1200 || period != 3000 {
		t.Errorf("read (captured) = (%d, %d), want (1200, 3000)", active, period)
	}

	ep.SetSignalLost()
	if _, lostErr := ep.HandleRequest(root, readReq()); !errors.Is(lostErr, pwm.ErrSignalLost) {
		t.Errorf("HandleRequest(read, after SetSignalLost) err = %v, want ErrSignalLost", lostErr)
	}

	// A configured InputTransport takes over entirely, including its own
	// error.
	ep.SetInputTransport(&fixedInputTransport{err: pwm.ErrSignalLost})
	if _, transportErr := ep.HandleRequest(root, readReq()); !errors.Is(transportErr, pwm.ErrSignalLost) {
		t.Errorf("HandleRequest(read, transport signal lost) err = %v, want ErrSignalLost", transportErr)
	}
	ep.SetInputTransport(&fixedInputTransport{active: 100, period: 500})
	resp, err = ep.HandleRequest(root, readReq())
	if err != nil {
		t.Fatalf("HandleRequest(read, transport ok): %v", err)
	}
	active, period, _ = pwm.DecodeWaveform(resp.Body)
	if active != 100 || period != 500 {
		t.Errorf("read (transport) = (%d, %d), want (100, 500)", active, period)
	}
}

// TestDrainTriggers_FIFOAndClears checks DrainTriggers returns queued events
// (including the Configure-applied default) in order and clears the queue
// (REQ-PWM-008).
func TestDrainTriggers_FIFOAndClears(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	cfg := pwm.Config{Enabled: true, Role: pwm.RoleOutput, DefaultActiveTicks: 400, DefaultPeriodTicks: 1000}
	if err := ep.Configure(root, cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if _, err := ep.HandleRequest(root, writeReq(800, 2000)); err != nil {
		t.Fatalf("HandleRequest(write): %v", err)
	}

	got := ep.DrainTriggers()
	if len(got) != 2 {
		t.Fatalf("DrainTriggers() = %+v, want 2 events", got)
	}
	if got[0].Kind != pwm.TriggerOutputUpdated || got[0].ActiveTicks != 400 || got[0].PeriodTicks != 1000 {
		t.Errorf("trigger[0] (Configure default) = %+v, want {OutputUpdated 400 1000}", got[0])
	}
	if got[1].Kind != pwm.TriggerOutputUpdated || got[1].ActiveTicks != 800 || got[1].PeriodTicks != 2000 {
		t.Errorf("trigger[1] (write) = %+v, want {OutputUpdated 800 2000}", got[1])
	}
	if again := ep.DrainTriggers(); again != nil {
		t.Errorf("DrainTriggers() after drain = %+v, want nil", again)
	}
}
