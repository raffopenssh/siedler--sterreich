# Cadastre cells — `/api/viewport`, cell store, warming, lucky

The **0.02° aligned cell** is the unit of everything cadastre-shaped: bevdirect assembles it, we cache it
(≤ 24 h), the warm planner fills it, lucky picks from it, agent look-ups read it, the contrib stash keeps the
raw document for the nightly NE report. Key `vp:v1:i:j` with `i=floor(lon/0.02)`, `j=floor(lat/0.02)`
(Go `cellOf/cellsForBBox` in upstreams.go, JS `cellBox/cellOf/gridTiles`).

## `/api/viewport` (`srv/viewport.go`)

- Request `west,south,east,north` **grid-aligned** (`i*0.02 … (i+1)*0.02`); span ≤ 0.05°. Non-aligned bboxes
  get a ~150 m quantised key and are not prewarmable.
- Response `{parcels[], footprints[], landuse[], kgs[], ready, truncated, retry_after_s?, notice, cell, source,
  ne{ready, partial, epoch, kgs_missing, kgs_pending, parcels}}`.
  - Parcel row = bevdirect row (`parcel_id, kg_code, gnr, ez, area_sqm, lon, lat, complete, parts, dominant_ns,
    landuse_areas{ns:m²}, building_count, total_building_area_sqm, geometry`) + `ph`/`ezh` hashes
    (`addParcelHashes` at serve time) + terrain enrichment where a grid25 heightfield exists (`elev_m,
    elev_min_m, elev_max_m, slope_deg, aspect, dom_terrain, fracs, tree_frac`; `enrichParcel`) + `ne{}` for
    NE-ready KGs (see [ne.md](ne.md); then `fracs`/`dom_terrain` come from the 9 NE groups; parcels with < 30 %
    of their expected NE cells fall back to the heightfield).
  - Footprint: `footprint_id, parcel_id, ns_code, area_sqm, obb_length_m, obb_width_m, orientation_deg,
    geometry` (+ `ne{h_max_m, h_robust_m, stories_est, type, area_m2, dh_m}`). Landuse: `{code, area_sqm,
    geometry}`. `kgs[]` carries names + `enhanced` + `ne` flags.
- `buildCell` fetches bevdirect (`wait=10`), the NE cell and the srtm heightfield in parallel; caches **only
  when ready**, 24 h. `handleViewport` → `cachedFetchX` (singleflight `s.sf`, `X-Cache: HIT|MISS|MISS-SHARED|WARM`)
  + hot LRU of pre-gzipped cells (`hotCellGet/Put`, 80). Foreground builds make the warm loop yield
  (`warm.fg`). A ready build triggers `noteCellLoaded` (neighbour warming) and `neObserved` notes.
- `/api/viewport-landuse` = same cell build, landuse array only.
- 200 bodies get `Cache-Control: private, max-age=3600` (`browserCache`, skipped for `ready:false`).

## Cell store (`srv/cellstore.go`) — all server-side parcel look-ups

`cachedCell` (no fetch), `ensureCell`/`ensureCellStatus` (blocking, singleflight; 200/202/5xx + retry hint),
`parcelsNear(lon,lat,radiusM,maxCells)`, `lookupParcel(pid,lon,lat)` (cell first, then bevdirect `/parcel/{id}`,
24 h), `footprintsOf`, `landuseShares`, `parcelBBox`. Agent endpoints relay a cold cell as
`202 {status:"pending", retry_after_s}` + `Retry-After` (`relayCellStatus`).

Known limitation (deliberate): a cached cell can disagree with a live bevdirect `/ez`/`/parcel` answer
(e.g. `building_count`). Re-fetching on mismatch would defeat the cache that keeps BEV tile load low.

## Privacy — no cadastre ids in the DB

Claims/offers/harvests/quest targets are keyed by HMAC `parcel_hash`/`ez_hash` (`srv/parcelhash.go`); the
client maps them back via `resolveClaims()` from loaded geometry (`G.pidByHash`, `G.ezByHash`).

## Client loading (`game.js`)

1. `POST /api/session/create` never touches the cadastre (~20–100 ms; enqueues the Gemeinde's KGs for warming;
   treasures placed in goroutines → SSE `treasures_updated`).
2. Loading steps: Session → Landschaft (`/api/lidar/kg`, hillshade, trees) → Kataster
   (`fetchKGPolygonsBlocking(6000)`: fast layers first, then only the **centre cell** blocks, ≤ 6 s; subline "aus
   Zwischenspeicher" vs "live aus BEV-Kacheln") → Arten & Schätze → Bereit. The game opens even if the centre
   cell is still assembling.
3. On pan/zoom: `loadMoreParcels()` → `fetchKGPolygons()` → `gridTiles(paddedView, ≤ 12)` (nearest camera
   first, `tilesInAustria`) → `runPool(…, 4)` → `loadTileResilient(cell)` → `loadViewportGeometry(cell)`; then
   `buildEZIndex()`, `loadEnhancedForKGs()`. `loadFastLayers(c)` per cell in parallel (OSM lines, trees,
   buildings, N2K, Schläge, Hofstellen, GW points, Wasserschutz). During a Wassertropfen-Reise only 2 cells load.
- `loadViewportGeometry` dedups by cell key + ids (`G.vpTiles/polyIds/fpIds/luIds`), returns `{added, ready,
  truncated, retryAfter, cached}`, un-marks `G.vpTiles` when not ready. `loadTileResilient` re-polls
  `ready:false` up to 10× paced by `retry_after_s` (≤ 12 s), 4× when the breaker says `down`, only while the
  cell is still near the screen. Truncated cells are accepted as-is. `G.cellState` per cell → `DEV.cells()`.

## Warming (`srv/warm.go`, `warmactivity.go`, `warmboost.go`, `warmspread.go`)

`kg_warm` table (`kg_code, warmed_at, expires_at=+24 h, cells, parcels, reason`). Single worker, ~0.8 s between
cells, yields to foreground, skips KGs fresh ≥ 2 h. Queue is in-memory (a restart empties it).

- **Daily plan**: ~100 KGs/day in 20 patches of ≥ 5 KGs: a Gemeinde whose KGs are *all* srtm-enhanced — **any
  size, all of its KGs** (`warmSeedMaxCells` 600 is a sanity guard only; Sölden = 322 cells) + nearest neighbour
  KGs up to 5, one patch per 24h/20, ≤ 4 per Bundesland; persisted as `warm-plan:v4:<date>` in api_cache so
  restarts resume. v2.4 Gemeinden are seeded first (no state quota).
- **Neighbour**: first foreground build of a cell enqueues the KGs touching it + adjacent KGs (low prio,
  debounced 1 h per cell). **Session**: `POST /api/session/create` → `warmGemeinde` (medium prio).
- **Activity tiers**: `playerSeen()` (viewport, session create/join, agent look; seeded at startup from the
  newest session/claim/chat row) → `warmTier()`. **idle** (no player 24 h) runs every 5th daily patch only;
  **active** runs the full plan and every 2 h re-queues the v2.4 KGs of Gemeinden played in the last 7 days
  (≤ 40). `/api/warm/status` → `policy.tier`, `last_player`.
- **v2.4 adoption**: `neAdoptKGs` (on every registry refresh, once per KG generation `ne-adopt:v1:<kg>`) purges
  `vp:v1`/`ne:`/`trees:ne`/`buildings:ne`/`neheat` caches + `kg_warm` + `ne-obs` notes; it enqueues warming
  solely for KGs in today's plan while active.
- **Boost focus**: 2nd line of `WARM_BOOST` `focus=<Gemeinde|code|lon,lat>,<km>` queues every v2.4 KG within the
  radius (prio 1, nearest first) at startup, every 2 h and on `run-plan`; `policy.boost_focus{}` in status.
  Wien+15 km = 73 KGs / 218 cells (~40 min).
- Ops: `POST /api/warm/run-plan?reset=1` re-arms today's plan after a restart; `?kg=NNNNN` (ahead token) = one
  contrib job; `POST /api/warm/trim?reason=daily|max=N` drops queued jobs. `GET /api/warm/status`.

## Lucky (`/api/lucky`, `luckycluster.go`, `luckyinterest.go`, `neobserved.go`)

`GET /api/lucky` → `{gemeinde_code, name, lon, lat, enhanced, warm, kgs[], ne, interest, interest_why}`: only
Gemeinden whose KGs are warm (not expiring within 2 h) **and** srtm grid25; the spawn KG itself must be
enhanced. Picks a v2.4 destination ~2 of 3 times. Logged `lucky: pick`.

- **NE confirmed, not flagged**: the registry can flag a KG v2.4 hours before `/cells` serves it. Every ready
  cell build notes `kgs[].ne` per KG (`ne-obs:v1:<kg>` = 1/0, 24 h, positive wins; `neObservedSeed` scans cached
  cells at startup). `neConfirmedKGSet()` = registry v2.4 minus explicit 0 — lucky uses only this: with
  ≥ `luckyV24Min` 10 warm confirmed KGs the playable set is confirmed-only, and `spawnNEOK` rejects a spawn whose
  cached cell says the spawn KG has no NE.
- **Interest**: `luckyInterest(lon,lat)` scores ~350 m around a point 0..1 from cached cells only (land-use
  entropy, forest edge, water, roofs, Δh/slope, NE tall trees; ≥ 85 % one group = monotony ×0.4); `luckySpot`
  scans a 7×7 grid (~270 m steps). Cluster draws < `interestGood` 0.45 are redrawn (≤ 6), per-Gemeinde picks
  < 0.25; best dull one is the fallback. `GET /api/lucky?lon&lat` scores any spot (QA). Flat field ≈ 0.1,
  village edge + brook + wood ≈ 0.8.
- Centre = OSM settlement (`settlementCenter`, memoised; `prememoSettlements` kicks it for picker hits so
  session create never waits on umfeld).

## Search (`srv/search.go`, `createSearchBox` in game.js)

`GET /api/search-index` = whole admin table (~160 KB gz, ETag, 1 d) indexed in the browser (`SIDX`,
`snorm`/`sqnorm` diacritics-free, "St." → "Sankt") → Gemeinde/KG/code rows render synchronously per keystroke;
umfeld `/lookup` (120 ms) and `/search/address_osm` + `/toponyms/search` (220 ms; Nominatim 1 req/s upstream,
proxy caches 24 h) merge in under a striped progress bar. In-game the address query also goes out with the
current Gemeinde appended. Parcel intents `68/3`, `.68`, `12105-68/3`, `Dürnstein 68/3` → loaded polygons first
(`DEV.find`), else `GET /api/parcel-find?kg&gnr&lon&lat` (cached cells of the nearest KG first, cold cells built
within 8 s, then `202 progress{searched,total}`; ≤ 40 cells). Picker picks set `G._muniHint` and
`G.pendingSelect`. Recent picks in `localStorage siedler_recent_search`. xbrowser scene `search-parcel`.
