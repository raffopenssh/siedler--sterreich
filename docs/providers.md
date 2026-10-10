# Data providers & upstream contracts

The browser never talks to a data host — everything goes through our proxies. All upstream calls use
`upstreamGet`/`upstreamClient` (pooled, HTTP/2, 60 s timeout, per-host circuit breaker). Never `http.Get`;
never set `Accept-Encoding` by hand (Go gunzips transparently). KG codes are 5-digit strings with a leading
zero; compare with `unpadKG` (umfeld `/lookup` returns `3301`, cells carry `03301`).

| what | where | notes |
|---|---|---|
| **cadastre** — parcels, footprints, landuse polygons, EZ, Gemeinde/KG at point | **bevdirect-serve** `http://127.0.0.1:8787` (`bevAPI`, systemd `bevdirect-serve`, /opt/bevdirect) | Assembles live from `kataster.bev.gv.at` vector tiles (CC BY 4.0). World = fixed 0.02° grid `floor(lon/0.02), floor(lat/0.02)`. See [cadastre-cells.md](cadastre-cells.md). |
| **context** around a point | **umfeld-at** `https://umfeld-at.exe.xyz/api/v1` (`umfeldAPI`), ≤ 5 req/s | `/context` (land price, OSM distances, N2K, toponyms, RIS legal), `/search/municipalities`, `/lookup`, `/search/address_osm`, `/osm/geometry?bbox` (bbox only), `/natura2000/*`, `/toponyms/*`, `/legal/*`, `/land_prices/point…`. Statistik Austria, EEA, OSM, BEV DLM names, RIS. Also the NE declared twin `/ne/{kg}`, `/ne/manifest`, `/ne/{kg}/head`, and the contrib sink `/contrib/api/v1/ne/{kg}/report|chunks`. |
| **observed layer (NE cells)** | **srtm-lidar-at** `/api/v1/cells?bbox&format=columns&centres=1&layers=obs,trees,structures` | H3 res-12 cells for v2.4 KGs. See [ne.md](ne.md). |
| **landscape** | **srtm-lidar-at** `https://srtm-lidar-at.exe.xyz/api/v1` (`lidarAPI`, public tier: bbox/point/KG-code keyed only) | `/landscape?bbox`, `/trees/bbox`, `/buildings/bbox`, `/landmarks/bbox`, `/heightfield?bbox&cell=25&landcover=1` (404 where no grid25), `/kgs`, `/kg/{code}`, `/tiles/hillshade/{z}/{x}/{y}.png`. **No parcel or footprint ids** — the client assigns by point-in-parcel / centroid grid. |
| **admin table** | `srv/data/admin.json.gz` (embedded; BEV VGD 1:50 000, CC BY 4.0) | all 7 850 KGs: code, name, Gemeinde, district, state, bbox, area. Drives `/api/kg-geo/{kg}`, KG neighbours, the warm plan, `/api/lucky`, `kgsAlongPath`, `/api/search-index`, the contrib universe. |
| siblings | holzeinschlag-at, farm-subsidies-austria, groundwater-at | timber prices/history, INVEKOS Schläge/Hofstellen, water. farm host has a 3-slot semaphore (`hostSlots`; busy → 503 `status:"busy"` + `X-Upstream: busy`). |

## bevdirect-serve (our cadastre assembler)

Public repo `github.com/raffopenssh/vtcseamless` (MIT, preset `bevdirect/`), running **v0.3.3** (release binary,
2026-10-08). Cell output unchanged since v0.2.1 → registered as `bev_equivalent_tags` on the NE server, so
upgrades inside that class need no re-baselining. Library import path `github.com/raffopenssh/vtcseamless/bevdirect`
(not embedded yet).

- Endpoints `/viewport?west&south&east&north&layers=parcels,footprints,landuse&wait=s` (bbox ≤ 0.045°),
  `/parcel/{kg}-{gnr}?lon&lat`, `/ez?kg&ez&west..north`, `/municipality?lon&lat`, `/municipalities?q=`,
  `/kg/{kg}`, `/health` (→ `tile_cache{tiles,bytes,max_bytes,hits,misses}`, relayed in `/api/metrics.bevdirect`;
  `bevdirect.version_mismatch` flags drift from `bevdirectVersion` in `srv/licenses.go`).
- `ready:false,pending:true,retry_after_s` = still assembling, **never "no parcels"**. `complete:false` on a
  parcel = truncated at the fetched tile edge.
- **Aligned cells only**: a multi-cell `/viewport` keeps one truncated copy of parcels wider than cell + pad
  (0.028°), so a stitched viewport never equals an aligned-cell build. Everything that must be reproducible
  (NE reports) fetches one `/viewport` per aligned cell.
- Tiles live **only in RAM** (`-tile-cache-mb 1024` LRU, `-tile-ttl 24h`; no `.pbf` on disk, `-cache` ignored).
- Every response carries `notice` (© BEV … CC BY 4.0, bearbeitet) → shown in `#map-attrib`, relayed on
  agent endpoints and `/api/similar`. Anything derived is cached ≤ 24 h (`cadastreTTL`).

**Upgrade**: `curl -fsSL https://raw.githubusercontent.com/raffopenssh/vtcseamless/main/bootstrap.sh | PREFIX=/opt/bevdirect PORT=8787 VERSION=vX.Y.Z sudo -E bash`.
**install.sh overwrites the unit** (sets `User=root`, `-cells 160`, prefetch on) and does *not* restart a
running instance: restore `User=exedev`, `-cells 120 -prefetch 0` (keep `MemoryMax=3G`, `GOMEMLIMIT=2560MiB`),
`daemon-reload`, restart (`SOURCE.txt` in /opt/bevdirect). Then bump `bevdirectVersion` in `srv/licenses.go`
and the version in `impressum.html`/`imprint.html`.

## Cadastre-data hygiene (legal, non-negotiable)

Two tiers, both stated on the legal pages (impressum/imprint/datenschutz/privacy, `/api/licenses` policy) —
keep code and pages true to each other:

| tier | what | where it may live | lifetime |
|---|---|---|---|
| **raw** | BEV `.pbf` tiles; raw bevdirect `/viewport` documents kept for the night report | **RAM only** (bevdirect LRU; `contrib_stash`) | ≤ 24 h, lost on restart |
| **assembled / derived** | `vp:v1` cells in `api_cache` (SQLite), `ne-report` work cells, NEC1 `.nec` containers (per-H3-cell statistics), any `/parcel`, `/ez`, `/viewport` dump | disk allowed | **≤ 24 h, then gone** (janitor, `run.sh` prune, you) |

Nothing else: no parcel database, no cadastre ids in the DB (hashes only), nothing findable by GNR/EZ without a
location. `run.sh` prunes `data/ne-reports/work/` and `data/ne-reports/nec/*.nec` (> 24 h) on every run; the
digest-only reports `KG.<date>.json`/`.meta.json` carry no cadastre content and stay. When you dump a
`/viewport`, `/parcel` or `/ez` response for debugging (curl → `/tmp/*.json`, `--keep-cells`, test fixtures),
**delete it before you finish the task** — never commit one.

Audit: `find / -xdev -name "*.pbf"` (none), `ls data/ne-reports/work`, `find data/ne-reports/nec -mmin +1440`
(none), `grep -l footprints /tmp/*.json`, `sudo ls -l /proc/$(pidof bevdirect-serve)/fd` (no regular files),
`sqlite3 db.sqlite3 "select count(*) from api_cache where cache_key like 'vp:%' and expires_at < datetime('now')"`
(0 after the hourly janitor). Licence copies: `docs/licences/README.md`.

## KG universe & srtm registry

- **KG universe check** (`srv/kguniverse.go`): umfeld `GET /api/v1/kgs` (gzipped list, ETag /
  `X-KG-Universe-Hash`, 304 on If-None-Match). Contract: `kg_count = 7850`,
  `universe_hash = sha256(join(sorted(kg_code),"\n"))` hex. `kgUniverseBoot()` (sync in `Serve`) computes our
  hash from the admin table + loads the mirror `data/kg-universe.json.gz` (seed `data/kg-universe.seed.json.gz`
  committed); `kgUniverseInit` fetches at startup and revalidates hourly. umfeld ↔ ours ↔ srtm
  (`noteSrtmUniverseHash`) must agree, else `slog.Error`, one e-mail to the owner per fingerprint
  (`sendOwnerMail`, `SIEDLER_ALERT_MAIL`), `kg_universe.alert` in `/api/metrics` + `/api/warm/status`, and
  `kgUniverseOK()` pauses the daily warm plan + `neAdoptKGs`.
- **srtm KG registry** (`srv/kgregistry.go`): hourly `/api/v1/kgs?fields=codes&processed_only=0` +
  `/llm/manifest.json` with `If-None-Match`; the full rows (`/kgs?processed_only=0&limit=5000`, 2 pages) only
  when an ETag moved. Mirrored to `data/srtm-kgs.json.gz` (seed `data/srtm-kgs.seed.json.gz`), used when srtm
  is down (`source:"local-copy"`, 5 min TTL instead of 30). Hashes are recomputed on read, never trusted from
  disk. Only a live answer purges derived caches / adopts v2.4.
- `/api/enhanced-kgs` (`enhanced-kgs:v5`) → `{kgs:[{kg_code,kg_name,gemeinde_code,gemeinde_name,lon,lat,v2,v24,
  product_version,ne_status}], other{…pending}, kg_count, universe_hash, registry_hash, source, fetched_at,
  upstream_etag}` = **full v2.4 rows only** (`v2.4-partial`/v2.3/grid25-only are not enhanced). Picker glow,
  the "Enhanced Gelände" chip, `/api/lucky` and the daily warm plan key on it. Weak `ETag` = registry_hash,
  `X-Registry-Source`.
- **`ne_ready` is the v2.4 truth** (srtm afe147d): rows carry `ne_ready` (bool, authoritative), `ne_status`
  (served|partial|pending|not_processed), `ne_cells_published_at`; `product_version` reads `v2.4-pending` until
  cells are ingested (raw in `product_version_registry`). `kgNEServed()` (landscape.go) = `ne_ready` when
  present, else the product string. `registry_hash` folds `ne_ready|ne_status` in; `updated_at` bumps on
  publication so `neAdoptKGs` purges + re-warms the KG then.

## HTTP 202 / pending (`srv/upstream_pending.go`) and outages (`srv/breaker.go`)

- `parsePending()` normalises any upstream "still working" body to `{status:"pending", retry_after_s,
  progress{…}}`; `upstreamGetWait()` polls a bounded budget honouring `Retry-After`; `withWait()` adds `?wait=`.
  Proxies relay 202 + `Retry-After`, never cache it. Client `api()` retries GETs on 202 (≤ 45 s / 8 tries,
  `pendingBudgetMs` opt, `pendingNotice()` updates `#map-loading`), then resolves `pending:true` so callers
  schedule their own retry.
- Breaker: `upstreamClient.Transport` per host; 3 consecutive transport errors / 502–504 open it; while open
  every call answers synthetic **503 `{error, status:"down", service, retry_after_s}`** + `X-Upstream: down` in
  ~0 ms; one probe at a time (20 s → 3 min). A 503 *with* Retry-After counts as busy, not down (srtm tiles);
  srtm is probed via `/api/v1/health`. Only requests whose own context is not done count as failures — caller
  budgets (2.5 s timber) are not outages. `/api/metrics` books down/busy under `upstream_down_503`.
- Client: `G.upstreamDown` (silent for optional layers; only a cadastre outage toasts), `loadBboxLayer`/
  `loadPointLayer`/`loadTileResilient` re-schedule after the window (`layerRetryMs`).
- Outage drill: stop `bevdirect-serve` — breaker opens after 3 failures, chip counts down, map refills on restart.

## srtm landscape endpoints we consume (`srv/landscape.go`)

- `/api/lidar/kg/{code}` ("lidar slim") → `terrain, buildings[], top_trees[], top_objects[], product_version`
  from `/kg/{code}`, `/buildings/bbox`, `/trees/bbox?min_height=25`, `/landmarks/bbox` (`splitBBox` for big KGs);
  `parcels:[]` always. Client `fetchEnhancedKG` → `G.lidarKGTerrain`, `addLidarBuilding` (centroid grid
  `G.lidarBuildingIdx`), `G.topTrees[kg]`, `G.topObjects[kg]`.
- `/api/trees` → `{trees:[{lon,lat,h_m,crown_d_m,species?,vitality?}]}` per cell (NE-backed for v2.4 cells,
  see ne.md; legacy ≥ 20 m index elsewhere). `/api/buildings` →
  `{buildings:[{lon,lat,max_height_m,mean_height_m,stories_est,roof_type,type?,area_m2?,dh_m?}]}`;
  `lidarForFootprint(f, ring)` is the single lookup; storeys = `mean_height_m/2.9`. `/api/landmarks` →
  `{landmarks:[{type,lon,lat,height_m}]}`.
- `/api/landscape?bbox` (terrain, landcover fractions, `dom_terrain`, tallest trees) feeds timber and inspect.
- **Hillshade tiles** (`srv/tiles.go`): srtm renders are rate-limited (25/s, burst 150; z10–11 most expensive).
  Proxy keeps ≤ 8 upstream requests in flight (`tileSlots`), serves z < 12 as 204, stores tiles in api_cache for
  1 y (`hs:v2:` JSON `{etag,b64,fresh_until}`), revalidates with `If-None-Match` after 30 d; 429/503 +
  `Retry-After` pause the gate (exponential, cap 30 s) and are relayed as **503 + `Retry-After` +
  `X-Upstream: busy`**, never cached. `/api/metrics` → `tiles{}`.

## Sibling services (farm / holz / gw)

- `srv/siblings.go`: `bboxProxy`/`bboxLayer` (bbox quantised to ~150 m for the cache key, only `ready`
  answers cached), `hostSlots`, `prewarmKGs` (KGs along a water flowpath), `kgsAlongPath`.
- farm: `/api/schlaege` (INVEKOS fields: `crop_group, snar_name, area_ha, organic`), `/api/hofstellen`.
- holz: `timberStatePrices()` from `/data/prices/state/{1-9}.json` (fallback catalogue `timber:catalog` 24 h, then
  `fallbackPrices`), `POST /api/plot-context?fast=1` (2.5 s budget, `holzCold` backoff) → `estimate.history`.
- gw: `/api/water/point|points|station/{id}|protection|flowpath|gwi|parcel/{pid}`, `/api/well-quote`.
- `srv/dossier.go`: `GET /api/dossier/{kg}` = gw + holz + farm `/llm/kg` in parallel (6 h cache, 404 negative
  1 h) → `drought{level,label,status,sigma}`, `game{yield_factor,well_protection,subsidy_per_ha}`.

## Provider history

`docs/migration-2026-10.md` is the only place documenting past providers. Code, comments and other docs
describe the current setup only.
