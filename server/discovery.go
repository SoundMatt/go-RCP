package server

import (
	"time"

	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/discovery"
	"github.com/SoundMatt/go-RCP/v9/regmap"
)

// This file is Server's orchestration half of the discovery package (see
// discovery/doc.go): the mutex-guarded, clock-driven bookkeeping around a
// discovery.Claim that Claim itself deliberately does not do. It implements
// ROADMAP.md Milestone 46 (Discovery, v0.59.0).

// ReadDiscovery returns the whole register map encoded exactly as ReadEP0
// would (regmap.EncodeRegisterMap(s.rmap)) — but, unlike ReadEP0, it is
// answerable regardless of the server's current lifecycle.LifecycleState
// and regardless of whether the calling stream holds any
// regmap.AccessController grant for EP0. This is Milestone 46's one
// deliberate, narrowly-scoped bypass of the ordinary EP0 access-control
// gate: see regmap.AccessController's doc comment and server/doc.go's "EP0
// and the root client" section, both of which forward-reference exactly
// this method as the exception Milestone 45 intentionally left ungated.
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
	return regmap.EncodeRegisterMap(s.rmap)
}

// HandleDiscoveryRequest answers a discovery request framed in hdr. Per
// ROADMAP.md Milestone 46, a discovery request must use the untimed (NTSCF)
// AVTPDU header exclusively; hdr.Timed true means the presentation-
// timestamped (TSCF) variant, which HandleDiscoveryRequest rejects outright
// with discovery.ErrDiscoveryRequiresUntimedHeader rather than folding it
// down to best-effort execution the way Header.Disposition would for an
// ordinary request — discovery has no scheduled-execution concept for a
// timestamp to usefully express in the first place, so a timed discovery
// request is dropped rather than silently accepted.
//
// isACFGBB reports whether the originating acf.Message used the ACF_GBB
// encoding (acf.KindLong) rather than ACF_ABB. Per TC18 §12.6.1 Table 16,
// an ACF_GBB-framed discovery request gets the same disposition as a timed
// header: dropped without further response — HandleDiscoveryRequest returns
// discovery.ErrDiscoveryRequestIsACFGBB for that case, which a caller
// (udp.Router.Route) must treat identically to the timed-header rejection,
// i.e. as a silent drop, not a wire-level error response. This method takes
// a plain bool rather than an acf.MessageKind so this package does not need
// to depend on the acf package for one call site.
func (s *Server) HandleDiscoveryRequest(hdr avtp.Header, isACFGBB bool) ([]byte, error) {
	if hdr.Timed {
		return nil, discovery.ErrDiscoveryRequiresUntimedHeader
	}
	if isACFGBB {
		return nil, discovery.ErrDiscoveryRequestIsACFGBB
	}
	return s.ReadDiscovery(), nil
}

// SetConfigurationClaimTimeout overrides the duration a Discovery-stream
// configuration claim stays valid without a follow-up configuration request
// (see ClaimConfiguration). A non-positive d resets it to
// discovery.DefaultConfigurationClaimTimeout.
func (s *Server) SetConfigurationClaimTimeout(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d <= 0 {
		d = discovery.DefaultConfigurationClaimTimeout
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
// keeps the reservation from lapsing). It returns
// discovery.ErrConfigurationClaimed if a different stream currently holds
// an active claim.
//
// This claim is deliberately narrower than, and independent of,
// regmap.AccessController's root-client claim: it does not itself grant or
// gate any read or write, and Server's other configuration methods
// (ClaimRoot, AddEndpoint, WriteEP0, ...) do not consult it — layering a
// real discovery-triggered configuration workflow on top of it (e.g.
// requiring a caller to hold an active claim before ClaimRoot succeeds) is
// left to the Phase 17 control-plane migration (ROADMAP.md Milestone 55)
// that will actually build a client around this mechanism, not pre-decided
// here. What Milestone 46 guarantees is only the reservation/timeout
// bookkeeping itself, and that a read via ReadDiscovery is never blocked by
// it (see ReadDiscovery).
func (s *Server) ClaimConfiguration(stream avtp.StreamID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if s.claim.Active(now) && s.claim.Stream != stream {
		return discovery.ErrConfigurationClaimed
	}
	s.claim = discovery.Claim{Held: true, Stream: stream, Expires: now.Add(s.claimTimeout)}
	return nil
}

// ConfigurationClaimant returns the stream currently holding an active
// Discovery-stream configuration claim, and true — or the zero StreamID and
// false if no claim is held or the held claim has expired.
func (s *Server) ConfigurationClaimant() (avtp.StreamID, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if !s.claim.Active(now) {
		return avtp.StreamID{}, false
	}
	return s.claim.Stream, true
}

// ReleaseConfigurationClaim releases stream's own active configuration
// claim early, before its timeout would otherwise lapse it. It returns
// discovery.ErrNotConfigurationClaimant if stream does not currently hold
// an active claim (including if the claim already expired, or was never
// stream's to begin with).
func (s *Server) ReleaseConfigurationClaim(stream avtp.StreamID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if !s.claim.Active(now) || s.claim.Stream != stream {
		return discovery.ErrNotConfigurationClaimant
	}
	s.claim = discovery.Claim{}
	return nil
}
