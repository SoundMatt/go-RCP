# Multi-stage build for go-RCP binaries.
#
# SPEC_VERSION and TOOL_VERSION are supplied by CI (.github/workflows/docker-publish.yml
# and .github/workflows/release.yml) so the io.relay.spec-version OCI label (spec §13.5)
# and the go-rcp binary's own `version`/`capabilities` output (§12.1/§12.2) can never
# drift from the RELAY spec version this module targets and the git tag being built. The
# defaults below are only a fallback for an ad-hoc local `docker build` with no
# --build-arg; keep SPEC_VERSION in sync with the github.com/SoundMatt/RELAY
# requirement in go.mod.
ARG SPEC_VERSION=1.11
ARG TOOL_VERSION=dev

FROM golang:1.25-alpine AS builder
ARG TOOL_VERSION
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-X main.toolVersion=${TOOL_VERSION}" -o /bin/go-rcp ./cmd/go-rcp
RUN CGO_ENABLED=0 go build -trimpath -o /bin/controller    ./examples/quickstart/controller
RUN CGO_ENABLED=0 go build -trimpath -o /bin/zone          ./examples/quickstart/zone

# ── go-rcp image (RELAY-conformant CLI) ───────────────────────────────────────
FROM alpine:3.20 AS go-rcp
ARG SPEC_VERSION
RUN apk add --no-cache git ca-certificates
COPY --from=builder /bin/go-rcp /usr/local/bin/go-rcp
LABEL org.opencontainers.image.title="go-rcp" \
      org.opencontainers.image.source="https://github.com/SoundMatt/go-RCP" \
      org.opencontainers.image.licenses="MPL-2.0" \
      io.relay.tool="go-rcp" \
      io.relay.language="go" \
      io.relay.binary="go-rcp" \
      io.relay.spec-version="${SPEC_VERSION}"
ENTRYPOINT ["go-rcp"]

# ── controller image ───────────────────────────────────────────────────────────
FROM alpine:3.20 AS controller
ARG SPEC_VERSION
RUN apk add --no-cache git ca-certificates
COPY --from=builder /bin/controller /usr/local/bin/controller
LABEL org.opencontainers.image.title="go-RCP controller" \
      org.opencontainers.image.source="https://github.com/SoundMatt/go-RCP" \
      org.opencontainers.image.licenses="MPL-2.0" \
      io.relay.tool="go-rcp" \
      io.relay.language="go" \
      io.relay.binary="go-rcp" \
      io.relay.spec-version="${SPEC_VERSION}"
ENTRYPOINT ["controller"]

# ── zone image ─────────────────────────────────────────────────────────────────
FROM alpine:3.20 AS zone
ARG SPEC_VERSION
RUN apk add --no-cache git ca-certificates
COPY --from=builder /bin/zone /usr/local/bin/zone
LABEL org.opencontainers.image.title="go-RCP zone" \
      org.opencontainers.image.source="https://github.com/SoundMatt/go-RCP" \
      org.opencontainers.image.licenses="MPL-2.0" \
      io.relay.tool="go-rcp" \
      io.relay.language="go" \
      io.relay.binary="go-rcp" \
      io.relay.spec-version="${SPEC_VERSION}"
ENTRYPOINT ["zone"]
