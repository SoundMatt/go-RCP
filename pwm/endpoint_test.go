//fusa:test REQ-PWM-004
//fusa:test REQ-PWM-005
//fusa:test REQ-PWM-006
//fusa:test REQ-PWM-007
//fusa:test REQ-PWM-008

package pwm_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/pwm"
	"github.com/SoundMatt/go-RCP/regmap"
)

func writeReq(period, active uint32) acf.Message {
	return acf.Message{
		Kind:      acf.KindShort,
		ByteBusID: avtp.ByteBusID(1),
		Control:   acf.FlagWrite,
		Body:      pwm.EncodeWaveform(period, active),
	}
}

func readReq() acf.Message {
	return acf.Message{Kind: acf.KindShort, ByteBusID: avtp.ByteBusID(1), Control: acf.FlagRead}
}

// recordingOutputTransport is an OutputTransport test double that records
// every applied waveform, so tests can tell the configured Transport
// actually ran rather than the default store-for-readback behavior.
type recordingOutputTransport struct {
	period, active uint32
	calls          int
}

func (r *recordingOutputTransport) SetOutput(period, active uint32) error {
	r.period, r.active = period, active
	r.calls++
	return nil
}

// fixedInputTransport is an InputTransport test double returning a fixed
// waveform or a fixed error.
type fixedInputTransport struct {
	period, active uint32
	err            error
}

func (f *fixedInputTransport) Capture() (uint32, uint32, error) {
	return f.period, f.active, f.err
}

// TestHandleRequest_RoutesReadWriteRejectsNeither checks HandleRequest
// dispatches Write/Read for a RoleOutput endpoint, and rejects a request with
// neither flag (REQ-PWM-004).
func TestHandleRequest_RoutesReadWriteRejectsNeither(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	cfg := pwm.Config{Enabled: true, Role: pwm.RoleOutput, DefaultPeriodMicros: 1000, DefaultActiveMicros: 250}
	if err := ep.Configure(root, cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	resp, err := ep.HandleRequest(root, writeReq(2000, 1000))
	if err != nil {
		t.Fatalf("HandleRequest(write): %v", err)
	}
	period, active, err := pwm.DecodeWaveform(resp.Body)
	if err != nil || period != 2000 || active != 1000 {
		t.Errorf("DecodeWaveform(write response) = (%d, %d, %v), want (2000, 1000, nil)", period, active, err)
	}

	resp, err = ep.HandleRequest(root, readReq())
	if err != nil {
		t.Fatalf("HandleRequest(read): %v", err)
	}
	period, active, err = pwm.DecodeWaveform(resp.Body)
	if err != nil || period != 2000 || active != 1000 {
		t.Errorf("DecodeWaveform(read response) = (%d, %d, %v), want (2000, 1000, nil)", period, active, err)
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
	cfg := pwm.Config{Enabled: true, Role: pwm.RoleOutput, DefaultPeriodMicros: 1000, DefaultActiveMicros: 250}
	if err := ep.Configure(root, cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	wrongAddr := writeReq(100, 50)
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
// a write with ActiveMicros>PeriodMicros is rejected (REQ-PWM-005,
// REQ-PWM-006).
func TestHandleRequest_OutputAppliesDefaultUsesTransportRejectsBadWaveform(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	cfg := pwm.Config{Enabled: true, Role: pwm.RoleOutput, DefaultPeriodMicros: 1000, DefaultActiveMicros: 400}
	if err := ep.Configure(root, cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	// The default waveform is applied immediately, readable before any
	// write request.
	resp, err := ep.HandleRequest(root, readReq())
	if err != nil {
		t.Fatalf("HandleRequest(read, pre-write): %v", err)
	}
	period, active, _ := pwm.DecodeWaveform(resp.Body)
	if period != 1000 || active != 400 {
		t.Errorf("read (pre-write, default applied) = (%d, %d), want (1000, 400)", period, active)
	}

	tr := &recordingOutputTransport{}
	ep.SetOutputTransport(tr)
	if _, err := ep.HandleRequest(root, writeReq(5000, 2500)); err != nil {
		t.Fatalf("HandleRequest(write via transport): %v", err)
	}
	if tr.calls != 1 || tr.period != 5000 || tr.active != 2500 {
		t.Errorf("OutputTransport recorded = %+v, want {period:5000 active:2500 calls:1}", tr)
	}

	if _, err := ep.HandleRequest(root, writeReq(100, 200)); !errors.Is(err, pwm.ErrActiveExceedsPeriod) {
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

	if _, err := ep.HandleRequest(root, writeReq(100, 50)); !errors.Is(err, pwm.ErrWriteNotSupportedForInput) {
		t.Errorf("HandleRequest(write, RoleInput) err = %v, want ErrWriteNotSupportedForInput", err)
	}

	// No capture yet: signal loss.
	if _, err := ep.HandleRequest(root, readReq()); !errors.Is(err, pwm.ErrSignalLost) {
		t.Errorf("HandleRequest(read, never captured) err = %v, want ErrSignalLost", err)
	}

	ep.SetCapturedWaveform(3000, 1200)
	resp, err := ep.HandleRequest(root, readReq())
	if err != nil {
		t.Fatalf("HandleRequest(read, captured): %v", err)
	}
	period, active, _ := pwm.DecodeWaveform(resp.Body)
	if period != 3000 || active != 1200 {
		t.Errorf("read (captured) = (%d, %d), want (3000, 1200)", period, active)
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
	ep.SetInputTransport(&fixedInputTransport{period: 500, active: 100})
	resp, err = ep.HandleRequest(root, readReq())
	if err != nil {
		t.Fatalf("HandleRequest(read, transport ok): %v", err)
	}
	period, active, _ = pwm.DecodeWaveform(resp.Body)
	if period != 500 || active != 100 {
		t.Errorf("read (transport) = (%d, %d), want (500, 100)", period, active)
	}
}

// TestDrainTriggers_FIFOAndClears checks DrainTriggers returns queued events
// (including the Configure-applied default) in order and clears the queue
// (REQ-PWM-008).
func TestDrainTriggers_FIFOAndClears(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	cfg := pwm.Config{Enabled: true, Role: pwm.RoleOutput, DefaultPeriodMicros: 1000, DefaultActiveMicros: 400}
	if err := ep.Configure(root, cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if _, err := ep.HandleRequest(root, writeReq(2000, 800)); err != nil {
		t.Fatalf("HandleRequest(write): %v", err)
	}

	got := ep.DrainTriggers()
	if len(got) != 2 {
		t.Fatalf("DrainTriggers() = %+v, want 2 events", got)
	}
	if got[0].Kind != pwm.TriggerOutputUpdated || got[0].PeriodMicros != 1000 || got[0].ActiveMicros != 400 {
		t.Errorf("trigger[0] (Configure default) = %+v, want {OutputUpdated 1000 400}", got[0])
	}
	if got[1].Kind != pwm.TriggerOutputUpdated || got[1].PeriodMicros != 2000 || got[1].ActiveMicros != 800 {
		t.Errorf("trigger[1] (write) = %+v, want {OutputUpdated 2000 800}", got[1])
	}
	if again := ep.DrainTriggers(); again != nil {
		t.Errorf("DrainTriggers() after drain = %+v, want nil", again)
	}
}
