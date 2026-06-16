# go-RCP Roadmap

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
| **Transport stack** | v0.5.0 | UDP transport | Pure-Go UDP command/response transport with zone discovery |
| **Transport stack** | v0.6.0 | mDNS discovery | Zero-configuration zone controller discovery via mDNS/DNS-SD |
| **Transport stack** | v0.7.0 | TLS transport | Mutual TLS channel for zone-controller communication |
| **Transport stack** | v0.8.0 | Shared memory | Zero-copy intra-host command delivery via shared memory |
| **Transport stack** | v0.9.0 | Loaned samples | LoaningController interface extending zero-copy to all transports |
| **Transport stack** | v0.10.0 | TSN transport | IEEE 802.1Qbv-aware UDP transport for hard real-time Ethernet delivery |
| **Safety mechanisms** | v0.11.0 | Watchdog & heartbeat | CmdWatchdog scheduling, zone health state machine, liveness API |
| **Safety mechanisms** | v0.12.0 | Deadline monitoring | Zone-to-HPC liveness: alert when Status stops arriving within deadline |
| **Safety mechanisms** | v0.13.0 | Power state | CmdSleep/CmdWake, zone power state machine, bus-off recovery |
| **Safety mechanisms** | v0.14.0 | E2E protection | Sequence counter, CRC-16, replay guard on command frames |
| **Safety mechanisms** | v0.15.0 | Priority queuing | Per-zone priority queue honouring PriorityCritical/High/Normal |
| **Safety mechanisms** | v0.16.0 | Rate limiting | Per-zone token-bucket admission control against command flooding |
| **Verification** | v0.17.0 | Zone simulator | Timing-realistic zone controller simulator for SiL/HIL testing |
| **Verification** | v0.18.0 | Fault injection | Structured fault injection to validate watchdog, E2E, and replay-guard mechanisms |
| **Security** | v0.19.0 | Authorization | Command-level access control; ISO 21434 SL-2 policy enforcement |
| **Security** | v0.20.0 | Firmware update | CmdUpdate and firmware/ package for zone controller OTA delivery |
| **Topology** | v0.21.0 | Zone groups | Atomic multi-zone command broadcast with typed zone group sets |
| **Topology** | v0.22.0 | Zone proxy | Transparent zone proxy for multi-hop zonal topologies |
| **Topology** | v0.23.0 | Redundancy | Hot-standby Registry and HPC failover for ASIL-B fault tolerance |
| **Topology** | v0.24.0 | Multi-HPC federation | Multi-HPC active coordination over shared zone bus |
| **Tooling** | v0.25.0 | Observability | OpenTelemetry traces and Prometheus metrics adapter |
| **Tooling** | v0.26.0 | Admin API | HTTP admin interface for runtime registry inspection and control |
| **Tooling** | v0.27.0 | Record & replay | Record command/response/status streams to disk; replay for regression and forensics |
| **Tooling** | v0.28.0 | Config | YAML/JSON zone registry configuration |
| **Tooling** | v0.29.0 | Code generation | Zone manifest → typed Go controller stubs and fusa-annotated requirements |
| **Tooling** | v0.30.0 | Dynamic data | Runtime schema registry and typed payload codec for schema-less command payloads |
| **Remote access** | v0.31.0 | gRPC bridge | gRPC transport for cloud-connected zone controllers and remote diagnostics |
| **Remote access** | v0.32.0 | REST bridge | HTTP/SSE bridge for browser tooling and cloud integration |
| **Protocol bridges** | v0.33.0 | SOME/IP bridge | Bridge RCP commands to SOME/IP service methods |
| **Protocol bridges** | v0.34.0 | CAN bridge | Bridge RCP commands to CAN frames via go-CAN |
| **Protocol bridges** | v0.35.0 | DDS bridge | Bridge RCP Status to DDS topics and DDS samples to RCP commands via go-DDS |
| **Protocol bridges** | v0.36.0 | MQTT bridge | Bridge RCP Status to MQTT topics for cloud/telematics integration via go-mqtt |
| **Protocol bridges** | v0.37.0 | LIN bridge | Bridge RCP commands to LIN frames for low-bandwidth zone actuators via go-LIN |
| **Protocol bridges** | v0.38.0 | UDS bridge | Bridge RCP commands to ISO 14229 UDS service calls for zone controller diagnostics |
| **Protocol bridges** | v0.39.0 | DoIP bridge | Bridge zone controller diagnostics over ISO 13400 Diagnostics over IP |
| **Platform** | v0.40.0 | RTOS / bare-metal | Zone controller client for Zephyr, FreeRTOS, and NuttX RTOS targets |
| **Certification** | v0.41.0 | Formal verification | TLA+ specification and model-checked proofs of health and watchdog state machines |
| **Certification** | v0.42.0 | ISO 21434 | Cybersecurity assurance case, TARA evidence, SL-2 gap report |
| **Certification** | v0.43.0 | Certification | ASIL-D gap analysis, structural coverage report, audit pack |

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
- Full go-FuSa v0.30.0 trace and check compliance

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

### 24. Multi-HPC Federation (v0.24.0)

- Multiple active HPCs each owning disjoint zone subsets on the same zone bus
- `FederatedRegistry` coordinates zone ownership: each HPC registers a lease on the zones it owns; a lease server arbitrates conflicts
- Cross-HPC command forwarding: HPC-A can send a command to a zone owned by HPC-B via the federation layer; transparent to the caller
- Ownership transfer: zones can be migrated between HPCs at runtime (e.g. powertrain HPC hands off body zones during shutdown)

---
### Phase 8 — Tooling
---

### 25. Observability (v0.25.0)

- OpenTelemetry trace spans for every `Send` and `Subscribe` call
- Prometheus-compatible metrics: command latency histogram, error rate, zone health gauge, power state distribution, deadline miss counter
- `monitor` subcommand in `rcptool` for live zone status dashboard

### 26. Admin API (v0.26.0)

- HTTP admin interface (`admin/` package, mirrors go-DDS `admin/`)
- `GET /zones` — list all registered zones with health, power state, and last-seen timestamp
- `GET /zones/{zone}` — single-zone detail: health history, command rate, deadline miss count
- `POST /zones/{zone}/send` — send a command via JSON body; returns response JSON
- `GET /events` — SSE stream of all health, power, and deadline-miss events
- `GET /metrics` — Prometheus scrape endpoint
- Bearer auth enforced on all write endpoints (depends on v0.19.0 Authorization)

### 27. Record & Replay (v0.27.0)

- `record/` package — records all `Command`, `Response`, and `Status` streams to a structured binary log on disk
- Ring-buffer mode for always-on black-box recording with configurable retention window
- Replay: feed a recorded log back through any `Controller`/`Registry` implementation for regression testing against a new version
- `rcptool record` and `rcptool replay` subcommands
- Log format is append-only and checksummed — suitable as FuSa incident forensics evidence

### 28. Config (v0.28.0)

- YAML/JSON zone registry configuration (zone ID, transport, address, certificates)
- Hot-reload of zone addresses without restart via `fsnotify`

### 29. Code Generation (v0.29.0)

- Zone manifest schema (YAML/JSON): declares zone IDs, supported command types, payload schemas, and ASIL levels
- `rcptool gen <manifest.yaml>` generates typed Go controller stubs with `//fusa:req` annotations pre-populated
- Generated stubs implement the `Controller` interface; the generator emits matching `_test.go` skeletons and `.fusa-reqs.json` entries
- Eliminates hand-written boilerplate when adding a new zone type; keeps requirements, code, and tests in sync from declaration

### 30. Dynamic Data (v0.30.0)

- Runtime payload schema registry: named types (e.g. `"braking.BrakeCommand"`) registered with a Go struct and a codec at startup
- `DynamicPayload` carries a schema name alongside raw bytes; `Decode[T](p DynamicPayload) (T, error)` reconstructs the typed value without compile-time knowledge of all payload types
- Admin API and `rcptool monitor` display decoded payload fields when a matching schema is registered; fall back to hex for unregistered types
- Code generation (v0.29.0) emits `RegisterSchema` calls for each declared payload type, wiring the two features together
- Useful for cloud tools and dashboards that connect after deployment and need to interpret payloads without a recompile

---
### Phase 9 — Remote Access
---

### 31. gRPC Bridge (v0.31.0)

- gRPC transport (`bridge/grpc/`) for cloud-connected zone controllers and remote HPC diagnostic access
- `Subscribe` server-streaming RPC: cloud consumer receives `Status` updates in real time
- `Send` unary RPC: remote caller dispatches a `Command` and receives the `Response`
- Bearer auth interceptors; filter and transform hooks; YAML config via `LoadConfig`/`ApplyConfig`
- Enables remote diagnostic tools and cloud dashboards to interact with zone controllers without a local HPC connection

### 32. REST Bridge (v0.32.0)

- HTTP/SSE bridge (`bridge/rest/`) for browser-based tooling and cloud integration
- `POST /zones/{zone}/commands` — dispatch a `Command` as JSON; returns `Response` JSON
- `GET /zones/{zone}/status` — SSE stream of `Status` updates for a single zone
- `GET /zones` — SSE stream of all zone health and power-state change events
- Bearer auth on all write endpoints; CORS support for browser clients
- Complements the gRPC bridge: REST/SSE for interactive dashboards and scripts; gRPC for high-throughput cloud consumers

---
### Phase 10 — Automotive Protocol Bridges
---

### 33. SOME/IP Bridge (v0.33.0)

- Bridge `CmdSet`/`CmdGet` to SOME/IP service method calls via go-SOMEIP
- Bidirectional: SOME/IP events → RCP `Status` updates

### 34. CAN Bridge (v0.34.0)

- Bridge `CmdSet`/`CmdGet` to CAN frames via go-CAN
- Configurable CAN ID mapping per zone and command type

### 35. DDS Bridge (v0.35.0)

- Bridge RCP `Status` updates to DDS topics via go-DDS (sensor-fusion consumers receive zone telemetry as typed DDS samples)
- Bridge DDS samples → RCP `CmdSet`/`CmdGet` for ADAS pipeline → zone actuator control
- Bidirectional QoS mapping: DDS Reliability/Durability → RCP Priority

### 36. MQTT Bridge (v0.36.0)

- Bridge RCP `Status` to MQTT topics for cloud telemetry and fleet management via go-mqtt
- Bridge MQTT command messages → RCP `CmdSet` for remote zone actuation
- Configurable topic prefix per zone (e.g. `rcp/zone/front-left/status`)

### 37. LIN Bridge (v0.37.0)

- Bridge `CmdSet`/`CmdGet` to LIN frames via go-LIN for low-bandwidth zone actuators (seat motors, mirror adjustment, window regulators)
- Configurable LIN frame ID and field mapping per zone and command type
- LIN schedule table management: RCP commands inserted as unconditional or event-triggered frames

### 38. UDS Bridge (v0.38.0)

- Bridge RCP commands to ISO 14229 UDS service calls for zone controller diagnostics
- `CmdReset` → UDS ECUReset (0x11); `CmdGet` → ReadDataByIdentifier (0x22); `CmdSet` → WriteDataByIdentifier (0x2E)
- Configurable UDS addressing mode per zone (physical, functional, extended)
- UDS negative response codes surfaced as typed `ResponseStatus` values

### 39. DoIP Bridge (v0.39.0)

- ISO 13400 Diagnostics over IP transport for workshop and EOL diagnostic access to zone controllers
- `DoIPController` implements the `Controller` interface; routes `CmdGet`/`CmdReset` to UDS services over the DoIP wire protocol
- Logical address and routing activation management per zone
- Enables `rcptool` to act as a DoIP tester for factory-floor zone controller flashing and diagnostics

---
### Phase 11 — Platform
---

### 40. RTOS / Bare-Metal (v0.40.0)

- Zone controller client library targeting Zephyr, FreeRTOS, and NuttX RTOS
- Pure C API generated from the go-RCP interface definitions (CGo bridge or separate C implementation sharing the wire format)
- Implements the same UDP framing, E2E protection, and watchdog protocol as the Go library
- No heap allocation on the RTOS side: all buffers statically allocated at compile time
- Integration test: Zephyr zone controller on QEMU communicating with go-RCP HPC over loopback

---
### Phase 12 — Certification & Formal Methods
---

### 41. Formal Verification (v0.41.0)

- TLA+ specification of the zone health state machine (Healthy → Degraded → Faulted → Recovering)
  - Properties verified: no deadlock, no livelock; liveness (a zone that becomes healthy is eventually detected as Healthy); safety (a Faulted zone is detected within 2× the watchdog period)
- TLA+ specification of the watchdog protocol: HPC sends `CmdWatchdog` at interval T; zone resets if no kick arrives within deadline D
  - Properties verified: if kicks cease, zone reaches Faulted within D + network round-trip; a resumed kick stream returns zone to Healthy
- TLA+ specification of the anti-replay guard: sequence counter window W; frames outside the window are rejected
  - Properties verified: no valid in-window frame is ever rejected; a replayed frame is always rejected
- Model-checking results and counter-example traces published in `FORMAL_VERIFICATION.md` — ASIL-D evidence that the safety state machines are correct by construction
- `tla/` directory contains all `.tla` and `.cfg` files; reproducible via the TLC model checker

### 42. ISO 21434 / Cybersecurity (v0.42.0)

- Threat Analysis and Risk Assessment (TARA) covering command injection, replay attacks, rogue zone controller registration, OTA firmware tampering, and denial-of-service via command flooding
- Security requirements mapped to TARA findings; implemented controls (TLS, Authorization, E2E replay guard, rate limiting, mDNS authentication) traced as countermeasures
- IEC 62443 SL-2 gap report (`iec62443-gap-report.json`) — closes open items from `.fusa-iec62443.json`
- Penetration test evidence: structured attack scenarios against UDP, TLS, admin HTTP, gRPC, and REST endpoints
- `TARA.md` and `CYBERSECURITY.md` published alongside the safety case

### 43. Certification (v0.43.0)

- ASIL-D gap analysis report (decomposition paths from current ASIL-B)
- Structural coverage report: statement, branch, MC/DC
- Audit pack for customer qualification (requirements traceability matrix, FMEA, safety case, TARA cross-reference, formal verification summary)
