package regmap

// StreamLimits is the request-stream configuration table: the capacity
// bounds a client negotiates against when opening request streams against
// this server.
type StreamLimits struct {
	// MaxRequestStreams bounds how many distinct request streams (distinct
	// StreamID values) the server accepts concurrently.
	MaxRequestStreams uint8

	// MaxInFlightPerStream bounds how many outstanding (unacknowledged)
	// requests a single stream may have open at once.
	MaxInFlightPerStream uint16
}

// QueueConfig is the response/acknowledge-queue configuration table: how the
// server batches outgoing responses and acknowledgements before flushing
// them, and how it keeps an idle queue alive.
type QueueConfig struct {
	// FlushThreshold is the number of queued responses/acks that triggers
	// an immediate flush, regardless of FlushTimeMillis. A value of 0 means
	// this queue never flushes on a count threshold — see Validate.
	FlushThreshold uint16

	// FlushTimeMillis bounds how long a response/ack may sit queued before
	// the server force-flushes it even if FlushThreshold hasn't been
	// reached. A value of 0 means this queue never flushes on a time
	// threshold — see Validate.
	FlushTimeMillis uint32

	// HeartbeatIntervalMillis is the period, while the queue is otherwise
	// idle, at which the server emits a keep-alive so the client can
	// distinguish "nothing to report" from "the link is down". A value of 0
	// disables the heartbeat.
	HeartbeatIntervalMillis uint32
}

// Validate reports whether q is a plausible queue configuration: at least
// one of FlushThreshold or FlushTimeMillis must be nonzero, or the queue
// would never flush at all and every response/ack would sit queued forever.
// This is the guard condition Server.AdvanceToFullyConfigured runs — a
// queue configuration that can never flush is rejected rather than silently
// accepted and left to hang at runtime.
func (q QueueConfig) Validate() error {
	if q.FlushThreshold == 0 && q.FlushTimeMillis == 0 {
		return ErrQueueConfigInvalid
	}
	return nil
}
