# ── Stage 1: build the React SPA ─────────────────────────────────────────────
FROM node:24-alpine AS ui-builder
WORKDIR /src/ui
COPY ui/package.json ui/package-lock.json ./
RUN npm ci
COPY ui/ ./
RUN npm run build

# ── Stage 2: build the Go binary (CGO-free, pure-Go sqlite driver) ───────────
FROM golang:1.26-alpine AS go-builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# The built SPA must be present before go build so //go:embed picks it up.
COPY --from=ui-builder /src/ui/dist ./ui/dist
RUN CGO_ENABLED=0 go build -o /omnishelf ./cmd/omnishelf

# ── Stage 3: runtime ──────────────────────────────────────────────────────────
FROM alpine:3.22

# wget ships with alpine's busybox; ca-certificates for outbound TMDB/OpenLibrary TLS.
RUN apk add --no-cache ca-certificates

COPY --from=go-builder /omnishelf /usr/local/bin/omnishelf

# Run as the unprivileged TrueNAS SCALE "apps" UID (568) rather than root.
# The mounted /data and /images host paths must be writable by this UID
# (chown -R 568:568 on the datasets) or startup fails its write probe.
RUN addgroup -g 568 -S omnishelf && adduser -u 568 -S -G omnishelf omnishelf
USER 568:568

EXPOSE 8080 443

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/api/health || wget -qO- --no-check-certificate https://127.0.0.1:443/api/health || exit 1

ENTRYPOINT ["omnishelf"]