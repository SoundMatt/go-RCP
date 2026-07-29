package mock

import "errors"

// Errors returned by this package's TC18 test doubles (Endpoint, Client,
// ClientRegistry, Fixture). The legacy Controller/Registry in mock.go
// continue to return the root package's own rcp.Err* sentinels, unchanged
// (see mock.go's Deprecated note).
var (
	// ErrClosed is returned by Endpoint/Client/ClientRegistry methods once
	// Close has been called.
	ErrClosed = errors.New("rcp/mock: closed")

	// ErrWrongEndpoint is returned by Endpoint.HandleRequest when handed a
	// request addressed to a different byte_bus_id than the Endpoint
	// answers for.
	ErrWrongEndpoint = errors.New("rcp/mock: request addressed to a different endpoint")

	// ErrDropped is returned by Client.Request when the in-process Router
	// it addresses drops the request outright (avtp.DispositionDrop — see
	// udp.Router.Route), the same "no reply at all" case a real transport's
	// Controller would observe as a request that silently never gets a
	// response.
	ErrDropped = errors.New("rcp/mock: request dropped (avtp.DispositionDrop)")

	// ErrAlreadyExists is returned by ClientRegistry.Register when key is
	// already registered.
	ErrAlreadyExists = errors.New("rcp/mock: registry key already registered")

	// ErrNotFound is returned by ClientRegistry when a lookup key is not
	// registered.
	ErrNotFound = errors.New("rcp/mock: registry key not registered")
)
