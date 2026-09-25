# Siedler Österreich — Agent Guide

Multiplayer browser game where players explore real Austrian cadastre data, claim parcels, and convert land to nature reserves. Settlers IV pixel-art aesthetic.

## Quick Start

```bash
go build -o siedler ./cmd/srv/   # build
sudo systemctl restart srv        # deploy (runs ./siedler on :8000)
journalctl -u srv -f              # logs
go generate ./db/...              # after editing db/queries/*.sql
```

Live at `https://siedler-oesterreich.exe.xyz:8000/`. SQLite DB at `./db.sqlite3`.

## Architecture

```
cmd/srv/main.go          → entrypoint, flag parsing
srv/server.go            → all HTTP handlers, game logic, SSE, cadastre proxy
srv/static/game.js       → entire frontend (~2400 lines, single file)
srv/static/index.html    → all screens (welcome, picker, lobby, loading, game)
srv/static/style.css     → Settlers IV pixel-art theme
db/migrations/NNN-*.sql  → schema migrations (auto-applied on startup)
db/queries/game.sql      → sqlc queries → generates db/dbgen/
```

There is NO framework — vanilla JS canvas rendering, vanilla Go net/http, SQLite via sqlc.

## Cadastre API

All cadastre data comes from `https://cadastre-process-api.exe.xyz/api/v1`. Full docs at `/api/v1/docs/llm.txt`.

Proxied through the Go server at `/api/cadastre/` with 1-hour SQLite cache. The frontend calls `GET /api/cadastre/...` which the server forwards to the external API.

Key endpoints used:
- `/spatial/bbox?layers=parcels` — point parcels in a bounding box (initial load + pan)
- `/export/geojson?kg=XXXXX&layers=parcels` — full polygon geometry per KG
- `/export/geojson?kg=XXXXX&layers=building_footprints` — real building shapes
- `/export/geojson?kg=XXXXX&layers=landuse` — landuse polygons (forest, water, roads)
- `/search/municipalities?contains_lon=...&contains_lat=...` — reverse geocode
- `/search/ez?kg=...&ez=...` — EZ (Einlagezahl) detail with all parcels
- `/lookup?q=...&type=gemeinde` — municipality search for picker
- `/search/address_osm?q=...` — in-game address search

Parcel properties include: `parcel_id`, `kg_code`, `gnr`, `ez`, `area_sqm`, `landuse_summary`, `building_count`, `total_building_area_sqm`, `lon`, `lat`.

## Game State (frontend)

All state lives in the global `G` object in game.js:

```js
G.player        // {id, name, coins, xp, ...}
G.session        // {id, municipality_name, center_lon, center_lat, invite_code, ...}
G.parcels        // GeoJSON features from /spatial/bbox (point data)
G.parcelPolys    // GeoJSON features from /export/geojson (polygon data, per KG)
G.buildingFootprints  // real building polygons
G.landusePolys   // forest/water/road polygons
G.ezIndex        // "kg_code-EZnnn" → [features] — groups parcels by Einlagezahl
G.claimed        // [{parcel_id, player_id, kg_code, ez, converted_to, ...}] from DB
G.treasures      // unclaimed treasures on map
G.challenges     // player's active quests
G.cam            // {lon, lat, zoom} — map camera
G.sel            // currently selected parcel feature
G.ezHighlight    // {kg, ez} for gold pulse on related parcels
G.kgsLoaded      // Set of KG codes already fetched
```

## Screen Flow

`welcome` → `pick` (municipality picker) → `loading` → `game`

The lobby screen exists but is bypassed — `startSinglePlayer()` creates a session and goes straight to loading.

## Data Loading Sequence

1. Create session via `POST /api/session/create`
2. Load point parcels via `/spatial/bbox` (for municipality detection + fallback)
3. Load polygon geometry for the **viewport** via `/api/viewport` (fast path — see below)
4. Build EZ index from loaded polygon data
5. Load claimed parcels, treasures, challenges, players, biodiversity, chat from our API
6. On pan/zoom: `loadMoreParcels()` → `fetchKGPolygons()` → `loadViewportGeometry()` → `buildEZIndex()` incrementally

### Viewport fast path (`/api/viewport`)

The old approach loaded each visible KG's **entire** `export/geojson` (multi-MB:
~0.85MB parcels + ~2MB footprints + ~7MB landuse per KG). Replaced by the
cadastre API's R-tree viewport endpoints, which return polygon geometry for **just
the current bbox** in ~100ms:

- Upstream: `GET /spatial/parcels?west=&south=&east=&north=` and `GET /spatial/footprints?...`
  — each returns `{parcels|footprints:[{...,geometry}], ready, truncated}` straight
  from a cached R\*Tree (no json.gz load). `ready=false` means a KG is still warming
  (retry shortly).
- Server proxy `GET /api/viewport` (`handleViewport` in server.go): fetches both
  layers in parallel, rounds coords, merges into `{parcels, footprints, ready,
  truncated}`, gzips (~40KB), caches 6h keyed by a ~150m-quantized bbox (only when
  `ready`). Cache HITs serve in ~2ms.
- Frontend `loadViewportGeometry(bbox)` (game.js): dedups by `parcel_id`/`footprint_id`
  and by quantized tile (`G.polyIds`, `G.fpIds`, `G.vpTiles`), merges into
  `G.parcelPolys` / `G.buildingFootprints` with the same `{properties, geometry}`
  shape the renderer + EZ index expect. Footprint props now include real shape data
  (`obb_length_m`, `obb_width_m`, `orientation_deg`, `ns_code`, etc.).
- **Tiling + retries (important).** Upstream warms geometry per KG lazily and caps
  rows per request, surfacing this as `ready:false` / `truncated:true`. Ignoring
  either makes the map silently stop filling in (big empty green areas). So:
  `fetchKGPolygons()` splits the padded viewport into ≤0.02° tiles via `tileBox()`
  (nearest-camera first, ≤12 tiles) and runs them through `runPool(..., 4)`;
  `loadTileResilient()` retries `ready:false` tiles up to 4× with backoff (only
  while still on screen) and subdivides `truncated` tiles into quarters (depth ≤2).
  `loadViewportGeometry()` returns `{added, ready, truncated}` and un-marks its
  `G.vpTiles` entry when not ready/truncated so a re-fetch is allowed. Server-side
  `buildViewportWarm` retries `ready:false` twice (2s apart) so all clients share
  one warm-up via singleflight. `#map-loading` shows while tiles are in flight
  (`vpBusy()`).
- **Never gate polygon loading on zoom/span.** `viewBounds()` is in *device*
  pixels, so span thresholds trip much earlier than expected on wide/retina
  screens. Only the capped 800-row `/spatial/bbox` point fallback is span-gated.
- Landuse backdrop (CAD-1): `/api/viewport` carries no landuse polygons; for
  tiles containing a non-enhanced KG, `loadViewportLanduse(b)` fetches the
  same bbox from `GET /api/viewport-landuse` (upstream `/spatial/landuse`,
  `tolerance_m=1`, ~90 KB gz per tile, 6 h cache) via the shared
  `loadBboxLayer()` helper (tile + feature dedup, `ready:false` retries).
  Enhanced KGs skip it (lidar dominant-type + OSM cover the backdrop). The
  old whole-KG `export/geojson?layers=landuse` stream (~7 MB) is gone.
- Per-parcel landuse (CAD-2): viewport parcel rows carry `dominant_ns` and
  `landuse_areas {code: m²}` measured from polygons. `extractLuCode()` prefers
  `dominant_ns` over symbol-count weighting; `getLanduseName()` lists the
  measured shares. Point-fallback parcels still use the weighted mode.

## Map Rendering

### Multi-part geometry (MultiPolygon) — read this before touching a renderer

Upstream returns **MultiPolygon** for any parcel with detached parts. This is
common (alpine Gemeindegut / Almen split by a ridge or river) and those are the
*biggest* parcels: around Nauders 32 of 48 km² on screen was MultiPolygon,
including one 16.5 km² parcel. Code doing `geometry.coordinates[0]` or
`type !== 'Polygon' → return` silently renders nothing — which looks exactly
like "the viewport loader is broken".

Always go through the helpers in game.js instead of indexing coordinates:
`geomAllRings(g)`, `biggestRing(g)`, `isAreaGeom(g)`, `pipGeom(lon,lat,g)`,
`pipRings(lon,lat,rings)`, `featureLonLat(f)`. (`geomOuterRings` is a
deprecated alias of `geomAllRings`.) Building footprints are always single
Polygons.

**Ring order / holes.** Upstream fixed its ring contract on 2026-08-06
(feedback #13): `coordinates[0]` of every part is now the exterior ring (CCW),
following rings are holes (CW), disjoint shells are MultiPolygon parts, and
`/export/geojson`, `/spatial/parcels` and `/parcels/geometry/batch` agree byte
for byte. Before that, ring[0] was often a tiny sliver (84108-3394/1 =
`[335, 95, 20, 8506967]` m²), so hit-testing on `coordinates[0]` picked the
sliver while the even-odd fill drew the parcel — parcels looked normal but were
**unclickable**. We still hit-test **all rings with even-odd**: identical to the
contract when it holds, it excludes holes correctly (ring[0]-only did not), and
it survives a stale cache. **Part order is not part of the contract** — never
index into parts, sort or scan.

**`matched:false` ≠ "no parcel exists."** Upstream also fixed a tile-clipping
bug that lost/invented parcel area (feedback #14; 84108-3391 was stored at 0.47
of its 1.52 km²). The fix is ingest-side, so KGs still carry the old assembly
until reprocessed. Treat `matched:false` as "no parcel in *their* geometry".
After any upstream geometry fix, purge our cache:
`DELETE FROM api_cache WHERE cache_key LIKE 'viewport:%' OR cache_key LIKE
'%/export/geojson%' OR cache_key LIKE '%/spatial/%' OR cache_key LIKE
'%geometry/batch%';` then `VACUUM;`

### Austrian border

`srv/static/austria.json` = simplified ADM0 outline (geoBoundaries gbOpen /
BEV, CC-BY-SA, Douglas-Peucker 0.0002° ≈ 20 m, 12.4k verts, ~75KB gzip),
loaded in the background into `G.atBorder` (array of lon/lat rings).
Cadastre data stops at the state line, so foreign land would otherwise be
indistinguishable from unloaded land. Used for:
`drawForeignShading()` (dim + hatch outside, even-odd),
`drawAustriaBorderLine()` (red-white-red, above content), the minimap outline,
`updateAbroadBadge()` (`#abroad-badge`), and `tilesInAustria()` which drops
`/api/viewport` tiles fully outside Austria. `insideAustria()` returns **true**
while the outline is still loading — never gate data loading on it strictly.

Pure canvas 2D — no map library. Coordinate system:
- `toScreen(lon, lat)` → pixel coords
- `toGeo(x, y)` → WGS84 coords
- `mapScale()` = `2^(zoom-14) * 25000`
- Camera: `G.cam.lon`, `G.cam.lat`, `G.cam.zoom` (13–20)

Render order in `render()`:
1. Grass texture (pre-generated pattern tile)
2. Landuse polygons (forest, water, roads, fields)
3. Parcel polygons (with terrain color from `landuse_summary`)
4. Point parcels (fallback for parcels without polygon data)
5. Forest sprites (trees on forest-type parcels)
6. Building footprints (isometric pixel-art buildings)
7. Treasures (animated chests)
8. EZ highlight (gold pulsing outline on same-EZ parcels)
9. Selected parcel highlight (gold dashed outline)
10. Scale bar

### Landuse codes (BEV Nutzungssymbole)

`NS_TABLE` in game.js is the **single source of truth**: code → `{abbr, name,
terrain, price}`. `LANDUSE_TERRAIN`, `LANDUSE_NAMES` and `ABBR_MAP` are derived
from it at load; `LANDUSE_POLY_COLORS` (polygon fills) is keyed by the same codes.

Upstream corrected its German NS labels in Aug 2026 (BEV Schnittstellen-
beschreibung "Katastralmappe SHP" V2.9, Tab. 8). **The codes never changed —
only the text**, so never match on label strings. Only 26 codes exist
(40,41,42,48,52–65,72,83,84,87,88,92,95,96); anything else reports
"Unbekannt - Code NN". Canonical table: `GET /api/v1/landuse/codes`.

Two corrections that broke us: **48** is `Äcker, Wiesen oder Weiden` (farmland,
Austria's most common code, 3.76M parcels) — we rendered and priced it as road
surface; **83** is `Gebäudenebenflächen` (a Baufläche) — we treated it as
Fels/Sumpf. Roads are now **95** (`So(Str)`), rail **92**, parking **42**.

NS entries are **symbol counts, not areas**. A 17.9 ha field can carry three
stray building/road glyphs. `nsWeight(code)` down-weights traffic (0.25) and
building (0.5) symbols when picking a parcel's dominant use in
`parseLanduseSummary()` / `extractLuCode()` (weighted mode, not first entry).
Upstream's `land_prices` applies the same idea server-side: prefer
`buy_total_blended_eur` + `class_source:"area"` over the single-class total.

### Treasure markers & field textures

- `drawTreasure()` (game.js): Settlers-era collectible presentation — dithered
  isometric ground ring in the rarity colour (`TREASURE_RARITY`: EN/VU/NT/LC,
  coins gold, xp cyan), contact shadow, bobbing sprite, bouncing pixel arrow,
  pixel-cross sparkles, name tag at zoom ≥ 16.5. Species sprites live in
  `drawSpeciesTreasure(ctx,x,y,t,s)`; loot in `drawLootSprite()` (chest, gem,
  seed, map). `treasureHitRadius()` ≥ 22px (26 on touch). Collect →
  `spawnCollectFX()` burst + floating reward; `drawCollectFX()`. A rAF loop
  renders at ~25fps only while `_treasuresOnScreen > 0` or FX play.
- **Canvas type scale `MAP_FONT`** (`label` 14px VT323, `small` 11px VT323,
  `pixel` 9px Press Start 2P) — use these for every in-map label, never ad-hoc sizes.
- Fields (NS 48 / `TERRAIN.farm`): `drawFieldPattern()` fills the parcel with a
  cached canvas pattern (`fieldKind(hash)`: harvest tracks+stubble, ploughed
  furrows, mown swaths) rotated along the parcel's longest edge; sprites
  (`drawCropSprite` sheaf/haystack, `drawMeadowSprite`) sit on a staggered
  lattice of the same kind (zoom ≥ 16).

## Database Schema

SQLite with sqlc. Key tables:

- `players` — id, name, rejoin_token, coins (start: 10000), xp, level
- `game_sessions` — id, invite_code, municipality_code/name, center_lon/lat
- `session_players` — many-to-many join
- `parcel_claims` — session_id, player_id, parcel_id, kg_code, gnr, **ez**, area_sqm, landuse, converted_to, purchase_price
- `treasures` — lon/lat, type (coins/xp/rare_seed/ancient_map), value, found_by
- `challenges` — quest system (explore/restore/treasure types)
- `chat_messages` — per-session chat
- `api_cache` — cadastre API response cache (1hr TTL)

### Adding a migration

1. Create `db/migrations/NNN-name.sql` (NNN = next number, 3 digits)
2. Include `INSERT OR IGNORE INTO migrations (migration_number, migration_name) VALUES (NNN, 'NNN-name');` at the end
3. Migrations run automatically on startup in numeric order

### Adding/editing queries

1. Edit `db/queries/game.sql` (sqlc annotation format: `-- name: QueryName :one/:many/:exec`)
2. Run `go generate ./db/...`
3. Use via `s.Q.QueryName(ctx, params)`

## API Endpoints

### Auth
- `POST /api/register` — create player with name; returns `rejoin_token` (the only time it's sent). On 409 (name taken) it also returns `suggested` — a server-verified free name the client auto-retries with.
- `GET /api/suggest-name` — a guaranteed-unused Adjective+Noun name (numbered suffix fallback). Used for the welcome-screen prefill and the 🎲 reroll; never generate names client-side (only ~900 combos vs. hundreds of players).
- All mutating player endpoints require header `X-Player-Token: <rejoin_token>` matching `player_id` (server: `authPlayer`; client: `api()` helper sends `G.playerToken` or the `rejoin` URL param). `Player.RejoinToken` has `json:"-"` (sqlc override in `db/sqlc.yaml`) so it never leaks via player lists/SSE.

### Session
- `POST /api/session/create` — new game session
- `POST /api/session/join` — join via invite_code
- `GET /api/session/{id}` — session info
- `GET /api/session/{id}/players|parcels|treasures|challenges|biodiversity|chat`
- `GET /api/session/{id}/events` — SSE stream

### Game Actions
- `POST /api/claim-parcel` — buy one parcel (sends: parcel_id, kg_code, gnr, ez, area_sqm, landuse, building_count, total_building_area)
- `POST /api/claim-ez` — bulk buy all unclaimed parcels in an EZ (20% discount, max 100 parcels)
- `POST /api/convert-parcel` — convert to biodiversity/forest (awards XP)
- `POST /api/sell-parcel` — sell at 60% of purchase price
- `POST /api/claim-treasure` — collect map treasure
- `POST /api/complete-challenge` — complete a quest

### Pricing

`calculatePrice(areaSqm, landuse, buildingCount, totalBuildingArea)` in server.go:
- Base price/m² by NS code: table `nsBasePrice` in server.go, mirrored as the
  `price` field of `NS_TABLE` in game.js (Gebäude 0.5, Garten 0.45,
  Gebäudenebenfläche 0.45, Betriebsfläche 0.4, Freizeit 0.35, Weingarten 0.35,
  Äcker/Wiesen/Weiden 0.3, Dauerkulturen 0.3, Parkplatz 0.25, Wald 0.2,
  Friedhof 0.2, Verbuschung 0.15, Bahn 0.15, Abbau 0.15, Alm 0.12, Straße 0.1,
  Forststraße 0.1, Krummholz 0.1, Verkehrsrand 0.1, Feuchtgebiet/Gewässerrand
  0.08, Gewässer 0.05, vegetationsarm 0.05, Fels/Gletscher 0.03; unknown 0.15)
- Density multiplier: built-up ratio >0.3 = 2×, 0.05–0.3 = 1–2×, no buildings = 0.5×
- Clamped to 10–5000 coins. Mirrored in JS `calcPrice()` for client display.

## Real-Time (SSE)

Server broadcasts events via `s.broadcast(sessionID, data)`. Frontend handles in `handleEvent(d)`:
- `parcel_claimed`, `parcel_converted`, `parcel_sold`, `parcel_harvested` (`forest:true` for timber)
- `ez_claimed` — bulk EZ purchase
- `player_joined`, `challenge_completed`
- `chat` — new message

## EZ (Einlagezahl) System

Austrian land register folio grouping parcels under one ownership entry. A farm might be EZ 42 containing 8 parcels (house lot, barn, three fields, forest, road access, garden).

- Frontend builds `G.ezIndex` keyed by `"kg_code-EZnnn"` after loading KG polygon data
- Selecting a parcel with an EZ shows an info panel: parcel count, total area, ownership stats
- Bulk claim available at 20% discount via `POST /api/claim-ez`
- Gold pulsing highlight on all EZ parcels when one is selected (`drawEZHighlight()`)
- Rebuild index incrementally when new KGs load: `buildEZIndex()`

## Style Guide

- Settlers IV / retro pixel-art aesthetic
- Fonts: `Press Start 2P` (headers, labels), `VT323` (body text, stats)
- Color palette in CSS `:root` vars: `--gold`, `--green`, `--panel`, `--bg`, etc.
- All UI text in German (Austrian context)
- Toast notifications via `toast(msg, 'ok'|'err')`
- Mobile: bottom-sheet sidebar, touch pan/pinch-zoom

## Common Patterns

### Adding a new game feature
1. Add DB migration if needed → `db/migrations/NNN-name.sql`
2. Add sqlc query → `db/queries/game.sql` → `go generate ./db/...`
3. Add HTTP handler → `srv/server.go` (register in `Serve()`, implement handler)
4. Add frontend logic → `srv/static/game.js` (data loading, rendering, UI)
5. Add HTML if needed → `srv/static/index.html` (popup sections, sidebar sections)
6. Build & restart: `go build -o siedler ./cmd/srv/ && sudo systemctl restart srv`

### Adding a new map layer
1. Fetch from cadastre API in `fetchKGPolygonsBlocking()` / `fetchKGPolygons()`
2. Store in `G.someNewLayer`
3. Add draw function `drawSomething(ctx)` — use `toScreen()` for coordinate projection
4. Call it in `render()` at the right z-order position

### Adding a new popup/panel
1. Add HTML in `index.html` inside `#screen-game`
2. Style in `style.css`
3. Show/hide in game.js event handlers

## Known Limitations

- Auth is bearer-token only (rejoin token in URL) — anyone with the rejoin link is the player
- Parcel polygons only load for KGs visible at zoom; panning loads more incrementally
- Canvas rendering (no WebGL) — performance drops with very dense urban areas
- Price calculation is duplicated in Go and JS — keep them in sync
- EZ index only contains parcels from loaded KGs (not full municipality EZ data from API)
- No transaction wrapping on bulk EZ claim (individual parcel inserts)

## Slow Zenodo / HTTP 202 (both upstreams) — `srv/upstream_pending.go`

Both upstreams lazily pull per-KG products from the Zenodo mirror and **never
hang on it**: an endpoint whose whole result depends on a cold file answers
**HTTP 202 + `Retry-After`** with a progress block (cadastre:
`{status:"pending", warming:{kgs[],zenodo{status},retry_after_s}}`; lidar
GPKG-backed `/kg/X/{buildings,segments,…}`: `{status:"fetching", fetch{pct,
eta_s}, retry_after_s}`); cadastre viewport/batch endpoints instead return
200 + `ready:false` + the same `warming` block. Repeating the identical
request converges. `?wait=<s>` (max 120) makes upstream block itself.

Our handling — one rule everywhere:
- Server: `parsePending()` normalises both shapes to
  `{status:"pending", retry_after_s, progress{state,pct,eta_s}, kgs[], zenodo}`;
  `upstreamGetWait()` polls a bounded budget honouring Retry-After;
  `withWait()` adds `?wait=`. `/export/geojson` asks `wait=30` (+1 poll) and
  serves a stale cached copy over a 202; `/api/kg` and every proxy
  (`cachedFetch`, lidar-slim) relay a **202 + Retry-After** with the normalised
  body and never cache it. `/api/viewport` waits ≤7s paced by upstream's
  `retry_after_s` and forwards `retry_after_s` + `warming` on `ready:false`.
- Client: `api()` retries any GET that gets a 202 after `retry_after_s`
  (≤45s / 8 tries, `pendingBudgetMs` opt), updating `#map-loading` with
  pct/ETA via `pendingNotice()`; past the budget it resolves the body with
  `pending:true` so callers (`fetchKGLayer`→`loadLanduseBackground`,
  `fetchEnhancedKG`, `loadViewportGeometry`) schedule their own later retry.
  `loadTileResilient` paces on `res.retryAfter` (≤8 tries).
- **Never treat 202 / `ready:false` / `pending:true` as "no data here"**, and
  never cache such a response.

## Environment

- **All upstream calls must use `upstreamGet` / `upstreamClient`** (shared pooled
  client: 24 idle conns/host, 32 max, HTTP/2, 60s timeout). Never
  `http.Get`/`http.DefaultClient` — it keeps only 2 idle conns per host (we fan
  out 4-12 viewport tiles × 2 layers at one host, so nearly every request paid a
  fresh TLS handshake) and has no timeout (a hung request pins a singleflight
  key and blocks all waiters forever).
- **Never set `Accept-Encoding` by hand** on upstream calls. Go's transport sets
  it and decompresses transparently (~-60% on the wire); setting it manually
  means you must gunzip yourself.
- **KG codes: compare with `unpadKG`.** Upstream `/lookup` returns `kg_code`
  without the leading zero (`3301`), while `/query?kg=` requires the padded form
  (`03301`). A literal comparison silently breaks every KG in states 1-9.
- Our gzip middleware runs at level 5 (not BestSpeed): -22% bytes on viewport
  payloads for ~1ms CPU.
- Prefer batch/viewport endpoints over per-ID loops. Add `?geometry=0` /
  `attrs_only` when only attributes are needed. Quantize coordinates in any
  query that fires on every pan, or the response cache never hits.
- Go 1.24+, SQLite via modernc.org/sqlite (pure Go). WAL + busy_timeout=5s +
  synchronous=NORMAL set via DSN pragmas (apply to all pooled conns);
  `SetMaxOpenConns(8)`. Hourly `cacheJanitor` prunes expired `api_cache` rows
  (the file once ballooned to 3.2GB of dead cache).
- All upstream proxy handlers (cadastre, viewport, lidar, lidar-slim,
  enhanced-kgs, kg-data) dedupe concurrent cache-miss fetches via
  singleflight (`s.sf`, helper `s.cachedFetch`): N users hitting the same
  cold key share one upstream request (`X-Cache: MISS-SHARED`). Fetch
  closures use `context.Background()` so one client disconnect doesn't fail
  the waiters. Error responses are no longer cached.
- Static assets: `?v=` query → cached immutable 1y. **Bump the `?v=` version in
  `index.html` whenever game.js/style.css change**, or clients keep old code.
- systemd service: `/etc/systemd/system/srv.service`
- Binary: `./siedler`, DB: `./db.sqlite3`
- Cadastre API: `https://cadastre-process-api.exe.xyz/api/v1` (docs: `/api/v1/docs/llm.txt`)
- Port 8000, proxied via exe.dev HTTPS

## Enhanced Mode (LiDAR)

For KGs processed by `https://srtm-lidar-at.exe.xyz:8000/api/v1` (srtm-lidar API):
- `GET /api/enhanced-kgs` — registry of processed KGs (15min cache); "Auf Glück" prefers these ~90%
- `GET /api/lidar/kg/{code}` — slim KG JSON (server strips vertex_heights, flag-filters top trees ≤60m / objects ≤120m; 6h cache). Per parcel it also emits `dom_terrain` = the dominant *natural* land cover for ground fill (impervious road/roof/parking/path skipped, falls back to next-largest via `area_summary`; empty if genuinely all-impervious). Giant trees are harvested from every parcel's `top_trees` (≥25m, rf_conf≥0.5, `parcel_top_tree:PID:i` QA flags applied, deduped on a ~15m grid, capped 120) — far more than the KG-level `top_10_trees`.
- `GET /api/lidar/...` — generic proxy (1h cache; overlay/elevation/dtm blocked — too slow)
- `GET /api/similar?parcel_id=&lon=&lat=&area=&bcount=&barea=&lu=&radius=5000&limit=40` — "similar parcels within 5km" (handleSimilarParcels). Combines cadastre `/spatial/point` (size band 0.4–2.5×, optional landuse prefilter, attrs_only) with cached lidar-slim KG data. Score 0..1: size ratio, landuse-summary Jaccard, built-density; when both sides have srtm data also terrain (slope/elev/aspect/dom/forest_frac) with the **srtm `fracs` composition vector** (area_summary fractions, histogram intersection) weighted dominantly (0.45 of terrain term). Reference KG lidar-slim warms synchronously (≤3s) on cold cache; candidate KGs warm in background. Response: `{ref:{slope,elev,aspect,forest_frac,dom,fracs}, results:[{score,parts,parcel_id,lon,lat,area_sqm,distance_m,slope,elev,aspect,forest_frac,dom,fracs,ez,kg_code,gnr,building_count,landuse_summary}], candidates, scored, lidar_terms, took_ms}`. Cached 1h per parcel. Cold ~0.7s, HIT ~2ms. Lidar-slim per-parcel JSON now also carries `fracs` ({type:fraction}, ≥2%, 2 decimals).
- Per-parcel OSM proximity: `GET /api/cadastre/osm/parcel/{parcel_id}` (existing proxy, 1h cache) → `{osm:{dist_road_m, dist_major_road_m, dist_rail_m, dist_transit_m, dist_train_station_m, dist_water_m, dist_settlement_m, remoteness, *_name/*_fclass}}`. First-ever call for a KG can take ~15s upstream (then persisted upstream); point variant `/api/cadastre/osm/point?lon=&lat=` is always few ms.
- N2K bonus treasures (`treasure_type='n2k_species'`, 2× value) placed in a goroutine at session create via `generateN2KTreasures`; SSE `treasures_updated`

Gameplay: giant trees (`G.topTrees`) are hidden until the player collects their first treasure (`G.tallUnlocked`, persisted via `treasures_found` on `GET /api/player/{id}`); then golden "hint" trees show until tapped (`G.tallRevealed`), which pops in all giant trees — 3 hints when fully zoomed out (zoom<14), 12 otherwise; after reveal the 6 tallest stay visible when zoomed out so at least one is always locatable. Claiming a parcel containing giant trees awards bonus XP (server: `tall_tree_count`/`tall_tree_max_h` on `/api/claim-parcel`, capped +300). Parcel fill uses the corrected lidar dominant land cover (`dom_terrain` → `DOM_TERRAIN` map, with `IMPERVIOUS_DOM` client fallback) over cadastre landuse when available. Buildings without lidar data in enhanced KGs get default height (1–2 stories by footprint) and default roof (pitched, flat if >900px²). Picker map glows cyan on enhanced municipalities.

Loading: only the 2 nearest KGs block the loading screen (`fetchKGPolygonsBlocking` ranks by camera distance); remaining KGs stream in background. `/export/geojson`, OSM and N2K proxy responses cache 24h (others 1h).

Frontend (`loadEnhancedForKGs`, all background, never blocks loading): `G.enhancedKGs`, `G.lidarParcels` (elevation tint ≥z15, slope hatching ≥z16.5), `G.lidarBuildingIdx` (real building heights/roof types, matched by centroid grid + `G.lidarGen` invalidation), `G.topTrees`/`G.topObjects` (landmark sprites), `G.osmLines` (roads/water/rail; majors-only <z15), `G.n2kSites` (hatched overlay, toggle `#btn-n2k`), `G.landPrices` (lazy per-parcel market value in popup). GPS: `#btn-gps`, `G.geo`, follow-mode disabled on manual pan. Popup enhanced rows: `renderEnhancedPopupRows` (`#pp-enhanced`, mobile "Mehr ▸" expander).

## Nature reserves (Naturschutz / Brache) — living overlay

Converted parcels (`claim.converted_to === 'biodiversity'`) are drawn by
`drawNatureReserves(ctx, claimMap)` in `render()` **above the cached base
layer** (like treasures), so grass can wave without redrawing the map. The
base only paints the fill + `drawFieldPattern(..., 'wild')` tussock mottling;
`drawLanduseSprites` skips them.

- `natureScene(f)` builds one hash-stable scene per parcel (cached in
  `NATURE.scenes`, geo coords): jittered ≤2600-point sampling, `vnoise` clump
  noise, distance-to-edge → succession zones (Saum <3.5 m: bramble, saplings,
  hawthorn, thistle; herb clumps: umbel/mullein/teasel/nettle/flowers; open
  patches: short grass, molehills, anthills, mushrooms) plus rare landmarks
  (≤2 snags — `v%3==0` has a hornet colony in the trunk cavity, ≤1 hive row,
  ≤1 pond, nest boxes). Each item has `pr` (priority); the renderer draws
  items with `pr < frac` where `frac = (sp·pxPerM/cell)²`, so screen density is
  constant across zoom and LOD never flickers. Kinds: `NK.*`.
- Wind: `windAt(x,y,t)` travelling gusts; `nBlade()` bends stems in 3 segments.
  Pixel unit `u` = 1 / 2 / 3 at zoom ≤17.5 / ≤19 / >19; all sprites use
  `wildPx()` so they scale.
- Fauna (`drawNatureFauna`): butterflies + bees at flower items, dragonfly at
  the pond, crow on snags, swallows over >3000 m², a hare crossing every 45 s.
- Device gating: `natureAnimLevel()` from `giantAnimBudget()` — 0 static
  (prefers-reduced-motion), 1 phones (15 fps via `treasureAnimLoop`, 60 %
  density, fewer fauna), 2 desktop (25 fps). `NATURE.quality` self-tunes if the
  overlay exceeds ~9 ms/frame. The rAF loop runs only while `NATURE.onScreen>0`.
- Ordinary meadows/stubble/gardens/vineyards get rare hives / nest boxes via
  `drawSporadicHabitat()` (~14 % of parcels, hash-stable) in the base layer.

## Forest plots — Holzernte & Naturwald (`srv/timber.go`, game.js "FOREST PLOTS")

A forest stand (`claimIsForest`: NS 56, or lidar tree cover ≥ 50 % on a
non-crop parcel; server: `estimateTimber().IsForest`) replaces the generic
Naturschutz/Aufforsten buttons with two options:

- **🪓 Holzernte** (`POST /api/harvest-forest`): coins now = real net timber
  value / `eurPerCoin` (10). Reuses `parcel_claims.harvested_at`/`harvests`.
  The stand then regrows on real time (`forestPhase()` ↔ `forestStage()`):
  Schlag < 40 min → Jungwuchs < 90 → Stangenholz < 150 → Baumholz
  (harvestable again, value 50 % → 100 % by 510 min). Quest "Holzknecht"
  counts `SUM(harvests) WHERE landuse='56'`; Erntedank subtracts those.
- **🌳 Naturwald** (`POST /api/convert-parcel` with `wildforest`): permanent,
  forest-only, only on a Baumholz stand; XP = `120 + min(180, Vfm/10)`.
  Counts toward the 30 % bio target (`GetSessionBiodiversityPercent`) and
  the Waldmeister/Naturschützer counters; quest "Waldhüter".

**Estimate** (`GET /api/forest-value?parcel_id=&kg=&area=&lu=&session_id=`,
cached 1 h when v3 / 10 min otherwise, singleflight per parcel):
1. Stand facts from the *cached* lidar-slim only (never warms): `fracs.tree`,
   `tree_h {mean,max}` (new slim field from `height_distribution.tree`; older
   caches fall back to `0.72·ndsm_max_m`), elevation, slope. No lidar → NS 56
   defaults (85 % canopy, h 18 m). Species mix from elevation
   (conifer share, larch above 800/1200 m, pine in lowlands).
2. **Fast path**: `POST srtm /api/v3/trees` (product apex inventory,
   `include_trees:false, fallback_live:false`) with a **2.5 s budget** — ~0.2 s
   when the KG's light GPKG is warm, minutes when cold (Zenodo). A
   timeout/202 marks the KG cold in `v3cold` for 4 min; the heuristic is used
   meanwhile. When it answers: `volume_m3_est_total`, `n_trees`, `h_mean_m`,
   `by_leaf_type`, `by_species_hint`, `area_ha_canopy` replace the heuristic
   (`source:"v3"`). Only for `isV2Product` KGs and < 60 ha.
3. Stock heuristic (Ertragstafel-ish): `Vfm/ha ≈ 0.9·h_mean^1.95` × canopy ha;
   `Efm = 0.8·Vfm`; CO₂ ≈ 0.9 t/Vfm. Assortments by height (sawlog share
   `(h−10)/18` capped 0.7, fuelwood 10–35 %, rest industrial).
4. **Prices** from `https://holzeinschlag-at.exe.xyz/data/timber_price_catalog.json`
   (weekly refresh upstream; we cache 24 h as `timber:catalog`): per-state LK
   series (`LK_BLFIM2b` spruce/fir Media 2b, `LK_BLLA3aplus` larch,
   `LK_BLKI2aplus` pine, `LK_BLBU3plus` beech, `LK_ISFI_FMO` Schleifholz,
   `LK_BHH/LK_BHW` fuelwood ×1.4 RM→Fm), national STAT series as fallback,
   `fallbackPrices` table last. `kgState()` maps the KG code prefix to the
   Bundesland (0/1/2 NÖ, 3 Bgld, 4+50–54 OÖ, 55–59 Sbg, 6 Stmk, 7 Ktn, 8 T, 9 Vbg).
   `/api/plot-context` there is 10–60 s and single-threaded — not used.
5. Harvest cost 28/36/45 €/Efm by slope (>20°, >30°) + 150 € fixed.

**Rendering.** `drawForestOverlay()` (after `drawNatureReserves`, same
LOD/animation machinery, counts into `NATURE.onScreen`) draws
`forestScene(f, 'wild'|'schlag')`:
- *wild*: clump noise → old growth (veterans, snags, deadwood, root plates),
  gaps (saplings, bramble, thorn, fireweed), mixed stand; Waldmantel with
  bramble/thorn within 3 m of the edge. Species table by elevation
  (`F_SPECIES_LOW/MID/HIGH`, pioneers `F_PIONEER`), 4 size classes in
  `fTree()`, wind bends trunks (`bend` factor). Fauna: woodpecker on snags,
  deer crossing every 60 s, jay.
- *schlag*: stumps, slash, ≤2 Holzpolter near the edge, ~2 % Überhälter
  (`bend 2.6`, visibly wind-bent), crows. Regrowth items carry `birth`
  (cycle fraction) and pop in as `forestStage().t` passes it; saplings grow
  to size 1. Ground: `TERRAIN.schlag`/`regrow` + `fieldPattern` kind 7
  (ruts). Below zoom 15 wild forests fall back to plain forest sprites
  (`getTreeStyle`), regrown-but-not-full-value stands use style `'young'`.
- Popup: `forestPopupRows()` (Holzvorrat, Bestand, Holzerlös with price
  source/date, CO₂) in the Gelände section; `#pp-field` row shows the stand
  stage. `G.forestValues[pid]` is the lazy cache; the fetch callback re-runs
  `showParcelPopup` so the action buttons pick up the coins/XP.

## Sibling roadmap — `/llm/ahead` (`srv/llmahead.go`) and what we consume

`GET /llm/ahead` is the to-do list we hand the sibling data services
(cadastre, srtm, holz, farm, gw) plus a live conformance harness
(`/llm/ahead/check/{service}`). **Token-gated** (`srv/aheadauth.go`): send
`X-Ahead-Token: $(cat ahead.key)` (or `?token=`), anything else is a plain
404. Token = env `SIEDLER_AHEAD_TOKEN` or `./ahead.key` (auto-generated,
gitignored). The public `/llms.txt` and `/llm/game` must never link to it —
agents should not learn which upstream services we build on. Same rule for
the browser: **no direct calls to sibling hosts from game.js** — everything
goes through our proxies (hillshade tiles: `GET /api/tiles/hillshade/{z}/{x}/{y}.png`,
`srv/tiles.go`, 30 d api_cache as base64, 204 = no data).
Nearly everything is green upstream; the
checkbox in the markdown means **used by the game**, driven by the
`aheadUsed` map (ID → where in our code). `?unused=1` (or `?used=0`, also
on `?format=json`) lists only items we don't consume yet — that is the view
to give an agent. **When you start consuming an item, add it to `aheadUsed`.**

Consumed today (`srv/siblings.go` unless noted):
- **ALL-4** `prewarmMunicipality()` on session create → cadastre + srtm `POST /prewarm` for the Gemeinde's KGs.
- **CAD-1** `GET /api/viewport-landuse` (see Viewport fast path).
- **CAD-2** `dominant_ns` / `landuse_areas` (game.js, no server code).
- **HOLZ-2** `timberStatePrices()` in `timber.go`: `/data/prices/state/{1-9}.json` (3 KB) replaces the 736 KB catalogue; catalogue path kept as fallback.
- **LID-4** `drawRelief()` (game.js, section "REALISM LAYERS"): srtm
  `/tiles/hillshade/{z}/{x}/{y}.png` (25 m DTM, WebMercator, CORS, 1 y cache)
  fetched via our proxy `/api/tiles/hillshade/…` (never directly — hides the
  upstream host), z = clamp(round(zoom+1), 10, 15), each
  tile corner-mapped through `toScreen()` (our plate-carrée ×1.35 vs Mercator
  differs by < 1 px inside a tile). Tiles are re-centred once on load (flat ≈
  grey 180 → 128, clamped 88..188) and composited `overlay` **deliberately
  faint** (α 0.34 → 0.10 with zoom, blurred by 25 m cell size at z ≥ 16.5 via a
  viewport scratch canvas so tile seams don't show). Anything stronger reads as
  "too dark, too many patterns" — colour + sprites must stay the focus. While
  tiles cover the view (`_reliefActive`) the per-parcel lidar slope/aspect tint
  in `drawParcelPoly` is skipped. `DEV.relief(bool)` toggles (localStorage
  `reliefOn`), `DEV.relief()` reports tile stats.
- **LID-2** `GET /api/trees` (bboxProxyOpt, 6 h) → `loadTrees(b)` per viewport
  tile: the ≤5 tallest measured apices per parcel (`h_m`, `crown_d_m`) into
  `G.apexTrees` / `G.apexByParcel`. `drawForestSprites` draws them at their real
  position via `drawApexTree()` (size ∝ height, crown/height > 0.62 → broadleaf
  variant, height tag for the parcel's tallest at zoom ≥ 17.5) and shrinks the
  procedural filler accordingly. Apices ≥ 32 m also join `G.topTrees['apex']`
  so giant-tree gameplay works in every indexed KG, not only where the 1 MB
  lidar-slim is loaded; `tallIndex()` dedupes slim vs apex crowns within ~12 m.
  `DEV.apex()` / `DEV.apex(pid)`.
- **FARM-4** `GET /api/hofstellen` (24 h) → `loadHofstellen(b)`; `drawHofstellen()`
  puts a pixel tractor + hay bales SE of the real farmstead point (zoom ≥ 15,
  label ≥ 17, drawn after footprints); `hofOnParcel(pid)` (lazy point-in-parcel
  index). `DEV.hof()`.
- **Forest sprite density**: `prand(seed,i)` (integer mix) replaced the old
  `(hash+i*k)%10000` lattice sampling that left most points outside the polygon;
  placement now rejection-samples until `treeCount` trees land inside, and at
  zoom ≥ 15.5 the cap grows with on-screen area (1 tree / 1400 px², ≤ 260).
- **Visual restraint rule**: field textures α 0.5/0.35, Wasserschutz = blue
  wash + thin dashed edge (no hatch), relief faint. Colour + sprites carry the
  map; patterns are hints. Field cycle is 60 min (`FIELD_CYCLE_S`/`fieldCycle`).
- **LID-3** `GET /api/buildings` (bboxProxyOpt, 6 h) → `loadBuildings(b)` per viewport
  tile into `G.bldgByFp[footprint_id]` = `{max_height_m, mean_height_m, stories_est,
  roof_type_hint, exact:true}`. `lidarForFootprint(f, ring)` is the single lookup
  for renderer (`drawBuildingFootprints`) and building popup: footprint_id first,
  then the ~20 m centroid grid from the lidar-slim (`findLidarBuilding`). Storeys
  come from `mean_height_m/2.9` (upstream `stories_est` is ridge/3). `DEV.bldg()`.
- **HOLZ-3** `plotHistory()` in `timber.go`: `POST holz /api/plot-context?fast=1`
  with the parcel polygon, 2.5 s budget in parallel with v3 (`holzCold` backoff on
  429/timeout). `estimate.history` = `{forest_share_2000_pct, loss_total_ha,
  loss_recent_ha, last_loss_year, last_loss_ha, young_frac, stock_factor,
  net_flux_tco2e_ha}`; `stock_factor` scales the NS-56 heuristic Vfm (lidar heights
  already see a young stand). Popup row "🛰️ Waldgeschichte" + CO₂ balance suffix
  (`forestPopupRows`). `handleHarvestForest` pays the cached popup estimate
  (`timber:<pid>`) so v3/history-backed numbers match. `DEV.timber(pid, force)`.
- **CAD-5** `watchAssembly()` in `server.go`: every `/api/viewport` fetch records
  upstream's per-KG `assembly {version, reprocessed_at}` (`assembly:<kg>` cache row,
  1 y). A changed tag purges `viewport:*`, `vplanduse:*`, `bldg:*` and the KG's
  `geom:parcels:*`, export/spatial and `timber:*` rows — no manual purge after
  upstream geometry fixes anymore.
- **KG names** (glitch #10): `kgName(kg)` / `ensureKGName(kg)` — viewport rows carry
  no `kg_name`; a lazy `CAD /lookup?type=kg` fills `G.kgNames`. Never print a bare KG
  code in UI; use `kgName(kg) || 'KG ' + kg`. `DEV.kgName(kg)`.
- **Giant-tree labels**: `labelSlotFree()` pre-pass in `drawTopLandmarks` (tallest
  wins), tags queued in `_treeLabelQueue` and drawn by `flushTreeLabels()` above all
  sprites, pill background like treasure tags.
- **Session centre**: `settlementCenter()` (server, create) snaps the Gemeinde
  centroid to the OSM place node ≤ 8 km. **Flowpath prewarm**: `kgsAlongPath()` +
  `prewarmKGs()` in `siblings.go` (once per path, `gwflow-warm:` marker).
- **FARM-2** `GET /api/schlaege` → `loadSchlaege(b)` per viewport tile (24 h cache). AMA INVEKOS
  field polygons (`crop_group`, `snar_name`, `area_ha`, `organic`; CC BY 4.0).
  `parcelSchlag(p)` = Schlag under the parcel centroid (cached per `G.schlagGen`);
  `fieldKindFor(p, hash)` maps `CROP_GROUPS` → field kind (getreide 0 sheaves,
  mais/sonst 1 maize stooks, obst/wein 2 haystacks, gruenland/alm/brache 3 meadow),
  falling back to the hash. Cycle *phase* stays hash-based. Popup row `#pp-crop`
  ("Anbau: 🌽 Körnermais · 6,3 ha · 🌿 Bio"). `doHarvest()` sends `crop_group`;
  server `fieldPhaseAtCrop()` uses it for meadow-vs-crop (`cropMeadow` mirrors `CROP_MEADOW`).

Deliberately **not** consumed (optimisations that conflict with our model or add
no realism): CAD-3 `tolerance_m` (breaks per-parcel dedup), CAD-4 `include=`
enrichment, LID-1 slim fields on `/query/parcels`. LID-5 (queue) is 401 upstream,
HOLZ-4 (stands) 404. We only use **fast paths**: anything that can take > 3 s
per call is out, whatever it would add.

## Gemeinde-Chronik dossiers & water mechanics (gw / holz / farm siblings)

Backend: `srv/dossier.go` (`GET /api/dossier/{kg}` = gw + holz + farm `/llm/kg`
in parallel, 6 h cache, 404 negative-cached 1 h; derives `drought{level,label,
status,sigma}` and `game{yield_factor,well_protection,subsidy_per_ha}`) and
`srv/water.go` (GW-1…8 proxies: `/api/water/point|points|station/{id}|
protection|flowpath|gwi`, `GET /api/well-quote`, `POST /api/dig-well`,
`GET /api/field-economy`; harvest payout = crop × drought factor (+ well
protection) + Förderung; Naturschutz ×1.5 XP inside a Wasserschutzgebiet;
+80 XP Pegelwart when `gw_station:true` on claim). Claims carry `well_at` /
`well_depth_m`. SSE: `well_dug`, `parcel_harvested{meadow,drought}`.

Frontend (game.js section "WATER & GEMEINDE-CHRONIK", all lazy/background):
- **HUD chip** `#water-chip` (GW-4) inside `#hud-badges` (flex row shared with
  `#enhanced-badge` — both are `position:static` now; `drawN2KOverlay` reads
  the row to place its label). `updateWaterChip()` runs from
  `loadMoreParcels()`/`fetchKGPolygons()`; KG = `currentWaterKG()` (sticky
  `kgAtCamera()`), dossier via `loadDossier(kg)` → `G.dossiers[kg]`
  (`'loading'` | dossier | `{error}`), `G.drought` = current KG's block. Colour
  classes `st-normal|low|very_low|high`, `.pulse` at level ≥ 2. Phones show
  "💧 Dürre −2,2σ" only. Sidebar row `#sb-chronik` mirrors it. Herald hint
  `drought` (one-shot, suppressed while a Wasserweg runs).
- **Chronik panel** `#dossier-popup` (`openDossier(kg, tab)`, tabs
  water/forest/farm, `renderDossier(d)`): pp-grid rows + one `pxChart()` (≤ 24
  div bars, `.bad/.warn/.cur`), `.season-cal` 12 cells from
  `season_profile` (current month outlined), `.ds-rule` gold box = the game
  rule (yield ×, well %, Naturwald XP, Förderung 🪙/ha), `segBar()` for
  `archetype_mix`. Entry points: chip, sidebar row, `openKGSummary()` link,
  Herald link. Wasser tab hosts the Messstellen checkbox (`setGwVisible`) and
  the "Weg des Wassers" button.
- **Field economy** (`fieldEconomy(pid)` 60 s cache → `renderFieldEconomyRows`):
  owned NS-48 parcels get row `#pp-eco` ("Ernte 🌾 42🪙 + 🏛 18🪙 = 60🪙",
  ☀️ malus line) or "🏛 Förderung 2🪙 alle 60 min" for meadows; harvest button
  uses `economy.total`; meadows get "🏛 Förderung abholen" / disabled "in N min"
  (gate = `harvested_at` + `FIELD_CYCLE_S`). `doHarvest()` sends `organic` from
  `parcelSchlag()`; `harvestToast(res)` breaks the payout down.
- **Brunnen** (GW-1): `wellButtonHTML()` (lazy `fetchWellQuote` → depth, price,
  protection in the title) → `doDigWell()`; row `#pp-well`; `drawWells()` pixel
  well in the base layer (after forest sprites, zoom ≥ 16, `wellPointFor(f)`
  hash-stable inside the ring). `baseSignature` counts wells.
- **Messstellen** (GW-2): `loadPointLayer()` (point sibling of `loadBboxLayer`)
  → `loadGwPoints(b)` per viewport tile into `G.gwPoints`; `drawGwStations()`
  in the dynamic layer (zoom ≥ 15, sprites per category in
  `drawStationSprite`, co-located points fan out 12 px, labels ≥ z17);
  `hitStation()` in `onGameClick` before parcels → `openStation(s)`
  (`#station-popup`, metrics + `drawPixelLine()` 30-year canvas chart).
  `stationOnParcel(pid)` (index `G._stByParcel`) drives the popup row
  "📏 Messstelle · +80⚡ Pegelwart" and `gw_station` on claim.
- **Wasserschutzgebiete** (GW-5): `loadWaterProtection(b)` via `loadBboxLayer`
  into `G.wpZones` (with bbox); `drawWaterProtection()` in the base layer right
  after `drawN2KOverlay` (blue hatch + dashed edge, label ≥ z16), gated by the
  same `G.n2kVisible` / `#btn-n2k`. `waterProtectionAt(lon,lat)` → popup badge
  and the Naturschutz button label "+150⚡ 💧"; `doConvert()` sends lon/lat.
- **Wassertropfen-Reise** (GW-6): `startFlow(lon,lat)` → `G.flow` {pts, cum,
  dur, follow}; the MERIT path (~500 m vertices) is **snapped onto the OSM
  watercourses we draw** by `refineFlow(F)`: `riverChains()` merges
  `G.osmLines` water ways (river/stream/canal/drain) end-to-start into chains,
  `snapToRiver()` projects each MERIT vertex (≤ 250 m, rivers preferred), and
  every segment whose ends hit the same chain is replaced by the chain's
  vertices (adjacent vertices move onto the river — no spurs). Runs at start,
  every 1.5 s of the trip and whenever a KG's water lines arrive, keeping the
  droplet at its current fraction. Pace: 20 s + 1.5 s/km, ≤ 150 s, camera
  zoom 15. `DEV.flowInfo()` reports refined segments / chains. OSM `cat=water`
  lines load for **every** KG in `loadWaterForKG` (roads/rail enhanced-only).
  `flowAnimLoop()` (setTimeout 40 ms, only while `G.flow`) moves
  the camera along the droplet (time-based lerp; manual pan sets
  `follow=false`; `loadMoreParcels()` every 2.5 s), `#flow-chip` shows
  "23 / 65 km · Kainach" then "Kainach → Mur → Schwarzes Meer · 65 km";
  `drawFlowPath()` cyan line + white travelling dashes + gauge checkpoints +
  2-px droplet. `clearFlow()` on ✕.
- **Picker tint** (GW-7): `loadPickerGwi(munis)` = `/api/water/gwi` joined with
  cadastre `/spatial/kgs?fields=kg_code,gemeinde_code` (state bbox, ~18 KB gz)
  → `G.gwiByGemeinde` mean GWI → `gwiTint()` wash in `drawMuniPoly()`;
  legend `#pick-gwi-legend`.
- Popup rows for all of it are appended in `waterPopupRows(pid, rows)` from
  `renderEnhancedPopupRows`. DEV: `DEV.dossier(kg, tab)`, `DEV.station(id)`,
  `DEV.flow(lon, lat)` / `DEV.flow(false)`, `DEV.water()`. Screenshots in
  `docs/screenshots/water/`, glitches #10–17 in `docs/glitches.md`.

## Quests → Herald briefings

`GET /api/session/{id}/challenges` returns each open quest with live
`progress`/`goal` (server `questProgressFor`, same counters as
`autoCompleteChallenges`). The sidebar renders a progress bar for goal>1.
**Tapping a quest** calls `Herald.brief(id)`: the herald plays a 2-line
briefing built by `questBriefing(c)` in game.js — the quest card, then a
"So geht's / Wo?" line with live context (nearest treasure distance, owned
unconverted parcels, tallest nearby giant tree, coins) — and on the last
line shows a gold **action button** (`#herald-act`) that does the thing:
`questPing(lon,lat,zoom)` flies there and pulses gold rings + a 📍 for 5 s
(`drawQuestPing`, drawn just before the scale bar), then opens the parcel
popup where relevant. Tapping the same quest again toggles the herald off;
on mobile the bottom sheet collapses so the herald is visible. All strings
are in the i18n dictionary. QA: `await DEV.quest()` (list),
`await DEV.quest(id|title)` (open briefing), `await DEV.quest(id, true)`
(also run the action).

## Screenshots / QA scripting (`window.DEV`)

game.js exposes a `DEV` helper for browser automation (no UI). Rejoin a
session directly with
`/?lang=de&dev=1&pid=<id>&pname=<name>&rejoin=<token>&sid=<session>` (`dev=1`
skips the loading-screen dwell; `#v=lon,lat,zoom` sets the initial camera),
then in `browser eval`:

```js
await DEV.goto(15.5205, 48.3955, 17.5)   // camera + load tiles + wait idle
await DEV.parcel('12105-68/3')           // select + popup (optionally center)
DEV.ez('12105', 430); DEV.kg('12105'); DEV.tree(0)   // EZ / KG stats / tree histogram
DEV.trees('locked'|'hint'|'revealed')    // giant-tree gameplay state
DEV.ezCandidates(4, 20); DEV.parcelsNear(p => p.building_count > 0)
DEV.chrome(false); DEV.sidebar(false); DEV.freeze()  // clean hero shots
DEV.closeAll(); DEV.state()
await DEV.quest(); await DEV.quest('Schatzsucher', true)   // quest briefing + action
```

Use `emulate_custom` with DPR 2 (desktop 1920×1080) or `emulate_device`
(phone) for hi-res captures. Keep a glitch log while shooting — see
`docs/glitches.md`.
