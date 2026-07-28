package deadline

import "errors"

// Configuration errors.
var (
	// ErrInvalidDeadline is returned when a Config's Deadline is not
	// strictly positive — a deadline of zero or less would mean every
	// stream is permanently either LivenessDead or in a state this package
	// cannot meaningfully distinguish, so Validate rejects it up front
	// rather than accepting a configuration that can never report liveness.
	ErrInvalidDeadline = errors.New("rcp/deadline: Deadline must be positive")

	// ErrNoHeartbeatConfigured is returned by DeadlineForQueue when the
	// given regmap.QueueConfig has HeartbeatIntervalMillis == 0 — a queue
	// with heartbeats disabled carries no liveness signal for this function
	// to derive a Deadline from at all.
	ErrNoHeartbeatConfigured = errors.New("rcp/deadline: QueueConfig has no configured heartbeat interval")

	// ErrInvalidMissedHeartbeats is returned by DeadlineForQueue when
	// missedHeartbeats is less than 1 — a deadline that trips before even a
	// single heartbeat could have arrived is not a meaningful "missed
	// heartbeats" threshold.
	ErrInvalidMissedHeartbeats = errors.New("rcp/deadline: missedHeartbeats must be at least 1")
)
