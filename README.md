# OmniShelf

A local-first, self-hosted media tracker for TV shows, movies, books, and video
games. OmniShelf ships as a **single Go binary** with the React frontend
embedded via `go:embed`, running in **one Docker container** on TrueNAS SCALE.

- `/` — embedded React SPA
- `/api/*` — JSON REST API (JWT in an HttpOnly cookie)
- `/images/*` — cached posters/covers served off the NAS
- Nightly TMDB sync (03:00) and a legacy TV Time CSV importer
- Barcode scanning for books (ISBN → OpenLibrary) and games (UPC/EAN → ScanDex + IGDB)
- SQLite (WAL) for storage — no external database

## Security model

OmniShelf is designed for a **trusted household on a private network**
(LAN and/or Tailscale). It is *not* hardened for exposure to the open
internet — do not port-forward it. Highlights:

- **Invite-only registration** — no open signup, no default account. Codes are
  single-use, generated with `crypto/rand`, and consumed atomically.
- **Sessions** — 7-day HMAC-SHA256 JWTs in an `HttpOnly`, `SameSite=Lax`
  cookie; passwords hashed with bcrypt (cost 12).
- **Brute-force protection** — 10 failed login/invite attempts per source IP
  in 15 minutes returns `429 rate_limited`. Proxy headers are not trusted for
  the client IP.
- **Secret hygiene** — the app refuses to start if `OMNISHELF_JWT_SECRET` is
  unset, shorter than 32 characters, or left at the `.env.example`
  placeholder. Third-party API keys stay server-side and are never sent to
  the browser.
- **Container** — runs as unprivileged UID 568 (the TrueNAS SCALE `apps`
  user), not root.
- **HTTP hardening** — security headers incl. a same-origin Content-Security-
  Policy, `nosniff`, and clickjacking denial; request-body caps on the
  unauthenticated auth endpoints; read-header/idle timeouts on the server.
- **TLS** — the app serves plain HTTP and does not terminate TLS; Tailscale
  (or another reverse proxy) provides HTTPS. Because the LAN origin is plain
  HTTP, the session cookie is deliberately **not** marked `Secure`.

## Building & running locally

```sh
# Frontend must be built first so go:embed picks up ui/dist.
cd ui && npm ci && npm run build && cd ..
CGO_ENABLED=0 go build -o omnishelf ./cmd/omnishelf

OMNISHELF_JWT_SECRET="$(openssl rand -hex 32)" \
OMNISHELF_DATA_DIR=./data OMNISHELF_IMAGES_DIR=./images \
TMDB_API_KEY=... OMNISHELF_CONTACT_EMAIL=you@example.com \
./omnishelf
```

The app listens on `:8080` (`http://localhost:8080`). For repeat runs, copy
`.env.example` to `.env` and fill it in. Note the JWT secret must survive restarts or every session is
invalidated, so persist the generated value rather than regenerating per run.

---


### 1. Host datasets & permissions

Create two persistent datasets on your pool and map them into the container.
The database and cached images live **outside** the container so they survive
image upgrades:

| Host path (dataset)         | Container path | Purpose                      |
|-----------------------------|----------------|------------------------------|
| `/mnt/media-tracker/data`   | `/data`        | SQLite DB + WAL files        |
| `/mnt/media-tracker/images` | `/images`      | Cached posters / book covers |

The container runs as the unprivileged TrueNAS `apps` user (**UID/GID 568**),
so both datasets must be writable by that user. Either set the dataset ACL
owner to `apps` in the UI (Datasets → Permissions), or from the TrueNAS shell:

```sh
chown -R 568:568 /mnt/media-tracker/data /mnt/media-tracker/images
```

OmniShelf probes both directories on startup and refuses to start (exit
non-zero) if either is missing or read-only, so a broken volume mount or
wrong ownership surfaces immediately in the app logs.

### 2. Generate the secrets

```sh
openssl rand -hex 32   # → value for OMNISHELF_JWT_SECRET
```

Keep this value stable across restarts and upgrades — changing it invalidates
every session. Get a TMDB v3 API key from
[themoviedb.org](https://www.themoviedb.org/settings/api); ScanDex and IGDB
credentials are optional and only needed for the games module.

### 3. Create the Custom App

Apps → **Discover Apps** → **⋮ → Install via YAML / Custom App**:

1. **Image:** `omnishelf:latest` (or your registry tag).
2. **Port:** container `8080` → host `8080` (or any free host port).
3. **Storage:** add two **Host Path** volumes using the table above
   (`/mnt/media-tracker/data` → `/data`, `/mnt/media-tracker/images` → `/images`).
4. **Environment variables:** see the table below. TrueNAS stores app env
   vars in its config — treat the JWT secret and API keys like passwords and
   don't paste them into anything world-readable.
5. Deploy. TrueNAS reads the container `HEALTHCHECK` (`GET /api/health`) and
   marks the app healthy once the DB ping and images volume both pass.

### Environment variables

| Var                       | Default        | Purpose                                                          |
|---------------------------|----------------|------------------------------------------------------------------|
| `OMNISHELF_PORT`          | `8080`         | HTTP listen port                                                 |
| `OMNISHELF_DATA_DIR`      | `/data`        | SQLite location (map to `/mnt/media-tracker/data`)               |
| `OMNISHELF_IMAGES_DIR`    | `/images`      | Cached image root (map to `/mnt/media-tracker/images`)           |
| `OMNISHELF_JWT_SECRET`    | **(required)** | HMAC signing key, ≥ 32 chars. App refuses to start if unset, too short, or left at the example placeholder. |
| `TMDB_API_KEY`            | **(required for TV/movies)** | TMDB v3 API key (never sent to the browser)        |
| `OMNISHELF_CONTACT_EMAIL` | **(required for books)** | Injected into the OpenLibrary `User-Agent`             |
| `SCANDEX_USER_ID`         | *(optional)*   | ScanDex game barcode lookups; unset → games module reports "not configured" |
| `SCANDEX_ACCESS_TOKEN`    | *(optional)*   | ScanDex access token                                             |
| `IGDB_CLIENT_ID`          | *(optional)*   | IGDB (Twitch developer) client for game covers/summaries         |
| `IGDB_CLIENT_SECRET`      | *(optional)*   | IGDB client secret                                               |

### Health check

`GET /api/health` is **unauthenticated** and returns
`200 {"status":"ok","db":"ok"}` when the SQLite ping and the images volume are
both healthy, or `503` otherwise (failure specifics go to the container logs,
not the response). The container's `HEALTHCHECK` polls it:

```dockerfile
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/api/health || exit 1
```

### First-run: bootstrap an invite code

Registration is invite-only. There is no default account — generate a
single-use invite code with the built-in `invite` subcommand, then hand it to
your first user so they can register. Run it inside the container so it uses
the same `/data` database and env vars:

```sh
# From the TrueNAS shell (adjust the app/container name):
docker exec -it <omnishelf-container> omnishelf invite create
# prints e.g.  K7QP4MNR9TX2WJ3H
```

The user visits the app, registers with `{username, password, inviteCode}`,
and the code is consumed atomically (a second attempt with the same code gets
a 409). Repeat `invite create` for each additional household member.

### Mobile barcode scanning needs HTTPS (Tailscale)

The barcode scanner uses the browser camera API, which browsers only expose
in a **Secure Context** (HTTPS or `localhost`). Over plain LAN HTTP
(`http://<nas-ip>:8080`) mobile browsers block the camera, and the Scan page
detects this and shows a banner pointing you at the HTTPS URL instead of a
broken camera view.

To scan on a phone, reach OmniShelf over **Tailscale**, which terminates
HTTPS for you:

```
https://omnishelf.<your-tailnet>.ts.net
```

1. Install Tailscale on the NAS (or run OmniShelf behind a Tailscale sidecar)
   and enable **HTTPS certificates** / MagicDNS in the Tailscale admin console.
2. Expose port 8080 via `tailscale serve` so the tailnet hostname proxies to
   the container. Prefer `serve` (tailnet-only) over Funnel — Funnel publishes
   the app to the public internet, which this app is not hardened for.
3. Open the `https://…ts.net` URL on your phone — the Secure Context unlocks
   the camera and barcode scanning works. All API/image paths are relative, so
   the same build works on both the LAN-IP and Tailscale origins with no
   reconfig.

### Upgrading

1. Build and push the new image tag.
2. Update the Custom App's image reference and redeploy — `/data` and
   `/images` live on the host datasets, so nothing is lost. SQLite schema
   migrations run automatically at startup.
3. Keep `OMNISHELF_JWT_SECRET` unchanged so existing sessions survive.
