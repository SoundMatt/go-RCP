package crcsafe

import "errors"

// CRC safe-point errors.
var (
	// ErrShortSafePoint is returned by Verify when a message's Body is too
	// short to contain a trailing CRC32 safe point at all.
	ErrShortSafePoint = errors.New("rcp/crcsafe: message body too short to contain a CRC32 safe point")

	// ErrCRCMismatch is returned by Verify (and, wrapping it, by Guard's
	// HandleRequest) when a message's trailing CRC32 safe point does not
	// match the CRC computed over its covered fields — this milestone's
	// dedicated CRC error code: execution is skipped entirely rather than
	// forwarded to the wrapped Handler (ROADMAP.md Milestone 50), unlike the
	// old e2e package's separate replay-guard framing for a similar failure.
	ErrCRCMismatch = errors.New("rcp/crcsafe: CRC32 safe point mismatch")
)

// Watchdog errors.
var (
	// ErrSequenceViolation is returned (wrapped with fmt.Errorf's %w) by
	// Supervisor.Observe when a stream's configured
	// StreamConfig.RequireMonotonicSequence rule rejects an arrival's
	// sequence number.
	ErrSequenceViolation = errors.New("rcp/crcsafe: stream sequence number failed its configured monotonicity check")
)
