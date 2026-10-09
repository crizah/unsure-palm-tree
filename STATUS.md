# 🍛 Food Map — Project Status

> Free & subsidised meals near you, backed by government and NGO listings. No login, no community edits, no tracking.
> **Delhi + Bangalore** to start · Go + SQLite · Leaflet · phone-first.

*Last updated: 2026-10-09*

---

## 📍 Where we are

| Stage | Status |
|---|---|
| 1. Scaffold (Go API, static web app, `/` and `/locate`) | ✅ Done |
| 2. Data (schema, seed script, one-time geocoding) | ✅ Done, coordinates need review |
| 3. API + store (nearby search, filters) | ✅ Done |
| 4. `/locate` UI (map, filters, responsive sheet) | ✅ Done |
| 5. Address search + geocode endpoint | ⏳ Not started |
| 6. Hardening, tests, mobile polish | 🟡 Partly (headers, rate limit, gzip done; tests not) |
| 6b. Split into `api/` + `web/` with separate envs | ✅ Done |
| 7. Deployment (VPS, Caddy, systemd) + README | ⏳ Not started |

---

## 🧱 Architecture

Two independent apps, each with its own env. The browser loads the static site, which calls the API cross-origin.

```
crizah/
├── api/                       # Go server (JSON only) → deploy to a VPS
│   ├── cmd/server/main.go     # server, routes, middleware wiring
│   ├── cmd/seed/main.go       # builds data/places.db from data/places.json
│   ├── internal/config/       # .env + env loading, origin validation
│   ├── internal/store/        # read-only SQLite queries (R*Tree + haversine)
│   ├── internal/api/          # handlers, validation, CORS, rate limit, gzip
│   ├── data/                  # places.json, geocode_cache.json, places.db
│   ├── .env / .env.example    # PORT, FRONTEND_URL
│   └── go.mod
└── web/                       # static site, zero dependencies → deploy to Vercel
    ├── src/                   # index.html ("/"), locate.html ("/locate"), css, js, vendor/leaflet
    ├── build.mjs              # copies src → dist, injects API_URL + CSP
    ├── dev.mjs                # local server for dist (clean URLs like Vercel)
    ├── vercel.json            # build command, clean URLs, security headers
    └── .env / .env.example    # API_URL, WEB_PORT
```

Node is only used to build/serve the static files; there are no npm dependencies.

---

## ✅ What's done

### Data
- 74 places extracted from the old `index.html` (Delhi 52, Bangalore 22).
- Three types kept from the source data: **Government**, **NGO**, **Religious**.
- SQLite schema with indexes on `city`, `type`, `(city, type)` and an **R\*Tree** (`places_rt`) for the nearby search.
- Seed tool geocodes once via Nominatim (1 req/s, cached), so the server never geocodes our own data.

### API (GET only, JSON)
| Endpoint | What it does |
|---|---|
| `GET /api/places?lat&lng&radius&city&type&q&limit` | Nearby places sorted by distance, or filtered by city/type/text |
| `GET /api/cities` | Cities with counts, drives the city chips |

- Strict validation: lat/lng ranges, radius 0.1–50 km, limit 1–100, allow-listed city and type, search text ≤ 64 chars.
- Coordinates are rounded to ~100 m before querying; never logged or stored.

### Frontend (`/locate`)
- Full-screen Leaflet map, greyscale basemap, square markers (black = Government, white = NGO, grey = Religious).
- **Near me** button uses the browser's geolocation, only on click.
- City chips, type chips, debounced text search, results list with distance, timings, cost and a Directions link.
- **Desktop:** sidebar + map. **Phones (≤720px):** map fills the screen with a draggable bottom sheet (peek / half / full).
- All text is inserted with `textContent` (no `innerHTML`), so data can't inject markup.

### Security & performance
- Read-only DB: opened with `mode=ro&immutable=1`, no write endpoints, no cookies, no accounts.
- Strict CSP (only the tile server is allowed as a third party), `nosniff`, `no-referrer`, `Permissions-Policy`, HSTS.
- Per-IP token-bucket rate limit on `/api/*`, server timeouts, panic recovery, gzip.
- Raw `.html` files under `/static` are blocked; pages are only reachable via their routes.

### Verified
- All routes return the expected status; invalid input returns 400.
- Security headers present; SQL-metacharacter search is harmless.
- Screenshots reviewed at phone (390px) and desktop (1280px) widths.

---

## ⚠️ Known issues & decisions

### 1. Basemap tiles (needs a decision before launch)
CARTO's free tiles now need an API key (they render an "API KEY REQUIRED" watermark), so the plan's first choice no longer works. Currently using **OSM public tiles + a CSS greyscale filter**.
- Fine for development and light traffic only; OSM's policy forbids heavy production use.
- Before launch: **self-host PMTiles** (a Delhi + Bangalore extract is tens of MB) or use a keyed provider.
- It's one constant (`TILE_URL` in `web/src/js/locate.js`) plus the tile origin in `web/build.mjs` (CSP).

### 2. Coordinate quality
| Result | Count |
|---|---|
| Address-level | 15 |
| Area-level (centre of the neighbourhood) | 50 |
| No coordinates | 9 |

- No-coordinate rows: 5 "citywide / multiple points" entries, plus Malk Ganj, Paschim Puri and two generic "JJ Cluster" rows. They show in the list, not on the map.
- Some area-level pins look wrong (e.g. **Rani Garden** lands near Saket; **Jai Vihar** is doubtful).
- Places in the same area share one pin (Dwarka Sec 1 & 3, Mangolpuri G & N, three in Rajajinagar).
- Fix: correct by hand in `api/data/geocode_cache.json`, then `cd api && go run ./cmd/seed -offline`.

### 3. Leftovers
- Old `index.html` is still in the repo (superseded). Delete when happy.
- The seed data came from your original HTML; the real government DB is still to come.
- Nothing is committed yet.

---

## 🚧 What's left (in order)

1. **Address search.** `GET /api/geocode?q=` proxying Nominatim with a cache and a 1 req/s limit; search own places first, fall back to Nominatim; then wire the search box to recentre the map.
2. **Fix coordinates.** Review and correct the area-level pins, ideally with real coordinates from the government DB when it arrives.
3. **Tests.** Store (nearby/bbox/filter queries), API (validation, limits), rate limiter.
4. **Basemap.** Self-host PMTiles (or pick a keyed provider) and drop the OSM public server.
5. **Real data ingest.** `cmd/ingest` to load the government/NGO DB into the same schema (drop-in replacement for `places.json`); keep `source`, `source_url`, `verified_at` per row.
6. **Mobile polish.** Test on real devices (sheet drag, geolocation prompt, safe-area insets), marker clustering if density grows.
7. **Deploy.** Small VPS (t3.micro is plenty): systemd unit or Dockerfile, Caddy for automatic HTTPS, `-trust-proxy` flag behind the proxy, optional Cloudflare in front.
8. **README.** Run, seed, deploy instructions.
9. **Home page.** `/` is intentionally empty for now.

---

## 🛠️ How to run

**API** (terminal 1)
```bash
cd api
cp .env.example .env                 # PORT=8080, FRONTEND_URL=http://localhost:3000
go run ./cmd/seed -offline           # only if data/places.db is missing / data edited
go build -o bin/server ./cmd/server && ./bin/server
#   -env <path>    .env file (default ./.env; real env vars win)
#   -trust-proxy   trust X-Forwarded-For (only behind a proxy you control)
```

**Web** (terminal 2)
```bash
cd web
cp .env.example .env                 # API_URL=http://localhost:8080, WEB_PORT=3000
npm run dev                          # builds dist/ and serves it
```

Open <http://localhost:3000/locate>. Re-run `npm run dev` after editing `web/src` (it rebuilds on start).

### Configuration

| App | Variable | Purpose |
|---|---|---|
| api | `PORT` | Port the server listens on (required) |
| api | `FRONTEND_URL` | Origin(s) allowed by CORS, comma-separated, e.g. `https://foodmap.vercel.app,http://localhost:3000` |
| web | `API_URL` | Public origin of the API; baked in at build time and added to the CSP (required) |
| web | `WEB_PORT` | Local dev server port only |

URLs must be bare origins (`https://example.com`, no path); both apps refuse to start/build otherwise. `.env` files are git-ignored.

### Deploy
- **Web → Vercel:** set the project's *Root Directory* to `web`, add env var `API_URL=https://<your-api-origin>`. `vercel.json` handles the rest.
- **API → VPS:** run the binary behind Caddy (HTTPS), set `PORT` and `FRONTEND_URL=https://<your-vercel-domain>`, use `-trust-proxy`, and disable Caddy access logging of query strings (they contain coordinates).

---

## 🔒 Design principles (don't break these)

- **No login, no signup, no writes.** The DB is immutable at runtime.
- **Location stays private.** Coordinates are rounded, never logged, never stored.
- **Fast.** No framework, small payloads, static frontend, tiny Go API.
- **Phone first.** 44 px tap targets, 16 px inputs (no iOS zoom), bottom sheet.
- **Black, white, grey.** Square corners, no colour accents.
