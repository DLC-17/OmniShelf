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
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/api/health || exit 1
ENTRYPOINT ["omnishelf"]
