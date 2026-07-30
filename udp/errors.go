package udp

import "errors"

// Transport-lifecycle errors, mirroring the sentinel-error posture the
// retired udp package and every sibling transport (shmem, tlstransport) in
// this repo already used.
var (
	// ErrClosed is returned by Controller/Server methods once Close has
	// been called.
	ErrClosed = errors.New("rcp/udp: transport closed")

	// ErrTimeout is returned when a Controller request's context is
	// cancelled or expires before a matching response arrives.
	ErrTimeout = errors.New("rcp/udp: request timed out")

	// ErrAlreadyExists is returned by Registry when a key is already
	// registered.
	ErrAlreadyExists = errors.New("rcp/udp: registry key already registered")

	// ErrNotFound is returned by Registry when a lookup key is not
	// registered.
	ErrNotFound = errors.New("rcp/udp: registry key not registered")
)

// Router/dispatch errors.
var (
	// ErrUnknownEndpoint is the error Router.Route reports (as a wire-level
	// error response, see doc.go) when a request's ByteBusID has no
	// registered Handler.
	ErrUnknownEndpoint = errors.New("rcp/udp: no handler registered for byte_bus_id")

	// ErrDuplicateEndpoint is returned by Router.Register when addr already
	// has a registered Handler.
	ErrDuplicateEndpoint = errors.New("rcp/udp: byte_bus_id already has a registered handler")

	// ErrReservedAddress is returned by Router.Register for an attempt to
	// register a Handler at regmap.EP0 — EP0 is always answered by the
	// Router's own EP0Handler (see ep0.go), never by a caller-registered
	// Handler.
	ErrReservedAddress = errors.New("rcp/udp: byte_bus_id 0 (EP0) cannot have a registered handler")

	// ErrRequestMustReadOrWrite is returned by EP0Handler when a request
	// sets neither acf.FlagRead nor acf.FlagWrite, mirroring every Phase 14
	// endpoint type's own identically-named error for the same condition.
	ErrRequestMustReadOrWrite = errors.New("rcp/udp: EP0 request must set the Read or Write control flag")

	// ErrWrongEndpoint is returned by EP0Handler when handed a request
	// whose ByteBusID is not regmap.EP0 — a Router-internal invariant that
	// should never observably fire outside a misuse of EP0Handler directly.
	ErrWrongEndpoint = errors.New("rcp/udp: request addressed to a different endpoint than EP0Handler serves")

	// ErrShortBuffer is returned by DecodeErrorBody when handed an empty
	// buffer — there is no leading ErrorCode byte to read.
	ErrShortBuffer = errors.New("rcp/udp: buffer too short")
)
