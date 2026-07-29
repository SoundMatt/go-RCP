// Package mock provides in-process test doubles for go-RCP's TC18
// server/endpoint/register-map model: Endpoint (implements
// request.Handler, see endpoint.go), Client (an in-process fake of
// *udp.Controller that calls a *udp.Router's own Route method directly, see
// client.go), ClientRegistry (client.go's caller-keyed collection, see
// client_registry.go), and Fixture (a Server+Router+root-Client bundle, see
// fixture.go).
//
// Through ROADMAP.md Milestone 58 (v0.70.0) this package also carried a
// second, frozen fake — Controller/Registry/Handler — implementing the
// pre-TC18 bespoke rcp.Controller/rcp.Registry interfaces this repo has
// since replaced. That surface was deliberately kept byte-for-byte
// unchanged from Milestone 57 through Milestone 58 because cmd/go-rcp,
// cmd/rcptool, optional_test.go, adapt_test.go, and
// safety/command_latency_test.go still depended on it; Milestone 59 (v1.0.0,
// Phase 18 "Cutover") moved every one of those onto Client/Endpoint/
// ClientRegistry/Fixture above and deleted the frozen fake, since the
// rcp.Controller/rcp.Registry interfaces it implemented no longer exist.
package mock
