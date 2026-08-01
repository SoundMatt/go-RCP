# go-RCP

A Go implementation of the OPEN Alliance TC18 Remote Control Protocol
Specification v0.5.1_RC, connecting a central computer to distributed RC
Servers whose endpoints are addressed by stream ID and byte-bus-ID rather
than a zonal addressing scheme.

RCP connects a high-performance central computer to distributed RC Servers
over IEEE 1722 AVTPDU/ACF framing, keeping application logic centralised
while each server exposes its declared endpoints (GPIO, SPI, I2C, UART,
ADC, PWM, LIN, CAN, ISELED, MDIO, Wakeup, ...) through a shared
register-map configuration model. See ROADMAP.md for the full protocol
replacement program and its milestone-by-milestone history — as of v1.0.0
this is the version where go-RCP first claims TC18 conformance; there is
no compatibility shim for the bespoke Zone/Command/Response/Status
protocol this module implemented before v1.0.0.

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
go-rcp discover                    # read the demo RC Server's register map
go-rcp send <byte_bus_id>          # write a demo payload; --format json streams an NDJSON sink (§11.2)
go-rcp monitor                     # poll every demo endpoint on an interval
go-rcp convert --protocol RCP      # RELAY interop driver (§11.2)
```

`discover`/`send`/`monitor` run against an in-process demo RC Server built
into the binary — wire your own `*udp.Controller`/`*udp.Server` pair (or any
type satisfying `rcp.Controller`) for a real deployment. `cmd/rcptool`, an
older, non-conformant CLI kept for backward compatibility through v0.71.0,
was retired at v1.0.0 once the bespoke API it targeted was removed.

## Packages

The repo ships 59 packages beyond the root module. The table below groups
them by concern; see each package's doc comment (`go doc ./<pkg>`) for
details.

| Package | Description |
|---|---|
| `.` | Root types: `Adapt`/`Controller` (the RELAY `relay.Caller` bridge), the RELAY-mandatory error sentinels, and `Loan` (zero-copy payload buffer) |
| `mock` | In-process test doubles: `Endpoint`/`Client`/`ClientRegistry`/`Fixture` for the TC18 server/endpoint model |
| `loan` | Zero-copy request-body loaning via a `sync.Pool`, wrapping `*udp.Controller` |

### TC18 protocol implementation (ROADMAP.md Part II)

go-RCP's core protocol and every satellite package below implement the
OPEN Alliance TC18 Remote Control Protocol, phase by phase; see ROADMAP.md
Part II for the full program and each satellite package's disposition.

| Package | Description |
|---|---|
| `avtp` | IEEE 1722 AVTPDU/ACF wire format for TC18 RCP (Milestone 44, v0.57.0) — untimed/timestamped AVTPDU headers, the short/long RCP message encodings, and stream_id/byte_bus_id/transaction_num addressing |
| `server` | RC Server configuration lifecycle, register map, and Discovery for TC18 RCP (Milestones 45/46, v0.58.0/v0.59.0) — the 3-state lifecycle and its transition guards, the generic/functional per-endpoint register split, EP0 and the root-client/restricted-stream access model, the HW pin-mapping table, request-stream/response-queue configuration, the grant-independent register-0 discovery read, the timeout-releasable configuration-claim, and client-side conformant-server recognition/topology persistence |
| `gpio` | GPIO endpoint type for TC18 RCP (Milestone 47, v0.60.0) — up to 32 pins, the eight write-semantics (replace/OR/AND/AND-NOT/XOR/saturating add/saturating subtract/reconfigure), and per-pin change-trigger signals |
| `spi` | SPI endpoint type for TC18 RCP (Milestone 47, v0.60.0) — controller-only, up to six sub-opcode-selected chip-select channels, raw full-duplex transfer payloads, per-channel clock/mode/timing configuration, and transfer-complete/chip-select-edge triggers |
| `i2c` | I2C endpoint type for TC18 RCP (Milestone 48, v0.61.0) — controller-only, single-bus raw transfer payloads including the address byte(s) themselves, configurable bus speed and inter-transaction trailing time, and transaction-complete triggers |
| `uart` | UART endpoint type for TC18 RCP (Milestone 48, v0.61.0) — independent TX/RX request handling sharing one functional config, FIFO-drain-or-timeout read completion with fragmented delivery of partial data, and TX-complete/RX-data-available triggers |
| `adc` | ADC endpoint type for TC18 RCP (Milestone 48, v0.61.0) — single-channel up to 16-bit resolution, the three-layer sample/average/combine model, and the two continuous-sampling mechanisms (triggered off another endpoint, or self-triggered off its own measurement-done event) |
| `pwm` | PWM endpoint type for TC18 RCP (Milestone 48, v0.61.0) — output and input roles sharing a symmetric period/active-duration waveform shape, with input response-only and failing explicitly on signal loss |

### RCP control-plane concerns (spec §13.7.2)

| Package | Description |
|---|---|
| `admin` | HTTP admin interface for runtime server/endpoint inspection |
| `authz` | Client-side stream/endpoint access-control policy |
| `certgap` | ASIL-D gap analysis helpers |
| `config` | YAML/JSON server/stream/register-map configuration loading |
| `deadline` | Liveness monitoring of endpoint response cadence |
| `dyndata` | Runtime schema registry and typed payload codec |
| `e2e` | CRC32 safe-point mechanism and watchdog-driven safe-state entry |
| `faultinject` | Structured fault injection for validating fault handling |
| `federation` | Coordination of multiple HPCs, each owning a disjoint endpoint subset |
| `firmware` | OTA firmware delivery riding on top of a raw-byte endpoint or the UDS bridge |
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
| `mdns` | Optional rendezvous helper: advertises/discovers a server's `avtp.StreamID` via mDNS/DNS-SD, pointing a caller at candidate UDP addresses for `Discover` |
| `shmem` | Zero-copy intra-host request/response transport |
| `tsn` | IEEE 802.1Qbv-aware (time-sensitive networking) UDP transport |
| `udp` | Pure-Go AVTPDU/ACF-over-UDP/IP transport (`Controller`, `Server`, `Router`, `Registry`) |

### Safety, reliability, and observability

| Package | Description |
|---|---|
| `observe` | OpenTelemetry tracing + metrics wrapper for a Controller |
| `powerstate` | Wake-handshake retransmission pacing (`wakeup.Endpoint`) |
| `prioqueue` | Client-side priority queue mirroring `request.Kind`'s server-side ordering |
| `ratelimit` | Per-endpoint token-bucket admission control |
| `record` | Always-on black-box recording of request/response traffic |
| `redundancy` | Hot-standby Controller pair for ASIL-B fault tolerance |
| `safety` | Latency and timing evidence for ASIL-B compliance |
| `sim` | Timing-realistic endpoint simulator (ADC sample interval, PWM cycle timing) |
| `watchdog` | ASIL-B watchdog and heartbeat orchestration |
| `zonegroup` | Atomic multi-endpoint-group request broadcast |

### Tooling

| Package | Description |
|---|---|
| `capi` | C-compatible handle-based API for go-RCP controllers, addressed by `avtp.StreamID`/`avtp.ByteBusID` |
| `codegen` | Generates `request.Handler` stubs and go-FuSa requirement scaffolding from a server/endpoint manifest |
| `proxy` | Transparent RC Server proxy for multi-hop topologies |
| `tc18gap` | Machine-readable registry of the TC18 normative clauses this module does **not** implement, keyed to `.fusa-reqs.json` |

## Install

```bash
go get github.com/SoundMatt/go-RCP
```

## Quick start

```go
import (
    "context"
    "fmt"

    "github.com/SoundMatt/go-RCP/acf"
    "github.com/SoundMatt/go-RCP/avtp"
    "github.com/SoundMatt/go-RCP/mock"
)

// Fixture bundles an in-process server.Server, its Router, and a root
// Client — see mock/fixture.go. A real deployment dials *udp.Controller
// against a *udp.Server instead (see udp/doc.go).
fx, _ := mock.NewFixture(avtp.NewStreamID([6]byte{0x02, 0, 0, 0, 0, 1}, 1), false)
defer fx.Close()

const gpioAddr avtp.ByteBusID = 1
_ = fx.Router.Register(gpioAddr, mock.NewEndpoint(gpioAddr, func(_ avtp.StreamID, req acf.Message) (acf.Message, error) {
    return acf.Message{
        Kind: req.Kind, ByteBusID: req.ByteBusID, TransactionNum: req.TransactionNum,
        Control: acf.FlagResponse | acf.FlagWrite,
    }, nil
}))

resp, err := fx.Root.Write(context.Background(), gpioAddr, []byte(`{"actuator":"indicator","state":"on"}`))
if err != nil {
    panic(err)
}
fmt.Println(resp.Control.Has(acf.FlagError)) // false
```

## Docker quickstart

```bash
docker compose -f docker/docker-compose.yml up --build
```

Starts one RC Server container (`zone`) and one client container
(`controller`) writing to it once a second, communicating over a real
UDP/IP bridge network.

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
