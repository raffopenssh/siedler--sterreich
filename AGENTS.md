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

bevdirect-serve: public repo `github.com/raffopenssh/vtcseamless` (MIT, preset `bevdirect/`), currently **v0.3.3** (release binary, installed 2026-10-08; empty/unrequested layers are `[]` instead of `null` — the frozen `ne_cells/canon.py` crashed on null; no geometry change, same comparable tag class as v0.3.2/v0.3.1, cell output unchanged since v0.2.1 — registered as `bev_equivalent_tags` on the NE server, no re-baselining). **Aligned cells only for NE builds:** a multi-cell `/viewport` keeps one truncated copy of parcels wider than cell + pad (0.028°), so a stitched viewport never equals the operator's aligned-cell build (vtcseamless DEPLOY.md v0.3.3); `tools/ne-report` fetches one `/viewport` per aligned cell and passes the union of the cells as `--input-bbox`. Since v0.3.1 BEV tiles live **only in RAM** (`-tile-cache-mb 1024` LRU, `-tile-ttl 24h`; no `.pbf` on disk, no `bevcache/` dir, no prune timer; `-cache` is ignored and dropped). `/health` → `tile_cache{tiles,bytes,max_bytes,hits,misses}`, relayed in `/api/metrics` → `bevdirect`; `bevdirect.version_mismatch` flags drift between the running binary and `bevdirectVersion`. Upgrade: `curl -fsSL https://raw.githubusercontent.com/raffopenssh/vtcseamless/main/bootstrap.sh | PREFIX=/opt/bevdirect PORT=8787 VERSION=vX.Y.Z sudo -E bash` — **install.sh overwrites the unit** (sets `User=root`, `-cells 160`, prefetch on) and does *not* restart a running instance: restore `User=exedev`, `-cells 120 -prefetch 0` (keep `MemoryMax=3G`, `GOMEMLIMIT=2560MiB`), `daemon-reload`, restart (`SOURCE.txt` in /opt/bevdirect). Bump `bevdirectVersion` in `srv/licenses.go` and the version in `impressum.html`/`imprint.html` on upgrade. Never persist raw tiles on our side — only assembled `/viewport` docs (api_cache cells, `tools/ne-report` work dir) may be cached, **and nothing cadastre-shaped may sit on disk > 24 h**: `run.sh` prunes `data/ne-reports/work/` (> 24 h) on every run; when you dump a `/viewport`, `/parcel` or `/ez` response for debugging (curl → `/tmp/*.json`, `--keep-cells`, test fixtures), **delete it before you finish the task** — never commit one. Audit: `find / -xdev -name "*.pbf"`, `ls data/ne-reports/work`, `grep -l footprints /tmp/*.json`, `sudo ls -l /proc/$(pidof bevdirect-serve)/fd` (should show no regular files). Legal pages (impressum/imprint/datenschutz/privacy, `/api/licenses` policy) state “tiles RAM-only, ≤ 24 h” — keep them true. Library import path `github.com/raffopenssh/vtcseamless/bevdirect` (not embedded yet).
Live: `https://siedler-oesterreich.exe.xyz:8000/`. DB `./db.sqlite3`. Service
`/etc/systemd/system/srv.service`. Maintenance mode: `touch MAINTENANCE`
(bypass cookie `siedler_dev=1` / `?dev=1`). Provider change log:
`docs/migration-2026-10.md` (the only place that documents past providers —
code, comments and docs elsewhere describe the current setup only).

## Layout

```
cmd/srv/main.go          entrypoint, flags
srv/server.go            routes (Serve()), game logic, SSE, cachedFetch, generic proxies
srv/viewport.go          /api/viewport: one 0.02° cadastre cell (bevdirect) + srtm terrain enrichment
srv/cellstore.go         read-side helpers over cached cells (parcelsNear, lookupParcel, ensureCell…)
srv/upstreams.go         provider base URLs, 0.02° grid helpers (cellOf/cellsForBBox), embedded admin.json.gz
srv/warm.go              cell prewarming (daily plan, neighbour, session), /api/lucky, /api/warm/status
srv/contrib.go           quarterly rotation of v2.4 KGs for the nightly NE epoch report (/api/contrib/plan)
srv/necells.go           NE cells (srtm v2.4 observed layer): fetch per cell, parcel/footprint enrichment, NE trees/buildings, /api/ne, v2.4 adoption
srv/landscape.go         srtm public-tier adapters (/api/enhanced-kgs, /api/lidar/kg, trees/buildings/landmarks,
                         /api/landscape, /api/parcel-context, /api/osm-lines, /api/n2k, /api/municipality…)
srv/siblings.go          farm/holz layer proxies (bboxProxy, hostSlots), prewarmKGs, kgsAlongPath
srv/water.go, dossier.go groundwater mechanics, Gemeinde-Chronik dossiers
srv/timber.go            forest value / harvest; similar.go similar parcels; treasures.go
srv/agent.go, agent_inspect.go, discover.go, leaderboard.go, llmahead.go  agent surface
srv/breaker.go, upstream_pending.go, metrics.go, parcelhash.go, licenses.go, tiles.go, cachestore.go
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
| **context** around a point | **umfeld-at** `https://umfeld-at.exe.xyz/api/v1` (`umfeldAPI`), ≤ 5 req/s | `/context` (land price, OSM distances, N2K, toponyms, RIS legal), `/search/municipalities`, `/lookup`, `/search/address_osm`, `/osm/geometry?bbox` (bbox only), `/natura2000/*`, `/toponyms/*`, `/legal/*`, `/land_prices/point…`. Statistik Austria, EEA, OSM, BEV DLM names, RIS. |
| **observed layer (NE cells)** — primary parcel enrichment | **srtm-lidar-at** `/api/v1/cells?bbox&format=columns&centres=1&layers=obs,trees,structures` (`srv/necells.go`), declared twin **umfeld-at** `/api/v1/ne/{kg}`, `/ne/manifest` | H3 res-12 cells (~307 m²) for KGs with product **v2.4**: cover[9 groups], canopy, LiDAR heights, NDVI, change, terrain, `consistency` (observed vs declared; parcel `verdict` only flags *big* changes: land-use ≥ 35 % & ≥ 3 cells, built ≥ 15 % & ≥ 2 cells — `vp:v1` cells must be purged when the rule changes), **every tree apex ≥ 3 m** (species, vitality) and **every structure** (type, heights). One fetch per 0.02° cell (`ne:v1:i:j`, 24 h, hot LRU 24 parsed); 404 = KG not processed (negative 1 h, `retry_after_s`), `meta.partial` for mixed cells. `neEnrichParcel` fills the legacy terrain fields **and** `ne{}` (`neParcel`); heightfield only where no NE. We report cadastre epochs back (`tools/ne-report`, `docs/ne-report.md`; POST goes to umfeld `/contrib/api/v1/ne/{kg}/report`, token `ne-peer.key`; **change protocol** since vtcseamless-py 0.2.0: reports carry `chunks_ap`, answered `want_chunks[]` are uploaded as stripped statistics rows to `…/ne/{kg}/chunks?observer=` from RAM — every chunk once, then only changed ones; `meta.chunks.stored`). |
| **landscape** | **srtm-lidar-at** `https://srtm-lidar-at.exe.xyz/api/v1` (`lidarAPI`, public tier: bbox/point/KG-code keyed only) | `/landscape?bbox`, `/trees/bbox`, `/buildings/bbox`, `/landmarks/bbox`, `/heightfield?bbox&cell=25&landcover=1` (404 where no grid25), `/kgs`, `/kg/{code}`, `/tiles/hillshade/{z}/{x}/{y}.png`. **No parcel or footprint ids** — the client assigns by point-in-parcel / centroid grid. |
| **admin table** | `srv/data/admin.json.gz` (embedded; BEV VGD 1:50 000, CC BY 4.0) | all 7 850 KGs: code, name, Gemeinde, district, state, bbox, area. Drives `/api/kg-geo/{kg}`, KG neighbours, the warm plan, `/api/lucky`, `kgsAlongPath`. |
| **KG universe check** (`srv/kguniverse.go`) | umfeld `GET /api/v1/kgs` (one gzipped list, ETag/`X-KG-Universe-Hash`, 304 on If-None-Match) | **Contract: `kg_count = 7850`, `universe_hash = sha256(join(sorted(kg_code),"\n"))` hex.** `kgUniverseBoot()` (sync in `Serve`) computes our hash from the admin table + loads the mirror `data/kg-universe.json.gz` (seed `data/kg-universe.seed.json.gz` committed); `kgUniverseInit` fetches at startup and revalidates hourly. umfeld ↔ ours ↔ srtm (`noteSrtmUniverseHash`) must agree, else `slog.Error`, one e-mail to the owner per fingerprint (`sendOwnerMail`, `SIEDLER_ALERT_MAIL`), `kg_universe.alert` in `/api/metrics` + `/api/warm/status`, and `kgUniverseOK()` pauses the daily warm plan + `neAdoptKGs`. KG codes are always 5-digit strings with leading zero. |
| **srtm KG registry** (`srv/kgregistry.go`) | hourly: srtm `/api/v1/kgs?fields=codes&processed_only=0` + `/llm/manifest.json`, both with `If-None-Match` (304 = unchanged); the full rows `/kgs?processed_only=0&limit=5000` (2 pages = all 7 850) only when the universe ETag or the manifest ETag moved | mirrored to `data/srtm-kgs.json.gz` (seed `data/srtm-kgs.seed.json.gz`), used when srtm is down (`source:"local-copy"`, 5 min TTL instead of 30). `/api/enhanced-kgs` (`enhanced-kgs:v5`) keeps `kgs[]` = **full v2.4 rows only** (`v2.4-partial`/v2.3/grid25-only are *not* enhanced; counts of those in `other{}`) — this is what the picker glows on, the "Enhanced Gelände" chip, `/api/lucky` and the daily warm plan key on and adds `kg_count, universe_hash, registry_hash, source, fetched_at, upstream_etag`; weak `ETag` = registry_hash, `X-Registry-Source`. Hashes are recomputed on read, never trusted from disk. Only a live answer purges derived caches / adopts v2.4. |
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
G.terrainParcels[pid]  // per-parcel terrain from the cell row (elev, slope, aspect, dom, fracs)
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
- Parcel rows of NE-ready KGs also carry `ne{cells, cover{geb,bau,acker,gruen,wald,wasser,verkehr,alpen,sonst}, canopy, h_max_m, tree_n, tree_h_max_m, trees_tall, species{}, vitality{}, structures_n, structures_cover, structure_h_max_m, structure_types{}, consistency{code:share}, verdict, phenology, ndvi, dh_m, forest_loss_year, als_years, epoch}`; footprints `ne{h_max_m, h_robust_m, stories_est, type, area_m2, dh_m}`; `kgs[].ne`, top-level `ne{ready, partial, epoch, kgs_missing, parcels}`. `fracs`/`dom_terrain` then come from the 9 groups (`neGroupLC` → roof/parking/crop/grass/tree/water/road/rock/bare_soil). Parcels with < 30 % of their expected cells (mostly in an unprocessed neighbour KG) fall back to the heightfield.
- `buildCell` fetches bevdirect (`wait=10`), the NE cell and the srtm heightfield in parallel;
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
- `/api/umfeld/*` forwards only `umfeldPublicPrefixes` (context paths); anything
  else is 404 — parcel data exists solely as cached cells via `/api/viewport`.

### Cell store (`srv/cellstore.go`)

All parcel look-ups read our cached cells (≤ 24 h):
`cachedCell` (no fetch), `ensureCell`/`ensureCellStatus` (blocking, singleflight;
200/202/5xx + retry hint), `parcelsNear(lon,lat,radiusM,maxCells)`,
`lookupParcel(pid,lon,lat)` (cell first, then bevdirect `/parcel/{id}`, 24 h),
`footprintsOf`, `landuseShares`, `parcelBBox`. Agent endpoints relay a cold cell
as `202 {status:"pending", retry_after_s}` + `Retry-After` (`relayCellStatus`).

### Warming (`srv/warm.go`)

`kg_warm` table (`kg_code, warmed_at, expires_at=+24 h, cells, parcels, reason`).
Single worker, ~0.8 s between cells, yields to foreground, skips KGs fresh ≥ 2 h.
- **Daily plan**: ~100 KGs/day in 20 patches of ≥ 5 KGs: a Gemeinde whose
  KGs are *all* srtm-enhanced (grid25) — **any size, all of its KGs** (a Gemeinde
  is a lucky destination only when warm as a whole; `warmSeedMaxCells` 600 is a
  sanity guard like ne-report's `--max-cells`, Sölden = 322 cells, never
  size-exclude) + nearest neighbour KGs (any) up to 5, one patch per 24h/20,
  ≤ 4 per Bundesland; persisted as `warm-plan:v4:<date>` in api_cache so restarts resume.
- **Neighbour**: first foreground build of a cell enqueues the KGs touching it +
  adjacent KGs (low prio, debounced 1 h per cell).
- **v2.4 first**: `neReadyKGSet()` (registry `v24` flag = srtm `product_version` v2.4) — the daily plan seeds Gemeinden with NE KGs first (no state quota), `neAdoptKGs` (on every registry refresh, once per KG generation `ne-adopt:v1:<kg>`) purges their `vp:v1`/`ne:`/`trees:ne`/`buildings:ne`/`neheat` caches + `kg_warm` (warming only for planned KGs, see activity tiers). `/api/warm/status` → `v24_kgs[]`, `v24_warm`. `/api/lucky` picks a v2.4 destination ~2 of 3 times (`ne:true`).
- **Activity tiers** (`srv/warmactivity.go`): `playerSeen()` (viewport, session create/join, agent look; seeded at startup from the newest session/claim/chat row) → `warmTier()`. **idle** (no player for 24 h) runs only every 5th daily patch (4 destinations/day) and keeps nothing else warm; **active** runs the full plan and every 2 h re-queues the v2.4 KGs of Gemeinden played in the last 7 days (≤ 40). `neAdoptKGs` only *purges*; it enqueues warming solely for KGs in today's plan while active. Never keep the whole v2.4 universe warm (1 300+ KGs → GBs/day of BEV tiles + NE docs with nobody playing — that was the 72-min RX spike). `/api/warm/status` → `policy.tier`, `last_player`.
- **Boost focus** (`srv/warmboost.go`): 2nd line of `WARM_BOOST` `focus=<Gemeinde name|code|lon,lat>,<km>`
  (e.g. `focus=Wien,30`) — while boosted, *in addition to* the boost plan, every v2.4 KG whose centre lies within
  the radius is queued (prio 1 → ahead of daily patches, nearest first, fresh skipped) at startup, every 2 h and
  on `run-plan`; `/api/warm/status` → `policy.boost_focus{label, v24_kgs, warm, cells}`. Wien+15 km = 73 KGs / 218 distinct cells (~40 min); 30 km would be ~960 cells. `POST /api/warm/run-plan?reset=1` re-arms today's plan after a restart (clears `started`, queues only the patches already due; the rest fire on their slots). `POST /api/warm/trim?reason=daily|max=N` drops queued jobs (queue is in-memory — a restart empties it; started plan patches are not re-queued).
- **Session**: `POST /api/session/create` → `warmGemeinde` (medium prio);
  `prewarmKGs` (siblings.go) adds KGs along a water flowpath.
- **Interest** (`srv/luckyinterest.go`): the spawn point inside a chosen cluster/Gemeinde is nudged to the most
  *visually interesting* spot — `luckyInterest(lon,lat)` scores the ~350 m around a point 0..1 from cached cells only
  (land-use variety entropy, forest edge, water, roofs, Δh/slope, NE tall trees; ≥ 85 % one group = monotony ×0.4);
  `luckySpot` scans a 7×7 grid (~270 m steps) keeping the KG playable and the cluster ≥ n−1. Cluster draws
  < `interestGood` 0.45 are redrawn (≤ 6), per-Gemeinde picks < 0.25; the best dull one is the fallback.
  Answer carries `interest`, `interest_why`; `GET /api/lucky?lon&lat` scores any spot (QA). Flat field ≈ 0.1, village edge + brook + wood ≈ 0.8.
- **Contrib rotation** (`srv/contrib.go`, separate from warming): `GET /api/contrib/plan` = today's
  v2.4 KGs for the nightly NE epoch report (`tools/ne-report/run.sh` → umfeld `/contrib`). Each KG gets
  a day of the quarter by `hash(quarter, kg)` → the whole universe (1 400 now, ~4 000 soon) is reported at
  least once a quarter; nights are filled to ≥ `contribNightMin` 40 KGs with not-yet-reported KGs due later
  (ahead of schedule, `fill[]`/`ahead_days`); **KGs the prewarmer built < 24 h ago go first** (`cheap[]`, BEV
  tiles still on bevdirect's disk → CPU only, **uncapped** — everything warm & unreported is reported that night);
  unreported KGs of the last 2 nights are caught up; cap 120 applies only to KGs needing fresh tiles. Unit timeout 8 h.
  Reports are built from **aligned cells only** with `--input-bbox` = the union of those cells (stable bbox per cell block → the operator's coverage rule `coverage{bbox, cells_n, best_cells_n, status}` can compare them; `change_suspect:"coverage_lossy"` = lossy build, its diffs are never surfaced — `meta.change.surfaced`), KGs whose umfeld domain contains no whole aligned cell are skipped (`--min-full-cells 1`, ~55 % of KGs; `0` = report every KG). Verified 2026-10-06 (first real run — until then a silenced SyntaxError in run.sh made every night fall back to the
  3-KG sample): 242 KGs in 88 min, 241 ok / 1 skipped, 241× POST 200, ~13 s build + ~4 s fetch per KG. Recipe for a big
  night: `POST /api/warm/run-plan` in the afternoon → everything lands in `cheap[]`. The plan answers `source:"none"` for
  seconds during an srtm registry full refresh; run.sh retries 4× 30 s before using the fallback sample.
  `reported_quarter`/`left_quarter` come from the `data/ne-reports/KG.<date>.json` files. Reads bevdirect directly, writes
  no `kg_warm`/cells → `/api/lucky` unaffected. `/api/warm/status` → `contrib{}`. **Never feed `v24_kgs` to run.sh.** Public counter `GET /api/contrib/stats` (`srv/contrib_stats.go`: latest report per KG → kgs, Σ cells_n, Σ KG km² from the admin table, universe, 10 min cache) feeds the „Beitrag zum Nutzungsmonitoring“ callout on impressum/imprint (`static/contrib-stats.js`, `.legal-callout`/`.legal-stats` in legal.css).
- **`ne_ready` is the v2.4 truth** (srtm afe147d, 2026-10-07): registry rows carry `ne_ready` (bool, authoritative),
  `ne_status` (served|partial|pending|not_processed), `ne_cells_published_at`; `product_version` reads
  `v2.4-pending` until the cells are ingested (metered, hours; ~365 pending now, self-promoting), raw value in
  `product_version_registry`. `kgNEServed()` (landscape.go) = `ne_ready` when present (old v1/v2.3 rows with
  `ne_ready:true` are real), else the product string (pre-afe147d mirror). `registry_hash` folds `ne_ready|ne_status`
  in; `updated_at` is bumped on publication so `neAdoptKGs` purges + re-warms the KG then. `/api/enhanced-kgs`
  `kgs[].ne_status`, `other.pending`. `/cells` meta: `kgs_missing` (union) + `kgs_pending` (poll after
  `meta.retry_after_s`) + `kgs_not_processed`; a 200 cell with pending KGs is cached only until the hint (30 min–6 h),
  a 404 `status:"pending"` likewise (≥ 5 min). Relayed as `ne.kgs_pending` on `/api/viewport` and `/api/ne`.
- **NE confirmed, not flagged** (`srv/neobserved.go`, belt and braces under the above): srtm's registry flagged KGs
  v2.4 hours before `/cells` served them (2026-10-07: 61 of 354 flagged KGs with built cells had no NE cells —
  19454 Gerersdorf etc., two lucky players landed there). Every ready cell build notes `kgs[].ne` per KG
  (`ne-obs:v1:<kg>` = 1/0, 24 h, positive wins; `neObservedSeed` scans cached cells at startup when no notes exist;
  `neAdoptKGs` drops the note). `neConfirmedKGSet()` = registry v2.4 minus explicit 0 — **lucky uses only this**:
  with ≥ `luckyV24Min` 10 warm confirmed KGs the playable set is v2.4-confirmed only (cluster cold list too), and
  `spawnNEOK` rejects a spawn whose cached cell says the spawn KG has no NE (`lucky: spawn rejected, v2.4 KG without
  NE cells`). `cellData.KGsNE` carries the per-KG flag.
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

**Parcel popup layout** (`#parcel-popup`, `showParcelPopup`): flex column `.pp-head` (title + `.pp-sub` with the
KG ▸ / EZ ▸ links) → `.pp-body` (the only scroller) → `#pp-actions` **pinned** action bar — the buy/harvest/sell
buttons are always on screen, desktop and phone. Essentials grid = Fläche, Nutzung (top-2 shares, full list in
Details), game state rows (Feld/Wald/Anbau/Ernte/Brunnen/Wert), Besitzer, Preis. Everything else is folded in
`.pp-sec` sections (`det` 📋 Details: Parzelle id, Ried, full Nutzung, Bebauung · `bldg` · `env`), each header
carrying a one-line `.pp-sec-sum` (`ppSecSummary(name, text)`) so the folded state still informs. Defaults
`G.ppSec` (only `bldg` open on desktop); explicit toggles persist in `localStorage siedler_ppsec` (`G.ppSecUser`).
Action bar: `.pp-act-main` = the one headline action (Kaufen / Ernten / Förderung / Holzernte, else Naturschutz)
+ the icon-only 🔍 similar button (`.pp-act-ico`, count badge via `setSimilarBtn`), `.pp-act-row` = secondary
(Naturwald/Aufforsten/Brunnen/Verkaufen), then offers / notes. Peek keeps `.pp-head`. xbrowser scenes
`popup-actions` (asserts the first button is inside the popup and viewport), `popup-details`.

**Hints vs. popups.** Canvas beacons (`edgePoint` → `avoidObstacles`) are clamped
to `hudSafeInsets()` and steered around open popups (`_hudObstacles`; full-width
phone sheets become insets); a covered target gets a chevron aimed at it. Desktop
popups are corner-anchored and `placePopupClear(id,lon,lat)` toggles `.flip` so
they never sit on the tapped spot; right-anchored panels use `right:60px` (zoom
column). Phones: every popup is a sheet (grabber, swipe header down → `.peek`,
again → close, tap → expand; `initPopupSheets`). Call `invalidateHudInsets()`
after moving a popup programmatically (class changes are observed automatically).

**Performance tier (`PERF`, game.js).** Measured, not guessed: `perfNoteBuild(ms)` (CPU time of every completed
base build, EMA) and `perfNoteFrame(ms)` (live frame minus pump time). Build EMA > 450 ms or sustained frames > 34 ms
→ **low tier** (`perfSlow()`): nature/forest overlays static, treasure loop 10 fps, building footprints flat boxes
below z17, crop sprites only ≥ z17, forest filler thinned, NE apex budget halved, wheel ease 70 ms. ≤ 2 cores / ≤ 2 GB
/ reduced-motion start low; recovery after 10 cheap builds. `DEV.perf()` / `DEV.perf('low'|'high'|null)`, `?perf=low`.
Rules that keep it smooth, learned the hard way (2026-10-08, "ruckelig in Wien"): **never rebuild the base while a zoom
gesture is live** (`zooming` in `drawCachedBase` — the ease changes zoom every frame, so each frame restarted and
pumped a build that was discarded next frame); the minimap base is keyed on *data*, not camera, and slid affinely
during motion (`MINI.baseW`); living overlays visit `statefulPolys(claimMap)` (claims + harvest rows), never all
18 000 polygons; per-feature memos for terrain (`f._ter`), veg (`f._veg`), toponym text width (`t._mw`); layout reads
(`getBoundingClientRect`) are cached 400 ms; toponym placement is reused ≤ 150 ms while the camera moves; heavy base
steps (landuse polys, footprints, forest/crop sprites) are generators that yield every 64–512 features; each pump
slice ends with a 1 px `drawImage` into a scratch canvas (`_flushCtx`) so Chromium's deferred canvas raster is paid
inside the slice instead of as one 200 ms hitch at the first blit. Wheel: `ZOOM_LEAD` 1.25 caps how far the target may
run ahead, a direction reversal drops the queued lead (`smoothZoomBy`), `zoomStep` dt cap 400 ms (frame-rate independent
on 3 fps machines). Drills: `python3 tools/perf_wheel.py <cdp-port> <cpu-throttle> [lon lat zoom] [auto|low|high]`
(real CDP wheel notches: frame gaps, long tasks, overshoot, reversal latency) and `tools/cpuprof.py <port> "<js>" [rate]`
(V8 sampling profile → top self/total functions); port via `ss -ltnp | grep headless-shell`. `DEV.goto` hangs under
`Profiler.start` ("Promise was collected") — set `G.cam` + `loadMoreParcels()` directly in drills.

**Camera in the URL.** `syncViewHash()` (from `renderNow`, 600 ms debounce) keeps `#v=lon,lat,zoom` current while playing; `startGameWithLoading` applies it, so reload / "Weiter ▸" / rejoin links reopen the exact viewport — no cookies, no storage. **Enhanced badge** (`#enhanced-badge`): shown only while ≥ 60 % of the visible parcel area (`enhancedViewShare()`, bbox pass ≤ 1 per 400 ms, hysteresis: hides < 50 %) carries terrain/NE enrichment; hide = CSS `.fade-out` (`badgeVisible`), `display:none` after 650 ms.

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
`cacheJanitor` prunes expired rows), `kg_warm`, `parcel_harvest_state` (hashed, regrowth clock per parcel).

`s.Q` is `Store` (`srv/cachestore.go`) wrapping sqlc: api_cache bodies > 2 KB
are stored gzipped (gzip magic detected on read, transparent to callers) —
never read `api_cache.data` with raw SQL expecting JSON. 200 bodies from
`cachedFetch` get `Cache-Control: private, max-age=3600` (`browserCache`,
skipped for `ready:false`) so reloads don't re-download cells/layers.
Disk hygiene: bevdirect keeps tiles in RAM only (no prune timer needed), journald
capped at 100 M (`/etc/systemd/journald.conf.d/size.conf`). `db.sqlite3` has
`auto_vacuum=INCREMENTAL` (set by hand 2026-10-08) and the janitor runs
`PRAGMA incremental_vacuum` after each prune, so the file tracks the live
api_cache (~1.2 GB = one day of warmed cells + NE docs) instead of growing. Big
regenerable disk users: `~/.cache/go-build` (`go clean -cache`), `~/.cache/ms-playwright`
(xbrowser), `/tmp` debris, `tools/xbrowser/out`.

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
`/sell-parcel` (60 % × regrowth), `/harvest-parcel`, `/harvest-forest`, `/dig-well`,
`/claim-treasure`, `/complete-challenge`, `/offer-parcel`, `/offer-respond`,
chat + rules + block/report.

**Search** (`srv/search.go`, client `createSearchBox` in game.js — one engine for the picker
`#input-search` and the in-game bar `#game-search-input`). `GET /api/search-index` = the whole
admin table (2 092 Gemeinden + 7 850 KGs, codes, centre, span; ~160 KB gz, ETag, 1 d) indexed in
the browser (`SIDX`, `snorm`/`sqnorm` diacritics-free, "St." → "Sankt") → Gemeinde/KG/code rows
render **synchronously per keystroke**; umfeld `/lookup` (Ortschaften + PLZ, 120 ms) and
`/search/address_osm` + `/toponyms/search` (220 ms; Nominatim is 1 req/s upstream, proxy caches 24 h)
merge in under a striped progress bar without wiping rows; in-game the address query is also sent
*with the current Gemeinde appended* (`currentMuniName()`). Parcel intents `68/3`, `.68`, `12105-68/3`,
`Dürnstein 68/3` → loaded polygons first (`DEV.find`), else `GET /api/parcel-find?kg&gnr&lon&lat`
(cached cells of the KG nearest the hint first, cold cells built within 8 s, then `202 progress{searched,total}`
— the client polls and shows "Durchsuche KG … 3/12 Zellen"; ≤ 40 cells). Picker picks set
`G._muniHint` (exact spawn → `spawn_exact`) and `G.pendingSelect` (parcel popup opens after loading,
`checkPendingSelect`). Recent picks in `localStorage` `siedler_recent_search`. xbrowser scene `search-parcel`.

**Geo/data proxies.** `/api/viewport`, `/api/viewport-landuse`, `/api/lucky`,
`/api/warm/status`, `/api/kg-geo/{kg}`, `/api/municipality?lon&lat`,
`/api/municipalities?q=|list=all|state=&format=geojson`, `/api/parcel-context`,
`/api/osm-lines?…&cat=road,rail,water`, `/api/n2k`, `/api/enhanced-kgs`,
`/api/lidar/kg/{code}` (`parcels:[]` always), `/api/lidar/*` (generic, slow paths
blocked), `/api/trees`, `/api/buildings`, `/api/landmarks`, `/api/landscape`,
`/api/similar`, `/api/forest-value`, `/api/building-info`, `/api/kg-summary/{code}`,
`/api/schlaege`, `/api/hofstellen`, `/api/water/*`, `/api/well-quote`,
`/api/field-economy`, `/api/dossier/{kg}`, `/api/tiles/hillshade/{z}/{x}/{y}`,
`/api/umfeld/*` (umfeld context paths only), `/api/licenses`.
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

- `/api/enhanced-kgs` → `{kgs:[{kg_code,kg_name,gemeinde_code,gemeinde_name,lon,lat,v2,v24,product_version}]}`
  from srtm `/kgs`, **full v2.4 only** (v2 and v24 always true). Picker glows on enhanced municipalities;
  lucky picks only them.
- `/api/lidar/kg/{code}` ("lidar slim") → `terrain, buildings[], top_trees[],
  top_objects[], product_version` built from `/kg/{code}`, `/buildings/bbox`,
  `/trees/bbox?min_height=25`, `/landmarks/bbox` (`splitBBox` for big KGs).
  Client `fetchEnhancedKG` → `G.lidarKGTerrain`, `addLidarBuilding` (centroid grid
  `G.lidarBuildingIdx`), `G.topTrees[kg]`, `G.topObjects[kg]`.
- **NE-backed layers**: for an aligned cell of a v2.4 KG `/api/trees` returns every NE apex ≥ 4 m (`neTreeMinH`; ≤ `neTreeLimit` 6000 per cell, **thinned spatially** — k-th tallest per H3 cell first, so hedgerows/orchards survive and only closed forest is thinned; output tallest first; cache `trees:ne:v2`) with `species`, `vitality` (`source:"ne-cells"`). Unprocessed KGs (srtm `processed:false`, e.g. Wolfsbach 03225) only have the legacy ≥ 20 m index — nothing shorter exists there. Client draws ≥ 10 m below z16, ≥ 6 m below z17, everything at street level. Also `/api/buildings` every structure (`type`, `area_m2`, `dh_m`; walls/fences dropped); partial cells merge legacy rows for the uncovered part (`legacy_rows`). `GET /api/ne?west..north` → heat columns (lon/lat, cover, canopy, h_max, consistency, phenology) for overlays. Client: `apexVariant()` uses species/vitality (dead/declining → snag), `lidarForFootprint()` prefers `props.ne`, popup `neRows()` ("👁 Beobachtet" verdict `NE_VERDICT`, Bäume, Bauwerke, Satellit, Veränderung), `#map-attrib-ne` row via `noteNE()`, `DEV.ne(pid?)`. Agent inspect `terrain.observed` + narration.
- **NE heat overlay** (`#btn-ne` pixel-eye SVG in the zoom column, hidden until `noteNE` sees NE data, classes `off|mode-consistency|mode-canopy`; cycles off → `consistency` → `canopy`): `setNEHeat(mode)` → `loadNEHeat(c)` per cell (`GET /api/ne?…&v=2`, plain int arrays — never `[]uint8`, Go would base64 it; cache key `neheat:v2`) → `G.neHeat[key]` typed arrays → `drawNEHeat()` in the cached base layer (H3-res-12-sized hexes, ~10 m; part of `baseSignature`). Consistency paints only discrepant cells (`NE_HEAT_COL`), canopy a teal wash; both use cached radial splat sprites (`neSplat`). Minimap: one 1–2 px dot per discrepant parcel (verdict colour), no heat wash. `DEV.neHeat(mode?)`.
- **NE everywhere else**: `/api/kg-summary/{code}` carries `ne{cells, coverage, cover{9 groups}, canopy, tree_n, trees_tall, tree_h_max_m, structures_n, consistency{}, discrepant, forest_loss, forest_loss_last, species{}, structure_types{}, epoch}` = `neKGSummary(kg)` over the cached NE cells (no fetch) → KG card **👁 Spähbericht** (`kgObservedHTML`: cover bar + 4 stat tiles; the Abweichung tile toggles the map overlay). Picker: `G.neKGs` / `enhancedGemeinden[].ne` (registry `v24`) → green ring + "👁 beobachtet" badge, legend `#pick-ne-legend`. Agent look rows carry `ne_verdict, ne_canopy, ne_tree_n, ne_structures_n, bonus_spurenleser_xp` (`neBonusXP`).
- **NE mechanic**: `parcel_claims.ne_verdict` (migration 016) = `neVerdictOf(pid,lon,lat)` from the cached cell at claim time (client `doClaim` sends `lon/lat`). `neDiscrepant(verdict)` (not consistent/unknown) → **+60 XP Spurenleser** on claim (`ne_bonus_xp`, `ne_verdict` in the answer) and quest **"Spurenleser"** (`challenge_type: observe`, hidden until `G.neCells>0`, briefing points at the nearest discrepant parcel or turns the overlay on); `convert biodiversity` on a `forest_loss` claim pays 200 XP (`ne_restore`, Wiederbewaldung). Popup row shows the bonus on unclaimed parcels. Agent inspect `game.actions[].bonus_spurenleser_xp`. xbrowser scenes `ne-heat`, `ne-heat-canopy`, `ne-popup`, `ne-quest`, `ne-back` (jump to Kohlschwarz 63330).
- `/api/trees` → `{trees:[{lon,lat,h_m,crown_d_m}]}` per cell → `loadTrees(c)` →
  `G.apexTrees`, `assignApexTrees(newPolys)` (PIP → `G.apexByParcel`);
  `drawApexTree()` at real position, apices ≥ 32 m join `G.topTrees['apex']`.
- `/api/buildings` → `{buildings:[{lon,lat,max_height_m,mean_height_m,stories_est,roof_type}]}`
  → `loadBuildings(c)`; `lidarForFootprint(f, ring)` is the single lookup
  (centroid grid, `findLidarBuilding`); storeys = `mean_height_m/2.9`.
- `/api/landmarks` → `{landmarks:[{type,lon,lat,height_m}]}` (landmark sprites).
- Relief: `drawRelief()` composites hillshade tiles via `/api/tiles/hillshade/…`
  (`srv/tiles.go`, 204 = no data) `overlay`, deliberately faint
  (α 0.34 → 0.10); while active the per-parcel slope/aspect tint is skipped.
  `DEV.relief(bool)` (stats: states, inflight, queued, pausedMs, throttled).
  **srtm tile budget** (renders are rate-limited upstream: 25/s, burst 150;
  z10–11 most expensive): proxy keeps ≤ 8 upstream requests in flight
  (`tileSlots`), serves z < 12 as 204, stores tiles in api_cache for 1 y
  (`hs:v2:` JSON `{etag,b64,fresh_until}`) and revalidates with
  `If-None-Match` after 30 d (304 = free); 429/503 + `Retry-After` pause the
  gate (exponential, cap 30 s) and are relayed to the browser as **503 +
  `Retry-After` + `X-Upstream: busy`**, never cached. The breaker treats a
  503 *with* Retry-After as busy, not down, and probes srtm via
  `/api/v1/health`. Client `TILEQ`: ≤ 8 in flight, centre-first queue, drops
  tiles that left the **real** viewport (`realView()` — the padded 1.5× base
  canvas only draws cached tiles, `_bbReal`), fetches nothing while the camera
  moves (`reliefCamSettled`, 220 ms; sustained motion such as the
  Wassertropfen-Reise gets one wave per 1.5 s), honours Retry-After with
  back-off. `/api/metrics` → `tiles{}` (upstream hit/render, 304, throttled).
  xbrowser turns relief off after the game opens and only exercises it in the
  `relief` scene.
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

## Harvest state, regrowth value & the deal ceremony (`srv/harveststate.go`)

- **Harvest state outlives ownership.** `parcel_harvest_state(session_id, parcel_hash, kind forest|field|meadow,
  crop_group, harvested_at, harvests)` — hashes only, like every other table. Written on every harvest
  (`recordHarvestState`) and on sale (`handleSellParcel`), seeded into the next claim (`inheritedHarvest` →
  `seedInheritedHarvest`, also on `/api/claim-ez`; stale rows — forest > 510 min, field > 4 h — are ignored).
  `GET /api/session/{id}/harvests` lists it; client `G.harvestStates` (loaded with `loadClaimed`, resolved by
  hash like claims) and **`harvestOf(pid, claim)`** is the single accessor — `fieldStage`, `forestStage(hv)`,
  `getParcelTerrain`, `drawForestOverlay`, the sprites and the popup all take it, so an unowned Kahlschlag
  still shows stumps and a stubble field stays stubble. Offers transfer the claim row in place (clock kept).
- **Value follows the stand/crop.** `regenFactor(kind, harvestedAt, pid, crop, now)` → price multiplier:
  forest `0.4 + 0.6·min(1, min/510)`, crop field `0.75 + 0.25·progress` (stubble/harvested = 0, ripe = 1),
  meadows/others 1. Claim price = `calculatePrice × regen` (answer carries `base_price, regen, regen_progress,
  inherited`), sell = `sellPrice(purchase, regen)` = 60 % × regen (answer `sell_price, regen`). Client mirror
  `regenOf(p, claim)` / `regenLabel` / `sellQuote` (popup row **Wert** `#pp-regen` with the pixel bar, sell button
  shows the quote; `REGEN_FLOOR_*` must match). Agent inspect `game.regrowth{}` + discounted `price_coins`.
- **Per-crop cycles.** `cropCycle(crop, kind)` / JS `cropCycleS`: Getreide 60 min, Mais 90, Feldfrucht (sonst) 45,
  Obst 180, Wein 240; hash kinds 60/90/150; meadow Förderung stays `fieldCycle` 60; timber `forestFullValueMin`
  510. `fieldPhase.Cycle` carries it (`nextPayoutAt`). The phase offset stays hash-based (mod 60 min) so
  neighbours still ripen at different times. Claim/sell requests send `crop_group` (`parcelCrop(p)`).
- **Deal ceremony** (`dealFX(kind, f, coins, hold?)` → `G.fx` entries with `deal`, drawn by `drawDealFX` from
  `drawCollectFX`, so the treasure anim loop drives it; geometry in lon/lat): buy = coins arc from the purse
  (`hudCoinPoint()` = `#st-coins`/`#s-coins`), gold survey outline, banner pole in the player colour drops in with
  dust puffs, `−N 🪙`; sell = banner yanked out, wooden `VERKAUFT` sign stamps, coins arc back, `+N 🪙`. The purse
  ticks (`tweenCoins`, `.coin-tick/.coin-up/.coin-down`; `updateStats` leaves the counter alone while
  `COINTWEEN.active`). Pixel unit `u` by zoom like the living overlays. QA: `DEV.deal('buy'|'sell', hold 0..1,
  pid?)`, `DEV.deal('clear')`; xbrowser scenes `deal-buy`, `deal-sell`.

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

## Schildersturm — smashable labels (hidden feature, `SMASH` in game.js, `srv/smash.go`)

Decorative canvas labels (toponyms, giant-tree tags, treasure name tags, the Natura-2000
chip, Messstellen/gauge names) go through `smashDraw(ctx, id, bx, by, w, h, paint)`:
it registers the **exact** box for the frame (`SMASH.reg`) and calls `paint(ctx)` unless
the label is shattered (`SMASH.gone`). A precise tap (no slop; smallest box wins,
checked in `onGameClick` **after every real tappable** — treasures, markers, stations,
trees — and before footprints/parcels) → `smashHit`: Mario-brick debris (label painted
once off-screen, cut into chunks, gravity), impact flash + sparks, spinning coin, "+N".
Coins = `smashCoinsFor(visible_ms)` = `max(1, round(12·e^(−ms/6000)))` — the fresher
the sign, the more (server mirror `smashCoins`, keep in sync). `POST /api/smash-label`
`{player_id, session_id, visible_ms, label}` → `{coins, today, cap, capped, player}`;
in-memory guard: ≥ 300 ms between smashes (429), 400 coins/player/UTC day. Labels regrow
after 45–60 s **or** as soon as the camera pans ≥ 55 % of the screen / zooms ≥ ½ level
(`smashCamMoved`, evaluated eagerly in `smashFrameBegin`); the toponym slot stays
reserved while shattered so no neighbour pops in. Only labels drawn live in `renderNow`
are smashable — never register from the cached base layer (coordinates differ).
Debris runs the anim loop at 16 ms while `SMASH.fx.length`. `DEV.smash()` /
`DEV.smash(true)` / `DEV.smash('reset')`, xbrowser scene `smash`.

## Quests → Herald

Built-in quests live in `generateChallenges` + `backfillChallenges` (+ `questProgressFor`/`questSatisfied`/`questProgress` counters). `GET /api/session/{id}/challenges` returns live `progress/goal`
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
