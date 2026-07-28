# go-RCP

A Go library implementing the Remote Control Protocol (RCP) for zonal control in automotive systems.

RCP connects a high-performance central computer to distributed Ethernet-based zone controllers, keeping application logic centralised while remote zones provide access to local I/O, sensors, CAN/LIN gateways, and actuators.

[![CI](https://github.com/SoundMatt/go-RCP/actions/workflows/ci.yml/badge.svg)](https://github.com/SoundMatt/go-RCP/actions/workflows/ci.yml)
[![DCO](https://github.com/SoundMatt/go-RCP/actions/workflows/dco.yml/badge.svg)](https://github.com/SoundMatt/go-RCP/actions/workflows/dco.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/SoundMatt/go-RCP.svg)](https://pkg.go.dev/github.com/SoundMatt/go-RCP)

## CLI

`cmd/go-rcp` is the RELAY-conformant CLI (spec §11.1/§11.2) — this is what CI's
`relay conform --strict` gate exercises:

```bash
go install github.com/SoundMatt/go-RCP/cmd/go-rcp@latest

go-rcp version --format json      # tool + spec version (§12.1)
go-rcp capabilities                # capabilities document (§12.2)
go-rcp status --format json        # self-assessed health
go-rcp discover                    # list registered zones
go-rcp send <zone>                 # send a command; --format json streams an NDJSON sink (§11.2)
go-rcp monitor                     # stream Status from all zones
go-rcp convert --protocol RCP      # RELAY interop driver (§11.2)
```

`cmd/rcptool` is an older, non-conformant CLI kept for backward compatibility;
prefer `go-rcp` for anything spec-related.

## Packages

The repo ships ~46 packages beyond the root module. The table below groups
them by concern; see each package's doc comment (`go doc ./<pkg>`) for
details.

| Package | Description |
|---|---|
| `.` | Core interfaces: `Controller`, `Registry`, `Command`, `Response`, `Status`, `Zone` |
| `mock` | In-process mock controller and registry — zero dependencies, default for unit tests |
| `loan` | `LoaningController` wrapper — zero-copy payload loaning via a `sync.Pool` |

### TC18 protocol replacement program (ROADMAP.md Part II)

The packages above implement go-RCP's original bespoke Zone/Command
protocol. The following implement the OPEN Alliance TC18 Remote Control
Protocol replacement instead, phase by phase; see ROADMAP.md Part II for the
full program and each satellite package's disposition.

| Package | Description |
|---|---|
| `avtp` | IEEE 1722 AVTPDU/ACF wire format for TC18 RCP (Milestone 44, v0.57.0) — untimed/timestamped AVTPDU headers, the short/long RCP message encodings, and stream_id/byte_bus_id/transaction_num addressing |

### RCP control-plane concerns (spec §13.7.2)

| Package | Description |
|---|---|
| `admin` | HTTP admin interface for runtime registry inspection |
| `authz` | Command-level access control for the RCP stack |
| `certgap` | ASIL-D gap analysis helpers |
| `config` | YAML/JSON zone registry configuration loading |
| `deadline` | Liveness monitoring of zone controller Status streams |
| `dyndata` | Runtime schema registry and typed payload codec |
| `e2e` | End-to-end (E2E) protection for command payloads |
| `faultinject` | Structured fault injection for validating fault handling |
| `federation` | Coordination of multiple HPCs, each owning a disjoint zone subset |
| `firmware` | OTA firmware delivery for zone controllers |
| `formal` | Lightweight formal-verification helpers |
| `iso21434` | ISO/SAE 21434 cybersecurity engineering artifacts |

### Protocol bridges (spec §13.7.2 `*br` naming + others)

| Package | Description |
|---|---|
| `canbr` | CAN bus bridge |
| `ddsbr` | DDS (Data Distribution Service) bridge |
| `doipbr` | DoIP (Diagnostics over IP, ISO 13400) bridge |
| `grpcbridge` | gRPC transport bridge |
| `linbr` | LIN (Local Interconnect Network) bus simulation/bridge |
| `mqttbr` | Pure-Go in-process MQTT broker bridge |
| `restbridge` | HTTP/JSON + SSE bridge |
| `someip` | SOME/IP bridge |
| `udsbr` | UDS (Unified Diagnostic Services, ISO 14229) bridge |

### Transports and discovery

| Package | Description |
|---|---|
| `mdns` | Zero-configuration zone-controller discovery via mDNS/DNS-SD |
| `shmem` | Zero-copy intra-host command transport |
| `tlstransport` | Mutual-TLS TCP transport |
| `tsn` | IEEE 802.1Qbv-aware (time-sensitive networking) UDP transport |
| `udp` | Pure-Go UDP transport |
| `wire` | Shared binary frame format used by the UDP and TLS transports |

### Safety, reliability, and observability

| Package | Description |
|---|---|
| `observe` | OpenTelemetry tracing + metrics wrapper for a Controller |
| `powerstate` | Zone controller power state transitions |
| `prioqueue` | Per-zone priority queue that serialises dispatch |
| `ratelimit` | Per-zone token-bucket admission control |
| `record` | Always-on black-box recording of command/response/status traffic |
| `redundancy` | Hot-standby Controller pair for ASIL-B fault tolerance |
| `safety` | Latency and timing evidence for ASIL-B compliance |
| `sim` | Timing-realistic zone controller simulator |
| `watchdog` | ASIL-B watchdog and heartbeat mechanism |
| `zonegroup` | Atomic multi-zone command broadcast |

### Tooling

| Package | Description |
|---|---|
| `capi` | C-compatible handle-based API for go-RCP controllers |
| `codegen` | Generates typed Go controller stubs and go-FuSa requirement scaffolding |
| `proxy` | Transparent zone proxy for multi-hop zonal topologies |

## Install

```bash
go get github.com/SoundMatt/go-RCP
```

## Quick start

```go
import (
    rcp "github.com/SoundMatt/go-RCP"
    "github.com/SoundMatt/go-RCP/mock"
)

reg := mock.NewRegistry()
defer reg.Close()

ctrl, _ := reg.Lookup(rcp.ZoneFrontLeft)

cmd := &rcp.Command{
    ID:       1,
    Zone:     rcp.ZoneFrontLeft,
    Type:     rcp.CmdSet,
    Priority: rcp.PriorityNormal,
    Payload:  []byte(`{"actuator":"indicator","state":"on"}`),
}

resp, err := ctrl.Send(context.Background(), cmd)
if err != nil {
    log.Fatal(err)
}
fmt.Println(resp.Status) // OK
```

## Zones

| Constant | Value | Description |
|---|---|---|
| `ZoneFrontLeft` | 1 | Front-left zone controller |
| `ZoneFrontRight` | 2 | Front-right zone controller |
| `ZoneRearLeft` | 3 | Rear-left zone controller |
| `ZoneRearRight` | 4 | Rear-right zone controller |
| `ZoneCentral` | 5 | Central zone controller |

## Command types

| Constant | Value | Description |
|---|---|---|
| `CmdNoop` | 0 | No-op / keepalive |
| `CmdSet` | 1 | Set an output or actuator state |
| `CmdGet` | 2 | Query current state |
| `CmdReset` | 3 | Reset zone controller |
| `CmdWatchdog` | 4 | Watchdog kick |

## Docker quickstart

```bash
docker compose -f docker/docker-compose.yml up --build
```

Starts a controller and two zone controller containers communicating over a bridge network.

## Safety

go-RCP targets deployment in automotive safety-critical environments.

- Safety standard: ISO 26262 ASIL-B / IEC 61508 SIL-2
- Security standard: IEC 62443 SL-2
- go-FuSa static analysis runs in CI on every PR
- All requirements are traced to tests in `.fusa-reqs.json`
- HARA, FMEA, safety case, and SBOM are regenerated on every release

See [SAFETY_PLAN.md](SAFETY_PLAN.md), [SECURITY.md](SECURITY.md), and [INCIDENT-RESPONSE.md](INCIDENT-RESPONSE.md).

## License

[Mozilla Public License v2.0](LICENSE). Copyright © Matt Jones.
