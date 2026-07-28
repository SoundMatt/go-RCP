// Package discovery implements the RC Server discovery mechanism for the
// OPEN Alliance TC18 Remote Control Protocol (RCP), as described by the
// "OPEN Alliance TC18 Remote Control Protocol Specification v0.5.1_RC": a
// register-0 read answerable in any lifecycle.LifecycleState and regardless
// of regmap.AccessController grants, a Discovery-stream configuration Claim
// that coexists with (but is distinct from) the regmap package's
// root-client claim, and the client-side helpers for recognizing a
// conformant server and persisting its discovered Topology.
//
// # A note on this package's history and shape (RELAY spec v1.14 §13.7.2)
//
// Through v0.66.0 this mechanism lived directly in the server package
// alongside the lifecycle state machine (lifecycle) and the register-map
// model (regmap). RELAY spec v1.14's §13.7.2 cross-language module-name
// registry distinguishes all three as separate concerns — cpp-RCP,
// rust-RCP, and c-RCP already split them into three modules on that basis
// — so this package was split out to match. This package holds the pure
// data model: Claim and its Active check (claim.go), and the
// IsConformantServer/Topology/DiscoverTopology/WriteTopology/ReadTopology
// helpers (topology.go). The mutex-guarded, clock-driven orchestration
// around a Claim — ReadDiscovery, HandleDiscoveryRequest,
// SetConfigurationClaimTimeout, ClaimConfiguration, ConfigurationClaimant,
// ReleaseConfigurationClaim — stays on server.Server, since that bookkeeping
// needs the Server's own injectable clock and lock (see server/discovery.go);
// Claim itself deliberately does not read the clock or hold a lock, mirroring
// how request.SafeStateCheck is a policy a caller supplies rather than
// something the request package computes itself.
package discovery
