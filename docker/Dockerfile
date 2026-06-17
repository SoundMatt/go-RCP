# Multi-stage build for go-RCP binaries.

FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /bin/go-rcp        ./cmd/go-rcp
RUN CGO_ENABLED=0 go build -trimpath -o /bin/controller    ./examples/quickstart/controller
RUN CGO_ENABLED=0 go build -trimpath -o /bin/zone          ./examples/quickstart/zone

# ── go-rcp image (RELAY-conformant CLI) ───────────────────────────────────────
FROM alpine:3.20 AS go-rcp
RUN apk add --no-cache git ca-certificates
COPY --from=builder /bin/go-rcp /usr/local/bin/go-rcp
LABEL org.opencontainers.image.title="go-rcp" \
      org.opencontainers.image.source="https://github.com/SoundMatt/go-RCP" \
      org.opencontainers.image.licenses="MPL-2.0" \
      io.relay.tool="go-rcp" \
      io.relay.language="go" \
      io.relay.binary="go-rcp" \
      io.relay.spec-version="0.2"
ENTRYPOINT ["go-rcp"]

# ── controller image ───────────────────────────────────────────────────────────
FROM alpine:3.20 AS controller
RUN apk add --no-cache git ca-certificates
COPY --from=builder /bin/controller /usr/local/bin/controller
LABEL org.opencontainers.image.title="go-RCP controller" \
      org.opencontainers.image.source="https://github.com/SoundMatt/go-RCP" \
      org.opencontainers.image.licenses="MPL-2.0" \
      io.relay.tool="go-rcp" \
      io.relay.language="go" \
      io.relay.binary="go-rcp" \
      io.relay.spec-version="0.2"
ENTRYPOINT ["controller"]

# ── zone image ─────────────────────────────────────────────────────────────────
FROM alpine:3.20 AS zone
RUN apk add --no-cache git ca-certificates
COPY --from=builder /bin/zone /usr/local/bin/zone
LABEL org.opencontainers.image.title="go-RCP zone" \
      org.opencontainers.image.source="https://github.com/SoundMatt/go-RCP" \
      org.opencontainers.image.licenses="MPL-2.0" \
      io.relay.tool="go-rcp" \
      io.relay.language="go" \
      io.relay.binary="go-rcp" \
      io.relay.spec-version="0.2"
ENTRYPOINT ["zone"]
