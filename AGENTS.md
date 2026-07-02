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
2. Load point parcels via `/spatial/bbox`
3. Load KG polygon data (parcels + building_footprints + landuse) via `/export/geojson` — one request per KG, in parallel
4. Build EZ index from loaded polygon data
5. Load claimed parcels, treasures, challenges, players, biodiversity, chat from our API
6. On pan/zoom: `loadMoreParcels()` → `fetchKGPolygons()` → `buildEZIndex()` incrementally

## Map Rendering

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

Landuse codes follow Austrian BEV Nutzungssymbol system (40–97). See `LANDUSE_TERRAIN`, `ABBR_MAP`, `LANDUSE_POLY_COLORS` maps in game.js.

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
- `POST /api/register` — create player with name
- `POST /api/login` — find player by name

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
- Base price/m² by landuse: Baufläche 0.5, Wiese 0.3, Wald 0.2, Verkehr 0.1, Gewässer 0.05
- Density multiplier: built-up ratio >0.3 = 2×, 0.05–0.3 = 1–2×, no buildings = 0.5×
- Clamped to 10–5000 coins. Mirrored in JS `calcPrice()` for client display.

## Real-Time (SSE)

Server broadcasts events via `s.broadcast(sessionID, data)`. Frontend handles in `handleEvent(d)`:
- `parcel_claimed`, `parcel_converted`, `parcel_sold`
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

- No persistent auth — players identified by localStorage `pid`/`pname` + rejoin token
- Parcel polygons only load for KGs visible at zoom; panning loads more incrementally
- Canvas rendering (no WebGL) — performance drops with very dense urban areas
- Price calculation is duplicated in Go and JS — keep them in sync
- EZ index only contains parcels from loaded KGs (not full municipality EZ data from API)
- No transaction wrapping on bulk EZ claim (individual parcel inserts)

## Environment

- Go 1.24+, SQLite via modernc.org/sqlite (pure Go)
- systemd service: `/etc/systemd/system/srv.service`
- Binary: `./siedler`, DB: `./db.sqlite3`
- Cadastre API: `https://cadastre-process-api.exe.xyz/api/v1` (docs: `/api/v1/docs/llm.txt`)
- Port 8000, proxied via exe.dev HTTPS

## Enhanced Mode (LiDAR)

For KGs processed by `https://srtm-lidar-at.exe.xyz:8000/api/v1` (srtm-lidar API):
- `GET /api/enhanced-kgs` — registry of processed KGs (15min cache); "Auf Glück" prefers these ~90%
- `GET /api/lidar/kg/{code}` — slim KG JSON (server strips vertex_heights, flag-filters top trees ≤60m / objects ≤120m; 6h cache)
- `GET /api/lidar/...` — generic proxy (1h cache; overlay/elevation/dtm blocked — too slow)
- N2K bonus treasures (`treasure_type='n2k_species'`, 2× value) placed in a goroutine at session create via `generateN2KTreasures`; SSE `treasures_updated`

Frontend (`loadEnhancedForKGs`, all background, never blocks loading): `G.enhancedKGs`, `G.lidarParcels` (elevation tint ≥z15, slope hatching ≥z16.5), `G.lidarBuildingIdx` (real building heights/roof types, matched by centroid grid + `G.lidarGen` invalidation), `G.topTrees`/`G.topObjects` (landmark sprites), `G.osmLines` (roads/water/rail; majors-only <z15), `G.n2kSites` (hatched overlay, toggle `#btn-n2k`), `G.landPrices` (lazy per-parcel market value in popup). GPS: `#btn-gps`, `G.geo`, follow-mode disabled on manual pan. Popup enhanced rows: `renderEnhancedPopupRows` (`#pp-enhanced`, mobile "Mehr ▸" expander).
