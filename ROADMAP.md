# go-RCP Roadmap

## Full Protocol Replacement — Read This First

Everything from here through "Phase 12" below (v0.1.0–v0.43.0, plus the
RELAY-tracking releases that followed through v0.56.1 — see the note at the
top of Part II) built out a self-consistent but **bespoke** Zone/Command/
Response/Status protocol over a proprietary 16-byte frame header. It was
never an implementation of the real industry standard it was named after. A
conformance gap analysis against the OPEN Alliance TC18 Remote Control
Protocol Specification v0.5.1_RC — the actual published standard this
library was meant to track — found that the two share **nothing** at the
wire level: a different addressing model (five fixed `Zone` values vs.
per-server Endpoints addressed by a stream/bus-id pair), different framing
(a flat custom header vs. IEEE 1722 AVTPDU/ACF messages), a different
request model (four fixed command types vs. a multi-kind conditional-request
taxonomy with sequencers), and no equivalent at all for a real server
lifecycle/register-map configuration model or an end-to-end safety-CRC
mechanism.

The maintainer has authorized a **full replacement**, not a gap-patch:
go-RCP's core protocol is being rebuilt to actually be the OPEN Alliance
TC18 Remote Control Protocol. **Part II** of this document (below the
existing history) lays out that replacement phase by phase, dependency
ordered, plus an explicit disposition — replace, adapt, deprecate, or keep
— for every satellite package this repo ships today.

**This is a breaking change, deliberately, with no compatibility shim.**
Once Part II's core phases land, every package, tool, and downstream
consumer that calls `Controller.Send(ctx, *Command)`, switches on
`Zone`/`CommandType`/`ResponseStatus`, or treats `Status` as a periodic
broadcast will need to be rewritten against the new Endpoint/register-map
API. There is no way to keep today's `Controller`/`Registry` interfaces and
also be conformant, because the two protocols disagree about what an
addressable thing even *is* (a `Zone` vs. an Endpoint on a specific server)
and what a request carries (one of four fixed command types vs. a
sequencer-gated, optionally-fragmented, optionally-CRC-protected message
with five distinct kinds). A shim translating old `Zone` semantics onto new
Endpoint semantics was considered for this document and rejected: made
faithful, it would still misrepresent the result as conformant when it
structurally can't be (the whole point of this program is to stop doing
that); made thin, it saves callers no real migration work while doubling
the surface this team has to keep correct through a safety-relevant
rewrite. Plan a hard migration, gated on the v1.0.0 milestone at the end of
Part II. See Part II's intro for the full reasoning and its satellite-package
table for a per-package call.

---

## Vision

go-RCP is a Go-native Remote Control Protocol for automotive zonal architecture.

The project focuses on:

- Reliable command delivery from a central computer to distributed zone controllers
- Safety-first design with traceability to ISO 26262 ASIL-B requirements
- Modern Go developer experience — zero CGo, pure interfaces, swappable transports
- Deterministic latency suitable for hard real-time automotive contexts
- Observability by default — metrics, heartbeats, and watchdog support built in

---

## Guiding Principles

1. Pure Go first — no CGo unless strictly necessary
2. Safety as a first-class concern — requirements in `.fusa-reqs.json`, traced to tests
3. Simplicity over completeness — clean interfaces, not a protocol kitchen sink
4. Testability by default — mock backend ships with the library
5. Zonal architecture native — Zone is a first-class type, not an afterthought
6. Transport-agnostic — swap in-process mock for UDP or TCP without API changes

---

## Release Plan

| Phase | Version | Theme | Summary |
|---|---|---|---|
| **Foundation** | v0.1.0 | Foundation | Core interfaces, mock backend, CI, go-FuSa, Docker quickstart ✅ |
| **Foundation** | v0.2.0 | Requirements | 79 atomic SEOOC ASIL-B requirements, full go-FuSa coverage ✅ |
| **Safety groundwork** | v0.3.0 | Hardening | Mock correctness fixes, benchmarks, safety timing evidence ✅ |
| **Safety groundwork** | v0.4.0 | HARA expansion | Comprehensive hazard analysis — delayed delivery, corruption, impersonation, flooding, HPC crash ✅ |
| **Transport stack** | v0.5.0 | UDP transport | Pure-Go UDP command/response transport with zone discovery ✅ |
| **Transport stack** | v0.6.0 | mDNS discovery | Zero-configuration zone controller discovery via mDNS/DNS-SD ✅ |
| **Transport stack** | v0.7.0 | TLS transport | Mutual TLS channel for zone-controller communication ✅ |
| **Transport stack** | v0.8.0 | Shared memory | Zero-copy intra-host command delivery via shared memory ✅ |
| **Transport stack** | v0.9.0 | Loaned samples | LoaningController interface extending zero-copy to all transports ✅ |
| **Transport stack** | v0.10.0 | TSN transport | IEEE 802.1Qbv-aware UDP transport for hard real-time Ethernet delivery ✅ |
| **Safety mechanisms** | v0.11.0 | Watchdog & heartbeat | CmdWatchdog scheduling, zone health state machine, liveness API ✅ |
| **Safety mechanisms** | v0.12.0 | Deadline monitoring | Zone-to-HPC liveness: alert when Status stops arriving within deadline ✅ |
| **Safety mechanisms** | v0.13.0 | Power state | CmdSleep/CmdWake, zone power state machine, bus-off recovery ✅ |
| **Safety mechanisms** | v0.14.0 | E2E protection | Sequence counter, CRC-16, replay guard on command frames ✅ |
| **Safety mechanisms** | v0.15.0 | Priority queuing | Per-zone priority queue honouring PriorityCritical/High/Normal ✅ |
| **Safety mechanisms** | v0.16.0 | Rate limiting | Per-zone token-bucket admission control against command flooding ✅ |
| **Verification** | v0.17.0 | Zone simulator | Timing-realistic zone controller simulator for SiL/HIL testing ✅ |
| **Verification** | v0.18.0 | Fault injection | Structured fault injection to validate watchdog, E2E, and replay-guard mechanisms ✅ |
| **Security** | v0.19.0 | Authorization | Command-level access control; ISO 21434 SL-2 policy enforcement ✅ |
| **Security** | v0.20.0 | Firmware update | CmdUpdate and firmware/ package for zone controller OTA delivery ✅ |
| **Topology** | v0.21.0 | Zone groups | Atomic multi-zone command broadcast with typed zone group sets ✅ |
| **Topology** | v0.22.0 | Zone proxy | Transparent zone proxy for multi-hop zonal topologies ✅ |
| **Topology** | v0.23.0 | Redundancy | Hot-standby Registry and HPC failover for ASIL-B fault tolerance ✅ |
| **Topology** | v0.24.0 | Multi-HPC federation | Multi-HPC active coordination over shared zone bus | ✅
| **Tooling** | v0.25.0 | Observability | OpenTelemetry traces and Prometheus metrics adapter | ✅
| **Tooling** | v0.26.0 | Admin API | HTTP admin interface for runtime registry inspection and control | ✅
| **Tooling** | v0.27.0 | Record & replay | Record command/response/status streams to disk; replay for regression and forensics | ✅
| **Tooling** | v0.28.0 | Config | YAML/JSON zone registry configuration | ✅
| **Tooling** | v0.29.0 | Code generation | Zone manifest → typed Go controller stubs and fusa-annotated requirements | ✅
| **Tooling** | v0.30.0 | Dynamic data | Runtime schema registry and typed payload codec for schema-less command payloads | ✅
| **Remote access** | v0.31.0 | gRPC bridge | gRPC transport for cloud-connected zone controllers and remote diagnostics ✅ |
| **Remote access** | v0.32.0 | REST bridge | HTTP/SSE bridge for browser tooling and cloud integration ✅ |
| **Protocol bridges** | v0.33.0 | SOME/IP bridge | Bridge RCP commands to SOME/IP service methods ✅ |
| **Protocol bridges** | v0.34.0 | CAN bridge | Bridge RCP commands to CAN frames via go-CAN ✅ |
| **Protocol bridges** | v0.35.0 | DDS bridge | Bridge RCP Status to DDS topics and DDS samples to RCP commands via go-DDS ✅ |
| **Protocol bridges** | v0.36.0 | MQTT bridge | Bridge RCP Status to MQTT topics for cloud/telematics integration via go-mqtt ✅ |
| **Protocol bridges** | v0.37.0 | LIN bridge | Bridge RCP commands to LIN frames for low-bandwidth zone actuators via go-LIN ✅ |
| **Protocol bridges** | v0.38.0 | UDS bridge | Bridge RCP commands to ISO 14229 UDS service calls for zone controller diagnostics ✅ |
| **Protocol bridges** | v0.39.0 | DoIP bridge | Bridge zone controller diagnostics over ISO 13400 Diagnostics over IP ✅ |
| **Platform** | v0.40.0 | RTOS / bare-metal | Zone controller client for Zephyr, FreeRTOS, and NuttX RTOS targets ✅ |
| **Certification** | v0.41.0 | Formal verification | TLA+ specification and model-checked proofs of health and watchdog state machines ✅ |
| **Certification** | v0.42.0 | ISO 21434 | Cybersecurity assurance case, TARA evidence, SL-2 gap report ✅ |
| **Certification** | v0.43.0 | Certification | ASIL-D gap analysis, structural coverage report, audit pack ✅ |

---

## Milestones

---
### Phase 1 — Foundation
---

### 1. Foundation (v0.1.0) ✅

- Core `rcp.go` interfaces: `Controller`, `Registry`, `Command`, `Response`, `Status`
- `mock/` in-process backend
- `cmd/rcptool` CLI (discover, send, monitor)
- `examples/quickstart/` controller and zone
- Docker multi-stage build + compose quickstart
- CI: unit tests (cross-platform), fuzz (short), golangci-lint, go-FuSa, DCO
- Release workflow: safety artifact regeneration on tag
- Docker publish workflow: GHCR multi-arch images

### 2. Requirements (v0.2.0) ✅

- 79 atomic SEOOC requirements across 10 groups (REQ-ZONE, REQ-PRI, REQ-CMD, REQ-STATUS, REQ-ERR, REQ-CMDSTRUCT, REQ-RESP, REQ-STAT, REQ-CTRL, REQ-REG)
- 45 ASIL-B + 34 ASIL-A requirements; zero coverage gaps
- Full go-FuSa v0.30.0 trace and check compliance ✅

---
### Phase 2 — Safety Groundwork
---

### 3. Hardening (v0.3.0)

**Mock correctness fixes**
- `Registry.Lookup` returns `ErrClosed` (not `ErrNotFound`) after `Close()`
- New sentinel error `ErrZoneMismatch`; `Controller.Send` returns it when `cmd.Zone != controller.Zone()`
- Payload copy-on-send in `Controller.Send` and `Controller.Publish` to prevent cross-zone aliasing

**Benchmarks** (`mock/mock_bench_test.go`)
- `BenchmarkSend_RoundTrip` — command dispatch + response, parameterised by payload size (1 B → 64 KB)
- `BenchmarkSend_Concurrent` — `b.RunParallel` across GOMAXPROCS goroutines
- `BenchmarkPublish_FanOut` — 1 publish → N subscribers (1, 2, 4, 8, 16)
- `BenchmarkRegistry_Lookup` — hot-path registry lookup under concurrent reads
- All benchmarks use `b.ReportAllocs()`; zero-alloc Send on the mock path is a target

**Safety timing evidence** (`safety/command_latency_test.go`)
- 30-second workload: N zone controllers publishing status at realistic rates (100 Hz watchdog, 10 Hz telemetry) under 64 MiB/s GC pressure
- Measures Send latency (P50 / P99 / P999 / Max) and Publish→Subscribe delivery latency
- Asserts Max Send latency < watchdog half-period (5 ms at 100 Hz)
- Captures GC STW pause statistics from `runtime.MemStats.PauseNs`
- Writes `COMMAND_LATENCY.md` containing a structured GSN argument (Claim, Goal, Strategy, Evidence, Assumptions, Residual risk) — FuSa audit evidence

### 4. HARA Expansion (v0.4.0)

Expands `.fusa-hara.json` from 3 hazards to comprehensive coverage. New hazards and the safety goals they generate:

- **H-004** Delayed command delivery — zone responds within protocol timeout but too slowly for real-time control; ASIL-B → SG-004: maximum end-to-end latency shall be bounded and monitored
- **H-005** Corrupted command payload — bit error causes wrong actuator value; ASIL-B → SG-005: payload shall be integrity-protected (CRC) and rejected on failure
- **H-006** Zone controller impersonation — rogue device responds as a legitimate zone; ASIL-B → SG-006: zone identity shall be authenticated before commands are accepted
- **H-007** HPC crash without graceful shutdown — zones left active with no watchdog kicks; ASIL-B → SG-007: zone controllers shall enter a safe state if watchdog kicks cease
- **H-008** Priority inversion — PriorityCritical command blocked behind Normal commands under load; ASIL-B → SG-008: PriorityCritical commands shall never be delayed by lower-priority commands
- **H-009** Command flooding by faulty HPC software — runaway loop overwhelms zone controller; ASIL-B → SG-009: HPC shall enforce per-zone command rate limits
- **H-010** Replay of a valid past command in a new context; ASIL-B → SG-010: commands shall carry sequence counters rejected outside an anti-replay window
- Updates `HARA.md` with ASIL decomposition rationale for each new hazard
- New safety goals feed directly into the requirements for v0.11.0–v0.16.0

---
### Phase 3 — Transport Stack
---

### 5. UDP Transport (v0.5.0)

- Length-framed binary command/response protocol over UDP (SOME/IP-aligned framing: message ID, session ID, length prefix)
- Static unicast zone discovery; optional multicast announcement
- Integration tests with loopback interface
- `rcptool` gains `--transport udp --addr <host:port>` flag

### 6. mDNS Discovery (v0.6.0)

- Zero-configuration zone controller discovery via mDNS (RFC 6762) and DNS-SD (RFC 6763); Avahi-compatible
- Zone controllers self-announce as `_rcp._udp.local` service records carrying zone ID, address, and port
- HPC-side `Discoverer` interface: `Discover(ctx) (<-chan DiscoveryEvent, error)` with add/remove events
- `Registry.AutoRegister(ctx, discoverer)` wires discovered controllers into the registry automatically
- Configurable service-instance name format: `<zone-id>.<hostname>._rcp._udp.local`

### 7. TLS Transport (v0.7.0)

- Mutual TLS transport using standard `crypto/tls`
- Certificate pinning for zone controller identity verification
- Zero external dependency: no non-stdlib TLS libraries
- Addresses SG-006: zone identity authenticated via certificate before command acceptance

### 8. Shared Memory Transport (v0.8.0)

- Zero-copy intra-host command delivery via POSIX shared memory (`shm_open`/`mmap`) for zone controllers co-located on the same ECU
- `shmem.NewController` implements the `Controller` interface; swappable with UDP/TLS without API change
- Initial `LoaningController` implementation: `Loan()` returns a pre-allocated `Command` buffer from the shared region; `Commit()` delivers it without copying
- Linux only; falls back to UDP transport gracefully on other platforms via `auto.NewController`

### 9. Loaned Samples (v0.9.0)

- `LoaningController` interface extending `Controller` with `Loan() (*Command, error)` and `Commit(*Command) (*Response, error)`
- `LoaningRegistry` wraps any registry; `LookupLoaning(zone)` returns a `LoaningController` if the underlying transport supports it
- Implementations across all transports:
  - `shmem`: full zero-copy into the shared memory region (no allocation, no copy)
  - `mock`: pre-allocated pool; `BenchmarkSend_Loaned` must report 0 allocs/op
  - UDP/TLS: pool-backed `Command` buffers eliminate per-call allocation; one copy to the socket send buffer remains unavoidable
- Guarantee: `LoaningController.Commit` on the shmem and mock paths must not allocate — enforced by benchmark gate in CI
- `auto.NewLoaningController` selects shmem if available, falls back to pool-backed UDP

### 10. TSN Transport (v0.10.0)

- IEEE 802.1Qbv (Time-Aware Shaper) aware UDP transport for hard real-time Ethernet delivery
- Credit-Based Shaper (CBS, 802.1Qav) integration for bandwidth reservation per zone stream
- Frame preemption (802.1Qbu) support to protect `PriorityCritical` commands from frame bursts
- Deployment guide: required SO_PRIORITY socket options, VLAN tagging, and NIC configuration on Linux (Nvidia Orin / Renesas R-Car H3)
- Timing evidence: `safety/tsn_latency_test.go` — loopback measurements with TSN shaper active, demonstrating bounded worst-case delivery latency

---
### Phase 4 — Safety Mechanisms
---

### 11. Watchdog & Heartbeat (v0.11.0)

- Periodic `CmdWatchdog` scheduling with configurable interval per zone
- Zone health state machine: Healthy → Degraded → Faulted with configurable thresholds
- `Registry.WatchHealth()` channel for health state change events
- New requirements: REQ-WD-001..REQ-WD-00N (ASIL-B) — addresses SG-003, SG-007

### 12. Deadline Monitoring (v0.12.0)

- Zone-to-HPC direction: alert when `Status` updates from a zone controller stop arriving within a configured deadline
- `DeadlineMonitor` wraps any `Controller`; calls a `MissedDeadlineFn` callback if no `Status` is received within the deadline window
- Integrates with `Registry.WatchHealth()`: a deadline miss transitions the zone to Degraded after one miss, Faulted after N consecutive misses (configurable)
- Complements the watchdog (HPC→zone) to give full bidirectional liveness
- New requirements: REQ-DL-001..REQ-DL-00N (ASIL-B) — addresses SG-001, SG-004

### 13. Power State (v0.13.0)

- New command types `CmdSleep` and `CmdWake` added to `CommandType`
- Zone power state machine: Active → Sleeping → WakePending → Active; transitions driven by RCP commands and watchdog timeouts
- `Controller.PowerState()` returns the current zone power state
- `Registry.WatchPower()` channel for zone power state change events
- Bus-off recovery: automatic `CmdWake` retry with configurable backoff when a zone transitions from Sleeping unexpectedly
- New requirements: REQ-PWR-001..REQ-PWR-00N (ASIL-B)

### 14. E2E Protection (v0.14.0)

- 32-bit sequence counter per zone controller; rejects out-of-window frames
- CRC-16 frame check on command and response payload
- Anti-replay guard with configurable window size
- New requirements: REQ-E2E-001..REQ-E2E-00N (ASIL-B) — addresses SG-005, SG-010

### 15. Priority Queuing (v0.15.0)

- Per-zone send queue with three priority levels
- `PriorityCritical` bypasses normal queue backpressure
- Backpressure metrics exposed via OpenTelemetry counter
- Queue depth and drop rate added to `Status` telemetry
- New requirements: REQ-PQ-001..REQ-PQ-00N (ASIL-B) — addresses SG-008

### 16. Rate Limiting (v0.16.0)

- Per-zone token-bucket admission control on the HPC send path
- Configurable burst and sustained rate limits per priority level (`PriorityCritical` exempt by default)
- `ErrBusy` returned immediately when bucket is exhausted; no blocking
- Rate limit state exposed in `Status` telemetry and Prometheus metrics
- New requirements: REQ-RL-001..REQ-RL-00N (ASIL-B) — addresses SG-009

---
### Phase 5 — Verification
---

### 17. Zone Simulator (v0.17.0)

- Timing-realistic zone controller simulator for SiL/HIL testing without physical ECUs; implements the full `Controller` interface
- Configurable response latency distribution (constant, normal, or jitter model) and processing load
- Zone health and power state machines driven by injected fault schedules: Healthy → Degraded → Faulted → Recovering
- Watchdog miss detection: simulator transitions to Faulted if `CmdWatchdog` is not received within the configured deadline
- Deadline monitoring simulation: publishes `Status` at a configured rate; stops publishing on fault injection to trigger `DeadlineMonitor`
- Composable with the fault injection harness (v0.18.0)
- `sim/` package ships alongside `mock/`

### 18. Fault Injection (v0.18.0)

- Structured fault injection harness for validating safety mechanisms introduced in v0.11.0–v0.16.0
- Fault types: missed watchdog kick, missed Status deadline, corrupted CRC frame, replayed sequence number, late response (> timeout budget), dropped response, zone-mismatch command, admission-control exhaustion, spurious sleep transition
- Each fault is a typed value injected via a `FaultSchedule` applied to a `sim.Controller` or live UDP transport
- Regression suite: `safety/fault_injection_test.go` — for each fault type, assert the correct sentinel error is returned and the health/power state machine transitions correctly
- Writes `FAULT_INJECTION.md` — FuSa evidence cross-referencing HARA hazards H-001..H-010

---
### Phase 6 — Security
---

### 19. Authorization (v0.19.0)

- Command-level access control: a signed `AccessPolicy` declares which HPC identities may send which `CommandType` values to which zones
- `AuthController` wraps any `Controller`; verifies the caller's certificate against the access policy before forwarding commands
- Policy format: YAML/JSON, signed with the zone controller's TLS private key — policies are unforgeable without the zone's key
- `ErrForbidden` sentinel error returned on policy violation; logged to audit trail
- Aligns with IEC 62443 SL-2 target in `.fusa-iec62443.json`: authenticated identity + command-level authorisation
- New requirements: REQ-AUTH-001..REQ-AUTH-00N (ASIL-B / IEC 62443 SL-2) — addresses SG-006

### 20. Firmware Update / OTA (v0.20.0)

- New command type `CmdUpdate` added to `CommandType`
- `firmware/` package: chunked firmware delivery over RCP with integrity check (SHA-256) and rollback support
- `FirmwareSession` manages the multi-command exchange: Initiate → Transfer (N chunks) → Verify → Activate → Reset
- Zone controller authentication required before any `CmdUpdate` is accepted (depends on v0.19.0 Authorization)
- Delta update support: binary diff (bsdiff-compatible) to minimise transfer size over constrained links
- `rcptool update <zone> <firmware.bin>` subcommand

---
### Phase 7 — Topology & Scalability
---

### 21. Zone Groups (v0.21.0)

- `ZoneGroup` is a typed set of `Zone` values with named constants (e.g. `GroupRearPassenger`, `GroupAllZones`)
- `Registry.SendGroup(ctx, group, cmd)` dispatches a command atomically to all zones in the group and collects responses
- Partial-failure semantics: returns a `GroupResponse` carrying individual per-zone `Response` and error values; caller decides whether to treat partial success as failure
- `PriorityCritical` group commands are dispatched concurrently with a single shared deadline context

### 22. Zone Proxy (v0.22.0)

- Transparent proxy for cascaded zonal topologies (HPC → proxy → zone MCU)
- Command routing table: zone → upstream proxy address
- Latency budget enforcement at proxy boundary; budget violation → `ErrTimeout`

### 23. Redundancy (v0.23.0)

- `RedundantRegistry` wraps a primary and hot-standby `Registry`; promotes standby automatically on health-state change
- Heartbeat-based HPC liveness detection: standby activates if primary HPC misses N consecutive heartbeats
- Configurable promotion policy: automatic (zero-touch) or operator-confirmed
- State synchronisation: in-flight commands at failover are retried against the new primary with deduplication via `Command.ID`
- New requirements: REQ-RED-001..REQ-RED-00N (ASIL-B)

### 24. Multi-HPC Federation (v0.24.0) ✅

- Multiple active HPCs each owning disjoint zone subsets on the same zone bus
- `FederatedRegistry` coordinates zone ownership: each HPC registers a lease on the zones it owns; a lease server arbitrates conflicts
- Cross-HPC command forwarding: HPC-A can send a command to a zone owned by HPC-B via the federation layer; transparent to the caller
- Ownership transfer: zones can be migrated between HPCs at runtime (e.g. powertrain HPC hands off body zones during shutdown)

---
### Phase 8 — Tooling
---

### 25. Observability (v0.25.0) ✅

- OpenTelemetry trace spans for every `Send` and `Subscribe` call
- Prometheus-compatible metrics: command latency histogram, error rate, zone health gauge, power state distribution, deadline miss counter
- `monitor` subcommand in `rcptool` for live zone status dashboard

### 26. Admin API (v0.26.0) ✅

- HTTP admin interface (`admin/` package, mirrors go-DDS `admin/`)
- `GET /zones` — list all registered zones with health, power state, and last-seen timestamp
- `GET /zones/{zone}` — single-zone detail: health history, command rate, deadline miss count
- `POST /zones/{zone}/send` — send a command via JSON body; returns response JSON
- `GET /events` — SSE stream of all health, power, and deadline-miss events
- `GET /metrics` — Prometheus scrape endpoint
- Bearer auth enforced on all write endpoints (depends on v0.19.0 Authorization)

### 27. Record & Replay (v0.27.0) ✅

- `record/` package — records all `Command`, `Response`, and `Status` streams to a structured binary log on disk
- Ring-buffer mode for always-on black-box recording with configurable retention window
- Replay: feed a recorded log back through any `Controller`/`Registry` implementation for regression testing against a new version
- `rcptool record` and `rcptool replay` subcommands
- Log format is append-only and checksummed — suitable as FuSa incident forensics evidence

### 28. Config (v0.28.0) ✅

- YAML/JSON zone registry configuration (zone ID, transport, address, certificates)
- Hot-reload of zone addresses without restart via `fsnotify`

### 29. Code Generation (v0.29.0) ✅

- Zone manifest schema (YAML/JSON): declares zone IDs, supported command types, payload schemas, and ASIL levels
- `rcptool gen <manifest.yaml>` generates typed Go controller stubs with `//fusa:req` annotations pre-populated
- Generated stubs implement the `Controller` interface; the generator emits matching `_test.go` skeletons and `.fusa-reqs.json` entries
- Eliminates hand-written boilerplate when adding a new zone type; keeps requirements, code, and tests in sync from declaration

### 30. Dynamic Data (v0.30.0) ✅

- Runtime payload schema registry: named types (e.g. `"braking.BrakeCommand"`) registered with a Go struct and a codec at startup
- `DynamicPayload` carries a schema name alongside raw bytes; `Decode[T](p DynamicPayload) (T, error)` reconstructs the typed value without compile-time knowledge of all payload types
- Admin API and `rcptool monitor` display decoded payload fields when a matching schema is registered; fall back to hex for unregistered types
- Code generation (v0.29.0) emits `RegisterSchema` calls for each declared payload type, wiring the two features together ✅
- Useful for cloud tools and dashboards that connect after deployment and need to interpret payloads without a recompile

---
### Phase 9 — Remote Access
---

### 31. gRPC Bridge (v0.31.0) ✅

- gRPC transport (`bridge/grpc/`) for cloud-connected zone controllers and remote HPC diagnostic access
- `Subscribe` server-streaming RPC: cloud consumer receives `Status` updates in real time
- `Send` unary RPC: remote caller dispatches a `Command` and receives the `Response`
- Bearer auth interceptors; filter and transform hooks; YAML config via `LoadConfig`/`ApplyConfig`
- Enables remote diagnostic tools and cloud dashboards to interact with zone controllers without a local HPC connection

### 32. REST Bridge (v0.32.0) ✅

- HTTP/SSE bridge (`bridge/rest/`) for browser-based tooling and cloud integration
- `POST /zones/{zone}/commands` — dispatch a `Command` as JSON; returns `Response` JSON
- `GET /zones/{zone}/status` — SSE stream of `Status` updates for a single zone
- `GET /zones` — SSE stream of all zone health and power-state change events
- Bearer auth on all write endpoints; CORS support for browser clients
- Complements the gRPC bridge: REST/SSE for interactive dashboards and scripts; gRPC for high-throughput cloud consumers

---
### Phase 10 — Automotive Protocol Bridges
---

### 33. SOME/IP Bridge (v0.33.0) ✅

- Bridge `CmdSet`/`CmdGet` to SOME/IP service method calls via go-SOMEIP
- Bidirectional: SOME/IP events → RCP `Status` updates

### 34. CAN Bridge (v0.34.0) ✅

- Bridge `CmdSet`/`CmdGet` to CAN frames via go-CAN
- Configurable CAN ID mapping per zone and command type

### 35. DDS Bridge (v0.35.0) ✅

- Bridge RCP `Status` updates to DDS topics via go-DDS (sensor-fusion consumers receive zone telemetry as typed DDS samples)
- Bridge DDS samples → RCP `CmdSet`/`CmdGet` for ADAS pipeline → zone actuator control
- Bidirectional QoS mapping: DDS Reliability/Durability → RCP Priority

### 36. MQTT Bridge (v0.36.0) ✅

- Bridge RCP `Status` to MQTT topics for cloud telemetry and fleet management via go-mqtt
- Bridge MQTT command messages → RCP `CmdSet` for remote zone actuation
- Configurable topic prefix per zone (e.g. `rcp/zone/front-left/status`)

### 37. LIN Bridge (v0.37.0) ✅

- Bridge `CmdSet`/`CmdGet` to LIN frames via go-LIN for low-bandwidth zone actuators (seat motors, mirror adjustment, window regulators)
- Configurable LIN frame ID and field mapping per zone and command type
- LIN schedule table management: RCP commands inserted as unconditional or event-triggered frames

### 38. UDS Bridge (v0.38.0) ✅

- Bridge RCP commands to ISO 14229 UDS service calls for zone controller diagnostics
- `CmdReset` → UDS ECUReset (0x11); `CmdGet` → ReadDataByIdentifier (0x22); `CmdSet` → WriteDataByIdentifier (0x2E)
- Configurable UDS addressing mode per zone (physical, functional, extended)
- UDS negative response codes surfaced as typed `ResponseStatus` values

### 39. DoIP Bridge (v0.39.0) ✅

- ISO 13400 Diagnostics over IP transport for workshop and EOL diagnostic access to zone controllers
- `DoIPController` implements the `Controller` interface; routes `CmdGet`/`CmdReset` to UDS services over the DoIP wire protocol
- Logical address and routing activation management per zone
- Enables `rcptool` to act as a DoIP tester for factory-floor zone controller flashing and diagnostics

---
### Phase 11 — Platform
---

### 40. RTOS / Bare-Metal (v0.40.0) ✅

- Zone controller client library targeting Zephyr, FreeRTOS, and NuttX RTOS
- Pure C API generated from the go-RCP interface definitions (CGo bridge or separate C implementation sharing the wire format)
- Implements the same UDP framing, E2E protection, and watchdog protocol as the Go library
- No heap allocation on the RTOS side: all buffers statically allocated at compile time
- Integration test: Zephyr zone controller on QEMU communicating with go-RCP HPC over loopback

---
### Phase 12 — Certification & Formal Methods
---

### 41. Formal Verification (v0.41.0) ✅

- TLA+ specification of the zone health state machine (Healthy → Degraded → Faulted → Recovering)
  - Properties verified: no deadlock, no livelock; liveness (a zone that becomes healthy is eventually detected as Healthy); safety (a Faulted zone is detected within 2× the watchdog period)
- TLA+ specification of the watchdog protocol: HPC sends `CmdWatchdog` at interval T; zone resets if no kick arrives within deadline D
  - Properties verified: if kicks cease, zone reaches Faulted within D + network round-trip; a resumed kick stream returns zone to Healthy
- TLA+ specification of the anti-replay guard: sequence counter window W; frames outside the window are rejected
  - Properties verified: no valid in-window frame is ever rejected; a replayed frame is always rejected
- Model-checking results and counter-example traces published in `FORMAL_VERIFICATION.md` — ASIL-D evidence that the safety state machines are correct by construction
- `tla/` directory contains all `.tla` and `.cfg` files; reproducible via the TLC model checker

### 42. ISO 21434 / Cybersecurity (v0.42.0) ✅

- Threat Analysis and Risk Assessment (TARA) covering command injection, replay attacks, rogue zone controller registration, OTA firmware tampering, and denial-of-service via command flooding
- Security requirements mapped to TARA findings; implemented controls (TLS, Authorization, E2E replay guard, rate limiting, mDNS authentication) traced as countermeasures
- IEC 62443 SL-2 gap report (`iec62443-gap-report.json`) — closes open items from `.fusa-iec62443.json`
- Penetration test evidence: structured attack scenarios against UDP, TLS, admin HTTP, gRPC, and REST endpoints
- `TARA.md` and `CYBERSECURITY.md` published alongside the safety case

### 43. Certification (v0.43.0) ✅

- ASIL-D gap analysis report (decomposition paths from current ASIL-B)
- Structural coverage report: statement, branch, MC/DC
- Audit pack for customer qualification (requirements traceability matrix, FMEA, safety case, TARA cross-reference, formal verification summary)

---

**Note on v0.44.0–v0.56.1:** these releases (not detailed above) tracked
this repo's adoption of successive RELAY meta-specification versions
(canonical `Adapt`/`relay.Caller` conformance, error-chain wrapping, JSON
tag conventions, the `go-rcp` CLI, golden-vector and schema conformance
checks, and CI conformance gates) rather than changes to the RCP protocol
itself. They're out of scope for this revision; Part II picks up
versioning at v0.57.0.

---
---

# Part II — TC18 Protocol Replacement Program

## Program Vision

Replace go-RCP's bespoke Zone/Command/Response protocol with a conformant
implementation of the OPEN Alliance TC18 Remote Control Protocol, so that
"RCP" in this repo's name stops being a coincidence. Concretely:

- Wire-compatible framing: IEEE 1722 AVTPDU/ACF (NTSCF/TSCF, ACF_ABB/ACF_GBB)
- The real addressability model: RC Servers exposing Endpoints, reached via
  `(stream_id, byte_bus_id)`, not a fixed 5-value `Zone` enum
- The real RC Server lifecycle (a 3-state config/lock state machine) and
  register-map configuration model, generic and functional halves split
- All thirteen fully-specified endpoint types, in the dependency order
  that lets each phase be tested against the previous one
- The real conditional-request taxonomy and sequencer model, in place of a
  flat `CommandType` enum
- The real end-to-end CRC safe-point mechanism and safety-request variants,
  in place of the ad hoc CRC-16/replay-guard wrapper
- A deliberate, justified call on fragmentation, since the spec leaves it
  optional
- A named, individually-justified disposition for every existing satellite
  package — no package gets silently carried forward unexamined
- Re-satisfying RELAY conformance (the `Adapt`/`relay.Caller` bridge, the
  `go-rcp` CLI, golden vectors, and CI's `relay conform --strict` gate)
  against the new model, not just against the old one

## Guiding Principles (Part II additions)

7. Wire-format fidelity over convenience — if the spec and the old
   Command/Response model disagree, the spec wins; no "compatibility
   compromise" framings that are neither old nor new.
8. Dependency order over feature-completeness — no endpoint type, request
   kind, or satellite package migration lands ahead of the core primitives
   it depends on (see the phase ordering below).
9. Every satellite package gets an explicit call — silence is not a
   disposition; see the table in Phase 17.
10. Spec ambiguity is surfaced, not silently resolved — where the
    specification itself is incomplete or inconsistent (DAC, MDIO's
    scope-list omission, CAN's empty trigger table, I²C's speed-enum
    collision), go-RCP documents the gap and makes a conservative,
    reversible implementation choice rather than guessing silently.

## Program Release Plan

| Phase | Version | Theme | Summary |
|---|---|---|---|
| **TC18 Core — Wire & Server** | v0.57.0 | Wire format core | IEEE 1722 NTSCF/TSCF framing, ACF_ABB/ACF_GBB, the shared request-descriptor header, stream/transaction/bus-id addressing ✅ |
| **TC18 Core — Wire & Server** | v0.58.0 | RC Server lifecycle | 3-state config lifecycle, generic/functional register-map split, EP0 ✅ |
| **TC18 Core — Wire & Server** | v0.59.0 | Discovery | Discovery-stream claiming, timeout/lapse behaviour, register-0 read/response ✅ |
| **TC18 Core — Basic Endpoints** | v0.60.0 | GPIO + SPI | First two endpoint types — simplest request/response shapes ✅ |
| **TC18 Core — Basic Endpoints** | v0.61.0 | I²C / UART / ADC / PWM | Remaining "basic" endpoint types ✅ |
| **TC18 Core — Requests** | v0.62.0 | Conditional request taxonomy | Compound, compound-wait, triggered, chained, timed requests + sequencers ✅ |
| **TC18 Core — Safety** | v0.63.0 | E2E CRC safe points | CRC32 safe-point mechanism, safety-request variants, watchdog-driven safe-state entry ✅ |
| **TC18 Core — Remaining Endpoints** | v0.64.0 | LIN / CAN incl. CAN XL / ISELED / MDIO / Wakeup | Remaining fully-specified endpoint types; DAC explicitly deferred ✅ |
| **TC18 Core — Fragmentation** | v0.65.0 | Fragmentation (GO) | Multi-AVTPDU fragmentation — explicit go decision below, justified ✅ |
| **Satellite Migration** | v0.66.0 | Safety & liveness rebuild | powerstate, watchdog, deadline, prioqueue rebuilt against the new model; e2e retired in favour of crcsafe ✅ |
| **Satellite Migration** | v0.67.0 | Transport & discovery migration | wire retired (no successor), udp rebuilt on avtp/acf, tsn/shmem/loan adapted; mdns's role narrowed to an optional rendezvous helper; tlstransport deprecated ✅ |
| **Satellite Migration** | v0.68.0 | Control-plane & topology adaptation | authz, ratelimit, redundancy, federation, zonegroup, proxy, admin, config, dyndata |
| **Satellite Migration** | v0.69.0 | Protocol bridge adaptation | canbr, linbr, ddsbr, mqttbr, someip, udsbr, doipbr, grpcbridge, restbridge |
| **Satellite Migration** | v0.70.0 | Tooling & test-double rebuild | mock, sim, capi, codegen, record, observe, faultinject, firmware |
| **Satellite Migration** | v0.71.0 | Certification refresh | formal, iso21434, certgap, safety re-scoped to the new state machines and attack surface |
| **Cutover** | v1.0.0 | TC18 conformance + RELAY re-certification | Legacy Zone/Command API removed; RELAY `Adapt`/golden-vectors/CLI re-satisfied against the new model |

---

## Milestones

---
### Phase 13 — TC18 Core: Wire Format & Server Model
---

### 44. AVTPDU / ACF Wire Format (v0.57.0) ✅

**Done (v0.57.0):** landed in the new `avtp` package (`avtp/avtpdu.go`,
`avtp/message.go`, `avtp/address.go`, `avtp/frame.go`; see `avtp/doc.go` for
the package-level design notes, including the explicit call-out that this
package's exact numeric subtype/field-width choices are this
implementation's own reasoned encoding pending confirmation against a public
interoperability reference, since the TC18 spec text itself is
members-confidential). `wire/` is untouched — this is a new, independent
package, not an edit to the old bespoke frame format it will eventually
replace at Phase 17 (Milestone 54, v0.67.0).

- New wire-format package implementing IEEE 1722 framing for RCP: both
  header variants — the untimed "execute as soon as possible" form and the
  presentation-timestamped form — with their respective length/sequence
  bookkeeping fields
- Both RCP message encodings: the short form with no timestamp field at
  all, and the longer form carrying a 64-bit timestamp slot; both share one
  request-descriptor header (message type, length, pad, addressing fields,
  the acknowledge/read/write/response/error/more-segments control bits, and
  the dual-purpose read-size/segment-number field)
- `stream_id` (sender MAC + locally-assigned suffix) and `byte_bus_id`
  (endpoint address, unique only within its own stream) addressing, plus
  `transaction_num` correlation scoped to the enclosing stream
- Validation rules for a missing/invalid/uncertain timestamp marker folding
  down to best-effort execution, and the timestamped header being dropped
  outright by a server with no time-sync support
- Explicit non-goal for this milestone: this repo targets Ethernet-carried
  AVTPDUs first. The specification allows the underlying network to be
  CAN(FD/XL) instead of Ethernet, and allows 1722-over-UDP/IP as an
  alternative to raw Ethernet framing — both are real transport options,
  not analogous to today's ad hoc UDP/TLS wire format, and are tracked as
  a follow-on rather than blocking this milestone
- Golden-vector-style fixtures (server request → expected byte layout) so
  later phases can regression-test against a frozen wire encoding

### 45. RC Server Lifecycle & Register Map (v0.58.0) ✅

**Done (v0.58.0):** landed in the new `server` package (`server/lifecycle.go`,
`server/registermap.go`, `server/access.go`, `server/pinmap.go`,
`server/queues.go`, `server/types.go`, `server/server.go`; see `server/doc.go`
for the package-level design notes, including the same explicit spec-fidelity
call-out avtp/doc.go established — this package's exact register byte
layouts and named-signal-index scheme are this implementation's own reasoned
encoding pending confirmation against a public interoperability reference,
since the TC18 spec text itself is members-confidential). `server` builds
directly on `avtp` (Milestone 44): `avtp.ByteBusID` addresses endpoints and
EP0, `avtp.StreamID` is what the root-client/restricted-stream access model
keys off of. The old bespoke-protocol `Zone`/`Controller`/`Registry` types in
the root module are untouched — this is a new, independent package, not an
edit to them.

- The three-state server lifecycle (an unconfigured bare-defaults state, a
  hardware-configuration-locked state, and a fully-configured state), with
  its transition guard conditions (plausibility checks before a state can
  advance, rejecting an inconsistent configuration instead of silently
  accepting it) and its register-locking behaviour (which fields are
  writable in which state, and which become permanently locked once fully
  configured regardless of who's asking)
- The register-map split this revision of the spec makes structural: a
  common/generic per-endpoint block owned by the server, separate from
  each endpoint's own type-specific functional configuration block
- EP0 (the server addressed as a pseudo-endpoint): whole-register-map
  read/write, the root-client concept (exactly one stream has full-register
  write access; every other stream is restricted to the endpoints assigned
  to it)
- The general server register block: identification fields, protocol
  version, capability/capacity counters, and pointers to every other
  configuration table this and later milestones define
- HW pin-mapping table (writable only pre-lock) and the per-endpoint-type
  named-signal-index scheme
- Request-stream and response/acknowledge-queue configuration tables,
  including the flush-threshold/flush-time batching and heartbeat behaviour
  those queues use
- Explicit note carried over from the specification's own review comments:
  the endpoint-address mapping table's ordering requirement is a
  client-side obligation with no server-side safety net — go-RCP's own
  client-side config tooling (see Phase 17's control-plane migration) must
  enforce correct ordering itself rather than assume the wire format does

### 46. Discovery (v0.59.0) ✅

**Done (v0.59.0):** landed in the existing `server` package (new files
`server/discovery.go` and `server/discovery_client.go`; see `server/doc.go`'s
new "Discovery (Milestone 46)" section). This milestone deliberately adds
directly onto the Milestone 45 package rather than a new one — `access.go`'s
own doc comment forward-referenced exactly this addition as something
Milestone 45 left ungated for Discovery to build "on top", not a fresh
package boundary. `Server.ReadDiscovery` is a narrowly-scoped, explicitly
carved bypass of the ordinary EP0 access-control gate (answerable in any
`LifecycleState` and regardless of `AccessController` grants); it does not
relax `CanAccess`/`Grant`/`Revoke` for any other address, and every other
read/write path from Milestone 45 is untouched. `Server.ClaimConfiguration`
is a new, timeout-releasable configuration-rights reservation, deliberately
independent of `AccessController.ClaimRoot` (narrower in scope, and not
wired to gate `ClaimRoot`/`AddEndpoint`/`WriteEP0` in this milestone — that
integration is left to the Phase 17 control-plane migration that will
actually build a client workflow around it, per this milestone's own scope).

- Discovery as a broadcastable, untimed, best-effort read of the register
  map starting at address 0, answerable by a server in **any** lifecycle
  state
- Discovery-stream claiming: the first discovery-triggered configuration
  attempt reserves configuration rights for its stream; a configurable
  timeout releases the reservation if no follow-up configuration request
  arrives; multiple clients can still read via discovery concurrently while
  one holds the configuration claim
- Client-side support for recognizing a conformant server (identification
  magic value, protocol version, vendor/device identification, endpoint
  count) and persisting discovered topology so re-discovery isn't mandatory
  every power cycle
- Depends on Phase 13's register-map milestone (discovery is just a read of
  that same map) and on the wire-format milestone (discovery requests use
  the untimed header exclusively — a timestamped discovery request is
  dropped)

---
### Phase 14 — TC18 Core: Basic Endpoint Types
---

### 47. GPIO + SPI Endpoints (v0.60.0) ✅

**Done (v0.60.0):** landed in two new packages, `gpio` (`gpio/doc.go`,
`gpio/types.go`, `gpio/config.go`, `gpio/semantics.go`, `gpio/request.go`,
`gpio/endpoint.go`) and `spi` (`spi/doc.go`, `spi/types.go`, `spi/config.go`,
`spi/request.go`, `spi/endpoint.go`; see each package's doc.go for its
package-level design notes, including the same explicit spec-fidelity
call-out avtp/doc.go and server/doc.go established — the exact write-
semantic count, request sub-opcode convention, and register byte layouts
below are this implementation's own reasoned encoding pending confirmation
against a public interoperability reference). Both packages build directly
on `server` (Milestones 45/46): each endpoint's functional configuration is
read/written through `server.Server.WriteFunctional`/`server.Server.ReadEndpoint`
exactly like any other endpoint's `FunctionalBlock`, and each package's
`Endpoint.HandleRequest` decodes and answers a plain `avtp.Message` using the
same request-descriptor header every endpoint type shares. `server` itself
is untouched by this milestone — no changes to its register-map, lifecycle,
or access-control code were needed.

- GPIO: up to 32 independently configured pins, a bitmask payload, and the
  eight write-semantics an incoming payload can combine with current state
  under (replace, OR, AND, XOR, saturating add/subtract, and a
  reconfiguration escape hatch), plus per-pin change/edge trigger signals
- SPI: controller-only, up to six independently pre-configured
  chip-select channels selected by the request's sub-opcode, raw
  transfer payloads, per-channel clock/mode/timing functional
  configuration, and transfer-complete/chip-select-edge triggers
- Chosen first because they have the simplest request/response payload
  shapes of the ten-plus endpoint types and exercise the read/write/
  reconfigure request shape that every later endpoint type reuses
- Depends on Phases 13's server/register-map and discovery milestones (an
  endpoint's functional config lives in, and is reached through, that same
  model) but explicitly **not** on Phase 15's conditional-request work —
  these ship against the plain, unconditional request kind only

### 48. I²C / UART / ADC / PWM Endpoints (v0.61.0) ✅

**Done (v0.61.0):** landed in four new packages, `i2c` (`i2c/doc.go`,
`i2c/types.go`, `i2c/config.go`, `i2c/request.go`, `i2c/endpoint.go`), `uart`
(`uart/doc.go`, `uart/types.go`, `uart/config.go`, `uart/request.go`,
`uart/endpoint.go`), `adc` (`adc/doc.go`, `adc/types.go`, `adc/config.go`,
`adc/request.go`, `adc/endpoint.go`), and `pwm` (`pwm/doc.go`,
`pwm/types.go`, `pwm/config.go`, `pwm/request.go`, `pwm/endpoint.go`; see
each package's doc.go for its package-level design notes, including the same
explicit spec-fidelity call-out avtp/doc.go, server/doc.go, gpio/doc.go, and
spi/doc.go established — the exact register/request byte layouts below are
this implementation's own reasoned encoding pending confirmation against a
public interoperability reference). All four packages build directly on
`server` (Milestones 45/46) exactly as gpio/spi did in Milestone 47: each
endpoint's functional configuration is read/written through
`server.Server.WriteFunctional`/`server.Server.ReadEndpoint`, and each
package's `Endpoint.HandleRequest` decodes and answers a plain `avtp.Message`
using the same request-descriptor header every endpoint type shares.
`server` itself is untouched by this milestone — no changes to its
register-map, lifecycle, or access-control code were needed. The I2C
bus-speed enum ambiguity this milestone calls out is flagged, not resolved,
in `i2c/doc.go`'s spec-fidelity note: this package assigns its own
freestanding five-value BusSpeed enumeration rather than guessing which
collision arm the source material intends. UART's read-must-be-payload-less
asymmetry versus GPIO/PWM is documented in `uart/doc.go`, including why its
future (Phase 15) compound-wait equivalent will compare against accumulated
RX FIFO content instead of a scalar register value. ADC's two
continuous-sampling mechanisms are both implemented as external callers
repeatedly invoking `adc.Endpoint.Trigger` — off another endpoint's own
`DrainTriggers` queue, or off ADC's own `TriggerMeasurementDone` events —
keeping this package's request handling synchronous like every other Phase
14 endpoint type rather than running an internal timer/goroutine. PWM models
output and input as a single endpoint type with a `Role` switch (per
`server/types.go`'s single "OUT" PWM signal name), with `RoleInput` failing
explicitly with `ErrSignalLost` on signal loss rather than returning stale
data.

- I²C: controller-only, raw byte-stream payload including address bytes
  (no protocol-level address parsing at this layer), configurable bus
  speed and inter-transaction trailing time. Flag the bus-speed enum
  ambiguity in the source spec as an open item to confirm against a later
  revision rather than hard-coding an assumption
- UART: independent TX/RX request handling sharing one functional-config
  block, FIFO-drain-or-timeout read completion with fragmented delivery of
  partial data, and the read-must-be-payload-less asymmetry versus GPIO/PWM
  (its compound-wait equivalent compares against accumulated RX data
  instead)
- ADC: single-channel, up to 16-bit resolution, a three-layer sample/
  average/combine model, and the two ways a client keeps it sampling
  continuously (triggered off another endpoint, or self-triggered off its
  own "measurement done" event) since the endpoint never samples on its own
- PWM output and PWM input: symmetric two-field (period, active-duration)
  payload shape; input is response-only and fails explicitly on signal
  loss rather than returning stale data or hanging
- Depends on the same Phase 13 milestones as GPIO/SPI; independent of each
  other within this milestone (can land in any order, or in parallel)

---
### Phase 15 — TC18 Core: Conditional Requests & Safety
---

### 49. Conditional Request Taxonomy & Sequencers (v0.62.0) ✅

**Done (v0.62.0):** landed in one new package, `request` (`request/doc.go`,
`request/kind.go`, `request/sequencer.go`, `request/envelope.go`,
`request/chained.go`, `request/ticket.go`, `request/dispatcher.go`; see
`request/doc.go` for the package-level design notes, including the same
explicit spec-fidelity call-out avtp/doc.go, server/doc.go, and every Phase
14 endpoint package's doc.go established — the envelope byte layouts, the
sequencer wraparound-advance rule, the mandatory/optional cancellation
split, and the fixed cross-type priority ordering below are this
implementation's own reasoned encoding pending confirmation against a
public interoperability reference). This package retrofits the six Phase
14 endpoint types' already-shipped plain-request path onto the same
request-lifecycle state machine by wrapping, not editing: every one of
`gpio.Endpoint`, `spi.Endpoint`, `i2c.Endpoint`, `uart.Endpoint`,
`adc.Endpoint`, and `pwm.Endpoint` already satisfies `request.Handler`'s
`HandleRequest(avtp.StreamID, avtp.Message) (avtp.Message, error)` shape
exactly as shipped, so none of those six packages needed a single line
changed. The wire-level envelope marker claims one of the two control bits
`avtp` left reserved through Milestone 48 (`avtp.FlagExtended`, a small,
additive, backward-compatible change — see avtp/doc.go's own "Milestone 49
addendum") rather than taxing every plain request with an extra body byte.
`Dispatcher` is the resulting request-lifecycle state machine for one
endpoint: every ticket, regardless of Kind — including the plain,
unconditional shape Phase 14 already shipped — advances through
StateQueued → StateStarted → StateExecuting → StateFinalized, with
`Dispatcher.Submit` handling admission/decoding (including an optional
`AccessCheck` gate this design needs precisely because `KindCompoundWait`
and every cancellation variant never reach the wrapped `Handler` at all,
and would otherwise bypass an endpoint's own access-control check
entirely) and `Dispatcher.Pump` handling readiness evaluation, the fixed
cross-type priority ordering (`Kind.Priority`), and finalization;
`Dispatcher.Dispatch` composes the two into one synchronous call for every
Kind that always resolves immediately. The two optional, narrower
cancellation variants this milestone chose — `KindCancelTransaction` (one
ticket, by `avtp.TransactionNum`) and `KindCancelSequencer` (every pending
Compound/CompoundWait ticket gated on one sequencer register, a
deliberate thematic pairing with this same milestone's Sequencer
primitive) — are this implementation's own reasoned choice of the "two
optional narrower variants" the roadmap called for, not a transcription of
the source specification's own split.

- The full conditional-request model on top of the basic request/response
  work from Phase 14: compound (sequencer-state-gated execution),
  compound-wait (a gated condition check that produces a response without
  touching endpoint output), triggered (execution keyed to another
  endpoint's trigger signal), chained (forced sequential execution of
  multiple requests in one frame), and timed (presentation-time execution
  without a timestamped header)
- Sequencers as the supporting primitive: persistent per-sequencer state
  registers that compound/compound-wait requests read and advance
- Cancellation requests (a mandatory "clear everything pending" plus two
  optional narrower variants) and the request lifecycle state machine
  (queued → started → executing → finalized, with type-specific behaviour
  at each transition) that governs how all request kinds — including the
  ones from Phase 14 — actually get scheduled and retired
- The fixed cross-type execution-priority ordering when multiple requests
  on one endpoint become due simultaneously
- Depends on every Phase 14 endpoint type existing first, since conditional
  requests are a request-handling layer that sits above specific endpoint
  behaviour, not a new endpoint type itself
- This is flagged as the single largest new-territory item for this repo:
  the old protocol has no equivalent request model of any kind, so this
  milestone is additive complexity, not a refactor of something existing

### 50. E2E CRC Safe Points & Safety Requests (v0.63.0) ✅

**Done (v0.63.0):** landed in one new package, `crcsafe`
(`crcsafe/doc.go`, `crcsafe/crc.go`, `crcsafe/guard.go`,
`crcsafe/watchdog.go`), plus a targeted extension of `request`'s own Kind
enum from Milestone 49 (`request/kind.go`, `request/envelope.go`,
`request/dispatcher.go`, `request/errors.go`; see `crcsafe/doc.go` and
request/doc.go's own "Milestone 50 addendum" for the design notes,
including the same explicit spec-fidelity call-out avtp/doc.go,
server/doc.go, and request/doc.go already established — the CRC32
polynomial choice, the exact covered-field byte layout, the
KindSafetyFlag bit position, and the StreamConfig field set are this
implementation's own reasoned encoding pending confirmation against a
public interoperability reference). `crcsafe.Compute`/`Protect`/`Verify`
implement the CRC32 safe-point mechanism as an explicit per-endpoint
opt-in mode, fully replacing the ad hoc CRC-16/CCITT-FALSE scheme the old
`e2e` package used — different polynomial (crc32.IEEE), different
coverage (the enclosing stream's addressing plus every field of the RCP
message: ByteBusID, TransactionNum, Control, ReadSizeOrSegment, Timestamp,
and Body — not just a bespoke payload), and different failure handling
(`crcsafe.Guard` skips calling the wrapped `request.Handler` outright and
reports the dedicated `ErrCRCMismatch` error, rather than the old
package's separate replay-guard framing); `crcsafe.ComputeFragmented` pins
down, ahead of Milestone 52, how this CRC coverage interacts with a
message reassembled from multiple segments (only the final segment
carries a CRC, computed over all segments combined). `request.Kind` gained
three safety-request ("MSB-set") variants — `KindCompoundSafety`,
`KindCompoundWaitSafety`, `KindTriggeredSafety` (see `Kind.IsSafety` and
`Kind.Base`) — only executable once `request.Dispatcher`'s configured
`SafeStateCheck` reports the requester's addressed endpoint is actually in
its configured safe state (`Dispatcher.Submit` refuses to admit one at all
with no `SafeStateCheck` configured), and specifically the ones that
*survive* `Dispatcher.PurgeNonSafety`, the new watchdog-driven purge of
every other pending ticket — a materially new safety mechanism with no
analogue in the old protocol at all. `crcsafe.Supervisor` is the per-stream
watchdog/sequence-monotonicity/overflow configuration driving that purge,
distinct from (and replacing, not adapting) the old client-push
`watchdog` package's model: it lives entirely server-side, timed from
request arrival via `Supervisor.Observe`, computes `InSafeState`
verdicts lazily against an injectable clock rather than pushing anything
on the wire, and adapts directly to `request.SafeStateCheck` via
`Supervisor.CheckFunc` for wiring into a `Dispatcher`. Neither the old
`e2e` nor the old `watchdog` package was touched — both remain fully
intact pending their own Milestone 53 (v0.66.0) migration, per Phase 17's
disposition table.

---
### Phase 16 — TC18 Core: Remaining Endpoints & Fragmentation
---

### 51. Remaining Endpoint Types (v0.64.0) ✅

- LIN commander: raw byte pass-through only — no frame ID/checksum/
  schedule-table awareness at this protocol layer. Flag explicitly: any
  future LIN client-side logic in go-RCP must own that framing itself; the
  endpoint does not do it for you
- CAN controller: Classical/FD/XL frame formats selected per-request, data
  frames only (no remote-frame support), with CAN XL's extra header fields
  and up to ~2 KB payloads. No trigger-signal table exists for CAN in the
  source specification at all — documented as an open gap, not silently
  invented
- ISELED: native 4b/5b-encoded daisy-chain protocol, an independent
  ISELED-native CRC layered on top of (not instead of) the general E2E
  mechanism from Phase 15, and optional multi-device response aggregation
- MDIO: minimal pass-through management-interface access (Clause 22/45
  style addressing), useful for exposing an integrated on-die PHY's
  registers even with no physical MDIO pins wired
- Wakeup control: the dedicated power-management endpoint — not a generic
  device interface — driving whole-server StandBy/Sleep transitions, the
  cold-start-vs-hot-start distinction, and the repeating wake-handshake
  message a server sends on waking from Sleep
- **DAC is explicitly out of scope for this milestone and this repo's
  v1.0.0 target.** The type code and its pin signal are enumerated in the
  specification, but no register map or request semantics exist for it in
  this revision — there is nothing conformant to build against. Revisit
  only if a later specification revision defines it
- Depends on Phase 14's endpoint groundwork and Phase 15's request model
  (LIN/CAN/ISELED all need conditional-request support to be useful in
  practice); Wakeup control additionally depends on Phase 13's server
  lifecycle (it drives that same state machine's power dimension)

**Done (v0.64.0):** landed in five new packages, one per endpoint type —
`lin`, `can`, `iseled`, `mdio`, `wakeup` — each following the
doc.go/types.go/config.go/request.go/endpoint.go shape Phase 14's six
packages established, each building on `server`'s register-map substrate
(Milestones 45/46) and dropping into `request.Dispatcher` unmodified
(Milestone 49) via the same `Endpoint.HandleRequest(avtp.StreamID,
avtp.Message) (avtp.Message, error)` shape every endpoint type shares; no
changes were needed to `server`, `avtp`, `request`, or `crcsafe`
themselves, since their extension points (the endpoint-type enum and
named-signal-index scheme in `server/types.go`) already anticipated these
five types. `lin.Endpoint` is raw byte pass-through only, exactly as
specified — it has no PID/checksum/schedule-table awareness at all,
flagged explicitly in `lin/doc.go` as future LIN client-side logic's own
responsibility to own, not this endpoint's. `can.Endpoint` selects
Classical/FD/XL framing per request via `can.Frame.Format` (payload caps
8/64/2048 bytes respectively), has no remote-frame field of any kind (data
frames only, structurally rather than by runtime rejection), and carries
CAN XL's extra header fields (`can.XLHeader`'s SDT/VCID/AF, reflecting the
publicly documented ISO 11898-1 CAN XL frame format, not TC18-specific
text); per Guiding Principle 10, `can/doc.go` documents CAN's absent
trigger-signal table as an open gap rather than inventing one — `can`
is the one Phase 16 package with no `DrainTriggers` method, a deliberate,
documented omission. `iseled.Endpoint` models a daisy chain addressed by
`iseled.Command`/`iseled.DeviceResponse`, each carrying its own
ISELED-native CRC8 (`iseled.ComputeCRC`) layered on top of — not instead
of — the general E2E mechanism `crcsafe` (Phase 15) already provides at
the RCP-message level (a caller is free to additionally wrap
`iseled.Endpoint` in `crcsafe.Guard`); `iseled.DeviceBroadcast` plus
`iseled.AggregatedResponse` implement the optional multi-device response
aggregation. `mdio.Endpoint` is minimal pass-through Clause 22/45-style
register access (`mdio.Request`'s `AddressMode`-selected PHY/device/
register addressing), useful even with no physical MDIO pins wired since
its `Transport` abstraction never assumes a real electrical bus underneath
it. `wakeup.Endpoint` is the dedicated power-management endpoint — its
`wakeup.PowerState` write/read drives Normal/StandBy/Sleep transitions
(`wakeup.PowerUnpowered` is deliberately never a requestable target or the
endpoint's own current state, flagged in `wakeup/doc.go` as this
implementation's own reasoned treatment of a state a running server cannot
itself request or observe being in), a Sleep→Normal wake determines
`wakeup.StartKind` (Hot/Cold, see `Endpoint.SetRetentionLost`) and queues
`Config.WakeHandshakeRepeatCount` repeating `wakeup.WakeHandshake` trigger
events for a caller's own transport loop to re-emit (`wakeup/doc.go`
documents this package's own reasoned, flagged-as-unverified reading of how
its power dimension relates to `server.LifecycleState`'s orthogonal
configuration-readiness axis from Phase 13, since the roadmap text here
does not spell out a more specific mechanical coupling and no
`server/lifecycle.go` change was made to invent one). DAC remains entirely
absent, as specified — no `dac` package exists. All five packages carry
the same explicit spec-fidelity disclaimer already established across
`avtp`/`server`/every endpoint package; `can`'s XL header field names and
`iseled`'s CRC-8 parameter choice are called out individually in their own
doc.go files as reflecting public reference material rather than
TC18-specific text. 44 new `//fusa:req`/`//fusa:test`-tagged requirements
(`REQ-LINEP-*`, `REQ-CANEP-*` — `EP` suffixes chosen specifically to avoid
colliding with the pre-existing `canbr`/`linbr` satellite packages'
already-claimed `REQ-CAN-*`/`REQ-LIN-*` IDs, since those are unrelated,
separately-scoped Phase 17 migration candidates — `REQ-ISELED-*`,
`REQ-MDIO-*`, `REQ-WAKEUP-*`) are 100% traced and tested per `gofusa
check`/`gofusa trace`.

### 52. Fragmentation (v0.65.0) — **GO** ✅

Fragmentation of a single logical request/response across multiple
physically-transmitted frames is explicitly optional in the specification.
This roadmap makes an explicit call rather than leaving it implicit:

**Decision: implement it, as a requirement for this repo's own v1.0.0, not
an optional add-on.** Rationale:

- CAN XL payloads (Phase 16) can run large enough that they cannot fit in
  a single frame at all on realistic MTUs — without fragmentation, CAN XL
  support from the previous milestone is fiction
- UART's read-with-timeout completion path (Phase 14) is explicitly
  designed around fragmented delivery of a partial FIFO drain; shipping
  UART without it means shipping a materially crippled endpoint
- Full register-map discovery reads (Phase 13) can exceed one frame's
  payload as the register map grows with endpoint count; without
  fragmentation, discovery has a silent, undocumented topology-size limit
- The specification's own fragmentation/E2E-CRC interaction rule (only the
  final segment carries the CRC, computed across all combined segments)
  only needs to be built once, and every endpoint type above benefits from
  it existing — better to build it deliberately in one dependency-ordered
  milestone than have three endpoint types each grow ad hoc partial
  workarounds
- Cost is bounded and well-scoped: this is a segmentation/reassembly layer
  on top of the wire format from Phase 13, not a new protocol concept

If a future spec revision changes fragmentation's status, this decision
should be revisited, but "don't build it" was rejected as leaving three
already-planned endpoint types (CAN XL, UART, and large-topology discovery)
either broken or quietly non-conformant.

**Done (v0.65.0):** landed in the new `fragment` package (`fragment/
doc.go`, `types.go`, `errors.go`, `segment.go`, `reassembler.go`,
`gateway.go`), following the same doc.go-first, spec-fidelity-disclaimed
package shape every Phase 13-16 milestone established, and consuming —
without modifying — every hook staged for it ahead of time: `avtp`'s
`FlagMoreSegments` bit and dual-purpose `ReadSizeOrSegment` field
(`avtp/message.go`), `crcsafe.ComputeFragmented` (`crcsafe/crc.go`), and
`request.Dispatcher.Submit`'s existing signature (`request/dispatcher.go`).
`fragment.Split` is the send-side half: it divides one logical
`avtp.Message` into ordered segments, setting `FlagMoreSegments` and a
0-based segment number on every segment but the last, and restoring the
last segment's own `ReadSizeOrSegment` to its original (non-segment-number)
meaning, exactly matching the field semantics `avtp/message.go` already
committed to. `fragment.Reassembler` is the receive-side half: `Add`
accumulates segments keyed by `fragment.Key` (stream/bus/transaction,
mirroring `avtp/doc.go`'s own addressing-scope rules) until a terminal
segment arrives — treating an ordinary unfragmented `Message` as a trivial,
immediately-complete one-segment sequence so a caller never special-cases
"is this fragmented?" — and `Finish`/`FinishProtected` return the
reassembled `Message`, the latter verifying the "only the final segment
carries the CRC, computed over the combined body" rule by calling
`crcsafe.ComputeFragmented` directly over the sequence's own per-segment
bodies. `fragment.Gateway` wraps a `request.Dispatcher` (via the minimal
`Submitter` interface, so neither package imports the other's concrete
type) so a fragmented request is fully reassembled before `Submit` ever
sees it and participates in the ordinary `StateQueued`/`StateStarted`/
`StateExecuting`/`StateFinalized` lifecycle unmodified, with
`Gateway.Response` as the symmetric send-side convenience for a resolved
response too large for one AVTPDU. Neither `can`, `uart`, nor
`server/discovery.go` needed any change: their existing request/response
bodies are simply segmentable by a caller that chooses to route them
through `Split`/`Gateway`, validated directly against representative CAN
XL-sized, UART-FIFO-sized, and discovery-register-map-sized payloads in
this package's own test suite. Per Guiding Principle 10, out-of-order
segments, duplicate segments, and a stalled/abandoned sequence are not
addressed by this roadmap's own text and are documented as this
implementation's own reasoned, reversible choices in `fragment/
reassembler.go`'s own doc comment: strict in-order-only non-terminal
segment numbering (abandoning a sequence on a gap or reordering, since the
terminal segment carries no segment number of its own once
`FlagMoreSegments` clears — a wire-format consequence, not a policy
choice, also documented there), silent tolerance of an exact-duplicate
retransmission, a caller-driven (`Sweep`, no background goroutine)
staleness timeout that only ever purges an incomplete sequence, and a
`Config.MaxSegments` bound against a sequence that never terminates. 12 new
`//fusa:req`/`//fusa:test`-tagged requirements (`REQ-FRAG-*`) are 100%
traced and tested per `gofusa check`/`gofusa trace`.

---
### Phase 17 — Satellite Package Migration
---

Every package this repo ships outside the core protocol gets one of four
calls: **REPLACE** (the current implementation encodes the wrong model
entirely and needs new logic, not new call sites), **ADAPT** (the
underlying mechanism/algorithm is still sound; it needs to be re-pointed at
the new Endpoint/request types), **DEPRECATE** (no place in the new model),
or **KEEP AS-IS** (genuinely orthogonal — unaffected by the protocol
replacement). No package is carried forward without an explicit call.

#### Disposition table

| Package | Call | Reason |
|---|---|---|
| `wire` | REPLACE | Its entire reason to exist is the old bespoke 16-byte frame header; the new wire format is IEEE 1722 AVTPDU/ACF (Phase 13), not a variant of this one |
| `udp` | REPLACE | Framing on the wire is the old `wire` package's header; the socket I/O scaffolding may carry over, but everything it encodes/decodes must become AVTPDU/ACF |
| `tlstransport` | DEPRECATE | The specification's link-security story is MACsec (802.1AE) at layer 2, opaque/product-specific per the register map, not mutual TLS over TCP; TLS-over-TCP doesn't fit the stream/AVTPDU addressing model. Revisit only as a bespoke, clearly-labelled non-spec transport option, not as "the" secure transport |
| `tsn` | ADAPT | IEEE 802.1Qbv/802.1Qav scheduling is a layer-2 QoS mechanism that is genuinely complementary to (not in conflict with) real IEEE 1722 delivery, and the specification's own time-synchronization bundle leans on the same gPTP foundation TSN already uses here. Keep the scheduler integration, replace the framing calls it wraps |
| `shmem` | ADAPT | Zero-copy intra-host IPC is a transport-layer optimization independent of frame shape; retarget its payload layout at the new request/response types |
| `loan` | ADAPT | The `sync.Pool`-backed zero-copy loaning pattern is not protocol-specific; only the pooled type changes |
| `mdns` | REPLACE | The specification defines its own mandatory, self-contained discovery mechanism (Phase 13) that every conformant server must answer in any lifecycle state; the old package's DNS-SD service-record model (zone ID as a service instance) has no mapping onto register-map-based discovery. May survive, at maintainer discretion, as an optional secondary IP-rendezvous helper for the UDP/IP transport variant — but it is not "the" discovery mechanism going forward |
| `e2e` | REPLACE | Wrong CRC entirely (CRC-16/CCITT-FALSE vs. the specification's CRC32 with a specific polynomial and coverage), and missing the safety-request/watchdog-purge model that doesn't exist in the old design at all (Phase 15) |
| `powerstate` | REPLACE | The three-state Active/Sleeping/BusOff model has no relationship to the specification's Normal/StandBy/Sleep/Unpowered model, its cold-start/hot-start distinction, or the wake-handshake message sequence (Phase 16's Wakeup endpoint) |
| `watchdog` | REPLACE | Architecture inversion, not a refactor: the old package is an HPC-side periodic push of a keepalive command; the specification's watchdog is server-side, reset by every inbound request, and drives automatic safe-state entry (Phase 15) rather than client-observed health states |
| `deadline` | REPLACE | Built around a periodic `Status` broadcast concept that doesn't exist in the new model; the nearest equivalents are per-endpoint triggers and response-queue heartbeat flushes, which have different failure semantics and need to be modeled from scratch |
| `prioqueue` | REPLACE | Priority is no longer a client-assigned enum; the specification fixes a fully-ordered execution priority by request *kind* (cancellation, triggered, timed, compound, compound-wait, chained, standard — Phase 15). A client-side priority queue needs to be rebuilt around choosing the right request kind, not tagging an arbitrary priority value |
| `ratelimit` | ADAPT | Token-bucket admission control is an algorithm independent of what's being rate-limited; re-key it by stream/endpoint instead of Zone |
| `authz` | ADAPT | The specification bakes a coarse access-control primitive into the server itself (root-client vs. per-endpoint-restricted streams); a client-side policy layer still has legitimate defense-in-depth value, rebuilt around stream/endpoint identity rather than Zone/CommandType, and explicitly positioned as a complement to — not a duplicate of — the server's own enforcement |
| `redundancy` | ADAPT | Hot-standby failover between two controllers is a pattern independent of what "controller" means underneath; re-point it at the new Controller-equivalent interface |
| `federation` | ADAPT | Ownership/leasing coordination across multiple HPCs is reusable; re-key ownership by server/endpoint instead of Zone |
| `zonegroup` | ADAPT | Atomic multi-target broadcast-and-collect is reusable; re-target it at endpoint groups. Note the specification already lets one frame carry several independently-addressed requests, so this package's role narrows to client-side ergonomics on top of that, not the only way to achieve it |
| `proxy` | ADAPT | The intercept/transform/forward pattern is reusable, but a real RCP-level proxy must handle stream_id/byte_bus_id remapping — an area the specification itself flags as a client-side responsibility with no server-side safety net (Phase 13). Rebuild carefully, not mechanically |
| `admin` | ADAPT | HTTP inspection surface is reusable; the data model moves from zones to servers/endpoints |
| `config` | ADAPT | YAML/JSON config loading is reusable; the schema moves from a zone registry to server/stream/register-map configuration |
| `dyndata` | ADAPT | A runtime schema registry for interpreting raw payload bytes is, if anything, more useful now — every endpoint's request/response payload is raw bytes with type-specific shape, exactly what this package already exists to decode |
| `canbr` | ADAPT | CAN is now a native RCP endpoint type (Phase 16), so "bridge RCP to CAN" narrows from a translation necessity to an ergonomics layer — e.g. exposing a familiar CAN-bus-shaped API on top of the native CAN endpoint for existing consumers. Rebuild the framing calls; the bridging concept survives in reduced scope |
| `linbr` | ADAPT | Same shift as `canbr` for LIN — but note the specification's LIN endpoint does *no* frame-level work (Phase 16), so whatever PID/checksum/schedule-table logic this package used to delegate elsewhere, it now has to own client-side |
| `ddsbr` | ADAPT | DDS pub/sub telemetry fan-out is not something TC18 RCP does natively; this bridge remains genuinely necessary, just re-pointed at endpoint responses/triggers instead of `Status` |
| `mqttbr` | ADAPT | Same reasoning as `ddsbr` — MQTT cloud/telematics integration is orthogonal to the core protocol and stays necessary, just re-pointed at the new types |
| `someip` | ADAPT | SOME/IP service-method bridging is orthogonal to TC18 RCP; re-point at endpoint requests/responses |
| `udsbr` | ADAPT | UDS diagnostics is unrelated to TC18 RCP's endpoint model; re-point at the new request/response types. Also a candidate transport for the firmware package's chunked-transfer needs (see `firmware` below) |
| `doipbr` | ADAPT | Same reasoning as `udsbr` |
| `grpcbridge` | ADAPT | Remote/cloud RPC access is orthogonal; re-point at the new Controller-equivalent interface |
| `restbridge` | ADAPT | Same reasoning as `grpcbridge` |
| `mock` | REPLACE | The reference test double must actually implement the new server/endpoint/register-map model to be useful for testing anything built in Phases 13-16; this is close to a from-scratch rewrite even though its *purpose* (in-process fake for unit tests) is unchanged |
| `sim` | REPLACE | A timing-realistic simulator needs realistic per-endpoint-type timing models (ADC sample intervals, PWM cycle timing, and so on) that don't exist in the old Zone model at all |
| `capi` | REPLACE | Its C ABI surface directly mirrors `Controller`/`Command`; once those are replaced, the exported struct/function layer has to be redesigned around Endpoint requests, not just recompiled against new types |
| `codegen` | ADAPT | Manifest-to-stub code generation is reusable; the manifest schema moves from zone declarations to server/endpoint declarations |
| `record` | ADAPT | Append-only checksummed traffic recording doesn't care what's inside the frames it records; update the captured frame type, keep the log format and replay engine |
| `observe` | ADAPT | OpenTelemetry tracing/metrics decoration is a generic wrapper pattern; re-point at the new Controller-equivalent interface |
| `faultinject` | ADAPT | Structured fault injection as a harness pattern is reusable; the fault-type catalogue needs updating to match the new safety mechanisms (CRC failure, safe-state entry, discovery-claim timeout, cancellation) in place of the old watchdog/E2E/replay-guard specifics |
| `firmware` | ADAPT | OTA firmware delivery is explicitly outside TC18 RCP's scope (the spec covers low-level interface access, not application/firmware distribution) but remains useful as an OEM-layer convenience riding on top of a raw-byte endpoint (UART/SPI) or the UDS bridge; keep the chunking/rollback/integrity logic, rebuild the transport calls underneath it |
| `formal` | REPLACE | The TLA+ models verify state machines (zone health, client-push watchdog, anti-replay window) that are being replaced outright; the modeling *methodology* carries over, but new proofs must be authored from scratch against the new lifecycle/power/safe-state machines |
| `iso21434` | ADAPT | TARA methodology and IEC 62443 gap-report tooling are reusable; the actual threat model and countermeasure mapping must be redone once the attack surface (new addressing, new discovery-claim window, new safety-CRC) changes |
| `certgap` | ADAPT | ASIL-D gap-analysis tooling is reusable; content must be regenerated against the new requirement set once Phases 13-16 produce one |
| `safety` | ADAPT | Latency/timing evidence generation (and its GSN-argument writeup) is protocol-agnostic; re-point measurement at the new request/response path |
| `admin`, `config`, `dyndata` HTTP/YAML surfaces | *(see individual rows above)* | — |
| `cmd/go-rcp`, `cmd/rcptool` | ADAPT / DEPRECATE | `go-rcp` is the RELAY-conformant CLI and must be rebuilt against the new model as part of the Phase 18 cutover (it is not a "satellite package" in the same sense — it's load-bearing for RELAY conformance, see Phase 18). `rcptool` is already documented as the older, non-conformant CLI kept only for backward compatibility; once the API it targets no longer exists, retire it rather than port it |
| `mock`'s test-only siblings (`examples/`, `docker/`) | ADAPT | Quickstart/demo code, not library packages; update once `mock` (above) is rebuilt, low urgency |

Root-module files (`rcp.go`, `adapt.go`, `optional.go`, `conformance_test.go`)
are not in this table because they're not satellite packages — they're the
core module and the RELAY-conformance shim on top of it. Their replacement
is Phases 13-16 (the types themselves) plus Phase 18 (re-satisfying
`Adapt`/`relay.Caller`, the optional `Health`/`Metrics`/`Drainer`
interfaces, and the golden-vector conformance tests against the new types).

### 53. Safety & Liveness Rebuild (v0.66.0) ✅

- Rebuild `e2e`, `powerstate`, `watchdog`, `deadline`, and `prioqueue` per
  the disposition table above
- These come first among the satellite migrations because every later
  bridge/tooling package either wraps one of these five directly or
  assumes the health/priority/power model they define

**Done (v0.66.0):** all five REPLACE-flagged packages rebuilt, none
adapted, per the disposition table's own reasoning for each:

- **`e2e` retired outright, no successor package.** Its entire job — CRC
  protection over the wire — is already fully superseded by `crcsafe`
  (Milestone 50, v0.63.0): `crcsafe.Compute`/`Protect`/`Verify`/`Guard`
  cover both the sender and receiver sides with the specification's own
  CRC32 coverage, and nothing about the old package's client-side
  sequence-counter framing has a meaningful new-model equivalent (the new
  protocol correlates by `avtp.TransactionNum`, not a client-maintained
  counter). Building a facade package here would only re-export `crcsafe`
  under a different name for no behavioral benefit, so the package is
  removed rather than kept as an empty wrapper.
- **`watchdog` rebuilt from scratch as orchestration glue**
  (`watchdog/doc.go`, `keeper.go`): `crcsafe/doc.go` documents the
  Supervisor-to-Dispatcher wiring (`SetSafeStateCheck`/`PurgeNonSafety`) as
  two lines a caller writes itself, on whatever cadence it judges
  appropriate, for one stream and one Dispatcher at a time. `watchdog.Keeper`
  is the missing piece for a server with more than one endpoint: `Watch`
  builds up a (stream → Dispatchers) association, and `Keeper.Tick` — a
  synchronous, caller-driven sweep, the same posture `crcsafe.Supervisor`
  and `fragment.Reassembler.Sweep` already establish — checks every watched
  stream's `InSafeState` and purges every Dispatcher registered against a
  tripped one in a single call, reporting a `PurgeEvent` per tripped stream
  (including a tripped-but-nothing-to-purge stream, kept distinguishable
  from a never-tripped one).
- **`powerstate` rebuilt from scratch as the wake-handshake pacing loop**
  (`powerstate/doc.go`, `driver.go`): `wakeup/doc.go` (Milestone 51,
  v0.64.0) explicitly leaves "a caller's own transport loop" responsible for
  re-emitting the repeating wake-handshake message at
  `wakeup.Config.WakeHandshakeIntervalMillis` cadence — nothing in `wakeup`
  paces or transmits it. `powerstate.Driver.Pump` drains a
  `wakeup.Endpoint`'s trigger queue, relays every `TriggerPowerStateChanged`
  as an `Event`, and transmits at most one queued `TriggerWakeHandshake`
  repeat per call through a caller-supplied `Transmitter` — a failed send
  leaves the repeat at the front of the queue for the next call to retry
  rather than losing it. `Driver.Acknowledge` forwards to
  `Endpoint.AcknowledgeWake` and additionally discards whatever this Driver
  had already pulled off that queue but not yet sent.
- **`deadline` rebuilt from scratch around a three-state liveness model**
  (`deadline/doc.go`, `monitor.go`, `queueconfig.go`): the roadmap's own
  text names two different-failure-semantics substitutes for the retired
  periodic-`Status`-broadcast concept — per-endpoint triggers (every
  endpoint type's own `Trigger`/`DrainTriggers`, Phases 14-16) and
  response-queue heartbeat flushes (`server.QueueConfig`.
  `HeartbeatIntervalMillis`, Milestone 45, `server/queues.go`) — and this
  package treats them as genuinely distinct signal classes rather than
  collapsing them into one boolean: `Monitor.ObserveTrigger` and
  `Monitor.ObserveHeartbeat` each update independent bookkeeping, and
  `Monitor.State` reports `LivenessAlive` (a trigger within `Deadline`),
  `LivenessIdle` (only a heartbeat within `Deadline` — the link is up, but
  nothing is happening upstream of it), or `LivenessDead` (neither). No
  goroutine, no timer — verdicts are computed lazily against an injectable
  clock, the same posture `crcsafe.Supervisor` already established.
  `DeadlineForQueue` derives a plausible `Config.Deadline` directly from a
  `server.QueueConfig`'s own `HeartbeatIntervalMillis`, so the two configs
  cannot silently drift apart.
- **`prioqueue` rebuilt from scratch around `request.Kind.Priority`**
  (`prioqueue/doc.go`, `queue.go`): the old client-assigned
  `PriorityCritical`/`High`/`Normal` enum has no equivalent at all in a
  protocol where `request.Kind` (Milestone 49) already fixes a total
  cross-type execution-priority ordering by request *kind*. The rebuilt
  `Queue` is a `container/heap`-backed structure identical in shape to the
  old one, but ranks strictly by `request.Kind.Priority()` (cancellation >
  chained > triggered > timed > compound-wait > compound > plain) with
  FIFO tie-breaking, so a caller expresses urgency by picking the request
  Kind that already matches what it's doing rather than tagging an
  arbitrary value the server would ignore anyway.

No other package in this repo imported any of the five old packages outside
their own test files, so no additional call sites needed updating.
22 new/updated `//fusa:req`/`//fusa:test`-tagged requirements replace the 40
retired ones (`REQ-WDG-*`, `REQ-PWR-*`, `REQ-DL-*`, `REQ-PQ-*` rewritten in
place under their original prefixes since each still names the same
package; `REQ-E2E-*` removed outright with no successor family, since
`e2e`'s role is now `REQ-CRC-*`'s), 100% traced and tested per `gofusa
check`/`gofusa trace`.

### 54. Transport & Discovery Migration (v0.67.0) ✅

- Rebuild `wire`/`udp` against the Phase 13 wire format; adapt `tsn` and
  `shmem`/`loan`; retire `mdns`'s role as primary discovery (Phase 13
  supersedes it); deprecate `tlstransport`

**Done (v0.67.0):** every package the disposition table calls out for this
milestone is rebuilt/adapted/narrowed/deprecated per its own reasoning
there:

- **`wire` retired outright, no successor package**, the same "no successor"
  precedent Milestone 53 (v0.66.0) already established for the old CRC-16
  `e2e` package: once `avtp.Header`/`acf.Message`/`acf.Frame` (Phase 13)
  exist, they already are the new wire format, so a separate `wire` package
  wrapping them would only re-export `acf.EncodeFrame`/`DecodeFrame` under
  new names for no behavioral benefit. `udp` imports `avtp`/`acf` directly;
  UDP's own per-datagram framing needs no additional length-prefixing on top
  of `acf.Frame`'s bytes, since one UDP datagram already carries exactly one
  AVTPDU.
- **`udp` rebuilt as an IEEE 1722-over-UDP/IP transport**
  (`udp/controller.go`, `server.go`, `router.go`, `ep0.go`, `registry.go`,
  `frame.go`) — the transport variant `avtp/doc.go`'s own "Explicit
  non-goal" section had already flagged and deferred to this milestone.
  `udp.Controller` addresses one destination RC Server by UDP address and
  presents its own `avtp.StreamID` identity, correlating requests to
  responses by `acf.Message.TransactionNum` in place of the retired
  `wire`/`udp`'s uint32 `Command.ID`. `udp.Server` decodes each inbound
  datagram and hands it to a `udp.Router`, which special-cases
  `regmap.EP0` (configuration/discovery, via `udp.EP0Handler` wrapping a
  `*server.Server` — the first place in this repo any package turns
  Milestones 45/46's Go-level `server.Server` calls into on-wire RCP-over-
  ACF traffic) and otherwise looks up a caller-registered `request.Handler`
  by `avtp.ByteBusID` — the exact interface every Phase 14/16 endpoint-type
  package's own `Endpoint.HandleRequest` method already satisfies, so
  wiring in `gpio.Endpoint`, `spi.Endpoint`, or a `request.Dispatcher`
  wrapping one, needs no adapter code. `Router.Route` also owns the one
  dispatch-wide decision every endpoint type shares — computing
  `avtp.Header.Disposition` against the server's own time-sync capability
  and dropping (no reply at all) a timestamped AVTPDU a non-time-
  synchronized server cannot honor — so no registered `Handler` has to
  reimplement that rule itself. This milestone's `Controller` originates
  only untimed (NTSCF) requests and executes a `DispositionScheduled`
  request immediately (identically to best-effort); actual schedule-and-
  wait-for-the-timestamp behaviour has no clock source to target yet and is
  left as a documented follow-on, per Guiding Principle 10.
- **`tsn` adapted, keeping its SO_PRIORITY socket wiring unchanged**
  (`sockprio_linux.go`/`sockprio_other.go` needed no edits at all — they
  only ever depended on `udp.Controller.RawConn`, never on the retired
  framing). `tsn.Controller` now wraps `*udp.Controller` and derives its
  IEEE 802.1p PCP value from `request.Kind.Priority()`'s fixed cross-type
  rank (Milestone 49) via `PCPMap`, in place of the retired three-level
  `rcp.Priority` enum this repo no longer has a `Priority`-tagged request
  type for.
- **`shmem` adapted around the same `udp.Router`** the networked transport
  uses (`shmem.Bus`/`Endpoint`/`Controller`), rather than re-implementing
  EP0/endpoint-`Handler` routing a second time — since Router's dispatch
  logic is exactly the frame-shape-independent part the disposition
  table's own "zero-copy intra-host IPC is independent of frame shape"
  reasoning calls for reuse of. The retired package's `sync.Pool`-backed
  `poolAlloc` never actually reused a popped buffer's backing array for its
  fresh copy and is not carried over; `shmem.Controller.Request` still
  copies a request body exactly once onto the bus (`copyBody`), just
  without a pool that would only be safe to recycle from with the `loan`
  package's own explicit-release tracking.
- **`loan` adapted to wrap `*udp.Controller` concretely** in place of the
  retired `rcp.Controller`/`rcp.LoaningController` interface pair — both
  root-module contracts Phase 17's disposition table explicitly leaves to
  Phase 18, and which `udp.Controller` cannot meaningfully implement before
  then. `loan.Controller.Loan`/`RequestLoaned` replace the old
  `Loan`/`SendLoaned` pair; the pooled buffer type itself is unchanged —
  still `*rcp.Loan`, the root package's own already wire-agnostic
  Payload+release-func struct — matching the disposition table's "only the
  pooled type changes" framing.
- **`mdns`'s role narrowed, not deleted**: `Announcer`/`Browser` now
  advertise/discover an `avtp.StreamID` (hex-encoded in a `stream=` TXT
  value) in place of the retired `Zone` enum's `zone=` value, since DNS-
  SD's zone-ID-as-service-instance model has no mapping onto TC18
  addressing — every other DNS-SD wire mechanic (`encodeName`/`decodeName`,
  the PTR/SRV/TXT/A record shapes, `Transport`) is unchanged, genuinely
  orthogonal to RCP's own wire format. This package is no longer presented
  as the discovery mechanism a caller relies on for a server's register
  map — that is `udp.Controller.Discover` calling into Milestone 46's own
  `HandleDiscoveryRequest` — and survives only as the disposition table's
  own "optional secondary IP-rendezvous helper": a way to learn candidate
  UDP addresses to point `Discover` at in the first place.
- **`tlstransport` marked `Deprecated` in its package doc**, per the
  disposition table's "revisit only as a bespoke, clearly-labelled non-spec
  transport option, not as 'the' secure transport" framing — kept, not
  deleted, but explicitly never migrated to `avtp`/`acf`/`server`. Since
  `wire` (the package it depended on) is retired, `tlstransport` now
  carries its own frozen, package-local copy of the old bespoke frame
  encode/decode logic (`legacyframe.go`) so it keeps compiling and
  behaving exactly as before, with no behavioral change and no MACsec
  implementation attempted here (out of this milestone's scope, per the
  roadmap's own framing).

`REQ-WIRE-*` removed outright with no successor family, the same
"no successor" disposition Milestone 53 applied to `REQ-E2E-*`.
`REQ-UDP-*`, `REQ-TSN-*`, `REQ-SHMEM-*`, `REQ-LOAN-*`, and `REQ-MDNS-*` are
rewritten in place under their original prefixes to describe the new
behaviour (`REQ-SHMEM-007`/`008`, the retired Subscribe/Status
requirements, have no new-model equivalent and are dropped; `REQ-UDP-013`/
`014` are new). `REQ-TLS-*` is unchanged, since `tlstransport`'s own
behaviour did not change. 40 requirements total across the five rebuilt/
adapted packages, 100% traced and tested per `gofusa check`/`gofusa trace`.
No other package in this repo imported `wire`, `udp`, `tsn`, `shmem`,
`loan`, `mdns`, or `tlstransport` outside their own test files, so no
additional call sites needed updating.

### 55. Control-Plane & Topology Adaptation (v0.68.0)

- Adapt `authz`, `ratelimit`, `redundancy`, `federation`, `zonegroup`,
  `proxy`, `admin`, `config`, `dyndata` per the table above
- Depends on Phase 53/54 landing first (these packages compose on top of
  the Controller-equivalent and health/priority primitives those phases
  define)

### 56. Protocol Bridge Adaptation (v0.69.0)

- Adapt `canbr`, `linbr`, `ddsbr`, `mqttbr`, `someip`, `udsbr`, `doipbr`,
  `grpcbridge`, `restbridge` per the table above
- `canbr`/`linbr` specifically need to absorb frame-level logic (LIN
  PID/checksum/schedule tables in particular) that the native endpoints
  deliberately don't provide (Phase 16)

### 57. Tooling & Test-Double Rebuild (v0.70.0)

- Rebuild `mock`, `sim`, and `capi`; adapt `codegen`, `record`, `observe`,
  `faultinject`, `firmware`
- Sequenced after the bridges (56) because `sim`'s realistic timing models
  and `faultinject`'s fault catalogue are easiest to validate once real
  endpoint behaviour exists to compare against

### 58. Certification & Formal-Methods Refresh (v0.71.0)

- Rebuild `formal`'s TLA+ specifications from scratch against the new
  lifecycle/power/safe-state machines; adapt `iso21434`, `certgap`,
  `safety`
- Deliberately last among the satellite migrations: certification
  artifacts describe behaviour that needs to already exist and be stable

---
### Phase 18 — Cutover
---

### 59. TC18 Conformance Cutover & RELAY Re-Certification (v1.0.0)

- Remove the legacy `Zone`/`Command`/`Response`/`Status`/`Controller`/
  `Registry` API surface once every satellite package that depended on it
  has migrated (Phases 53-58)
- Rebuild `Adapt`/`relay.Caller`, the optional `Health`/`Metrics`/`Drainer`
  interfaces, and the `go-rcp` CLI (`version`/`capabilities`/`status`/
  `send`/`monitor`/`convert`) against the new Endpoint/register-map model
  so RELAY conformance — the golden-vector tests, the schema conformance
  checks, and CI's `relay conform --strict` gate — is satisfied for the new
  protocol rather than silently dropped
- Update `.fusa-reqs.json`/`.fusa-hara.json` for the new requirement/hazard
  set; the old REQ-ZONE/REQ-CMD/REQ-PRI families retire with the API they
  described
- This is the version where go-RCP first claims OPEN Alliance TC18 Remote
  Control Protocol Specification v0.5.1_RC conformance; tag as `v1.0.0` to
  signal the breaking change through semver rather than softening it
