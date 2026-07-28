//fusa:test REQ-CRC-006
//fusa:test REQ-CRC-007
//fusa:test REQ-CRC-008

package crcsafe_test

import (
	"errors"
	"testing"
	"time"

	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/crcsafe"
)

// fakeClock is a manually-advanced clock for deterministic Supervisor tests,
// the same injectable-clock pattern server_test.go and ratelimit_test.go
// already establish for their own packages.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

// TestSupervisor_MonotonicSequence checks Observe accepts a strictly
// increasing sequence number, accepts a configured wraparound, and reports
// (and stickily records) a violation for anything else when
// RequireMonotonicSequence is set (REQ-CRC-006).
func TestSupervisor_MonotonicSequence(t *testing.T) {
	stream := testStream()
	clock := &fakeClock{t: time.Unix(0, 0)}
	sup := crcsafe.NewSupervisorWithClock(crcsafe.StreamConfig{
		Timeout:                  time.Hour,
		RequireMonotonicSequence: true,
		SequenceOverflowWraps:    true,
	}, clock.now)

	if err := sup.Observe(stream, 1); err != nil {
		t.Fatalf("Observe(first): %v", err)
	}
	if err := sup.Observe(stream, 2); err != nil {
		t.Fatalf("Observe(increasing): %v", err)
	}
	if sup.InSafeState(stream) {
		t.Fatalf("InSafeState = true after two well-formed increasing arrivals, want false")
	}

	if err := sup.Observe(stream, 2); !errors.Is(err, crcsafe.ErrSequenceViolation) {
		t.Errorf("Observe(repeat) err = %v, want ErrSequenceViolation", err)
	}
	if !sup.InSafeState(stream) {
		t.Errorf("InSafeState = false after a monotonicity violation, want true (sticky trip)")
	}

	// A subsequent, individually well-formed arrival must not clear the
	// sticky trip on its own — only Reset does.
	if err := sup.Observe(stream, 3); err != nil {
		t.Fatalf("Observe(after violation): %v", err)
	}
	if !sup.InSafeState(stream) {
		t.Errorf("InSafeState = false after a later well-formed arrival, want still true until Reset")
	}
	sup.Reset(stream)
	if sup.InSafeState(stream) {
		t.Errorf("InSafeState = true immediately after Reset, want false")
	}

	// Wraparound: math.MaxUint32 -> 0 is legitimate when configured.
	other := avtp.NewStreamID([6]byte{0x02, 0, 0, 0, 0, 1}, 1)
	if err := sup.Observe(other, ^uint32(0)); err != nil {
		t.Fatalf("Observe(max): %v", err)
	}
	if err := sup.Observe(other, 0); err != nil {
		t.Errorf("Observe(wrap to 0) err = %v, want nil (SequenceOverflowWraps configured)", err)
	}
	if sup.InSafeState(other) {
		t.Errorf("InSafeState(other) = true after a legitimate wraparound, want false")
	}
}

// TestSupervisor_TimeoutAndNeverObserved checks InSafeState reports true for
// a stream Observe has never been called for, false immediately after an
// Observe call, and true again once the configured Timeout has elapsed on
// the injectable clock (REQ-CRC-007).
func TestSupervisor_TimeoutAndNeverObserved(t *testing.T) {
	stream := testStream()
	clock := &fakeClock{t: time.Unix(0, 0)}
	sup := crcsafe.NewSupervisorWithClock(crcsafe.StreamConfig{Timeout: 100 * time.Millisecond}, clock.now)

	if !sup.InSafeState(stream) {
		t.Errorf("InSafeState(never observed) = false, want true")
	}

	if err := sup.Observe(stream, 1); err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if sup.InSafeState(stream) {
		t.Errorf("InSafeState immediately after Observe = true, want false")
	}

	clock.advance(99 * time.Millisecond)
	if sup.InSafeState(stream) {
		t.Errorf("InSafeState just under Timeout = true, want false")
	}

	clock.advance(2 * time.Millisecond) // now 101ms since the last arrival
	if !sup.InSafeState(stream) {
		t.Errorf("InSafeState past Timeout = false, want true")
	}

	// A fresh arrival rearms the clock.
	if err := sup.Observe(stream, 2); err != nil {
		t.Fatalf("Observe(rearm): %v", err)
	}
	if sup.InSafeState(stream) {
		t.Errorf("InSafeState immediately after a fresh arrival = true, want false")
	}
}

// TestSupervisor_CheckFuncMatchesInSafeState checks CheckFunc's adapted
// request.SafeStateCheck reports exactly what InSafeState itself reports
// (REQ-CRC-008).
func TestSupervisor_CheckFuncMatchesInSafeState(t *testing.T) {
	stream := testStream()
	clock := &fakeClock{t: time.Unix(0, 0)}
	sup := crcsafe.NewSupervisorWithClock(crcsafe.StreamConfig{Timeout: time.Hour}, clock.now)
	check := sup.CheckFunc()

	if check(stream) != sup.InSafeState(stream) {
		t.Fatalf("CheckFunc(never observed) = %v, want %v", check(stream), sup.InSafeState(stream))
	}
	if err := sup.Observe(stream, 1); err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if check(stream) != sup.InSafeState(stream) {
		t.Errorf("CheckFunc(observed) = %v, want %v", check(stream), sup.InSafeState(stream))
	}
}
