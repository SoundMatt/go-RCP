package crcsafe

import (
	"fmt"
	"sync"
	"time"

	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/request"
)

// StreamConfig configures one stream's request-arrival-timed watchdog
// (ROADMAP.md Milestone 50). This deliberately replaces, rather than
// adapts, the old client-push `watchdog` package's model: nothing is sent
// periodically by a client at all here — Supervisor times a stream's own
// inbound request arrivals against Timeout, entirely server-side, and
// separately tracks whether each arrival's sequence number is consistent
// with the configured monotonicity/overflow rule.
type StreamConfig struct {
	// Timeout is the maximum tolerated gap between successive request
	// arrivals from one stream before Supervisor.InSafeState reports that
	// stream's endpoints must have their configured safe state entered. A
	// stream Supervisor has never observed an arrival for at all is treated
	// as already timed out — there is no implicit grace period for "hasn't
	// sent anything yet".
	Timeout time.Duration

	// RequireMonotonicSequence, when true, treats any Observe call whose
	// sequence number does not strictly increase over the immediately
	// preceding one for the same stream (accounting for
	// SequenceOverflowWraps) as an immediate, sticky safe-state trip — see
	// Observe and Reset.
	RequireMonotonicSequence bool

	// SequenceOverflowWraps, when true, treats a sequence number of zero
	// immediately following the previous arrival's math.MaxUint32 as the
	// legitimate "next" value rather than a monotonicity violation.
	SequenceOverflowWraps bool
}

// streamState is one stream's Supervisor-internal watchdog bookkeeping.
type streamState struct {
	lastArrival time.Time
	haveSeq     bool
	lastSeq     uint32
	tripped     bool
}

// Supervisor is the server-side, per-stream watchdog ROADMAP.md Milestone
// 50 introduces in place of the old client-push `watchdog` package: it runs
// no goroutine and sends nothing on the wire at all — every verdict is
// computed lazily, on demand, from Observe's own bookkeeping and this
// Supervisor's injectable clock (see NewSupervisorWithClock, the same
// pattern server.NewServerWithClock and ratelimit.NewControllerWithClock
// already establish in this repo). A caller drives automatic safe-state
// entry by wiring CheckFunc into a request.Dispatcher's
// SetSafeStateCheck, and by calling a Dispatcher's PurgeNonSafety once
// InSafeState transitions to true for a stream that Dispatcher serves — see
// doc.go's integration example. All exported methods are safe for
// concurrent use.
type Supervisor struct {
	mu     sync.Mutex
	now    func() time.Time
	defCfg StreamConfig
	cfg    map[avtp.StreamID]StreamConfig
	state  map[avtp.StreamID]*streamState
}

// NewSupervisor returns a Supervisor applying defCfg to every stream unless
// overridden per-stream by Configure. It uses time.Now as its clock.
func NewSupervisor(defCfg StreamConfig) *Supervisor {
	return NewSupervisorWithClock(defCfg, time.Now)
}

// NewSupervisorWithClock is like NewSupervisor but accepts a custom clock
// function, used in tests to avoid real-time sleeps when exercising
// Timeout — the same injectable-clock pattern server.NewServerWithClock
// establishes.
func NewSupervisorWithClock(defCfg StreamConfig, now func() time.Time) *Supervisor {
	return &Supervisor{
		now:    now,
		defCfg: defCfg,
		cfg:    make(map[avtp.StreamID]StreamConfig),
		state:  make(map[avtp.StreamID]*streamState),
	}
}

// Configure overrides the watchdog configuration for one specific stream,
// replacing the default defCfg passed to NewSupervisor for that stream only.
// It takes effect on the stream's next Observe or InSafeState call.
func (s *Supervisor) Configure(stream avtp.StreamID, cfg StreamConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg[stream] = cfg
}

// configForLocked returns stream's effective StreamConfig. Callers must
// hold s.mu.
func (s *Supervisor) configForLocked(stream avtp.StreamID) StreamConfig {
	if cfg, ok := s.cfg[stream]; ok {
		return cfg
	}
	return s.defCfg
}

// Observe records that a request bearing sequence number seq arrived from
// stream right now, per this Supervisor's clock, and returns nil when that
// arrival is consistent with the stream's configured monotonicity/overflow
// rule, or a non-nil error (wrapping ErrSequenceViolation) when it is not.
// Either way, every call rearms the stream's inter-arrival timeout clock —
// arrival is what Timeout measures, so even a monotonicity-violating
// arrival counts as "the stream is still there" for that purpose,
// independent of whether it also (stickily) trips InSafeState. The first
// Observe call ever made for a stream can never itself violate
// monotonicity: there is no preceding value to compare seq against.
func (s *Supervisor) Observe(stream avtp.StreamID, seq uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg := s.configForLocked(stream)
	st, ok := s.state[stream]
	if !ok {
		st = &streamState{}
		s.state[stream] = st
	}

	var violation error
	if cfg.RequireMonotonicSequence && st.haveSeq {
		wrapped := cfg.SequenceOverflowWraps && st.lastSeq == ^uint32(0) && seq == 0
		if seq != st.lastSeq+1 && !wrapped {
			violation = fmt.Errorf("%w: stream %v: seq %d does not follow %d", ErrSequenceViolation, stream, seq, st.lastSeq)
			st.tripped = true
		}
	}

	st.lastArrival = s.now()
	st.haveSeq = true
	st.lastSeq = seq
	return violation
}

// InSafeState reports whether stream is currently judged to require its
// addressed endpoints' configured safe state entered: either Observe has
// never been called for stream at all, the stream's configured Timeout has
// elapsed since the last Observe call, or a past Observe call recorded a
// monotonicity violation that Reset has not yet cleared.
func (s *Supervisor) InSafeState(stream avtp.StreamID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	st, ok := s.state[stream]
	if !ok {
		return true
	}
	if st.tripped {
		return true
	}
	cfg := s.configForLocked(stream)
	return s.now().Sub(st.lastArrival) > cfg.Timeout
}

// CheckFunc adapts InSafeState to request.SafeStateCheck's shape, for
// wiring directly into a request.Dispatcher's SetSafeStateCheck:
//
//	sup := crcsafe.NewSupervisor(crcsafe.StreamConfig{Timeout: 50 * time.Millisecond})
//	dispatcher.SetSafeStateCheck(sup.CheckFunc())
func (s *Supervisor) CheckFunc() request.SafeStateCheck {
	return s.InSafeState
}

// Reset clears stream's recorded monotonicity violation (if any) and
// rearms its inter-arrival timeout clock from now, as if Observe had just
// been called with no sequence number to check. It is a caller's explicit
// "the fault has been handled, resume normal operation" action — nothing in
// this package calls Reset automatically, matching the general posture that
// entering a safe state is this milestone's automatic, fail-safe reaction,
// while leaving one is always a deliberate operator/caller decision.
func (s *Supervisor) Reset(stream avtp.StreamID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state[stream] = &streamState{lastArrival: s.now(), haveSeq: false}
}
