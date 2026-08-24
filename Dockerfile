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
# su-exec is needed to drop privileges from root to the omnishelf user in the entrypoint.
RUN apk add --no-cache ca-certificates su-exec

COPY --from=go-builder /omnishelf /usr/local/bin/omnishelf

COPY docker-entrypoint.sh /usr/local/bin/

# Create the unprivileged TrueNAS SCALE "apps" UID (568).
# We do NOT use the USER directive here because the entrypoint script needs
# to run as root to fix volume permissions before dropping privileges.
RUN addgroup -g 568 -S omnishelf && adduser -u 568 -S -G omnishelf omnishelf

EXPOSE 8080 443

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD if [ -n "$TLS_CERT_FILE" ]; then \
        wget -qO- --no-check-certificate https://127.0.0.1:${OMNISHELF_PORT:-443}/api/health || exit 1; \
      else \
        wget -qO- http://127.0.0.1:${OMNISHELF_PORT:-8080}/api/health || exit 1; \
      fi

ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["omnishelf"]