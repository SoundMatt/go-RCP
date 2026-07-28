package server

// LifecycleState is one of the RC Server's three configuration states. A
// server only ever advances forward, one state at a time, and only when the
// guard condition for that transition passes — see Server.AdvanceToHWLocked
// and Server.AdvanceToFullyConfigured.
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
	// configuration tables, are still writable.
	StateHWLocked

	// StateFullyConfigured is reached once every declared endpoint has a
	// plausible functional configuration block and the queue configuration
	// itself passes its own plausibility check. Every register field is
	// permanently locked from this point on, for every requester —
	// including the root client.
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
