# Threat Analysis and Risk Assessment

**Project:** go-RCP
**Standard:** ISO/SAE 21434 (Road Vehicle Cybersecurity Engineering)
**Document ID:** TARA-002 (supersedes any TARA content implied for the
retired pre-TC18 Zone/Command protocol at Milestone 42/v0.42.0 — that
milestone's own `TARA.md`/`CYBERSECURITY.md`/`iec62443-gap-report.json`
deliverables were never actually committed to this repository; the
`iso21434` package itself was, until this milestone, an unpopulated
generic engine — see `iso21434/tara.go`'s own doc comment)
**Version:** 1.0
**Date:** see the PR that introduced this document

Source of truth: `iso21434/tara.go` (`BuildTARA`, `BuildGoalRegistry`).
This document summarizes that content for readers who want the TARA
without reading Go source; the code is authoritative if the two ever
diverge.

---

## Component

go-RCP: this repository's implementation of the OPEN Alliance TC18 Remote
Control Protocol Specification v0.5.1_RC.

## Methodology

Risk = Impact × Attack-Feasibility, both rated 1–4 per the scale
`iso21434.ComputeRisk` implements (Negligible/Moderate/Major/Severe ×
Low/Medium/High/VeryHigh → Low/Medium/High/Critical). This methodology
carries over unchanged from before this milestone — see
`iso21434/iso21434.go`; only the concrete threat scenarios and
countermeasure mapping below are new.

## Threat scenarios and risk

| ID | Description (summary) | Impact | Feasibility | Risk level |
|----|------------------------|--------|-------------|------------|
| T-RCP-001 | StreamID spoofing / claim impersonation (no transport-layer peer authentication) | Major | Medium | High |
| T-RCP-002 | Discovery-claim window race/hijack (30s claim timeout) | Moderate | Medium | Medium |
| T-RCP-003 | CRC32 safe-point forgery (non-cryptographic checksum, public polynomial) | Severe | High | Critical |
| T-RCP-004 | Induced safe-state entry via traffic suppression (non-safety-ticket DoS) | Moderate | High | Medium |
| T-RCP-005 | Replay of a genuine past request when monotonicity checking is not enabled | Major | Medium | High |
| T-RCP-006 | Unauthenticated discovery read (topology/config disclosure) | Moderate | VeryHigh | High |

Full descriptions and damage scenarios: `iso21434/tara.go`.

## Countermeasure mapping and residual gaps

| Threat | Countermeasure | Status |
|--------|-----------------|--------|
| T-RCP-001 | `regmap.AccessController` root/grant model + `authz.Policy` (client-side) | **Open** — neither layer binds a StreamID cryptographically to its peer; `tlstransport` (mutual TLS) is deprecated and does not apply to this protocol's addressing model |
| T-RCP-002 | Bounded claim timeout (`discovery.DefaultConfigurationClaimTimeout`, 30s); `ReadDiscovery` never blocked by claim state | Closed |
| T-RCP-003 | `e2e.Guard`/`e2e.Verify` CRC32 check, dedicated `ErrCRCMismatch` | **Open** — integrity only, not authenticity; a capable attacker can recompute the same public checksum |
| T-RCP-004 | `request.Dispatcher.PurgeNonSafety` never touches safety-request Kinds | Closed — damage scenario is bounded to non-safety work by design |
| T-RCP-005 | `e2e.StreamConfig.RequireMonotonicSequence` (opt-in) | **Open** — not enabled by default; a deployer must configure it per stream |
| T-RCP-006 | None — deliberate, specification-driven design (discovery must be answerable pre-authentication) | **Accepted residual risk** — treat discovery responses as public at network-segmentation time |

Three of six threats (T-RCP-001, T-RCP-003, T-RCP-005) remain open at this
milestone. This is reported as-is rather than closed by fiat: T-RCP-001 and
T-RCP-003 need capability this repository does not implement today
(peer/message authentication beyond a non-cryptographic checksum), and
T-RCP-005's opt-in default is itself a considered trade-off this milestone
does not have grounds to overturn unilaterally (enabling
`RequireMonotonicSequence` unconditionally would need every legitimate
sender to guarantee strictly ordered delivery, which this repository has
not verified holds for every deployment topology).

## Relationship to certgap (ASIL-D gap analysis)

This TARA is deliberately scoped to ISO/SAE 21434 (cybersecurity); it does
not restate or duplicate `certgap`'s ISO 26262 (functional safety)
ASIL-D gap analysis — see `certgap/reqset.go` and its own doc comment for
that separate, safety-focused gap report. T-RCP-004's countermeasure
(safety-request Kinds are immune to the non-safety purge) is one place the
two analyses touch: a cybersecurity-motivated DoS threat happens to be
bounded by a safety-motivated design decision (Milestone 50's safety-request
tagging), but the two documents are tracked independently.
