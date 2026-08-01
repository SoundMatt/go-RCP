# Architecture

go-RCP's architecture follows the canonical cross-repo design at
[RELAY's `docs/RCP-ARCHITECTURE.md`](https://github.com/SoundMatt/RELAY/blob/main/docs/RCP-ARCHITECTURE.md),
shared with c-RCP, cpp-RCP, and rust-RCP.

## File-path mapping

| Lexicon term | This repo |
|---|---|
| wire / ACF layer | `acf/message.go` |
| framing / AVTP layer | `avtp/avtpdu.go` |
| response classification | *(missing — see below)* |
| conditional-request layer | `request/` package |
| Table 30 / evt[2:0] write semantics | `acf/evt.go` |
| endpoint-type modules | `gpio/`, `spi/`, `pwm/`, `adc/`, `i2c/`, `lin/`, `can/`, `uart/`, `iseled/`, `mdio/`, `wakeup/` |
| dispatch/routing | `udp.Router` (`udp/router.go`) |

## Conformance status against the canonical architecture

| Canonical choice | Status |
|---|---|
| Response classification (evt-first) | **not conformant** — no classifier exists at all; tracked |
| Table 30 centralization | conformant — `acf/evt.go` is the cross-repo reference implementation |
| Conditional-request module unification | **not conformant** — `request/` is a partial split (`chained.go` standalone, generic `kind.go`, separate `dispatcher.go`); target is one unified module per cpp-RCP/rust-RCP's shape |
| Per-function requirement tagging | **not conformant** — tags are collected at file/package level (top of `doc.go`), not per-function |
| `.fusa-reqs.json` schema (`tc18`/`tc18_master_id`/`status`) | **partial** — no citation or status fields exist yet; propagation in progress |
| Conditional-request req-id grouping | **not conformant** — uses one bucket (`REQ-REQ-*`); target is `REQ-CMP-*`/`REQ-TRIG-*`/`REQ-CHAIN-*`/`REQ-TIMED-*`/`REQ-CANCEL-*` |
