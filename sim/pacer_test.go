//fusa:test REQ-SIM-001
//fusa:test REQ-SIM-002
//fusa:test REQ-SIM-003
//fusa:test REQ-SIM-004
//fusa:test REQ-SIM-005
//fusa:test REQ-SIM-006

package sim_test

import (
	"testing"
	"time"

	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/sim"
)

func testStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55}, 1)
}

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }
func (c *fakeClock) advance(d time.Duration) {
	c.t = c.t.Add(d)
}

// TestPacer_FiresOnFirstPump verifies the very first Pump call always
// fires, with no grace period (REQ-SIM-001).
func TestPacer_FiresOnFirstPump(t *testing.T) {
	clk := &fakeClock{t: time.Now()}
	calls := 0
	p := sim.NewPacer(10*time.Millisecond, clk.now, func(avtp.StreamID) error {
		calls++
		return nil
	})
	fired, err := p.Pump(testStream())
	if err != nil {
		t.Fatalf("Pump: %v", err)
	}
	if !fired {
		t.Error("first Pump should fire unconditionally")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

// TestPacer_WithholdsUntilIntervalElapses verifies Pump is a no-op until
// Interval has elapsed on the injected Clock (REQ-SIM-002).
func TestPacer_WithholdsUntilIntervalElapses(t *testing.T) {
	clk := &fakeClock{t: time.Now()}
	calls := 0
	p := sim.NewPacer(10*time.Millisecond, clk.now, func(avtp.StreamID) error {
		calls++
		return nil
	})
	if _, err := p.Pump(testStream()); err != nil {
		t.Fatalf("first Pump: %v", err)
	}

	clk.advance(5 * time.Millisecond)
	fired, err := p.Pump(testStream())
	if err != nil {
		t.Fatalf("second Pump: %v", err)
	}
	if fired {
		t.Error("Pump fired before Interval elapsed")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (second Pump should not have fired)", calls)
	}

	clk.advance(6 * time.Millisecond) // total 11ms, past the 10ms interval
	fired, err = p.Pump(testStream())
	if err != nil {
		t.Fatalf("third Pump: %v", err)
	}
	if !fired {
		t.Error("Pump did not fire after Interval elapsed")
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
}

// TestPacer_PropagatesFireError verifies Pump surfaces fire's own error
// (REQ-SIM-003).
func TestPacer_PropagatesFireError(t *testing.T) {
	clk := &fakeClock{t: time.Now()}
	wantErr := errTest
	p := sim.NewPacer(time.Millisecond, clk.now, func(avtp.StreamID) error {
		return wantErr
	})
	_, err := p.Pump(testStream())
	if err != wantErr {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}

// TestPacer_Reset verifies Reset makes the next Pump fire unconditionally
// again, even before Interval would otherwise have elapsed (REQ-SIM-004).
func TestPacer_Reset(t *testing.T) {
	clk := &fakeClock{t: time.Now()}
	calls := 0
	p := sim.NewPacer(time.Hour, clk.now, func(avtp.StreamID) error {
		calls++
		return nil
	})
	if _, err := p.Pump(testStream()); err != nil {
		t.Fatalf("first Pump: %v", err)
	}
	p.Reset()
	fired, err := p.Pump(testStream())
	if err != nil {
		t.Fatalf("Pump after Reset: %v", err)
	}
	if !fired {
		t.Error("Pump after Reset should fire unconditionally")
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
}

// TestPacer_ReceivesRequester verifies Pump passes its requester argument
// through to fire (REQ-SIM-005).
func TestPacer_ReceivesRequester(t *testing.T) {
	clk := &fakeClock{t: time.Now()}
	var got avtp.StreamID
	p := sim.NewPacer(time.Millisecond, clk.now, func(requester avtp.StreamID) error {
		got = requester
		return nil
	})
	want := testStream()
	if _, err := p.Pump(want); err != nil {
		t.Fatalf("Pump: %v", err)
	}
	if got != want {
		t.Errorf("requester = %v, want %v", got, want)
	}
}

// TestNewADCPacer_And_NewPWMPacer_BehaveLikePacer verifies the named
// constructors are plain Pacer aliases with the same firing semantics
// (REQ-SIM-006).
func TestNewADCPacer_And_NewPWMPacer_BehaveLikePacer(t *testing.T) {
	clk := &fakeClock{t: time.Now()}
	adcCalls, pwmCalls := 0, 0
	adc := sim.NewADCPacer(time.Millisecond, clk.now, func(avtp.StreamID) error { adcCalls++; return nil })
	pwm := sim.NewPWMPacer(time.Millisecond, clk.now, func(avtp.StreamID) error { pwmCalls++; return nil })

	if _, err := adc.Pump(testStream()); err != nil {
		t.Fatalf("adc Pump: %v", err)
	}
	if _, err := pwm.Pump(testStream()); err != nil {
		t.Fatalf("pwm Pump: %v", err)
	}
	if adcCalls != 1 || pwmCalls != 1 {
		t.Errorf("adcCalls=%d pwmCalls=%d, want 1 and 1", adcCalls, pwmCalls)
	}
}

var errTest = &testError{"injected"}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }
