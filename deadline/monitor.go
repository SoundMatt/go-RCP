package deadline

import (
	"sync"
	"time"

	"github.com/SoundMatt/go-RCP/avtp"
)

// LivenessState is one stream's current liveness verdict, per Monitor.State.
type LivenessState uint8

const (
	// LivenessDead means neither a trigger nor a heartbeat has been
	// observed for this stream within the configured Deadline — including a
	// stream Monitor has never observed anything for at all.
	LivenessDead LivenessState = iota

	// LivenessIdle means at least one heartbeat has been observed within
	// the configured Deadline, but no trigger has: the link/queue itself
	// appears to be up, but nothing is actively happening upstream of it.
	LivenessIdle

	// LivenessAlive means at least one trigger has been observed within the
	// configured Deadline — genuine evidence of upstream activity, not just
	// an idle link.
	LivenessAlive
)

// String renders s for logs and test failure messages.
func (s LivenessState) String() string {
	switch s {
	case LivenessDead:
		return "Dead"
	case LivenessIdle:
		return "Idle"
	case LivenessAlive:
		return "Alive"
	default:
		return "Unknown"
	}
}

// Config configures a Monitor's liveness deadline.
type Config struct {
	// Deadline is the maximum tolerated gap since a stream's most recent
	// signal of any kind (trigger or heartbeat) before Monitor.State reports
	// LivenessDead for it, and since its most recent trigger specifically
	// before State stops reporting LivenessAlive (falling back to
	// LivenessIdle if a heartbeat is still recent enough, or LivenessDead
	// otherwise).
	Deadline time.Duration
}

// Validate reports whether c is a plausible configuration: Deadline must be
// strictly positive.
func (c Config) Validate() error {
	if c.Deadline <= 0 {
		return ErrInvalidDeadline
	}
	return nil
}

// streamState is one stream's Monitor-internal liveness bookkeeping.
type streamState struct {
	lastAny     time.Time
	lastTrigger time.Time
	haveTrigger bool
}

// Monitor tracks per-stream liveness purely from caller-reported signal
// arrivals (ObserveTrigger, ObserveHeartbeat) and a configured Deadline — no
// goroutine, no timer of its own, every verdict computed lazily against this
// Monitor's injectable clock, the same posture crcsafe.Supervisor already
// establishes for its own request-arrival timeout. All exported methods are
// safe for concurrent use.
type Monitor struct {
	mu    sync.Mutex
	now   func() time.Time
	cfg   Config
	state map[avtp.StreamID]*streamState
}

// NewMonitor returns a Monitor applying cfg to every stream, using time.Now
// as its clock. It reports an error (wrapping ErrInvalidDeadline) if cfg is
// not valid.
func NewMonitor(cfg Config) (*Monitor, error) {
	return NewMonitorWithClock(cfg, time.Now)
}

// NewMonitorWithClock is like NewMonitor but accepts a custom clock
// function, used in tests to avoid real-time sleeps when exercising
// Deadline — the same injectable-clock pattern crcsafe.NewSupervisorWithClock
// establishes.
func NewMonitorWithClock(cfg Config, now func() time.Time) (*Monitor, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Monitor{now: now, cfg: cfg, state: make(map[avtp.StreamID]*streamState)}, nil
}

// ObserveTrigger records that a caller drained at least one genuine trigger
// event (e.g. from an endpoint's own DrainTriggers) attributable to stream
// right now, per this Monitor's clock. It counts as evidence for both
// LivenessAlive and (implicitly) LivenessIdle.
func (m *Monitor) ObserveTrigger(stream avtp.StreamID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.stateForLocked(stream)
	now := m.now()
	st.lastAny = now
	st.lastTrigger = now
	st.haveTrigger = true
}

// ObserveHeartbeat records that a caller received a response-queue heartbeat
// (server.QueueConfig.HeartbeatIntervalMillis) attributable to stream right
// now, per this Monitor's clock. Unlike ObserveTrigger, this alone never
// produces LivenessAlive — only LivenessIdle at best — since a heartbeat
// only proves the link/queue is up, not that anything is actually
// happening.
func (m *Monitor) ObserveHeartbeat(stream avtp.StreamID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.stateForLocked(stream)
	st.lastAny = m.now()
}

// stateForLocked returns stream's streamState, creating one if needed.
// Callers must hold m.mu.
func (m *Monitor) stateForLocked(stream avtp.StreamID) *streamState {
	st, ok := m.state[stream]
	if !ok {
		st = &streamState{}
		m.state[stream] = st
	}
	return st
}

// State reports stream's current LivenessState: LivenessDead if stream has
// never been observed at all, or if its most recent signal of any kind is
// older than the configured Deadline; otherwise LivenessAlive if its most
// recent trigger is itself within Deadline, or LivenessIdle otherwise.
func (m *Monitor) State(stream avtp.StreamID) LivenessState {
	m.mu.Lock()
	defer m.mu.Unlock()

	st, ok := m.state[stream]
	if !ok {
		return LivenessDead
	}
	now := m.now()
	if now.Sub(st.lastAny) > m.cfg.Deadline {
		return LivenessDead
	}
	if st.haveTrigger && now.Sub(st.lastTrigger) <= m.cfg.Deadline {
		return LivenessAlive
	}
	return LivenessIdle
}

// Alive reports whether stream's current State is anything other than
// LivenessDead — a convenience for a caller that only cares about the
// binary "is this stream still there at all" question.
func (m *Monitor) Alive(stream avtp.StreamID) bool {
	return m.State(stream) != LivenessDead
}

// Forget discards stream's recorded bookkeeping, freeing the memory a
// long-running Monitor would otherwise accumulate for a stream that will
// never be observed again — the same purpose request.Dispatcher.Forget
// serves for a finalized ticket. After Forget, State(stream) reports
// LivenessDead exactly as if stream had never been observed at all.
func (m *Monitor) Forget(stream avtp.StreamID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.state, stream)
}
