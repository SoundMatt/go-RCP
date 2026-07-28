package deadline

import (
	"time"

	"github.com/SoundMatt/go-RCP/regmap"
)

// DeadlineForQueue derives a Config.Deadline from q's own configured
// heartbeat cadence (regmap.QueueConfig.HeartbeatIntervalMillis,
// regmap/queues.go, ROADMAP.md Milestone 45): the smallest gap a caller
// should tolerate before concluding the link itself is down, expressed as a
// caller-chosen number of consecutive missed heartbeats rather than a raw
// duration this package would otherwise have no principled way to pick on
// its own. It reports ErrNoHeartbeatConfigured if q has heartbeats disabled
// (HeartbeatIntervalMillis == 0) and ErrInvalidMissedHeartbeats if
// missedHeartbeats is less than 1.
func DeadlineForQueue(q regmap.QueueConfig, missedHeartbeats int) (time.Duration, error) {
	if q.HeartbeatIntervalMillis == 0 {
		return 0, ErrNoHeartbeatConfigured
	}
	if missedHeartbeats < 1 {
		return 0, ErrInvalidMissedHeartbeats
	}
	return time.Duration(q.HeartbeatIntervalMillis) * time.Duration(missedHeartbeats) * time.Millisecond, nil
}
