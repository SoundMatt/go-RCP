# go-RCP

A Go library implementing the Remote Control Protocol (RCP) for zonal control in automotive systems.

RCP connects a high-performance central computer to distributed Ethernet-based zone controllers, keeping application logic centralised while remote zones provide access to local I/O, sensors, CAN/LIN gateways, and actuators.

[![CI](https://github.com/SoundMatt/go-RCP/actions/workflows/ci.yml/badge.svg)](https://github.com/SoundMatt/go-RCP/actions/workflows/ci.yml)
[![DCO](https://github.com/SoundMatt/go-RCP/actions/workflows/dco.yml/badge.svg)](https://github.com/SoundMatt/go-RCP/actions/workflows/dco.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/SoundMatt/go-RCP.svg)](https://pkg.go.dev/github.com/SoundMatt/go-RCP)

## Packages

| Package | Description |
|---|---|
| `.` | Core interfaces: `Controller`, `Registry`, `Command`, `Response`, `Status`, `Zone` |
| `mock` | In-process mock controller and registry — zero dependencies, default for unit tests |

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
