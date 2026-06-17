//fusa:req REQ-ZONE-001
//fusa:req REQ-ZONE-002
//fusa:req REQ-ZONE-003
//fusa:req REQ-ZONE-004
//fusa:req REQ-ZONE-005
//fusa:req REQ-ZONE-006
//fusa:req REQ-ZONE-007
//fusa:req REQ-ZONE-008
//fusa:req REQ-PRI-001
//fusa:req REQ-PRI-002
//fusa:req REQ-PRI-003
//fusa:req REQ-CMD-001
//fusa:req REQ-CMD-002
//fusa:req REQ-CMD-003
//fusa:req REQ-CMD-004
//fusa:req REQ-CMD-005
//fusa:req REQ-CMD-006
//fusa:req REQ-STATUS-001
//fusa:req REQ-STATUS-002
//fusa:req REQ-STATUS-003
//fusa:req REQ-STATUS-004
//fusa:req REQ-STATUS-005
//fusa:req REQ-STATUS-006
//fusa:req REQ-ERR-001
//fusa:req REQ-ERR-002
//fusa:req REQ-ERR-003
//fusa:req REQ-ERR-004
//fusa:req REQ-ERR-005
//fusa:req REQ-ERR-006
//fusa:req REQ-ERR-007
//fusa:req REQ-ERR-008
//fusa:req REQ-ERR-009
//fusa:req REQ-ERR-010
//fusa:req REQ-ERR-011
//fusa:req REQ-CMDSTRUCT-001
//fusa:req REQ-CMDSTRUCT-002
//fusa:req REQ-RESP-001
//fusa:req REQ-RESP-002
//fusa:req REQ-RESP-003
//fusa:req REQ-STAT-001
//fusa:req REQ-STAT-002
//fusa:req REQ-STAT-003
//fusa:req REQ-STAT-004
//fusa:req REQ-STAT-005

// Package rcp provides the Remote Control Protocol for automotive zonal architecture.
//
// A central high-performance computer uses a Registry to discover zone controllers,
// then dispatches Commands to each Controller and receives Responses and Status
// telemetry in return.
package rcp

import (
	"context"
	"fmt"

	relay "github.com/SoundMatt/RELAY"
)

// SpecVersion is the RELAY specification version this package implements.
//
//fusa:req REQ-SPEC-001
const SpecVersion = relay.SpecVersion

// wrapErr holds a clean error message while maintaining an Unwrap chain
// so errors.Is traversal reaches RELAY sentinels.
type wrapErr struct {
	msg    string
	parent error
}

func (e *wrapErr) Error() string { return e.msg }
func (e *wrapErr) Unwrap() error { return e.parent }

// Mandatory RELAY sentinels (spec §5.1). Each wraps the corresponding
// relay package sentinel so errors.Is(err, relay.ErrXxx) returns true.
//
//fusa:req REQ-ERR-001
//fusa:req REQ-ERR-012
//fusa:req REQ-ERR-013
//fusa:req REQ-ERR-014
//fusa:req REQ-ERR-015
//fusa:req REQ-ERR-016
//fusa:req REQ-ERR-017
var (
	ErrClosed          = &wrapErr{"rcp: controller closed", relay.ErrClosed}
	ErrNotConnected    = &wrapErr{"rcp: not connected", relay.ErrNotConnected}
	ErrTimeout         = &wrapErr{"rcp: command timeout", relay.ErrTimeout}
	ErrPayloadTooLarge = &wrapErr{"rcp: payload too large", relay.ErrPayloadTooLarge}
)

// Protocol-specific sentinels (spec §5.4). Each wraps the appropriate
// mandatory sentinel so errors.Is traversal works at both levels.
//
//fusa:req REQ-ERR-002
//fusa:req REQ-ERR-003
//fusa:req REQ-ERR-004
//fusa:req REQ-ERR-005
//fusa:req REQ-ERR-006
//fusa:req REQ-ERR-007
//fusa:req REQ-ERR-018
//fusa:req REQ-ERR-019
//fusa:req REQ-ERR-020
//fusa:req REQ-ERR-021
var (
	ErrNotFound      = &wrapErr{"rcp: zone not found", ErrNotConnected}
	ErrAlreadyExists = &wrapErr{"rcp: zone already registered", ErrClosed}
	ErrBusy          = &wrapErr{"rcp: zone controller busy", ErrTimeout}
	ErrZoneMismatch  = &wrapErr{"rcp: zone mismatch", ErrNotConnected}
)

// Zone identifies a physical zone in the vehicle.
type Zone uint8

const (
	ZoneUnknown    Zone = 0
	ZoneFrontLeft  Zone = 1
	ZoneFrontRight Zone = 2
	ZoneRearLeft   Zone = 3
	ZoneRearRight  Zone = 4
	ZoneCentral    Zone = 5
)

// String returns a human-readable zone name.
func (z Zone) String() string {
	switch z {
	case ZoneFrontLeft:
		return "front-left"
	case ZoneFrontRight:
		return "front-right"
	case ZoneRearLeft:
		return "rear-left"
	case ZoneRearRight:
		return "rear-right"
	case ZoneCentral:
		return "central"
	default:
		return "unknown"
	}
}

// Priority determines command scheduling priority within a zone controller.
type Priority uint8

const (
	PriorityNormal   Priority = 0
	PriorityHigh     Priority = 1
	PriorityCritical Priority = 2
)

// CommandType classifies the intent of a command.
type CommandType uint16

const (
	CmdNoop     CommandType = 0 // keepalive / no-op
	CmdSet      CommandType = 1 // set an output or actuator
	CmdGet      CommandType = 2 // query current state
	CmdReset    CommandType = 3 // reset zone controller
	CmdWatchdog CommandType = 4 // watchdog kick
	CmdSleep    CommandType = 5 // request zone controller to enter low-power sleep
	CmdWake     CommandType = 6 // request zone controller to exit sleep and resume active operation
)

// ResponseStatus reports the outcome of a command execution.
type ResponseStatus uint8

const (
	StatusOK      ResponseStatus = 0
	StatusError   ResponseStatus = 1
	StatusTimeout ResponseStatus = 2
	StatusBusy    ResponseStatus = 3
	StatusUnknown ResponseStatus = 4
)

// String returns a human-readable status string.
func (s ResponseStatus) String() string {
	switch s {
	case StatusOK:
		return "OK"
	case StatusError:
		return "error"
	case StatusTimeout:
		return "timeout"
	case StatusBusy:
		return "busy"
	default:
		return "unknown"
	}
}

// Command is a control message dispatched to a zone controller.
type Command struct {
	ID       uint32      `json:"id"`
	Zone     Zone        `json:"zone"`
	Type     CommandType `json:"type"`
	Priority Priority    `json:"priority"`
	Payload  []byte      `json:"payload,omitempty"`
}

// Response is the acknowledgement returned by a zone controller.
type Response struct {
	CommandID uint32         `json:"command_id"`
	Zone      Zone           `json:"zone"`
	Status    ResponseStatus `json:"status"`
	Payload   []byte         `json:"payload,omitempty"`
}

// Status is a periodic telemetry update published by a zone controller.
type Status struct {
	Zone    Zone   `json:"zone"`
	Seq     uint32 `json:"seq"`
	Healthy bool   `json:"healthy"`
	Payload []byte `json:"payload,omitempty"`
}

// ZoneFromString returns the Zone constant matching the name returned by Zone.String().
// Returns (ZoneUnknown, ErrNotFound) for unrecognised strings.
//
//fusa:req REQ-MSG-001
//fusa:req REQ-MSG-002
func ZoneFromString(s string) (Zone, error) {
	switch s {
	case "front-left":
		return ZoneFrontLeft, nil
	case "front-right":
		return ZoneFrontRight, nil
	case "rear-left":
		return ZoneRearLeft, nil
	case "rear-right":
		return ZoneRearRight, nil
	case "central":
		return ZoneCentral, nil
	default:
		return ZoneUnknown, fmt.Errorf("rcp: unknown zone %q: %w", s, ErrNotFound)
	}
}

// Controller is the interface to a single zone controller endpoint.
type Controller interface {
	// Zone returns the zone this controller manages.
	Zone() Zone

	// Send dispatches a command and waits for the response.
	// Returns ErrClosed if the controller has been closed.
	// Returns ErrTimeout if ctx expires before a response arrives.
	// Returns ErrZoneMismatch if cmd.Zone does not equal the controller's zone.
	Send(ctx context.Context, cmd *Command) (*Response, error)

	// Subscribe returns a channel of periodic Status updates.
	// The channel is closed when ctx is cancelled or the controller closes.
	Subscribe(ctx context.Context) (<-chan *Status, error)

	// Close releases all resources held by the controller.
	// Safe to call multiple times.
	Close() error
}

// Loan is a payload buffer borrowed from a LoaningController's pool.
// The caller MUST either pass it to LoaningController.SendLoaned (transferring ownership)
// or call Return to release it back to the pool.
type Loan struct {
	Payload []byte
	release func()
}

// Return releases the Loan back to the pool without sending.
// Must not be called after the Loan has been passed to SendLoaned.
func (l *Loan) Return() {
	if l.release != nil {
		l.release()
	}
}

// NewLoan creates a Loan with the given payload and release function.
// Intended for use by LoaningController implementations in external packages.
func NewLoan(payload []byte, release func()) *Loan {
	return &Loan{Payload: payload, release: release}
}

// LoaningController extends Controller with zero-copy payload loaning.
// Transports that implement this interface allow the caller to obtain a
// pre-allocated buffer, fill it in-place, and send it with no extra copy.
type LoaningController interface {
	Controller
	// Loan returns a zeroed payload buffer of exactly size bytes.
	// Returns ErrClosed if the controller is closed.
	Loan(size int) (*Loan, error)
	// SendLoaned sends cmd whose Payload is the buffer from a prior Loan call.
	// Ownership of cmd.Payload transfers to the transport on return.
	// The caller must not access cmd.Payload after this call returns.
	SendLoaned(ctx context.Context, cmd *Command) (*Response, error)
}

// Registry discovers and manages a set of zone controllers.
type Registry interface {
	// Register adds a controller to the registry.
	// Returns ErrAlreadyExists if a controller for the same zone is already registered.
	Register(ctrl Controller) error

	// Deregister removes and closes the controller for the given zone.
	// Returns ErrNotFound if the zone is not registered.
	Deregister(zone Zone) error

	// Lookup returns the controller for the given zone.
	// Returns ErrNotFound if no controller is registered for the zone.
	Lookup(zone Zone) (Controller, error)

	// Controllers returns all currently registered controllers.
	Controllers() []Controller

	// Close closes all registered controllers and releases registry resources.
	// Safe to call multiple times.
	Close() error
}
