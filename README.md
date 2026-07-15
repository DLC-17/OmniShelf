# OmniShelf

A local-first, self-hosted media tracker for TV shows, movies, books, video games, music albums, and trading cards. OmniShelf ships as a **single Go binary** with the React frontend embedded via `go:embed`, running in **one Docker container** on TrueNAS SCALE.

- `/` — embedded React SPA
- `/api/*` — JSON REST API (JWT in an HttpOnly cookie)
- `/images/*` — cached/uploaded posters, covers, and artwork served off the NAS
- Nightly TMDB sync (03:00) and a legacy TV Time CSV importer
- Barcode scanning for books (ISBN → OpenLibrary) and games (UPC/EAN → ScanDex + IGDB)
- Barcode scanning for music (UPC/EAN → Discogs + MusicBrainz)
- Photo scanning for trading cards (Image → Google Vision OCR → YGOPRODeck / Pokémon TCG API)
- SQLite (WAL) for storage — no external database

---

## Supported Media Types & Modules

OmniShelf acts as a unified shelf for multiple media categories, utilizing specific external APIs and services to cache metadata locally:

### 📺 TV Shows & 🎬 Movies
*   **Source:** [The Movie Database (TMDB)](https://www.themoviedb.org/) (requires `TMDB_API_KEY`).
*   **TV Shows:** Tracks watch state at the episode level. The **Up Next** watchlist surfaces the next unwatched aired episode for one-tap progress tracking.
*   **Nightly Sync:** In-process scheduler updates all watched/plan-to-watch shows at 03:00 AM for new episodes and air dates.
*   **CSV Importer:** Resolves legacy TV Time CSV exports (`followed_shows.csv` / `seen_episodes.csv`) via normalized Levenshtein similarity.
*   **Movies:** Plain watchlist tracking (`PLAN_TO` / `COMPLETED`) without episodes or seasons.

### 📚 Books
*   **Source:** [OpenLibrary](https://openlibrary.org/) (requires `OMNISHELF_CONTACT_EMAIL` for User-Agent compliance).
*   **Identification:** Scan book EAN-13 ISBN barcodes using the browser camera or a handheld scanner. Falls back to text ISBN lookup or search.
*   **Tracking:** Supports page number progress tracking and timestamped text journal notes.

### 🎮 Video Games
*   **Source:** [ScanDex](https://scandex.net/) (optional barcode lookup) and [IGDB](https://www.igdb.com/) (optional cover, summaries, genres/keywords via Twitch OAuth).
*   **Identification:** Scan game UPC/EAN barcodes to resolve title/platform via ScanDex, then enrich metadata and covers via IGDB. Falls back to IGDB text search.
*   **Tracking:** Supports status updates (`PLAYING`, `PLAN_TO`, `COMPLETED`, `STOPPED`) and ownership tracking (`Physical`, `GOG`).

### 🎵 Music Albums
*   **Source:** [Discogs](https://www.discogs.com/) (optional barcode lookup) and [MusicBrainz](https://musicbrainz.org/) (optional name search and release-group lookup).
*   **Identification:** Scan music UPC/EAN barcodes via Discogs, or search by album/artist name via MusicBrainz.
*   **Tracking:** Artist-grouped list showing listen states (`LISTENING`, `PLAN_TO`, `COMPLETED`, `STOPPED`) and ownership formats (`Vinyl`, `CD`).

### 🃏 Trading Cards
*   **Source:** Google Cloud Vision OCR (requires `GOOGLE_APPLICATION_CREDENTIALS` service account), [YGOPRODeck](https://ygoprodeck.com/) (Yu-Gi-Oh!), and the [Pokémon TCG API](https://pokemontcg.io/) (Pokémon).
*   **Identification:** Snap a photo of a card; Google Vision OCR extracts text, classifying the card as Yu-Gi-Oh! (matches printed set code) or Pokémon (matches name and collector number), then resolves it against the upstream catalog.
*   **Tracking:** Track cards you own (`OWNED` status) with support for card formats (`Holo`, `Reverse Holo`).

---

## Security Model

OmniShelf is designed for a **trusted household on a private network** (LAN and/or Tailscale). It is *not* hardened for exposure to the open internet — do not port-forward it.

*   **Invite-only Registration:** No open signup, no default account. Single-use invitation codes are generated via `crypto/rand` using the admin CLI.
*   **Sessions:** 7-day HMAC-SHA256 JWTs in an `HttpOnly`, `SameSite=Lax` cookie. Passwords hashed using `bcrypt` (cost 12).
*   **Brute-force Protection:** 10 failed login/invite attempts per source IP within a 15-minute window returns `429 rate_limited`. Proxy headers are not trusted for client IP mapping.
*   **Secret Hygiene:** The application refuses to start if `OMNISHELF_JWT_SECRET` is unset, shorter than 32 characters, or matches the `.env.example` placeholder. Third-party API keys remain server-side.
*   **Container Privilege:** Runs as unprivileged UID/GID 568 (the TrueNAS SCALE `apps` user), not root.
*   **HTTP Hardening:** Security headers include a same-origin Content-Security-Policy (CSP), `X-Content-Type-Options: nosniff`, and clickjacking protections.
*   **TLS:** Serves plain HTTP and does not terminate TLS; Tailscale or a reverse proxy handles HTTPS. LAN session cookies are deliberately **not** marked `Secure` to allow HTTP LAN access.

---

## Building & Running Locally

The UI must be compiled before building the Go server so that `go:embed` can bundle the production assets:

```sh
# 1. Build the React frontend
cd ui && npm ci && npm run build && cd ..

# 2. Build the CGO-free Go binary
CGO_ENABLED=0 go build -o omnishelf ./cmd/omnishelf

# 3. Create folders and run
mkdir -p ./data ./images
OMNISHELF_JWT_SECRET="$(openssl rand -hex 32)" \
OMNISHELF_DATA_DIR=./data OMNISHELF_IMAGES_DIR=./images \
TMDB_API_KEY=... OMNISHELF_CONTACT_EMAIL=you@example.com \
./omnishelf
```

The app listens on `:8080` (`http://localhost:8080`). For repeat runs, copy `.env.example` to `.env` and fill it in. Note the JWT secret must survive restarts or every session is invalidated, so persist the generated value rather than regenerating per run.

---

## Deployment (TrueNAS SCALE)

OmniShelf runs as a single Docker container. Build the image:

```sh
docker build -t omnishelf:latest .
```

### 1. Host Datasets & Permissions

Create two persistent datasets on your pool and map them into the container. The database and cached images must live **outside** the container to survive updates:

| Host Path (Dataset) | Container Path | Purpose |
| :--- | :--- | :--- |
| `/mnt/media-tracker/data` | `/data` | SQLite database + WAL files |
| `/mnt/media-tracker/images` | `/images` | Mapped folder for cached/uploaded artwork |

The container runs as UID/GID **568** (`apps`). Grant appropriate write ownership:

```sh
chown -R 568:568 /mnt/media-tracker/data /mnt/media-tracker/images
```

OmniShelf checks both paths on startup and fails immediately if they are missing or unwritable.

### 2. Environment Variables & Secrets

Generate a JWT secret (keep this stable to preserve sessions):

```sh
openssl rand -hex 32
```

| Env Variable | Default | Purpose |
| :--- | :--- | :--- |
| `OMNISHELF_PORT` | `8080` | HTTP listen port |
| `OMNISHELF_DATA_DIR` | `/data` | SQLite directory path |
| `OMNISHELF_IMAGES_DIR` | `/images` | Cached image path |
| `OMNISHELF_JWT_SECRET` | *(required)* | HMAC signing key (≥ 32 chars) |
| `TMDB_API_KEY` | *(required for TV/movies)* | TMDB v3 API Key |
| `OMNISHELF_CONTACT_EMAIL` | *(required for books/music)* | Injected into OpenLibrary and MusicBrainz User-Agent |
| `SCANDEX_USER_ID` | *(optional)* | ScanDex game barcode lookup user ID |
| `SCANDEX_ACCESS_TOKEN` | *(optional)* | ScanDex game barcode lookup access token |
| `IGDB_CLIENT_ID` | *(optional)* | IGDB client ID (Twitch Developer portal) |
| `IGDB_CLIENT_SECRET` | *(optional)* | IGDB client secret (Twitch Developer portal) |
| `OMNISHELF_DISCOGS_TOKEN` | *(optional)* | Discogs token for music barcode lookups |
| `GOOGLE_APPLICATION_CREDENTIALS` | *(optional)* | Path to Google Cloud Vision JSON credentials |
| `POKEMONTCG_API_KEY` | *(optional)* | Pokémon TCG API key (increases rate limit) |

### 3. Install the App

Discover Apps → **Install via YAML / Custom App**:
1. **Image:** `omnishelf:latest`
2. **Port:** Container `8080` → Host `8080` (or any free port).
3. **Storage:** Map the datasets above as **Host Path** volumes (`/data` and `/images`).
4. **Environment:** Fill in the environment variables from the table above.

### Health Check

`GET /api/health` returns `200 {"status":"ok","db":"ok"}` when the database and `/images` dir are healthy, and `503` on failure. The container configures this automatically:

```dockerfile
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/api/health || exit 1
```

### First-run: Bootstrap an Invite Code

Run this inside the container to mint a single-use registration code:

```sh
docker exec -it <omnishelf-container> omnishelf invite create
# prints e.g. K7QP4MNR9TX2WJ3H
```

The user visits the site, enters the code, and registers their credentials.

### Mobile Barcode/Photo Scanning Needs HTTPS (Tailscale)

Modern browsers only expose the camera API (`getUserMedia`) in a **Secure Context** (HTTPS or `localhost`). Over plain HTTP LAN (`http://<nas-ip>:8080`), camera access is blocked.

To scan cards or barcodes on a phone, access OmniShelf over **Tailscale** (which handles HTTPS automatically):

```
https://omnishelf.<your-tailnet>.ts.net
```

1. Install Tailscale on your NAS and enable HTTPS certificates / MagicDNS in the Tailscale console.
2. Expose port 8080 via `tailscale serve` so the Tailnet hostname proxies to the container. (Do NOT use `funnel`, which exposes it to the open internet).
3. Open the `https://...ts.net` URL on your phone to unlock camera permissions.
