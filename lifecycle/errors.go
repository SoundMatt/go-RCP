package lifecycle

import "errors"

// Transition-guard errors.
var (
	// ErrLifecycleOutOfOrder is returned when a caller asks the server to
	// advance to a lifecycle state that does not immediately follow its
	// current one. States only ever advance one step at a time.
	ErrLifecycleOutOfOrder = errors.New("rcp/lifecycle: lifecycle states must advance one step at a time")

	// ErrFunctionalBlockIncomplete is returned when advancing to the fully-
	// configured state while at least one declared endpoint still has an
	// empty functional (type-specific) configuration block.
	ErrFunctionalBlockIncomplete = errors.New("rcp/lifecycle: endpoint has no functional configuration set")

	// ErrDemotionNotAuthorized is returned when a stream that is neither the
	// root client nor the current Discovery-stream configuration claimant
	// asks to demote the server from StateHWLocked back to
	// StateUnconfigured. See server.Server.DemoteToUnconfigured.
	ErrDemotionNotAuthorized = errors.New("rcp/lifecycle: demotion requires the root client or the active discovery-stream claimant")
)
