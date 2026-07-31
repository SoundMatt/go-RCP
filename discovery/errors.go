package discovery

import "errors"

// Discovery errors (ROADMAP.md Milestone 46).
var (
	// ErrDiscoveryRequiresUntimedHeader is returned by
	// server.Server.HandleDiscoveryRequest when a discovery request arrives
	// framed in a presentation-timestamped (TSCF) AVTPDU header. Discovery
	// has no scheduled-execution concept, so a timestamped discovery
	// request is dropped outright rather than folded down to best-effort.
	ErrDiscoveryRequiresUntimedHeader = errors.New("rcp/discovery: discovery request must use the untimed AVTPDU header")

	// ErrDiscoveryRequestIsACFGBB is returned by
	// server.Server.HandleDiscoveryRequest when a discovery read arrives
	// framed as ACF_GBB rather than ACF_ABB. Per TC18 §12.6.1 Table 16,
	// "AVTPDUs having a TSCF header are dropped without further response,
	// as well as requests in ACF_GBB format" — the two conditions carry
	// the same disposition (silent drop, no error response), so a caller
	// translating this error is expected to treat it identically to
	// ErrDiscoveryRequiresUntimedHeader (see udp.Router.Route).
	ErrDiscoveryRequestIsACFGBB = errors.New("rcp/discovery: discovery request must use ACF_ABB, not ACF_GBB")

	// ErrConfigurationClaimed is returned by
	// server.Server.ClaimConfiguration when a different stream currently
	// holds an unexpired Discovery-stream configuration claim.
	ErrConfigurationClaimed = errors.New("rcp/discovery: configuration rights are currently reserved by another stream")

	// ErrNotConfigurationClaimant is returned by
	// server.Server.ReleaseConfigurationClaim when the calling stream does
	// not currently hold an active Discovery-stream configuration claim.
	ErrNotConfigurationClaimant = errors.New("rcp/discovery: stream does not hold the active configuration claim")
)
