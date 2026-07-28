package discovery

import (
	"time"

	"github.com/SoundMatt/go-RCP/avtp"
)

// This file implements ROADMAP.md Milestone 46 (Discovery, v0.59.0): the
// deliberate, narrowly-scoped exception to the ordinary EP0 access-control
// gate that regmap.AccessController's own doc comment forward-references,
// plus the Discovery-stream configuration claim that layers on top of (not
// in place of) AccessController's permanent root-client claim. Discovery is
// "just a read of that same [register] map" (ROADMAP.md) — server.Server.
// ReadDiscovery reuses regmap.EncodeRegisterMap/regmap.DecodeRegisterMap
// wholesale rather than defining any byte layout of its own, so it carries
// none of avtp/doc.go's or regmap/doc.go's spec-fidelity caveats about
// register byte assignments. The one genuinely new wire-adjacent rule this
// milestone adds — a discovery request must arrive on the untimed (NTSCF)
// AVTPDU header, never the timestamped (TSCF) one — is enforced using the
// header-variant distinction avtp/avtpdu.go already exposes (Header.Timed),
// the same way Header.Disposition already folds a timed header with no
// time-sync support down to DispositionDrop.

// DefaultConfigurationClaimTimeout is the default duration a Discovery-
// stream configuration claim (see Claim) remains valid without a follow-up
// configuration request before it lapses. It is deliberately this package's
// own reasoned default, not a value transcribed from the TC18
// specification text, and is configurable per Server via
// server.Server.SetConfigurationClaimTimeout.
const DefaultConfigurationClaimTimeout = 30 * time.Second

// Claim is the Discovery-stream configuration claim: a narrower, timeout-
// releasable reservation of configuration rights than
// regmap.AccessController's own permanent, un-timed-out root-client claim
// (see AccessController and its doc comment). A held-but-expired claim is
// treated the same as no claim at all everywhere this type is read. Claim
// deliberately does not read the clock or hold a lock itself — the caller
// (server.Server) supplies "now" and owns the mutex, the same
// caller-supplies-the-clock posture request.SafeStateCheck takes.
type Claim struct {
	Held    bool
	Stream  avtp.StreamID
	Expires time.Time
}

// Active reports whether c is currently a live (held, unexpired) claim as
// of now.
func (c Claim) Active(now time.Time) bool {
	return c.Held && now.Before(c.Expires)
}
