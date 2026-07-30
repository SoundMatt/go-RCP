# Hazard Analysis and Risk Assessment

**Project:** go-RCP
**Standard:** ISO 26262:2018
**ASIL target:** ASIL-B (SEOOC — Safety Element Out Of Context)
**Document ID:** HARA-001
**Version:** 3.1
**Date:** 2026-07-30

Source of truth: `.fusa-hara.json`

Version 3.0 retargets every hazard/safety-goal description at the OPEN
Alliance TC18 Remote Control Protocol model this repository implements as
of ROADMAP.md Milestone 59 (v1.0.0) — endpoints addressed by
(`avtp.StreamID`, `avtp.ByteBusID`) rather than a fixed `Zone` enum. The
underlying risk profile (severity/exposure/controllability) is unchanged:
the hazards themselves are about vehicle-level consequences of lost,
delayed, corrupted, misdirected, or spoofed control traffic, which the
protocol replacement does not alter. Only the mechanism names and
milestone references below have moved.

---

## Operational Situations

| ID     | Description |
|--------|-------------|
| OS-001 | Normal vehicle operation — all RC Server endpoints reachable |
| OS-002 | Partial network fault — one or more RC Server endpoints unreachable |
| OS-003 | Safety-critical manoeuvre — emergency braking or collision avoidance active |
| OS-004 | HPC software fault — runaway process, crash, or OOM condition |
| OS-005 | Elevated network latency — congestion, EMI, or hardware degradation |
| OS-006 | Adversarial access — attacker present on the RCP Ethernet network |

---

## Hazard Table

| ID     | Description | Situations | S | E | C | ASIL | Safety Goal |
|--------|-------------|------------|---|---|---|------|-------------|
| H-001 | Loss of request delivery to a safety-critical endpoint | OS-001, OS-002, OS-003 | S3 | E4 | C2 | **ASIL-C** | SG-001 |
| H-002 | Spurious request delivered to the wrong endpoint | OS-001 | S2 | E3 | C2 | **ASIL-B** | SG-002 |
| H-003 | RC Server watchdog liveness kick not delivered | OS-001, OS-003 | S2 | E3 | C3 | **ASIL-B** | SG-003 |
| H-004 | Delayed request delivery — too slow for real-time control | OS-003, OS-005 | S2 | E3 | C3 | **ASIL-B** | SG-004 |
| H-005 | Corrupted request/response payload — wrong actuator value applied | OS-001, OS-003 | S3 | E2 | C2 | **ASIL-B** | SG-005 |
| H-006 | Endpoint impersonation by rogue device (StreamID spoofing) | OS-001, OS-006 | S3 | E2 | C2 | **ASIL-B** | SG-006 |
| H-007 | HPC crash — RC Server endpoints left active with no watchdog kicks | OS-001, OS-002, OS-004 | S2 | E3 | C2 | **ASIL-B** | SG-007 |
| H-008 | Priority inversion — a high-priority request blocked by lower-priority traffic | OS-001, OS-003 | S2 | E3 | C2 | **ASIL-B** | SG-008 |
| H-009 | Request flooding — runaway HPC loop overwhelms an RC Server endpoint | OS-001, OS-004 | S2 | E3 | C2 | **ASIL-B** | SG-009 |
| H-010 | Request replay — a past valid request retransmitted in a new context | OS-001, OS-006 | S2 | E2 | C2 | **ASIL-B** | SG-010 |

**Key:** S = Severity, E = Exposure probability, C = Controllability
ASIL derived per ISO 26262:2018 Part 3 Table 4.

---

## ASIL Decomposition Rationale

### H-001 — Loss of request delivery (S3/E4/C2 → ASIL-C)

A write request that is never delivered to a safety-critical endpoint can result in severe injury (S3). RC Server endpoints are reachable for the majority of driving time (E4). The driver retains partial control via mechanical/hydraulic backup (C2). ISO 26262-3:2018 Table 4 is a lookup table indexed by S/E/C, not a product of the three factors; looking up S3/E4/C2 in that table gives **ASIL-C**.

> **Open item — project ASIL target:** This document's stated SEOOC target (see header, line 5) is ASIL-B, but SG-001 — the safety goal fed by H-001 — now derives ASIL-C. Neither an ASIL-C-to-ASIL-B decomposition argument for SG-001 nor a project-wide retarget to ASIL-C exists yet. Until one of those is produced, SG-001 MUST be treated as requiring ASIL-C evidence; the project-wide ASIL-B claim elsewhere in this repository (README.md, ROADMAP.md, SAFETY_PLAN.md) has not been revisited as a consequence of this reclassification and remains open for a follow-up decision.

**SG-001** is addressed by the watchdog/deadline mechanisms rebuilt against the TC18 request/response model at ROADMAP.md Milestone 53 (v0.66.0) and the `ErrTimeout` sentinel `Adapt`/`Controller.Request` return on expiry. Treat this evidence as ASIL-C-level pending resolution of the open item above.

### H-002 — Endpoint mismatch (S2/E3/C2 → ASIL-B)

A request delivered to the wrong endpoint can cause unintended vehicle behaviour (S2). The exposure is extended driving time (E3). The driver may partially compensate (C2). S2 × E3 × C2 = ASIL-A/B; classified ASIL-B to match project target.

**SG-002** is addressed at the wire-addressing level: every endpoint type's `HandleRequest` (and `mock.Endpoint`'s own reference implementation) rejects a request whose `byte_bus_id` does not match the endpoint it was registered for (`ErrWrongEndpoint`), a structural guarantee introduced with the endpoint model itself (Milestone 47 onward, v0.60.0+) rather than a per-request runtime check the retired `ErrZoneMismatch` sentinel used to perform.

### H-003 — Watchdog miss (S2/E3/C3 → ASIL-B)

An RC Server that does not receive watchdog kicks may enter an indeterminate state (S2). Extended driving includes network stress periods (E3). A watchdog-triggered safe-state entry is not easily controllable (C3). S2 × E3 × C3 = ASIL-B.

**SG-003** is addressed by `e2e.Supervisor`'s inter-arrival-timeout watchdog, rebuilt at Milestone 53 (v0.66.0) and re-verified as a code-driven formal trace at Milestone 58 (`formal/safestate.go`, v0.71.0).

### H-004 — Delayed delivery (S2/E3/C3 → ASIL-B)

During an emergency manoeuvre, a request that arrives late but within the protocol timeout may still be too slow for safe actuation (S2). Network latency spikes occur in extended real-world operation (E3). The driver cannot compensate for a late braking request (C3). S2 × E3 × C3 = ASIL-B.

**SG-004** requires latency to be bounded and monitored. Evidence from `safety/COMMAND_LATENCY.md` (REQ-SAFETY-001), regenerated at Milestone 58 (v0.71.0) against a real loopback `udp.Server`/`udp.Router`/`request.Dispatcher` path, characterises this bound; `deadline` (Milestone 53, v0.66.0) enforces it at runtime.

### H-005 — Payload corruption (S3/E2/C2 → ASIL-B)

A bit-flipped actuator value could command an endpoint to apply an unexpected, potentially dangerous output (S3). Bit errors in automotive Ethernet are rare but possible under EMI (E2). The driver can partially correct an unexpected actuation (C2). S3 × E2 × C2 = ASIL-B.

**SG-005** is addressed by the `e2e` CRC32 safe-point mechanism (Milestone 42, v0.63.0; rebuilt against the TC18 model at Milestone 53, v0.66.0), covering both request and response bodies per the safety-request variants the specification defines.

### H-006 — Endpoint impersonation (S3/E2/C2 → ASIL-B)

A rogue device presenting a spoofed `avtp.StreamID` and accepting requests intended for a legitimate endpoint could silently discard them, causing loss of safety-critical actuation (S3). Physical access to the RCP network is required, making this less frequent (E2). The driver may partially compensate (C2). S3 × E2 × C2 = ASIL-B.

**SG-006** is an **open gap**, not a closed one: per `TARA.md`/`iso21434/tara.go` (T-RCP-001), neither `regmap.AccessController`'s root/grant model nor `authz.Policy` (Milestone 55, v0.68.0) authenticates that an AVTPDU's carried StreamID genuinely originates from the peer that opened it. `tlstransport`, this repo's earlier bespoke mutual-TLS-over-TCP option, was retired outright at Milestone 59 (v1.0.0) and would not have closed this gap regardless — link-layer authentication is a MACsec/802.1AE concern this repository does not implement.

### H-007 — HPC crash (S2/E3/C2 → ASIL-B)

If the HPC process terminates unexpectedly, RC Server endpoints lose watchdog refresh and, depending on local firmware, may freeze outputs in their last state — potentially active braking or steering (S2). Software crashes occur in real-world deployments (E3). The driver retains partial control (C2). S2 × E3 × C2 = ASIL-B.

**SG-007** requires RC Servers to detect watchdog cessation and enter a defined safe state, addressed by `e2e.Supervisor` (Milestone 53, v0.66.0).

### H-008 — Priority inversion (S2/E3/C2 → ASIL-B)

Under a burst of low-priority traffic, a high-priority request (e.g. an emergency-brake write) could be delayed if dispatch does not enforce strict ordering (S2). High request loads occur in extended driving (E3). The driver retains partial control during the delay (C2). S2 × E3 × C2 = ASIL-B.

**SG-008** requires a high-priority request to never queue behind a lower-priority one. Addressed server-side by `request.Kind`'s own ordering (part of the conditional-request taxonomy, Milestone 41, v0.62.0) and mirrored client-side by `prioqueue` (rebuilt at Milestone 53, v0.66.0) for a caller choosing which of several already-built requests to release next.

### H-009 — Request flooding (S2/E3/C2 → ASIL-B)

A runaway loop in HPC software could saturate an RC Server endpoint's request handling, causing it to drop incoming safety-critical requests (S2). Software faults do occur in production (E3). The driver may be unaware the endpoint is non-responsive (C2). S2 × E3 × C2 = ASIL-B.

**SG-009** requires the HPC to enforce per-endpoint rate limits. Addressed by `ratelimit` (rebuilt against `avtp.StreamID`/`avtp.ByteBusID` addressing at Milestone 55, v0.68.0).

### H-010 — Request replay (S2/E2/C2 → ASIL-B)

An attacker who captures a valid request and replays it in a later context (different speed, different manoeuvre) could cause unintended actuation (S2). Physical network access is required (E2). The driver may partially compensate (C2). S2 × E2 × C2 = ASIL-A/B; classified ASIL-B to align with overall security posture.

**SG-010** requires sequence-monotonicity anti-replay protection. Addressed by the `e2e` safe-point mechanism's sequence tracking (Milestone 42, v0.63.0; rebuilt at Milestone 53, v0.66.0) — opt-in per `iso21434/tara.go`'s own honesty note that it is not this package's default.

---

## Safety Goal to Milestone Mapping

| Safety Goal | Description | Addressed By |
|-------------|-------------|--------------|
| SG-001 | Detect request delivery failures | Milestone 53 (v0.66.0) Watchdog/Deadline rebuild — **ASIL-C** per H-001 reclassification (open item: exceeds stated project ASIL-B target, see rationale above) |
| SG-002 | Reject requests addressed to the wrong endpoint | Milestone 47+ (v0.60.0+) endpoint-type `HandleRequest` addressing guard |
| SG-003 | Watchdog liveness kick guaranteed | Milestone 53 (v0.66.0) `e2e.Supervisor` |
| SG-004 | Latency bounded and monitored | Milestone 58 (v0.71.0) REQ-SAFETY-001 ✅ (measured), Milestone 53 (v0.66.0) `deadline` (runtime enforcement) |
| SG-005 | Payload integrity via CRC32 safe-point | Milestone 42 (v0.63.0) / Milestone 53 (v0.66.0) `e2e` |
| SG-006 | Endpoint identity authenticated | **Open** — see H-006; `authz` (Milestone 55, v0.68.0) is a complementary policy layer, not authentication |
| SG-007 | Safe state on watchdog cessation | Milestone 53 (v0.66.0) `e2e.Supervisor` |
| SG-008 | High-priority requests never blocked | Milestone 41 (v0.62.0) `request.Kind` ordering, Milestone 53 (v0.66.0) `prioqueue` |
| SG-009 | Per-endpoint request rate limiting | Milestone 55 (v0.68.0) `ratelimit` |
| SG-010 | Anti-replay sequence monotonicity | Milestone 42 (v0.63.0) / Milestone 53 (v0.66.0) `e2e` (opt-in) |
