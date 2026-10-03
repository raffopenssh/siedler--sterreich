# Siedler Österreich — Agent Guide

Multiplayer browser game on real Austrian cadastre data: explore, claim parcels,
convert land to nature reserves. Settlers IV pixel-art look. Go `net/http` +
SQLite (sqlc) backend, single-file vanilla-JS canvas frontend. No frameworks.

## Quick Start

```bash
go build -o siedler ./cmd/srv/   # build
sudo systemctl restart srv        # deploy (runs ./siedler on :8000)
journalctl -u srv -f              # logs
go generate ./db/...              # after editing db/queries/*.sql
go build ./... && go vet ./srv/...
```

Live: `https://siedler-oesterreich.exe.xyz:8000/`. DB `./db.sqlite3`. Service
`/etc/systemd/system/srv.service`. Maintenance mode: `touch MAINTENANCE`
(bypass cookie `siedler_dev=1` / `?dev=1`). Provider history (Oct 2026):
`docs/migration-2026-10.md`.

## Layout

```
cmd/srv/main.go          entrypoint, flags
srv/server.go            routes (Serve()), game logic, SSE, cachedFetch, generic proxies
srv/viewport.go          /api/viewport: one 0.02° cadastre cell (bevdirect) + srtm terrain enrichment
srv/cellstore.go         read-side helpers over cached cells (parcelsNear, lookupParcel, ensureCell…)
srv/upstreams.go         provider base URLs, 0.02° grid helpers (cellOf/cellsForBBox), embedded admin.json.gz
srv/warm.go              cell prewarming (daily plan, neighbour, session), /api/lucky, /api/warm/status
srv/landscape.go         srtm public-tier adapters (/api/enhanced-kgs, /api/lidar/kg, trees/buildings/landmarks,
                         /api/landscape, /api/parcel-context, /api/osm-lines, /api/n2k, /api/municipality…)
srv/siblings.go          farm/holz layer proxies (bboxProxy, hostSlots), prewarmKGs, kgsAlongPath
srv/water.go, dossier.go groundwater mechanics, Gemeinde-Chronik dossiers
srv/timber.go            forest value / harvest; similar.go similar parcels; treasures.go
srv/agent.go, agent_inspect.go, discover.go, leaderboard.go, llmahead.go  agent surface
srv/breaker.go, upstream_pending.go, metrics.go, parcelhash.go, licenses.go, tiles.go
srv/static/game.js       entire frontend (~11k lines); index.html all screens; style.css theme
db/migrations/NNN-*.sql  auto-applied on startup; db/queries/game.sql → sqlc → db/dbgen/
```

## Data providers

The browser never talks to a data host — everything goes through our proxies.
All upstream calls use `upstreamGet`/`upstreamClient` (pooled, HTTP/2, 60 s
timeout, per-host circuit breaker). Never `http.Get`; never set
`Accept-Encoding` by hand (Go's transport gunzips transparently).

| what | where | notes |
|---|---|---|
| **cadastre** — parcels, footprints, landuse polygons, EZ, Gemeinde/KG at point | **bevdirect-serve** `http://127.0.0.1:8787` (`bevAPI`, systemd `bevdirect-serve`, /opt/bevdirect) | Assembles live from `kataster.bev.gv.at` vector tiles (CC BY 4.0). World = fixed 0.02° grid `floor(lon/0.02), floor(lat/0.02)`. Endpoints `/viewport?west&south&east&north&layers=parcels,footprints,landuse&wait=s`, `/parcel/{kg}-{gnr}?lon&lat`, `/ez?kg&ez&west..north`, `/municipality?lon&lat`, `/municipalities?q=`, `/kg/{kg}`, `/health`. `ready:false,pending:true,retry_after_s` = still assembling, **never "no parcels"**. Every response carries `notice` (© BEV … CC BY 4.0, bearbeitet) → shown in `#map-attrib`, relayed on agent endpoints. Anything derived is cached **≤ 24 h** (`cadastreTTL`). |
| **context** around a point | **umfeld-at** `https://umfeld-at.exe.xyz/api/v1` (`umfeldAPI`; alias `cadastreAPI` for legacy non-cadastre call sites), ≤ 5 req/s | `/context` (land price, OSM distances, N2K, toponyms, RIS legal), `/search/municipalities`, `/lookup`, `/search/address_osm`, `/osm/geometry?bbox` (bbox only), `/natura2000/*`, `/toponyms/*`, `/legal/*`, `/land_prices/point…`. Statistik Austria, EEA, OSM, BEV DLM names, RIS. |
| **landscape** | **srtm-lidar-at** `https://srtm-lidar-at.exe.xyz/api/v1` (`lidarAPI`, public tier: bbox/point/KG-code keyed only) | `/landscape?bbox`, `/trees/bbox`, `/buildings/bbox`, `/landmarks/bbox`, `/heightfield?bbox&cell=25&landcover=1` (404 where no grid25), `/kgs`, `/kg/{code}`, `/tiles/hillshade/{z}/{x}/{y}.png`. **No parcel or footprint ids** — the client assigns by point-in-parcel / centroid grid. |
| **admin table** | `srv/data/admin.json.gz` (embedded; BEV VGD 1:50 000, CC BY 4.0) | all 7 850 KGs: code, name, Gemeinde, district, state, bbox, area. Drives `/api/kg-geo/{kg}`, KG neighbours, the warm plan, `/api/lucky`, `kgsAlongPath`. |
| unchanged siblings | holzeinschlag-at, farm-subsidies-austria, groundwater-at | timber prices/history, INVEKOS Schläge/Hofstellen, water. farm host has a 3-slot semaphore (`hostSlots`; busy → 503 `status:"busy"` + `X-Upstream: busy`). |

Privacy: the DB stores **no cadastre ids** — claims/offers/harvests/quest targets
are keyed by HMAC `parcel_hash`/`ez_hash` (`srv/parcelhash.go`); the client maps
them back via `resolveClaims()` from loaded geometry (`G.pidByHash`, `G.ezByHash`;
`addParcelHashes` injects `ph`/`ezh` into cell bodies at serve time).

Ops: `GET /api/metrics` (route p50/p95, cache hit ratio, bevdirect health),
`GET /api/warm/status`, `GET /api/upstreams` (breakers), `tools/soak.sh`
(`USERS=20 SECONDS_=30`), browser `DEV.cells()/warm()/lucky()/timing()/soak()`.

## Game State (frontend)

```js
G.player, G.session, G.cam {lon,lat,zoom 13–20}, G.sel, G.ezHighlight {kg,ez}
G.parcelPolys        // parcel Features from /api/viewport (polygon geometry)
G.parcels            // point Features synthesised from parcelPolys (same properties object)
G.buildingFootprints // DKM NS-41 building polygons (footprint_id)
G.landusePolys       // landuse polygons {landuse_code, area_sqm}
G.lidarParcels[pid]  // per-parcel terrain from the cell row (elev, slope, aspect, dom, fracs)
G.ezIndex            // "kg_code-EZnnn" → [features]
G.claimed, G.treasures, G.challenges
G.kgsLoaded (Set), G.kgNames, G.terrainKGs, G.enhancedKGs
G.vpTiles / G.polyIds / G.fpIds / G.luIds   // dedup: cell keys, parcel ids, footprint ids, landuse keys
G.cellState          // per cell: state (loading|pending|ready|down|error|gaveup), xcache, ms — DEV.cells()
```

## Screen Flow & Loading

`welcome` → `pick` (municipality picker) → `loading` → `game`. Lobby is bypassed
(`startSinglePlayer()`).

1. `POST /api/session/create` — never touches the cadastre (~20–100 ms; enqueues
   the Gemeinde's KGs for warming; N2K/parcel treasures placed in goroutines →
   SSE `treasures_updated`).
2. Loading steps: Session → Landschaft (srtm `/api/lidar/kg` terrain, hillshade,
   trees — fast) → Kataster (`fetchKGPolygonsBlocking(6000)`: fast layers for the
   initial cells first, then only the **centre cell** blocks, ≤ 6 s; subline says
   "aus Zwischenspeicher" vs "live aus BEV-Kacheln") → Arten & Schätze → Bereit.
   The game opens even if the centre cell is still assembling.
3. On pan/zoom: `loadMoreParcels()` → `fetchKGPolygons()` → `gridTiles(paddedView,
   ≤12)` (nearest camera first, `tilesInAustria` filter) → `runPool(…, 4)` →
   `loadTileResilient(cell)` → `loadViewportGeometry(cell)`; then `buildEZIndex()`,
   `loadEnhancedForKGs()`. `loadFastLayers(c)` fires per cell in parallel (OSM
   lines, trees, buildings, N2K, Schläge, Hofstellen, GW points, Wasserschutz).
   During a Wassertropfen-Reise only 2 cadastre cells load.

### `/api/viewport` — one grid cell (`srv/viewport.go`)

- Request `west,south,east,north` **grid-aligned** (`i*0.02 … (i+1)*0.02`, client
  `cellBox/cellOf/gridTiles`); span ≤ 0.05°. Aligned cells key `vp:v1:i:j` (what
  the warm planner fills); other bboxes get a ~150 m quantised key and are not
  prewarmable.
- Response `{parcels[], footprints[], landuse[], kgs[], ready, truncated,
  retry_after_s?, notice, cell, source}`. Parcel row = bevdirect row (`parcel_id,
  kg_code, gnr, ez, area_sqm, lon, lat, complete, parts, dominant_ns,
  landuse_areas{ns:m²}, building_count, total_building_area_sqm, geometry`) plus,
  where a grid25 heightfield exists, `elev_m, elev_min_m, elev_max_m, slope_deg,
  aspect, dom_terrain, fracs, tree_frac` (`enrichParcel`). Footprint: `footprint_id,
  parcel_id, ns_code, area_sqm, obb_length_m, obb_width_m, orientation_deg,
  geometry`. Landuse: `{code, area_sqm, geometry}`. `kgs[]` carries names +
  `enhanced` flag.
- `buildCell` fetches bevdirect (`wait=10`) and the srtm heightfield in parallel;
  caches **only when ready**, 24 h. `handleViewport` → `cachedFetchX` (singleflight
  `s.sf`, `X-Cache: HIT|MISS|MISS-SHARED|WARM`) + hot LRU of pre-gzipped cells
  (`hotCellGet/Put`, 80). Foreground builds make the warm loop yield (`warm.fg`).
  A ready build also triggers `noteCellLoaded` (neighbour warming).
- `/api/viewport-landuse` still exists: same cell build, landuse array only.
- Client: `loadViewportGeometry` dedups by cell key + ids, returns `{added, ready,
  truncated, retryAfter, cached}`, un-marks `G.vpTiles` when not ready so a
  re-fetch is allowed. `loadTileResilient` re-polls `ready:false` up to 10× paced
  by `retry_after_s` (≤12 s), 4× when the breaker says `down`, only while the
  cell is still near the screen. Truncated cells are accepted as-is.
- **Never gate polygon loading on zoom/span** (`viewBounds()` is in device
  pixels). **Never treat 202 / `ready:false` / `pending:true` as "no data"**, and
  never cache such a response.
- Gone (410): `/api/kg/{code}` whole-KG export, `POST /api/geometry-batch/*`,
  and every cadastre-shaped path on `/api/cadastre/*` (only `umfeldPublicPrefixes`
  are forwarded).

### Cell store (`srv/cellstore.go`)

Everything that used to need a parcel index upstream reads our cached cells:
`cachedCell` (no fetch), `ensureCell`/`ensureCellStatus` (blocking, singleflight;
200/202/5xx + retry hint), `parcelsNear(lon,lat,radiusM,maxCells)`,
`lookupParcel(pid,lon,lat)` (cell first, then bevdirect `/parcel/{id}`, 24 h),
`footprintsOf`, `landuseShares`, `parcelBBox`. Agent endpoints relay a cold cell
as `202 {status:"pending", retry_after_s}` + `Retry-After` (`relayCellStatus`).

### Warming (`srv/warm.go`)

`kg_warm` table (`kg_code, warmed_at, expires_at=+24 h, cells, parcels, reason`).
Single worker, ~0.8 s between cells, yields to foreground, skips KGs fresh ≥ 2 h.
- **Daily plan**: 100 KGs/day in 20 patches of 5 KGs: a small Gemeinde whose
  KGs are *all* srtm-enhanced (grid25) + nearest neighbour KGs (any), one patch
  per 24h/20, ≤ 4 per Bundesland; persisted as `warm-plan:v2:<date>` in
  api_cache so restarts resume.
- **Neighbour**: first foreground build of a cell enqueues the KGs touching it +
  adjacent KGs (low prio, debounced 1 h per cell).
- **Session**: `POST /api/session/create` → `warmGemeinde` (medium prio);
  `prewarmKGs` (siblings.go) adds KGs along a water flowpath.
- `GET /api/lucky` → `{gemeinde_code, name, lon, lat, enhanced, warm, kgs[]}`,
  only Gemeinden whose KGs are warm (not expiring within 2 h) **and** srtm
  grid25 (`enhancedKGSet`); the spawn KG itself must be enhanced. Picks are
  logged `lucky: pick`.

Known limitation: a cached cell (≤ 24 h) can disagree with a live bevdirect
`/ez` or `/parcel` answer (e.g. `building_count`). Deliberately not fixed —
re-fetching cells on mismatch would defeat the cache that keeps BEV tile load low.

## HTTP 202 / pending (`srv/upstream_pending.go`) and outages (`srv/breaker.go`)

- `parsePending()` normalises any upstream "still working" body to
  `{status:"pending", retry_after_s, progress{…}}`; `upstreamGetWait()` polls a
  bounded budget honouring `Retry-After`; `withWait()` adds `?wait=`. Proxies
  relay 202 + `Retry-After`, never cache it. Client `api()` retries GETs on 202
  (≤45 s / 8 tries, `pendingBudgetMs` opt, `pendingNotice()` updates
  `#map-loading`), then resolves `pending:true` so callers schedule their own retry.
- Breaker: `upstreamClient.Transport` per host; 3 consecutive transport errors /
  502–504 open it; while open every call answers synthetic **503
  `{error, status:"down", service, retry_after_s}`** + `X-Upstream: down` in ~0 ms;
  one probe at a time (20 s → 3 min). Client records `G.upstreamDown` (silent for
  optional layers; only a cadastre outage toasts), `loadBboxLayer`/`loadPointLayer`/
  `loadTileResilient` re-schedule after the window (`layerRetryMs`). Only requests
  whose own context is not done count as failures — caller budgets (2.5 s timber)
  are not outages. `/api/metrics` books down/busy under `upstream_down_503`.

## Map Rendering (pure canvas 2D)

`toScreen(lon,lat)`, `toGeo(x,y)`, `mapScale() = 2^(zoom-14)*25000`. Render order
in `render()`: grass pattern → landuse polygons → parcel polygons (terrain
colour) → point parcels → forest sprites → building footprints → treasures →
EZ highlight → selection → scale bar. Living overlays (nature reserves, forest,
treasures, quest ping) draw above the cached base layer (`invalidateBase()`).

**MultiPolygon.** Parcels with detached parts are MultiPolygon (common and
*big* — Almen split by ridges). Never index `geometry.coordinates[0]` or bail on
`type !== 'Polygon'`; use `geomAllRings(g)`, `biggestRing(g)`, `isAreaGeom(g)`,
`pipGeom(lon,lat,g)`, `pipRings(lon,lat,rings)`, `featureLonLat(f)` (`geomOuterRings`
is a deprecated alias). Hit-test **all rings even-odd** (ring[0] of a part is the
exterior, following rings holes; part order is not a contract). Footprints are
single Polygons. Go side: `geomRings`, `pipRingsGo`, `bboxOfRaw` (viewport.go).

**Austrian border.** `srv/static/austria.json` → `G.atBorder`; `drawForeignShading()`,
`drawAustriaBorderLine()`, minimap, `updateAbroadBadge()`, `tilesInAustria()`.
`insideAustria()` returns **true** while loading — never gate strictly on it.

**Landuse codes (BEV Nutzungssymbole).** `NS_TABLE` in game.js is the single
source of truth (code → `{abbr,name,terrain,price}`); `LANDUSE_TERRAIN`,
`LANDUSE_NAMES`, `ABBR_MAP`, `LANDUSE_POLY_COLORS` derive from it. Only 26 codes
exist (40,41,42,48,52–65,72,83,84,87,88,92,95,96); never match on label text.
48 = Äcker/Wiesen/Weiden (farmland), 83 = Gebäudenebenflächen, roads 95, rail
92, parking 42. Cell rows carry measured `dominant_ns` + `landuse_areas` (m²);
`extractLuCode()` prefers `dominant_ns`, `getLanduseName()` lists shares.
`landuse_summary` symbol counts are a fallback weighted by `nsWeight()`
(traffic 0.25, building 0.5) in `parseLanduseSummary()`; server mirror
`dominantNS()` (agent.go). Water share = `waterFraction()` from NS 59/60 areas.

**Textures/markers.** `drawTreasure()` (rarity ring `TREASURE_RARITY`, bobbing
sprite, `treasureHitRadius()` ≥ 22 px), `drawFieldPattern()` (NS 48, kind from
`fieldKindFor`/hash, rotated along the longest edge), crop/meadow sprites at
zoom ≥ 16. Canvas type scale `MAP_FONT` (`label` 14px VT323, `small` 11px,
`pixel` 9px Press Start 2P) — never ad-hoc sizes. Visual restraint: field
textures α 0.5/0.35, Wasserschutz = blue wash + dashed edge, relief faint.
Colour + sprites carry the map; patterns are hints.

## Database

SQLite, WAL, busy_timeout 5 s, `SetMaxOpenConns(8)`. Tables: `players`
(coins start 10000, xp, level, rejoin_token, agent), `game_sessions`,
`session_players`, `parcel_claims` (hashed parcel/EZ, kg_code, area, landuse,
converted_to, purchase_price, harvested_at, harvests, well_at, well_depth_m),
`treasures`, `challenges`, `chat_messages`, `offers`, `api_cache` (hourly
`cacheJanitor` prunes expired rows), `kg_warm`.

Migration: `db/migrations/NNN-name.sql` ending with `INSERT OR IGNORE INTO
migrations (migration_number, migration_name) VALUES (NNN, 'NNN-name');`.
Query: edit `db/queries/game.sql` (`-- name: X :one|:many|:exec`) → `go generate
./db/...` → `s.Q.X(ctx, …)`.

## API Endpoints

**Auth.** `POST /api/register` (returns `rejoin_token` once; 409 carries
`suggested`), `GET /api/suggest-name`. Mutating calls need `X-Player-Token`
(`authPlayer`; client `api()` sends `G.playerToken`). `Player.RejoinToken` is
`json:"-"`.

**Session.** `POST /api/session/create|join`, `GET /api/session/{id}`,
`/players|parcels|treasures|challenges|biodiversity|chat|offers`, `/events` (SSE).

**Actions.** `POST /api/claim-parcel` (parcel_id, kg_code, gnr, ez, area_sqm,
landuse, building_count, total_building_area, tall_tree_*, gw_station),
`/claim-ez` (20 % off, ≤100), `/convert-parcel` (biodiversity|forest|wildforest),
`/sell-parcel` (60 %), `/harvest-parcel`, `/harvest-forest`, `/dig-well`,
`/claim-treasure`, `/complete-challenge`, `/offer-parcel`, `/offer-respond`,
chat + rules + block/report.

**Geo/data proxies.** `/api/viewport`, `/api/viewport-landuse`, `/api/lucky`,
`/api/warm/status`, `/api/kg-geo/{kg}`, `/api/municipality?lon&lat`,
`/api/municipalities?q=|list=all|state=&format=geojson`, `/api/parcel-context`,
`/api/osm-lines?…&cat=road,rail,water`, `/api/n2k`, `/api/enhanced-kgs`,
`/api/lidar/kg/{code}` (`parcels:[]` always), `/api/lidar/*` (generic, slow paths
blocked), `/api/trees`, `/api/buildings`, `/api/landmarks`, `/api/landscape`,
`/api/similar`, `/api/forest-value`, `/api/building-info`, `/api/kg-summary/{code}`,
`/api/schlaege`, `/api/hofstellen`, `/api/water/*`, `/api/well-quote`,
`/api/field-economy`, `/api/dossier/{kg}`, `/api/tiles/hillshade/{z}/{x}/{y}`,
`/api/cadastre/*` (umfeld non-cadastre paths only), `/api/licenses`.
Bbox layer proxies (`bboxProxy`, `bboxLayer`) quantise the bbox to ~150 m for the
cache key and cache only `ready` answers. **KG codes: compare with `unpadKG`**
(`/lookup` returns `3301`, cells carry `03301`).

### Pricing

`calculatePrice(areaSqm, landuse, buildingCount, totalBuildingArea)` (server.go),
mirrored by JS `calcPrice()` — keep in sync. Base €/m² per NS code in
`nsBasePrice` = `NS_TABLE.price` (Gebäude 0.5 … Wald 0.2 … Straße 0.1 …
Gewässer 0.05, Fels 0.03; unknown 0.15). Density: built-up ratio > 0.3 = 2×,
0.05–0.3 = 1–2×, none = 0.5×. Clamped 10–5000 coins.

## Real-Time (SSE)

`s.broadcast(sessionID, data)` → client `handleEvent(d)`: `parcel_claimed`,
`parcel_converted`, `parcel_sold`, `parcel_harvested` (`forest`, `meadow`,
`drought`), `ez_claimed`, `treasure_claimed`, `treasures_updated`, `well_dug`,
`player_joined`, `challenge_completed`, `offer_made|accepted|rejected|funds_needed`,
`chat`, `chat_mode`, `chat_hidden`, `chat_refresh`.

## EZ (Einlagezahl)

Land-register folio grouping parcels under one owner. `G.ezIndex["kg-EZnnn"]`
built by `buildEZIndex()` incrementally from loaded polygons; selecting shows
count/area/ownership, `drawEZHighlight()` pulses gold, bulk buy via
`/api/claim-ez`. Agent inspect reads the live bevdirect `/ez` for the folio block.

## Style Guide

Settlers IV pixel art; fonts `Press Start 2P` (headers) / `VT323` (body); CSS
`:root` palette (`--gold`, `--green`, `--panel`, `--bg`); all UI German (i18n
dictionary, `tr()`); `toast(msg,'ok'|'err')`; mobile bottom-sheet sidebar,
touch pan/pinch. Static assets `?v=` immutable 1 y — **bump `?v=` in index.html
whenever game.js/style.css change.** Gzip middleware level 5.

## Common Patterns

- **New feature**: migration → sqlc query → handler in `server.go` (register in
  `Serve()`) → game.js → index.html → build & restart.
- **New map layer**: server proxy via `bboxProxy`/`bboxLayer` (quantised cache,
  relay `ready:false`), client loader via `loadBboxLayer`/`loadPointLayer`
  called from `loadFastLayers(c)`, store in `G.x`, `drawX(ctx)` with
  `toScreen()`, insert in `render()` at the right z-order, `DEV.x()` helper.
- **New proxy**: `s.cachedFetch(w, key, fetch)` — singleflight, `X-Cache`,
  never caches errors/202; fetch closures use `context.Background()`.
- **Prefer bbox/cell endpoints over per-id loops**; quantise anything that fires
  on every pan or the cache never hits. Anything > 3 s per call is out.
- **New popup**: HTML in `#screen-game`, CSS, show/hide in game.js.

## Landscape, trees, buildings (srtm public tier)

- `/api/enhanced-kgs` → `{kgs:[{kg_code,kg_name,gemeinde_code,gemeinde_name,lon,lat,v2}]}`
  from srtm `/kgs` (v2 = grid25). Picker glows cyan on enhanced municipalities;
  lucky prefers them.
- `/api/lidar/kg/{code}` ("lidar slim") → `terrain, buildings[], top_trees[],
  top_objects[], product_version` built from `/kg/{code}`, `/buildings/bbox`,
  `/trees/bbox?min_height=25`, `/landmarks/bbox` (`splitBBox` for big KGs).
  Client `fetchEnhancedKG` → `G.lidarKGTerrain`, `addLidarBuilding` (centroid grid
  `G.lidarBuildingIdx`), `G.topTrees[kg]`, `G.topObjects[kg]`.
- `/api/trees` → `{trees:[{lon,lat,h_m,crown_d_m}]}` per cell → `loadTrees(c)` →
  `G.apexTrees`, `assignApexTrees(newPolys)` (PIP → `G.apexByParcel`);
  `drawApexTree()` at real position, apices ≥ 32 m join `G.topTrees['apex']`.
- `/api/buildings` → `{buildings:[{lon,lat,max_height_m,mean_height_m,stories_est,roof_type}]}`
  → `loadBuildings(c)`; `lidarForFootprint(f, ring)` is the single lookup
  (centroid grid, `findLidarBuilding`); storeys = `mean_height_m/2.9`.
- `/api/landmarks` → `{landmarks:[{type,lon,lat,height_m}]}` (landmark sprites).
- Relief: `drawRelief()` composites hillshade tiles via `/api/tiles/hillshade/…`
  (`srv/tiles.go`, 30 d cache, 204 = no data) `overlay`, deliberately faint
  (α 0.34 → 0.10); while active the per-parcel slope/aspect tint is skipped.
  `DEV.relief(bool)`.
- Per-parcel fill prefers `dom_terrain` (`DOM_TERRAIN`, `IMPERVIOUS_DOM` fallback)
  over cadastre landuse. Elevation tint ≥ z15, slope hatching ≥ z16.5.
- Giant trees: hidden until first treasure (`G.tallUnlocked`, `treasures_found`
  on `GET /api/player/{id}`), then golden hint trees (`G.tallRevealed`: 3 at
  zoom < 14, else 12); claiming a parcel with giant trees awards bonus XP
  (`tall_tree_count/max_h`, capped +300). Labels via `labelSlotFree()` +
  `flushTreeLabels()`.
- Context: `/api/parcel-context?pid&lon&lat&area_sqm&building_count&footprint_area_sqm&landuse_areas=52:917&kg=`
  → umfeld `/context` (24 h, pid only our cache key) → `data.{land_price, osm,
  natura2000, toponyms, legal, municipality}`; client `G.landPrices` lazy popup
  rows (`renderEnhancedPopupRows`, `#pp-enhanced`). `/api/osm-lines` → `G.osmLines`
  (water for every cell; roads/rail majors-only < z15). `/api/n2k` → `G.n2kSites`
  (hatched overlay, `#btn-n2k`). KG names: `kgName(kg)`/`ensureKGName(kg)` — never
  print a bare KG code.
- `/api/similar?parcel_id&lon&lat&area&bcount&barea&lu&limit` (`similar.go`):
  candidates from **our cached cells** around the point (`parcelsNear`), scored
  0..1 on size ratio, landuse-area composition, built density, terrain
  (`fracs` histogram); `source:"cells"`, cached 1 h (`similar:v6:`). Feeds agent
  inspect `similar` (top 8).

## Nature reserves (Naturschutz / Brache)

Converted parcels (`converted_to==='biodiversity'`) drawn by
`drawNatureReserves(ctx, claimMap)` above the base layer. `natureScene(f)` =
hash-stable scene (≤2600 samples, `vnoise` clumps, edge-distance succession
zones: Saum bramble/saplings/hawthorn, herb clumps, open patches; rare snags,
hive row, pond, nest boxes), items with priority `pr` drawn when `pr < frac` so
density is constant across zoom. Wind `windAt()`, `nBlade()`; pixel unit `u` =
1/2/3 by zoom via `wildPx()`. Fauna `drawNatureFauna` (butterflies, bees,
dragonfly, crow, swallows, hare). `natureAnimLevel()` 0 static / 1 phone 15 fps
/ 2 desktop 25 fps; `NATURE.quality` self-tunes at ~9 ms/frame; rAF only while
`NATURE.onScreen>0`. `drawSporadicHabitat()` gives ~14 % of ordinary parcels a
hive/nest box.

## Forest plots — Holzernte & Naturwald (`srv/timber.go`)

`claimIsForest` (NS 56 or tree cover ≥ 50 % on non-crop) swaps the generic buttons for:
- **🪓 Holzernte** `POST /api/harvest-forest`: coins = net timber value /
  `eurPerCoin` (10); stand regrows in real time (`forestPhase()`/`forestStage()`:
  Schlag <40 min → Jungwuchs <90 → Stangenholz <150 → Baumholz, value 50→100 %
  by 510 min). Quest "Holzknecht".
- **🌳 Naturwald** `POST /api/convert-parcel` `wildforest`: permanent, Baumholz
  only, XP `120 + min(180, Vfm/10)`, counts toward the 30 % bio target. Quest "Waldhüter".

Estimate `GET /api/forest-value?parcel_id&kg&area&lu&session_id&west&south&east&north`
(singleflight per parcel, cached 1 h / 10 min heuristic):
1. `standFacts`: cell row enrichment (`tree_frac`, elevation, slope, geometry) +
   srtm `/landscape?bbox&include=trees` (apex heights); no data → NS 56 defaults
   (85 % canopy, h 18 m). Species mix from elevation.
2. Stock heuristic `Vfm/ha ≈ 0.9·h_mean^1.95` × canopy ha; `Efm = 0.8·Vfm`;
   CO₂ ≈ 0.9 t/Vfm; assortments by height.
3. **HOLZ-3** `plotHistory()`: `POST holz /api/plot-context?fast=1` with the
   polygon, 2.5 s budget, `holzCold` backoff → `estimate.history`
   (`forest_share_2000_pct, loss_*, stock_factor, net_flux_tco2e_ha`).
4. Prices: `timberStatePrices()` from holz `/data/prices/state/{1-9}.json`
   (fallback: catalogue `timber:catalog` 24 h, then `fallbackPrices`); `kgState()`
   maps the KG prefix to the Bundesland.
5. Harvest cost 28/36/45 €/Efm by slope + 150 € fixed.
`handleHarvestForest` pays the cached popup estimate (`timber:<pid>`).

Rendering: `drawForestOverlay()` → `forestScene(f,'wild'|'schlag')` (old growth,
gaps, Waldmantel; stumps, slash, Holzpolter, Überhälter; regrowth items with
`birth`), species tables `F_SPECIES_LOW/MID/HIGH`, `fTree()` size classes; below
z15 plain sprites (`getTreeStyle`, `'young'`). Popup `forestPopupRows()`,
`G.forestValues[pid]`. `DEV.timber(pid, force)`.

## Fields, farms, water, Chronik (farm / gw / holz siblings)

- **FARM-2** `/api/schlaege` → `loadSchlaege(c)`: INVEKOS fields (`crop_group`,
  `snar_name`, `area_ha`, `organic`); `parcelSchlag(p)`, `fieldKindFor()` maps
  `CROP_GROUPS` → field kind; popup `#pp-crop`; `doHarvest()` sends `crop_group`
  (server `fieldPhaseAtCrop`, `cropMeadow`). Field cycle 60 min (`FIELD_CYCLE_S`).
- **FARM-4** `/api/hofstellen` → `drawHofstellen()` (tractor + bales, zoom ≥ 15),
  `hofOnParcel(pid)`.
- `srv/dossier.go`: `GET /api/dossier/{kg}` = gw + holz + farm `/llm/kg` in
  parallel (6 h cache, 404 negative 1 h) → `drought{level,label,status,sigma}`,
  `game{yield_factor,well_protection,subsidy_per_ha}`. Client: HUD chip
  `#water-chip` (`updateWaterChip`, `currentWaterKG()`), Chronik panel
  `#dossier-popup` (`openDossier(kg, tab)`, `pxChart()`, `segBar()`), sidebar `#sb-chronik`.
- `srv/water.go`: `/api/water/point|points|station/{id}|protection|flowpath|gwi|parcel/{pid}`,
  `/api/well-quote`, `POST /api/dig-well`, `/api/field-economy`. Harvest payout =
  crop × drought factor (+ well protection) + Förderung; Naturschutz ×1.5 XP in
  a Wasserschutzgebiet; +80 XP Pegelwart when `gw_station` on claim.
- Client: Brunnen (`wellButtonHTML`, `doDigWell`, `drawWells` z≥16), Messstellen
  (`loadGwPoints` → `drawGwStations` z≥15, `openStation`, `stationOnParcel`),
  Wasserschutzgebiete (`loadWaterProtection` → `drawWaterProtection`, gated by
  `G.n2kVisible`), field economy (`fieldEconomy(pid)` → `#pp-eco`, `harvestToast`).
- **Wassertropfen-Reise**: `startFlow(lon,lat)` → `G.flow`; MERIT path snapped
  onto drawn OSM watercourses (`refineFlow`, `riverChains`, `snapToRiver` ≤ 250 m);
  `flowAnimLoop()` moves the camera (20 s + 1.5 s/km, ≤150 s, zoom 15),
  `#flow-chip`, `drawFlowPath()`, `clearFlow()`. `DEV.flow(lon,lat)`, `DEV.flowInfo()`.
- Picker tint (GW-7): `loadPickerGwi(munis)` = `/api/water/gwi` joined with
  the admin table → `gwiTint()` in `drawMuniPoly()`.
- Popup rows appended in `waterPopupRows(pid, rows)`. DEV: `DEV.dossier`,
  `DEV.station`, `DEV.water()`. Glitch log `docs/glitches.md`.

## Quests → Herald

`GET /api/session/{id}/challenges` returns live `progress/goal`
(`questProgressFor`, same counters as `autoCompleteChallenges`). Tapping a quest
→ `Herald.brief(id)` → `questBriefing(c)` 2-line briefing + gold action button
`#herald-act` → `questPing(lon,lat,zoom)` (`drawQuestPing`) and opens the popup.
QA: `await DEV.quest()`, `DEV.quest(id|title)`, `DEV.quest(id, true)`.

## Agent surface

- `GET /` serves `/llm/game` markdown to agent UAs (`agentUASubstrings`) and
  `Accept: text/markdown`; `Link: rel=alternate|help|service-desc` headers;
  `/llms.txt`, `/openapi.json` (hand-written 3.1 — **keep in sync** when agent
  endpoints change), `/agents` + `/api/agents/leaderboard` (protected ha per
  `players.agent`, 60 s cache).
- `GET /api/agent/look`, `/api/agent/municipality`, `POST /api/agent/claim`
  (`agentSeen` gate). `fetchAgentParcels` reads cached cells (`parcelsNear`),
  applies the `landuse` filter locally via `dominantNS()`.
- `GET /api/agent/inspect?session_id&player_id&parcel_id[&lon&lat]`
  (`agent_inspect.go`): parallel fan-out under `inspectBudget` 4.5 s — parcel row
  from the cell (`lookupParcel`), folio via bevdirect `/ez`, umfeld `/context`,
  srtm `/landscape` + `/buildings/bbox`, farm Schläge/Hofstellen, gw point +
  stations + well quote, Chronik, cached timber, similar. Blocks past budget →
  `{pending:true}` in `pending[]`; blocks that no longer exist upstream are
  listed in `missing`. `game.actions[]`, `text` (`inspectNarrate`). Cold cell →
  202 pending; breaker open → 503 down.
- `/llm/ahead` (`llmahead.go`, token `X-Ahead-Token` from `SIEDLER_AHEAD_TOKEN` /
  `./ahead.key`, else 404): roadmap for the sibling services + conformance
  harness `/llm/ahead/check/{service}`. Checkbox = consumed by us (`aheadUsed`
  map — **add an entry when you start consuming an item**); items marked GONE
  target endpoints that no longer exist on the public tiers. Never link it from
  public docs.

## DEV helpers / QA (`window.DEV`)

Rejoin directly: `/?lang=de&dev=1&pid=<id>&pname=<name>&rejoin=<token>&sid=<session>`
(`dev=1` skips the loading dwell; `#v=lon,lat,zoom` sets the camera).

```js
await DEV.goto(15.5205, 48.3955, 17.5)   // camera + load cells + wait idle
await DEV.parcel('12105-68/3')           // select + popup
DEV.ez('12105', 430); DEV.kg('12105'); DEV.tree(0); DEV.trees('locked'|'hint'|'revealed')
DEV.cells(); DEV.warm(); DEV.lucky(); DEV.timing(); DEV.upstreams()
DEV.chrome(false); DEV.sidebar(false); DEV.freeze(); DEV.closeAll(); DEV.state()
DEV.apex(pid); DEV.bldg(); DEV.hof(); DEV.kgName(kg); DEV.relief(); DEV.quest('Schatzsucher', true)
```

Hi-res captures: `emulate_custom` DPR 2 (1920×1080) or `emulate_device` phone.
Multi-tab QA: register a 2nd player via `POST /api/register`, join with
`invite_code`, drive with curl (`X-Player-Token`); `curl -N
/api/session/{id}/events` shows SSE. Outage drill: stop `bevdirect-serve` —
breaker opens after 3 failures, chip counts down, map refills on restart.
Gotcha: `DEV.treasure(id)` / `DEV.parcel(id)` need the exact id. Screenshot
recipes: `docs/screenshot-recipes.md`; glitch log `docs/glitches.md`.

## Cross-browser QA harness (`tools/xbrowser/`)

`tools/xb.sh` walks every app state (welcome, invite, picker, loading, all
zooms, every popup, EZ, Chronik, Herald, treasures, giant trees locked/hint/
fog/revealed, Naturschutz/Naturwald/Schlag/field mocks, station, flow, similar,
search, chat, rules, mobile sheet, chrome off, GPS) in Chromium, Firefox and
WebKit (Safari engine), desktop 1440×900 + phone 390×844 @2x. Flags JS errors,
missing fonts, clipped text, off-screen chrome, dead canvas animations and
pixel Δ vs Chromium. Scenes live in `tools/xbrowser/scenes.mjs` — add one per
new feature. Report: `tools/xbrowser/out/report.html`, served by systemd
`xb-report` at `https://siedler-oesterreich.exe.xyz:8765/report.html`.
`tools/xb.sh --quick` for webkit+chromium core scenes; `--engines= --form= --only=`.
