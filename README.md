# OmniShelf

A local-first, self-hosted media tracker for TV shows and books. OmniShelf
ships as a **single Go binary** with the React frontend embedded via
`go:embed`, running in **one Docker container** on TrueNAS SCALE.

- `/` — embedded React SPA
- `/api/*` — JSON REST API (JWT in an HttpOnly cookie)
- `/images/*` — cached posters/covers served off the NAS
- Nightly TMDB sync (03:00) and a legacy TV Time CSV importer
- SQLite (WAL) for storage — no external database

## Building & running locally

```sh
# Frontend must be built first so go:embed picks up ui/dist.
cd ui && npm ci && npm run build && cd ..
CGO_ENABLED=0 go build -o omnishelf ./cmd/omnishelf

OMNISHELF_JWT_SECRET=dev-secret \
OMNISHELF_DATA_DIR=./data OMNISHELF_IMAGES_DIR=./images \
TMDB_API_KEY=... OMNISHELF_CONTACT_EMAIL=you@example.com \
./omnishelf
```

The app listens on `:8080` (`http://localhost:8080`).

---

## Deployment (TrueNAS SCALE)

OmniShelf runs as a single Docker container. Build the image (multi-stage
`Dockerfile` builds the SPA and a CGO-free binary):

```sh
docker build -t omnishelf:latest .
```

### Host datasets & volume mappings

Create two persistent datasets on your pool and map them into the container.
The database and cached images live **outside** the container so they survive
image upgrades:

| Host path (dataset)            | Container path | Purpose                       |
|--------------------------------|----------------|-------------------------------|
| `/mnt/media-tracker/data`      | `/data`        | SQLite DB + WAL files         |
| `/mnt/media-tracker/images`    | `/images`      | Cached posters / book covers  |

Both must be **writable** by the container — OmniShelf probes them on startup
and refuses to start (exit non-zero) if either is missing or read-only, so a
broken volume mount surfaces immediately in the TrueNAS app logs.

### TrueNAS SCALE — Custom App setup

Apps → **Discover Apps** → **Custom App**:

1. **Image:** `omnishelf:latest` (push to a registry the NAS can reach, or load
   the image onto the host).
2. **Port:** container `8080` → host `8080` (or any host port you prefer).
3. **Storage:** add two **Host Path** volumes using the table above
   (`/mnt/media-tracker/data` → `/data`, `/mnt/media-tracker/images` → `/images`).
4. **Environment variables:** see the next section.
5. Deploy. TrueNAS reads the container `HEALTHCHECK` (`GET /api/health`) and
   marks the app healthy once the DB ping and images volume both pass.

### Environment variables (spec §1.3)

| Var                       | Default                | Purpose                                                        |
|---------------------------|------------------------|----------------------------------------------------------------|
| `OMNISHELF_PORT`          | `8080`                 | HTTP listen port                                               |
| `OMNISHELF_DATA_DIR`      | `/data`                | SQLite location (map to `/mnt/media-tracker/data`)             |
| `OMNISHELF_IMAGES_DIR`    | `/images`              | Cached image root (map to `/mnt/media-tracker/images`)         |
| `OMNISHELF_JWT_SECRET`    | **(required)**         | HMAC signing key. App refuses to start if unset — set a long, stable random string; changing it invalidates every session. |
| `TMDB_API_KEY`            | **(required for TV)**  | TMDB v3 API key (never sent to the browser)                    |
| `OMNISHELF_CONTACT_EMAIL` | **(required for books)** | Injected into the OpenLibrary `User-Agent`                    |

### Health check

`GET /api/health` is **unauthenticated** and returns `200 {"status":"ok","db":"ok"}`
when the SQLite ping and the images volume are both healthy, or `503` with a
`detail` field otherwise. The container's `HEALTHCHECK` polls it:

```dockerfile
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/api/health || exit 1
```

### First-run: bootstrap an invite code

Registration is invite-only. There is no default account — generate a
single-use invite code with the built-in `invite` subcommand, then hand it to
your first user so they can register. Run it inside the container so it uses the
same `/data` database and env vars:

```sh
# From the TrueNAS shell (adjust the app/container name):
docker exec -it <omnishelf-container> omnishelf invite create
# prints e.g.  K7QP4MNR9TX2WJ3H
```

The user visits the app, registers with `{username, password, inviteCode}`, and
the code is consumed atomically (a second attempt with the same code gets a
409). Repeat `invite create` for each additional household member.

### Mobile barcode scanning needs HTTPS (Tailscale)

The book scanner uses the browser camera API, which browsers only expose in a
**Secure Context** (HTTPS or `localhost`). Over plain LAN HTTP
(`http://<nas-ip>:8080`) mobile browsers block the camera, and the Scan page
detects this and shows a banner pointing you at the HTTPS URL instead of a
broken camera view.

To scan on a phone, reach OmniShelf over **Tailscale**, which terminates HTTPS
for you:

```
https://omnishelf.<your-tailnet>.ts.net
```

1. Install Tailscale on the NAS (or run OmniShelf behind a Tailscale sidecar)
   and enable **HTTPS certificates** / MagicDNS in the Tailscale admin console.
2. Expose port 8080 via `tailscale serve` (or Tailscale Funnel) so the tailnet
   hostname proxies to the container.
3. Open the `https://…ts.net` URL on your phone — the Secure Context unlocks the
   camera and barcode scanning works. All API/image paths are relative, so the
   same build works on both the LAN-IP and Tailscale origins with no reconfig.

> The app itself serves plain HTTP and does not terminate TLS (out of scope for
> v1); Tailscale handles HTTPS. The session cookie is therefore not marked
> `Secure` so it works on the LAN origin too.
