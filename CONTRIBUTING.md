# Contributing to go-RCP

Thank you for your interest in contributing.

## Developer Certificate of Origin (DCO)

All contributions must be signed off under the
[Developer Certificate of Origin v1.1](https://developercertificate.org).

Add a `Signed-off-by` trailer to every commit:

```
git commit -s -m "feat: add awesome thing"
```

This produces:

```
feat: add awesome thing

Signed-off-by: Your Name <your@email.com>
```

If you forget to sign off, amend the commit:

```
git commit --amend -s
```

A GitHub Actions check (`DCO`) verifies every commit in a PR. PRs without
sign-offs will not be merged.

## Copyright

By contributing you agree that your contributions are licensed under the
[Mozilla Public License v2.0](LICENSE) and that copyright in go-RCP remains
with Matt Jones.

## Coding style

- `gofmt` — run `gofmt -w ./...` before pushing.
- `go vet` — must pass with zero warnings.
- `golangci-lint run` — must pass (config in `.golangci.yml`).
- Tests — new code must be accompanied by tests with `fusa:req` annotations.
- Run `go test -race -count=1 ./...` locally before opening a PR.

## Requirements and traceability

Every public behaviour must be captured as a requirement in `.fusa-reqs.json`
and traced to at least one test via a `// fusa:req REQ-xxx` annotation above
the test function. Untested requirements block CI.

## Pull requests

1. Fork the repo, create a branch from `main`.
2. Make your changes with signed-off commits.
3. `go test -race -count=1 ./...` must pass.
4. Open a PR targeting `main`.
5. All CI checks (test, lint, gofusa, DCO) must pass before merge.

## Project structure

| Directory | Contents |
|---|---|
| `.` | Core interfaces (`rcp.go`) |
| `mock/` | In-process mock implementation |
| `cmd/rcptool/` | CLI tool |
| `examples/quickstart/` | Controller and zone simulator examples |
| `docker/` | Dockerfile and docker-compose quickstart |
| `.github/workflows/` | CI, DCO, release, docker-publish |
| `.fusa-reqs.json` | Traced software requirements |
| `.fusa-hara.json` | Hazard analysis and risk assessment |
