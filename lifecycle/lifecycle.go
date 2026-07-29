package lifecycle

// LifecycleState is one of the RC Server's three configuration states. A
// server advances forward one state at a time, only when the guard
// condition for that transition passes — see server.Server.AdvanceToHWLocked
// and server.Server.AdvanceToFullyConfigured — and can additionally move
// backward exactly once, from StateHWLocked to StateUnconfigured, via
// server.Server.DemoteToUnconfigured. There is no path back to
// StateUnconfigured or StateHWLocked once StateFullyConfigured is reached.
type LifecycleState uint8

const (
	// StateUnconfigured is the server's bare-defaults starting state: no
	// hardware pin mapping has been locked in, and endpoints may still be
	// declared or redeclared.
	StateUnconfigured LifecycleState = iota

	// StateHWLocked is reached once the hardware pin-mapping table has
	// passed its plausibility check and been locked. Endpoint declarations
	// and the pin-mapping table are now frozen; each endpoint's functional
	// (type-specific) configuration block, and the request-stream/queue
	// configuration tables, are still writable. The root client, or
	// whichever stream currently holds the still-open discovery-stream
	// configuration claim, may demote the server back to
	// StateUnconfigured from here — see server.Server.DemoteToUnconfigured.
	StateHWLocked

	// StateFullyConfigured is reached once every declared endpoint has a
	// plausible functional configuration block and the queue configuration
	// itself passes its own plausibility check. The general/HW-pin block and
	// the request-stream/queue configuration tables are permanently locked
	// from this point on, for every requester, including the root client —
	// there is no path back to an earlier state. Each endpoint's functional
	// configuration block remains writable, via that endpoint's own
	// registered stream(s) or via the root client through EP0 — see
	// server.Server.WriteFunctional and server.Server.WriteEP0.
	StateFullyConfigured
)

// String renders s for logs and diagnostics.
func (s LifecycleState) String() string {
	switch s {
	case StateUnconfigured:
		return "unconfigured"
	case StateHWLocked:
		return "hw-locked"
	case StateFullyConfigured:
		return "fully-configured"
	default:
		return "unknown"
	}
}
