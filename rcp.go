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
	"errors"
)

// Sentinel errors returned by all rcp implementations.
var (
	ErrClosed        = errors.New("rcp: controller closed")
	ErrNotFound      = errors.New("rcp: zone not found")
	ErrAlreadyExists = errors.New("rcp: zone already registered")
	ErrTimeout       = errors.New("rcp: command timeout")
	ErrBusy          = errors.New("rcp: zone controller busy")
	ErrZoneMismatch  = errors.New("rcp: zone mismatch")
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
	ID       uint32
	Zone     Zone
	Type     CommandType
	Priority Priority
	Payload  []byte
}

// Response is the acknowledgement returned by a zone controller.
type Response struct {
	CommandID uint32
	Zone      Zone
	Status    ResponseStatus
	Payload   []byte
}

// Status is a periodic telemetry update published by a zone controller.
type Status struct {
	Zone    Zone
	Seq     uint32
	Healthy bool
	Payload []byte
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
