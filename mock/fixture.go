package mock

//fusa:req REQ-MFX-001
//fusa:req REQ-MFX-002
//fusa:req REQ-MFX-003

import (
	"fmt"

	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/server"
	"github.com/SoundMatt/go-RCP/udp"
)

// Fixture bundles an in-process server.Server, its Router, and a Client
// already claimed as the server's root client — the "everything wired up"
// starting point most unit tests targeting Phases 13-16 need, in place of
// this package's legacy NewRegistry's fixed five-Zone pre-population (see
// mock.go's Deprecated note): this milestone's model has no fixed endpoint
// set to pre-populate, so a Fixture starts with zero declared endpoints and
// a caller adds its own via Fixture.Server.AddEndpoint and
// Fixture.Router.Register (typically registering a *mock.Endpoint, or a
// real Phase 14/16 endpoint-type package's own Endpoint).
type Fixture struct {
	Server *server.Server
	Router *udp.Router
	Root   *Client
}

// NewFixture returns a Fixture whose Server has claimed rootStream as its
// root client, and whose Router answers timestamped (TSCF) requests per
// timeSyncSupported (see avtp.Header.Disposition).
func NewFixture(rootStream avtp.StreamID, timeSyncSupported bool) (*Fixture, error) {
	srv := server.NewServer()
	if err := srv.ClaimRoot(rootStream); err != nil {
		return nil, fmt.Errorf("rcp/mock: fixture: %w", err)
	}
	router := udp.NewRouter(udp.NewEP0Handler(srv), timeSyncSupported)
	return &Fixture{
		Server: srv,
		Router: router,
		Root:   NewClient(rootStream, router),
	}, nil
}

// Close closes Root. The Fixture's Server and Router hold no resources of
// their own to release.
func (f *Fixture) Close() error {
	return f.Root.Close()
}
