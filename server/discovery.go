package server

import (
	"time"

	"github.com/SoundMatt/go-RCP/avtp"
)

// This file implements ROADMAP.md Milestone 46 (Discovery, v0.59.0): the
// deliberate, narrowly-scoped exception to the ordinary EP0 access-control
// gate that AccessController's own doc comment forward-references, plus the
// Discovery-stream configuration claim that layers on top of (not in place
// of) AccessController's permanent root-client claim. Discovery is "just a
// read of that same [register] map" (ROADMAP.md) — it reuses
// EncodeRegisterMap/DecodeRegisterMap wholesale rather than defining any
// byte layout of its own, so it carries none of avtp/doc.go's or
// server/doc.go's spec-fidelity caveats about register byte assignments.
// The one genuinely new wire-adjacent rule this milestone adds — a
// discovery request must arrive on the untimed (NTSCF) AVTPDU header, never
// the timestamped (TSCF) one — is enforced using the header-variant
// distinction avtp/avtpdu.go already exposes (Header.Timed), the same way
// Header.Disposition already folds a timed header with no time-sync support
// down to DispositionDrop.

// DefaultConfigurationClaimTimeout is the default duration a Discovery-
// stream configuration claim (see ClaimConfiguration) remains valid without
// a follow-up configuration request before it lapses. It is deliberately
// this package's own reasoned default, not a value transcribed from the
// TC18 specification text, and is configurable per Server via
// SetConfigurationClaimTimeout.
const DefaultConfigurationClaimTimeout = 30 * time.Second

// configurationClaim is the Discovery-stream claim: a narrower, timeout-
// releasable reservation of configuration rights than AccessController's
// own permanent, un-timed-out root-client claim (see AccessController and
// its doc comment). A held-but-expired claim is treated the same as no
// claim at all everywhere this type is read.
type configurationClaim struct {
	held    bool
	stream  avtp.StreamID
	expires time.Time
}

// active reports whether c is currently a live (held, unexpired) claim as
// of now.
func (c configurationClaim) active(now time.Time) bool {
	return c.held && now.Before(c.expires)
}

// ReadDiscovery returns the whole register map encoded exactly as ReadEP0
// would (EncodeRegisterMap(s.regmap)) — but, unlike ReadEP0, it is
// answerable regardless of the server's current LifecycleState and
// regardless of whether the calling stream holds any AccessController
// grant for EP0. This is Milestone 46's one deliberate, narrowly-scoped
// bypass of the ordinary EP0 access-control gate: see AccessController's
// doc comment and server/doc.go's "EP0 and the root client" section, both
// of which forward-reference exactly this method as the exception
// Milestone 45 intentionally left ungated.
//
// ReadDiscovery takes no requester argument on purpose: per ROADMAP.md
// Milestone 46, discovery is a broadcastable, best-effort read any stream —
// known or not yet known to this server at all — may issue, so there is no
// grant to check and no identity this read itself depends on. (The
// Discovery-stream configuration claim below is a separate mechanism,
// established only once a caller goes on to actually request configuration
// rights, not by the read itself.)
func (s *Server) ReadDiscovery() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return EncodeRegisterMap(s.regmap)
}

// HandleDiscoveryRequest answers a discovery request framed in hdr. Per
// ROADMAP.md Milestone 46, a discovery request must use the untimed (NTSCF)
// AVTPDU header exclusively; hdr.Timed true means the presentation-
// timestamped (TSCF) variant, which HandleDiscoveryRequest rejects outright
// with ErrDiscoveryRequiresUntimedHeader rather than folding it down to
// best-effort execution the way Header.Disposition would for an ordinary
// request — discovery has no scheduled-execution concept for a timestamp to
// usefully express in the first place, so a timed discovery request is
// dropped rather than silently accepted.
func (s *Server) HandleDiscoveryRequest(hdr avtp.Header) ([]byte, error) {
	if hdr.Timed {
		return nil, ErrDiscoveryRequiresUntimedHeader
	}
	return s.ReadDiscovery(), nil
}

// SetConfigurationClaimTimeout overrides the duration a Discovery-stream
// configuration claim stays valid without a follow-up configuration request
// (see ClaimConfiguration). A non-positive d resets it to
// DefaultConfigurationClaimTimeout.
func (s *Server) SetConfigurationClaimTimeout(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d <= 0 {
		d = DefaultConfigurationClaimTimeout
	}
	s.claimTimeout = d
}

// ClaimConfiguration reserves configuration rights for stream, modelling
// ROADMAP.md Milestone 46's "the first discovery-triggered configuration
// attempt reserves configuration rights for its stream" behaviour: a client
// that has just performed a discovery read and now wants to configure this
// server calls ClaimConfiguration before proceeding.
//
// It succeeds — establishing or renewing the reservation for another full
// timeout window from now — if no other stream currently holds an active
// (unexpired) claim, or if stream itself already does (an idempotent
// re-claim that also serves as the "follow-up configuration request" that
// keeps the reservation from lapsing). It returns ErrConfigurationClaimed if
// a different stream currently holds an active claim.
//
// This claim is deliberately narrower than, and independent of,
// AccessController's root-client claim: it does not itself grant or gate
// any read or write, and Server's other configuration methods (ClaimRoot,
// AddEndpoint, WriteEP0, ...) do not consult it — layering a real
// discovery-triggered configuration workflow on top of it (e.g. requiring a
// caller to hold an active claim before ClaimRoot succeeds) is left to the
// Phase 17 control-plane migration (ROADMAP.md Milestone 55) that will
// actually build a client around this mechanism, not pre-decided here. What
// Milestone 46 guarantees is only the reservation/timeout bookkeeping
// itself, and that a read via ReadDiscovery is never blocked by it (see
// ReadDiscovery).
func (s *Server) ClaimConfiguration(stream avtp.StreamID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if s.claim.active(now) && s.claim.stream != stream {
		return ErrConfigurationClaimed
	}
	s.claim = configurationClaim{held: true, stream: stream, expires: now.Add(s.claimTimeout)}
	return nil
}

// ConfigurationClaimant returns the stream currently holding an active
// Discovery-stream configuration claim, and true — or the zero StreamID and
// false if no claim is held or the held claim has expired.
func (s *Server) ConfigurationClaimant() (avtp.StreamID, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if !s.claim.active(now) {
		return avtp.StreamID{}, false
	}
	return s.claim.stream, true
}

// ReleaseConfigurationClaim releases stream's own active configuration
// claim early, before its timeout would otherwise lapse it. It returns
// ErrNotConfigurationClaimant if stream does not currently hold an active
// claim (including if the claim already expired, or was never stream's to
// begin with).
func (s *Server) ReleaseConfigurationClaim(stream avtp.StreamID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if !s.claim.active(now) || s.claim.stream != stream {
		return ErrNotConfigurationClaimant
	}
	s.claim = configurationClaim{}
	return nil
}
