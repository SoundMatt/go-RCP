package wakeup

import "github.com/SoundMatt/go-RCP/server"

// EndpointType re-exports server.EndpointTypeWakeup so a caller that only
// imports this package doesn't also need to import server just to declare
// a Wakeup endpoint's type with server.Server.AddEndpoint.
const EndpointType = server.EndpointTypeWakeup

// PowerState is one of this package's four recognized whole-server power
// states. See doc.go's Scope section for which of these Endpoint actually
// transitions into/out of, and why PowerUnpowered is the one exception.
type PowerState uint8

const (
	// PowerNormal is full operation.
	PowerNormal PowerState = iota

	// PowerStandBy is a reduced-power state a caller can request and
	// resume from directly (no cold/hot-start distinction applies to it —
	// that distinction is specific to waking from PowerSleep, see doc.go).
	PowerStandBy

	// PowerSleep is the deepest software-visible low-power state Endpoint
	// itself drives. Waking from it back to PowerNormal is the transition
	// that triggers the cold/hot-start determination and the repeating
	// wake-handshake message (see doc.go).
	PowerSleep

	// PowerUnpowered represents total power loss. Per doc.go's Scope
	// section, this is never Endpoint's own current state and never a
	// write request's target — it exists in this enumeration only so a
	// caller has a name for the state a server was straightforwardly in
	// immediately before this process itself started running.
	PowerUnpowered

	powerStateCount // sentinel; keep last
)

// Valid reports whether s is one of this package's four recognized power
// states.
func (s PowerState) Valid() bool {
	return s < powerStateCount
}

// String renders s for logs and diagnostics.
func (s PowerState) String() string {
	switch s {
	case PowerNormal:
		return "Normal"
	case PowerStandBy:
		return "StandBy"
	case PowerSleep:
		return "Sleep"
	case PowerUnpowered:
		return "Unpowered"
	default:
		return "Unknown"
	}
}

// StartKind distinguishes how a wake from PowerSleep back to PowerNormal
// resumed, per doc.go's Scope section.
type StartKind uint8

const (
	// StartUnknown is Endpoint's initial value before any Sleep→Normal
	// transition has ever occurred.
	StartUnknown StartKind = iota

	// StartHot means retained context survived the just-ended sleep
	// period.
	StartHot

	// StartCold means retained context was lost during the just-ended
	// sleep period (see Endpoint.SetRetentionLost).
	StartCold
)

// String renders k for logs and diagnostics.
func (k StartKind) String() string {
	switch k {
	case StartHot:
		return "Hot"
	case StartCold:
		return "Cold"
	default:
		return "Unknown"
	}
}

// WakeHandshake is one instance of the repeating message a server sends
// while waking from PowerSleep, per doc.go's Scope section. Sequence
// numbers a single wake cycle produces start at 0 and increment by one for
// each successive WakeHandshake, so a receiver can dedupe repeats.
type WakeHandshake struct {
	Start    StartKind
	Sequence uint16
}
