package sim

import (
	"sync"
	"time"

	"github.com/SoundMatt/go-RCP/v9/avtp"
)

// Clock is an injectable time source, the same posture
// server.NewServerWithClock and e2e.NewSupervisorWithClock already
// establish in this repo for anything that measures elapsed time but must
// stay testable without real sleeps.
type Clock func() time.Time

// FireFunc performs one endpoint-specific timed action — e.g.
// (*adc.Endpoint).Trigger, or a caller-supplied closure invoking
// (*pwm.Endpoint).SetCapturedWaveform with a freshly synthesized waveform —
// and reports any error from that action. This package deliberately does
// not import adc, pwm, or any other endpoint-type package to obtain this
// action itself: a caller adapts whichever concrete endpoint it is pacing
// to this shape, the same "caller wires the concrete type in" pattern
// request.TriggerPump and e2e.SafeStateCheck already establish.
type FireFunc func(requester avtp.StreamID) error

// Pacer periodically invokes a FireFunc at most once per Interval, measured
// against an injectable Clock rather than a real-time goroutine timer — the
// same caller-driven, no-internal-goroutine posture powerstate.Driver.Pump
// (Milestone 53) already established for this repo's other "something
// needs to happen periodically but nothing here should own a timer
// goroutine" cases. A caller invokes Pump on whatever cadence suits its own
// test loop (a tight polling loop, a real time.Ticker, a single explicit
// call per test step, ...); Pacer itself never sleeps or spawns anything.
// All exported methods are safe for concurrent use.
type Pacer struct {
	mu       sync.Mutex
	interval time.Duration
	now      Clock
	fire     FireFunc
	last     time.Time
	armed    bool
}

// NewPacer returns a Pacer that invokes fire at most once per interval,
// timed against now.
func NewPacer(interval time.Duration, now Clock, fire FireFunc) *Pacer {
	return &Pacer{interval: interval, now: now, fire: fire}
}

// NewADCPacer names Pacer's constructor for an ADC channel's sample-
// interval timing model (ROADMAP.md Milestone 57): fire is typically
// (*adc.Endpoint).Trigger, invoked once per elapsed sampleInterval.
func NewADCPacer(sampleInterval time.Duration, now Clock, trigger FireFunc) *Pacer {
	return NewPacer(sampleInterval, now, trigger)
}

// NewPWMPacer names Pacer's constructor for a PWM endpoint's output/input
// cycle-timing model: fire is a caller-supplied closure that synthesizes
// one cycle's worth of waveform data and applies it, e.g. calling
// (*pwm.Endpoint).SetCapturedWaveform for a simulated RoleInput endpoint.
func NewPWMPacer(cyclePeriod time.Duration, now Clock, cycle FireFunc) *Pacer {
	return NewPacer(cyclePeriod, now, cycle)
}

// Pump fires unconditionally the first time it is ever called for this
// Pacer (there is no grace period before the first sample/cycle), or
// invokes fire and reports true once at least Interval has elapsed since
// the previous fire; otherwise it is a no-op and returns (false, nil).
func (p *Pacer) Pump(requester avtp.StreamID) (fired bool, err error) {
	p.mu.Lock()
	now := p.now()
	if p.armed && now.Sub(p.last) < p.interval {
		p.mu.Unlock()
		return false, nil
	}
	p.armed = true
	p.last = now
	p.mu.Unlock()
	return true, p.fire(requester)
}

// Reset clears Pacer's "already fired at least once" state, so the next
// Pump call fires unconditionally again — a caller's explicit "restart the
// cadence from now" action, e.g. after reconfiguring the endpoint's own
// sample interval.
func (p *Pacer) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.armed = false
}
