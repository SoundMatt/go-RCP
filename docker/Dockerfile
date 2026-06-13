# Multi-stage build for go-RCP quickstart binaries.

FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /bin/rcptool      ./cmd/rcptool
RUN CGO_ENABLED=0 go build -trimpath -o /bin/controller   ./examples/quickstart/controller
RUN CGO_ENABLED=0 go build -trimpath -o /bin/zone         ./examples/quickstart/zone

# ── rcptool image ──────────────────────────────────────────────────────────────
FROM alpine:3.21 AS rcptool
RUN apk add --no-cache ca-certificates
COPY --from=builder /bin/rcptool /usr/local/bin/rcptool
LABEL org.opencontainers.image.title="go-RCP rcptool" \
      org.opencontainers.image.source="https://github.com/SoundMatt/go-RCP" \
      org.opencontainers.image.licenses="MPL-2.0"
ENTRYPOINT ["rcptool"]

# ── controller image ───────────────────────────────────────────────────────────
FROM alpine:3.21 AS controller
RUN apk add --no-cache ca-certificates
COPY --from=builder /bin/controller /usr/local/bin/controller
LABEL org.opencontainers.image.title="go-RCP controller" \
      org.opencontainers.image.source="https://github.com/SoundMatt/go-RCP" \
      org.opencontainers.image.licenses="MPL-2.0"
ENTRYPOINT ["controller"]

# ── zone image ─────────────────────────────────────────────────────────────────
FROM alpine:3.21 AS zone
RUN apk add --no-cache ca-certificates
COPY --from=builder /bin/zone /usr/local/bin/zone
LABEL org.opencontainers.image.title="go-RCP zone" \
      org.opencontainers.image.source="https://github.com/SoundMatt/go-RCP" \
      org.opencontainers.image.licenses="MPL-2.0"
ENTRYPOINT ["zone"]
