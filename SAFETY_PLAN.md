# Software Safety Plan
## go-RCP — ISO 26262 ASIL-B / IEC 61508 SIL 2

**Document ID:** SSP-001
**Version:** 1.0
**Date:** 2026-06-13
**Status:** Released
**Author:** Matt Jones (matt@jellybaby.com)
**Standards:** ISO 26262:2018 Part 8 §7, IEC 61508-3:2010 §5

---

## 1. Purpose and scope

This Software Safety Plan (SSP) defines the lifecycle, activities, methods, and
responsibilities for the development and verification of go-RCP
(`github.com/SoundMatt/go-RCP`) in accordance with:

- ISO 26262:2018 — Road vehicles — Functional Safety (Parts 3, 4, 6, 8)
- IEC 61508:2010 — Functional Safety of E/E/PE Safety-related Systems (Part 3)

go-RCP is developed as a **Safety Element Out Of Context (SEOOC)** targeting
ASIL-B (ISO 26262) / SIL 2 (IEC 61508). Refer to `.fusa-hara.json` for the hazard
analysis and risk assessment that derives these levels.

**Out of scope:** System-level HARA, hardware fault model (FMEDA), airworthiness
(DO-178C), AUTOSAR integration. These are the integrating system's responsibility.

---

## 2. Applicable documents

| ID | Document | Location |
|---|---|---|
| HARA | Hazard Analysis & Risk Assessment | `.fusa-hara.json` |
| REQS | Software Requirements | `.fusa-reqs.json` |
| IEC62443 | IEC 62443 Security Target | `.fusa-iec62443.json` |
| IR | Incident Response Plan | `INCIDENT-RESPONSE.md` |
| SEC | Security Policy | `SECURITY.md` |

---

## 3. Safety lifecycle

| Phase | Activities | Artefacts |
|---|---|---|
| Requirements | Capture in `.fusa-reqs.json` with ASIL level | `.fusa-reqs.json` |
| Design | Interfaces defined in `rcp.go`; documented in `README.md` | `rcp.go`, `README.md` |
| Implementation | Go code with `fusa:req` annotations linking tests to requirements | `*.go` |
| Verification | Unit tests, fuzz tests, race detector, static analysis | `*_test.go`, CI |
| Safety analysis | HARA, FMEA, safety case | `.fusa-hara.json`, release artifacts |
| Release | `gofusa release` regenerates SBOM, provenance, gap reports | CI release workflow |

---

## 4. Development methods

| Method | Tool | Requirement |
|---|---|---|
| Static analysis | `go vet`, `golangci-lint`, `gofusa check` | All findings fixed before merge |
| Test coverage | `go test -race -count=1 ./...` | All requirements traced to tests |
| Fuzz testing | `go test -fuzz=...` | Seed corpus + 10 s in CI |
| Traceability | `fusa:req REQ-xxx` annotations | Checked by `gofusa trace` |
| DCO | `Signed-off-by` trailer | Enforced by DCO workflow |
| Code review | CODEOWNERS | All changes reviewed by @SoundMatt |

---

## 5. Independence

go-RCP is a SEOOC library. The integrating system is responsible for:
- System-level HARA determining final ASIL classification of the integrated system
- Hardware fault metrics (FMEDA)
- Integration and system testing against actual zone controller hardware
- Independence verification if required by the integrating project's safety plan

---

## 6. Maintenance

Safety artifacts are regenerated automatically on every tagged release via the
`release.yml` workflow. The `.fusa-reqs.json` and `.fusa-hara.json` files are
manually maintained and subject to CODEOWNERS review.
