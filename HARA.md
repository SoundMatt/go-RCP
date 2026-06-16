# Hazard Analysis and Risk Assessment

**Project:** go-RCP  
**Standard:** ISO 26262:2018  
**ASIL target:** ASIL-B (SEOOC — Safety Element Out Of Context)  
**Document ID:** HARA-001  
**Version:** 2.0  
**Date:** 2026-06-16  

Source of truth: `.fusa-hara.json`

---

## Operational Situations

| ID     | Description |
|--------|-------------|
| OS-001 | Normal vehicle operation — all zone controllers reachable |
| OS-002 | Partial network fault — one or more zone controllers unreachable |
| OS-003 | Safety-critical manoeuvre — emergency braking or collision avoidance active |
| OS-004 | HPC software fault — runaway process, crash, or OOM condition |
| OS-005 | Elevated network latency — congestion, EMI, or hardware degradation |
| OS-006 | Adversarial access — attacker present on the zone Ethernet bus |

---

## Hazard Table

| ID     | Description | Situations | S | E | C | ASIL | Safety Goal |
|--------|-------------|------------|---|---|---|------|-------------|
| H-001 | Loss of command delivery to safety-critical zone | OS-001, OS-002, OS-003 | S3 | E4 | C2 | **ASIL-B** | SG-001 |
| H-002 | Spurious command sent to wrong zone controller | OS-001 | S2 | E3 | C2 | **ASIL-B** | SG-002 |
| H-003 | Zone controller watchdog not kicked, unintended reset | OS-001, OS-003 | S2 | E3 | C3 | **ASIL-B** | SG-003 |
| H-004 | Delayed command delivery — too slow for real-time control | OS-003, OS-005 | S2 | E3 | C3 | **ASIL-B** | SG-004 |
| H-005 | Corrupted command payload — wrong actuator value applied | OS-001, OS-003 | S3 | E2 | C2 | **ASIL-B** | SG-005 |
| H-006 | Zone controller impersonation by rogue device | OS-001, OS-006 | S3 | E2 | C2 | **ASIL-B** | SG-006 |
| H-007 | HPC crash — zones left active with no watchdog kicks | OS-001, OS-002, OS-004 | S2 | E3 | C2 | **ASIL-B** | SG-007 |
| H-008 | Priority inversion — PriorityCritical blocked by Normal traffic | OS-001, OS-003 | S2 | E3 | C2 | **ASIL-B** | SG-008 |
| H-009 | Command flooding — runaway HPC loop overwhelms zone controller | OS-001, OS-004 | S2 | E3 | C2 | **ASIL-B** | SG-009 |
| H-010 | Command replay — past valid command retransmitted in new context | OS-001, OS-006 | S2 | E2 | C2 | **ASIL-B** | SG-010 |

**Key:** S = Severity, E = Exposure probability, C = Controllability  
ASIL derived per ISO 26262:2018 Part 3 Table 4.

---

## ASIL Decomposition Rationale

### H-001 — Loss of command delivery (S3/E4/C2 → ASIL-B)

A braking command that is never delivered can result in severe injury (S3). Zone controllers are reachable for the majority of driving time (E4). The driver retains partial control via mechanical/hydraulic backup (C2). ISO 26262 Table 4: S3 × E4 × C2 = ASIL-B.

**SG-001** requires the library to detect and report failures within the timeout period. This is addressed by the deadline monitoring mechanism (v0.12.0) and the existing `ErrTimeout` sentinel.

### H-002 — Zone mismatch (S2/E3/C2 → ASIL-B)

A command delivered to the wrong actuator zone can cause unintended vehicle behaviour (S2). The exposure is extended driving time (E3). The driver may partially compensate (C2). S2 × E3 × C2 = ASIL-A/B; classified ASIL-B to match project target.

**SG-002** is already addressed by `ErrZoneMismatch` guard introduced in v0.3.0 (REQ-CTRL-025).

### H-003 — Watchdog miss (S2/E3/C3 → ASIL-B)

A zone controller that does not receive watchdog kicks may reset into an indeterminate state (S2). Extended driving includes network stress periods (E3). A watchdog-triggered reset is not easily controllable (C3). S2 × E3 × C3 = ASIL-B.

**SG-003** is addressed by `CmdWatchdog` (v0.11.0).

### H-004 — Delayed delivery (S2/E3/C3 → ASIL-B)

During an emergency manoeuvre, a command that arrives late but within the protocol timeout may still be too slow for safe actuation (S2). Network latency spikes occur in extended real-world operation (E3). The driver cannot compensate for a late braking command (C3). S2 × E3 × C3 = ASIL-B.

**SG-004** requires latency to be bounded and monitored. Evidence from `safety/COMMAND_LATENCY.md` (REQ-SAFETY-001) characterises library-layer overhead; deadline monitoring (v0.12.0) will enforce the bound at runtime.

### H-005 — Payload corruption (S3/E2/C2 → ASIL-B)

A bit-flipped actuator value could command a braking zone to apply maximum deceleration unexpectedly (S3). Bit errors in automotive Ethernet are rare but possible under EMI (E2). The driver can partially correct an unexpected deceleration (C2). S3 × E2 × C2 = ASIL-B.

**SG-005** is addressed by the E2E protection layer (v0.14.0) providing CRC-16 frame check on every command and response.

### H-006 — Zone impersonation (S3/E2/C2 → ASIL-B)

A rogue device accepting braking commands could silently discard them, causing loss of safety-critical actuation (S3). Physical access to the zone bus is required, making this less frequent (E2). The driver may partially compensate (C2). S3 × E2 × C2 = ASIL-B.

**SG-006** is addressed by mutual TLS authentication (v0.7.0) and command-level authorisation (v0.19.0).

### H-007 — HPC crash (S2/E3/C2 → ASIL-B)

If the HPC process terminates unexpectedly, zone controllers lose watchdog refresh. Depending on their local firmware, they may freeze outputs in last state — potentially active braking or steering (S2). Software crashes occur in real-world deployments (E3). The driver retains partial control (C2). S2 × E3 × C2 = ASIL-B.

**SG-007** requires zone controllers to detect watchdog cessation and enter a safe state. Addressed by watchdog & heartbeat (v0.11.0).

### H-008 — Priority inversion (S2/E3/C2 → ASIL-B)

Under a burst of Normal-priority telemetry, a PriorityCritical braking command could be delayed if the transmission queue does not enforce strict priority (S2). High command loads occur in extended driving (E3). The driver retains partial control during the delay (C2). S2 × E3 × C2 = ASIL-B.

**SG-008** requires PriorityCritical to bypass all lower-priority queuing. Addressed by priority queuing (v0.15.0).

### H-009 — Command flooding (S2/E3/C2 → ASIL-B)

A runaway loop in HPC software could saturate the zone controller's input queue, causing it to drop incoming safety-critical commands (S2). Software faults do occur in production (E3). The driver may be unaware the zone is non-responsive (C2). S2 × E3 × C2 = ASIL-B.

**SG-009** requires the HPC to enforce per-zone rate limits, returning `ErrBusy` when exceeded. Addressed by rate limiting (v0.16.0).

### H-010 — Command replay (S2/E2/C2 → ASIL-B)

An attacker who captures a valid "set actuator X" command and replays it in a later context (different speed, different manoeuvre) could cause unintended actuation (S2). Physical bus access is required (E2). The driver may partially compensate (C2). S2 × E2 × C2 = ASIL-A/B; classified ASIL-B to align with overall security posture.

**SG-010** requires sequence counter anti-replay protection. Addressed by E2E protection (v0.14.0).

---

## Safety Goal to Milestone Mapping

| Safety Goal | Description | Addressed By |
|-------------|-------------|--------------|
| SG-001 | Detect command delivery failures | v0.11.0 Watchdog, v0.12.0 Deadline Monitoring |
| SG-002 | Reject zone-mismatched commands | v0.3.0 REQ-CTRL-025 ✅ |
| SG-003 | CmdWatchdog delivery guaranteed | v0.11.0 Watchdog |
| SG-004 | Latency bounded and monitored | v0.3.0 REQ-SAFETY-001 ✅ (library layer), v0.12.0 (runtime enforcement) |
| SG-005 | Payload integrity via CRC | v0.14.0 E2E Protection |
| SG-006 | Zone identity authenticated | v0.7.0 TLS, v0.19.0 Authorization |
| SG-007 | Safe state on watchdog cessation | v0.11.0 Watchdog |
| SG-008 | PriorityCritical never blocked | v0.15.0 Priority Queuing |
| SG-009 | Per-zone command rate limiting | v0.16.0 Rate Limiting |
| SG-010 | Anti-replay sequence counters | v0.14.0 E2E Protection |
