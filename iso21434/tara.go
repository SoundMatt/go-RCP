package iso21434

// This file is ROADMAP.md Milestone 58 (v0.71.0)'s ADAPT content: the
// generic ImpactRating×FeasibilityRating→RiskValue engine iso21434.go
// implements carries over unchanged (it is a methodology, not tied to any
// particular protocol), but the concrete threat scenarios and
// countermeasure claims below replace what would otherwise still describe
// the retired pre-TC18 Zone/Command protocol's attack surface — this
// package held no populated TARA content at all before this milestone (see
// FORMAL_VERIFICATION.md's sibling reasoning in formal.go's own doc
// comment for why "ADAPT" here means "first real content," not "edit
// existing content").
//
// # Attack surface covered
//
// Every ThreatScenario below is scoped to a concern that exists only in
// the TC18 replacement this program now implements:
//
//   - avtp.StreamID/avtp.ByteBusID addressing and the
//     regmap.AccessController root-client/grant model built on it
//     (T-RCP-001, T-RCP-006) — the pre-TC18 protocol addressed a small,
//     closed enum of Zone values with no analogous claim mechanism at all.
//   - the discovery package's Discovery-stream configuration Claim and its
//     timeout window (T-RCP-002) — Milestone 46; no equivalent existed
//     before it.
//   - the e2e package's CRC32 safe-point mechanism and its automatic
//     safe-state-entry watchdog (T-RCP-003, T-RCP-004, T-RCP-005) —
//     Milestone 50; the retired e2e package's CRC-16/CCITT-FALSE
//     mechanism and the retired watchdog package's client-push keepalive
//     are unrelated designs (see e2e/doc.go's own "A note on this
//     package's name").
//
// # A note on spec fidelity (Guiding Principle 10)
//
// These threat scenarios reason about the *behavior* this repository's own
// packages implement (confirmed by reading their doc comments and source,
// cited by file below), not about the OPEN Alliance TC18 Remote Control
// Protocol Specification v0.5.1_RC's own text, which is confidential to
// OPEN Alliance members. Where a package's own doc comment already flags a
// design choice as this implementation's own reasoned interpretation
// rather than a verified transcription of the spec (e.g. e2e/doc.go's CRC
// polynomial choice), the corresponding threat/countermeasure below
// inherits that same caveat rather than re-asserting it as spec fact.

// BuildTARA returns the Milestone 58 Threat Analysis and Risk Assessment
// for go-RCP's TC18 attack surface. Every ImpactRating/FeasibilityRating
// pair reflects this implementation's own risk judgment, not a value taken
// from the specification (which does not itself perform TARA — ISO/SAE
// 21434 is a process standard applied here, not a protocol requirement).
func BuildTARA() *TARA {
	return &TARA{
		Component: "go-RCP (OPEN Alliance TC18 Remote Control Protocol implementation)",
		Threats: []ThreatScenario{
			{
				ID:             "T-RCP-001",
				Description:    "An attacker on the same network segment sends AVTPDUs presenting a spoofed avtp.StreamID that has already claimed root (regmap.AccessController.ClaimRoot), or a StreamID an operator has granted access to a sensitive endpoint — nothing at the UDP transport layer cryptographically binds a StreamID to the peer that opened the socket presenting it.",
				DamageScenario: "Unauthorized configuration writes (WriteEP0/WriteFunctional/SetPinAssignment/SetQueueConfig) or unauthorized functional-endpoint control while impersonating an already-trusted stream identity.",
				Impact:         ImpactMajor,
				Feasibility:    FeasibilityMedium,
			},
			{
				ID:             "T-RCP-002",
				Description:    "An attacker races the legitimate configuring client during the Discovery-stream configuration Claim's window (discovery.DefaultConfigurationClaimTimeout, 30s) to establish itself as the configuration claimant first, or waits for a legitimate claim to time out and immediately re-claims before the intended operator retries.",
				DamageScenario: "Denial of configuration service to the legitimate operator, or a window in which an attacker-controlled claimant can steer initial server configuration before legitimate root claim/setup completes.",
				Impact:         ImpactModerate,
				Feasibility:    FeasibilityMedium,
			},
			{
				ID:             "T-RCP-003",
				Description:    "An attacker with write access to the wire (see T-RCP-001) recomputes e2e.Compute's CRC32 (crc32.IEEE, a non-cryptographic integrity checksum with a publicly known polynomial and fully documented covered-field layout) over a forged message and appends it, producing a safe point that e2e.Verify accepts as valid.",
				DamageScenario: "A forged safety-request (KindCompoundSafety/KindCompoundWaitSafety/KindTriggeredSafety) or forged plain request passes e2e.Guard's integrity check and reaches the wrapped endpoint, defeating the safe-point mechanism's role as a corruption/injection guard.",
				Impact:         ImpactSevere,
				Feasibility:    FeasibilityHigh,
			},
			{
				ID:             "T-RCP-004",
				Description:    "An attacker suppresses all traffic from a legitimate stream (e.g. selective packet drop) to force e2e.Supervisor.InSafeState past its configured Timeout, triggering request.Dispatcher.PurgeNonSafety and discarding every pending non-safety ticket for that stream as an unwanted side effect of an attacker-induced fault condition, without ever forging a message.",
				DamageScenario: "Denial of service against pending ordinary (non-safety) requests, disguised as a legitimate loss-of-contact safe-state entry rather than an obviously malicious act.",
				Impact:         ImpactModerate,
				Feasibility:    FeasibilityHigh,
			},
			{
				ID:             "T-RCP-005",
				Description:    "An attacker replays a previously observed, validly CRC-protected request (see T-RCP-003's converse: a *genuine* past message, not a forged one) against a stream whose e2e.StreamConfig.RequireMonotonicSequence is not enabled — go-RCP does not enable this per-stream check by default; it is an opt-in configuration choice a deployer must make.",
				DamageScenario: "A stale command (e.g. a superseded PowerState transition or functional-endpoint write) is accepted a second time, producing an unintended state change the original requester did not intend to repeat.",
				Impact:         ImpactMajor,
				Feasibility:    FeasibilityMedium,
			},
			{
				ID:             "T-RCP-006",
				Description:    "An attacker with only network-layer access (no claimed stream identity, no grant of any kind) sends an untimed (NTSCF) discovery read against regmap.EP0 — server.Server.ReadDiscovery/HandleDiscoveryRequest is deliberately answerable 'regardless of the server's current lifecycle.LifecycleState and regardless of whether the calling stream holds any regmap.AccessController grant,' per its own doc comment — to enumerate the server's full register map and topology.",
				DamageScenario: "Information disclosure of the server's endpoint topology, pin mapping, and functional configuration to an unauthenticated network observer, usable as reconnaissance for T-RCP-001/T-RCP-002.",
				Impact:         ImpactModerate,
				Feasibility:    FeasibilityVeryHigh,
			},
		},
	}
}

// BuildGoalRegistry returns the CybersecurityGoals mapped to BuildTARA's
// threat scenarios, marked Satisfied according to what this repository
// actually implements today. A goal marked Satisfied=false is a genuine,
// currently-open gap this milestone surfaces rather than papers over — see
// each goal's Claim for the specific package (if any) a future milestone
// would extend to close it.
func BuildGoalRegistry() *GoalRegistry {
	r := NewGoalRegistry()
	r.Add(CybersecurityGoal{
		ID:        "CG-RCP-001",
		ThreatID:  "T-RCP-001",
		Claim:     "Server-side access control (regmap.AccessController's root-client/grant model, fronted by udp.EP0Handler) and a client-side complementary policy layer (authz.Policy) gate every configuration and functional-endpoint request by StreamID — but neither layer authenticates that a given AVTPDU's carried StreamID genuinely originates from the peer that opened it; this is unmitigated at the transport layer this milestone builds (tlstransport, this repo's bespoke mutual-TLS-over-TCP option, was retired outright at ROADMAP.md Milestone 59 alongside the Zone/Command API it depended on and would not have mitigated this threat regardless, since mutual-TLS-over-TCP does not fit the stream_id/byte_bus_id addressing model at all — link-layer authentication is a MACsec/802.1AE concern this repository does not implement).",
		Satisfied: false,
	})
	r.Add(CybersecurityGoal{
		ID:        "CG-RCP-002",
		ThreatID:  "T-RCP-002",
		Claim:     "The configuration claim is bounded by discovery.DefaultConfigurationClaimTimeout (30s) rather than held indefinitely, limiting an attacker's denial-of-service window to that duration per attempt, and ReadDiscovery itself is never blocked by claim state so a legitimate operator can always observe server identity/topology while contesting a claim.",
		Satisfied: true,
	})
	r.Add(CybersecurityGoal{
		ID:        "CG-RCP-003",
		ThreatID:  "T-RCP-003",
		Claim:     "e2e.Guard/e2e.Verify reject any message whose trailing CRC32 safe point does not match, with a dedicated ErrCRCMismatch and no execution of the wrapped Handler — but CRC32 is an integrity check against corruption, not an authenticity check against a capable attacker who can compute the same public polynomial over a forged message; closing this gap needs a keyed/asymmetric authentication mechanism this package does not provide.",
		Satisfied: false,
	})
	r.Add(CybersecurityGoal{
		ID:        "CG-RCP-004",
		ThreatID:  "T-RCP-004",
		Claim:     "request.Dispatcher.PurgeNonSafety by design never touches KindCompoundSafety/KindCompoundWaitSafety/KindTriggeredSafety tickets, so the induced-safe-state DoS in T-RCP-004 can only ever discard non-safety work, never a safety-critical one — bounding the damage scenario's severity to Moderate rather than Major/Severe.",
		Satisfied: true,
	})
	r.Add(CybersecurityGoal{
		ID:        "CG-RCP-005",
		ThreatID:  "T-RCP-005",
		Claim:     "e2e.StreamConfig.RequireMonotonicSequence, when a deployer enables it per-stream via Supervisor.Configure, rejects (ErrSequenceViolation) and stickily trips InSafeState on any non-monotonic arrival, defeating simple replay — but this is opt-in, not the package's default, so an unconfigured stream remains exposed.",
		Satisfied: false,
	})
	r.Add(CybersecurityGoal{
		ID:        "CG-RCP-006",
		ThreatID:  "T-RCP-006",
		Claim:     "This is a deliberate specification-driven design choice (Milestone 46: discovery must be answerable by an unauthenticated, not-yet-known stream), not an oversight, so no code change closes it; the accepted residual risk is that discovery responses must be treated as public information at deployment/network-segmentation time, not gated by this library.",
		Satisfied: false,
	})
	return r
}
