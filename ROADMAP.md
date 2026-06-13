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

| Version | Milestones | Theme |
|---|---|---|
| v0.1.0 | Foundation | Core interfaces, mock backend, CI, go-FuSa, Docker quickstart ✅ |
| v0.2.0 | UDP transport | Pure-Go UDP command/response transport with discovery |
| v0.3.0 | Watchdog & heartbeat | CmdWatchdog scheduling, zone health monitoring, liveness API |
| v0.4.0 | E2E protection | Sequence counter, CRC-16, replay guard on command frames |
| v0.5.0 | Priority queuing | Per-zone priority queue honouring PriorityCritical/High/Normal |
| v0.6.0 | TLS transport | Mutual TLS channel for zone-controller communication |
| v0.7.0 | Zone proxy | Transparent zone proxy for multi-hop zonal topologies |
| v0.8.0 | Observability | OpenTelemetry traces and Prometheus metrics adapter |
| v0.9.0 | Config | YAML/JSON zone registry configuration |
| v0.10.0 | SOME/IP bridge | Bridge RCP commands to SOME/IP service methods |
| v0.11.0 | CAN bridge | Bridge RCP commands to CAN frames via go-CAN |
| v0.12.0 | Certification | ASIL-D gap analysis, structural coverage report, audit pack |

---

## Milestones

### 1. Foundation (v0.1.0) ✅
- Core `rcp.go` interfaces: `Controller`, `Registry`, `Command`, `Response`, `Status`
- `mock/` in-process backend
- `cmd/rcptool` CLI (discover, send, monitor)
- `examples/quickstart/` controller and zone
- Docker multi-stage build + compose quickstart
- CI: unit tests (cross-platform), benchmark smoke, fuzz (short), golangci-lint, go-FuSa, DCO
- Release workflow: safety artifact regeneration on tag
- Docker publish workflow: GHCR multi-arch images

### 2. UDP Transport (v0.2.0)
- Length-framed binary command/response protocol over UDP
- Static unicast and multicast zone discovery
- Integration tests with loopback interface

### 3. Watchdog & Heartbeat (v0.3.0)
- Periodic `CmdWatchdog` scheduling with configurable interval
- Zone health state machine: Healthy → Degraded → Faulted
- `Registry.WatchHealth()` channel for health state changes

### 4. E2E Protection (v0.4.0)
- 32-bit sequence counter per zone controller
- CRC-16 frame check on command and response payload
- Anti-replay guard: reject out-of-window sequence numbers

### 5. Priority Queuing (v0.5.0)
- Per-zone send queue with three priority levels
- PriorityCritical bypasses normal queue backpressure
- Backpressure metrics exposed via OpenTelemetry

### 6. TLS Transport (v0.6.0)
- Mutual TLS transport using standard `crypto/tls`
- Certificate pinning for zone controller identity
- Zero-dependency: no external TLS libraries

### 7. Zone Proxy (v0.7.0)
- Transparent proxy for cascaded zonal topologies
- Command routing table: zone → upstream proxy address
- Latency budget enforcement at proxy boundary

### 8. Observability (v0.8.0)
- OpenTelemetry trace spans for every Send/Subscribe call
- Prometheus-compatible metrics: command latency, error rate, zone health
- `monitor` web dashboard for live zone status

### 9. Config (v0.9.0)
- YAML/JSON zone registry configuration
- Hot-reload of zone addresses without restart

### 10. SOME/IP Bridge (v0.10.0)
- Bridge RCP commands to SOME/IP service methods via go-SOMEIP
- Bidirectional: SOME/IP events → RCP Status updates

### 11. CAN Bridge (v0.11.0)
- Bridge RCP `CmdSet`/`CmdGet` to CAN frames via go-CAN
- Configurable CAN ID mapping per zone

### 12. Certification (v0.12.0)
- ASIL-D gap analysis report
- Structural coverage report (statement, branch, MC/DC)
- Audit pack for customer qualification
