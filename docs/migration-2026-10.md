# Provider migration, Oct 2026 — cadastre from BEV tiles, context from umfeld-at, landscape from srtm public tier

## Why
BEV Katasterservice terms (https://www.bev.gv.at/Services/Geoinformationsdienste/Services/Katasterservice.html):
the former cadastre API host no longer publishes cadastre data. It is now
`umfeld-at.exe.xyz` and serves only **non-cadastre, point-keyed** context (Statistik Austria,
EEA N2K, WDPA, OSM, BEV DLM names, RIS). `srtm-lidar-at.exe.xyz` (public tier, port 443) serves
only **bbox/point/KG-code keyed** landscape data: no parcel ids, no footprints ids, no `/kg/{code}`
parcel lists, no `/query/parcels`, no `/flags`, no `/v3/trees`, no `/parcel/{id}`.
Both old hosts are gone/404. Private `/k/` tiers exist but **we do not use them** (that is the point).

## New upstreams (server-side only — the browser never talks to them)
| what | base | notes |
|---|---|---|
| cadastre (parcels, footprints, landuse, EZ-in-view, Gemeinde/KG lookup) | `http://127.0.0.1:8787` = **bevdirect-serve** (installed at /opt/bevdirect, systemd `bevdirect-serve`) | assembles live from `kataster.bev.gv.at` vector tiles (CC BY 4.0). World = fixed 0.02° cell grid `floor(lon/0.02), floor(lat/0.02)`; cells cached 6 h in RAM (160 LRU), tiles 24 h on disk. Endpoints: `/viewport?west&south&east&north&layers=parcels,footprints,landuse&wait=s` (bbox ≤ 0.045°), `/parcel/{kg}-{gnr}?lon&lat`, `/ez?kg&ez&west..north`, `/municipality?lon&lat`, `/municipalities?q=`, `/kg/{kg}`, `/health`. Parcel row: `parcel_id, kg_code, gnr (may start with "."), ez, rstatus, area_sqm, lon, lat, complete, parts, dominant_ns, landuse_areas{ns:m²}, building_count, total_building_area_sqm, geometry (Polygon/MultiPolygon)`. Footprint: `id, parcel_id, ns_code, area_sqm, lon, lat, obb_length_m, obb_width_m, orientation_deg, geometry`. Landuse: `id, ns, area_sqm, geometry`. `ready:false,pending:true,retry_after_s` = still assembling (never "no parcels"). `complete:false` = truncated at fetched tile edge. Every response: `notice` "© BEV, 2026 – … CC BY 4.0, bearbeitet" — must be shown. |
| context around a point | `https://umfeld-at.exe.xyz/api/v1` | `/context?lon&lat&area_sqm&building_count&footprint_area_sqm&landuse_areas=52:917,41:344&kg=63349[&include=]` → `data.{municipality,land_price,natura2000,protected_area,osm,toponyms,legal}`, `data.missing{}`; `/search/municipalities?contains_lon&contains_lat | ?q= | ?state=&format=geojson | ?list=all&limit=5000`; `/lookup?q=&type=gemeinde|kg|plz`; `/search/address_osm?q=`; `/land_prices/point?lon&lat&area_sqm…`; `/natura2000/point?lon&lat`, `/natura2000/site/{code}?geometry=1`, `/natura2000/search?list=all`; `/osm/point?lon&lat&area_sqm`; `/osm/geometry?bbox=w,s,e,n&cat=road,rail,water&major=1` (**bbox only — no `kg=`, no `water_parcels`**); `/toponyms/near|search`; `/legal/kg/{kg}`. ≤ 5 req/s. |
| landscape | `https://srtm-lidar-at.exe.xyz/api/v1` | `/landscape?bbox=w,s,e,n[&include=trees,buildings,landmarks]` (5–150 ms: terrain, landcover fractions + `dom_terrain`, trees.tallest, buildings summary); `/trees/bbox?bbox&min_height&limit` (`lon,lat,height_m,crown_d_m,confidence` — **no parcel_id**); `/buildings/bbox?bbox` (`lon,lat,height_max_m,height_mean_m,stories_est,roof,ground_elev_m` — **no footprint_id**); `/landmarks/bbox?bbox` (`type,lon,lat,height_m`); `/heightfield?bbox&cell=25&landcover=1` (25 m grid `z[][]`, `landcover[][]` codes, legend 1 tree 2 shrub 3 grass 5 water 10 roof 20 road 22 parking 24 rail 30 crop 31 orchard 33 garden 41 rock; 404 where no grid25); `/kgs?limit=1000&offset=` / `?bbox=` / `?codes=` (processed KGs, `bbox,centroid,product_version,grid25`); `/kg/{code}` dossier; `/tiles/hillshade/{z}/{x}/{y}.png`. bbox ≤ 0.03 deg². |
| embedded | `srv/data/admin.json.gz` (BEV VGD 1:50 000, Stichtag 2026-04-01, CC BY 4.0; copied from bevdirect) | all 7 850 KGs: `kg_code, kg_name, gemeinde_code, gemeinde_name, district_name, state_name, min/max lon/lat, area_sqkm`. Used for KG neighbours, kg→Gemeinde, warm planner, lucky pick. |
| unchanged | holzeinschlag-at, farm-subsidies-austria, groundwater-at | as before |

## Our server API — what changes for the browser (game.js)
Contracts kept identical where possible; the differences:

* `GET /api/viewport?west&south&east&north` → `{parcels[], footprints[], landuse[], ready, truncated, retry_after_s?, notice, cells[], kgs[]}`.
  * Tiles **must be grid-aligned**: `west=i*0.02, east=(i+1)*0.02, south=j*0.02, north=(j+1)*0.02` (client helper `gridTiles(bbox)`), so client dedup, our 24 h `api_cache` key (`vp:<i>:<j>`), the prewarm planner and bevdirect's cells all agree. Non-aligned requests are accepted but not prewarmable.
  * parcel props = bevdirect row (above) **plus** terrain enrichment from the srtm heightfield when grid25 exists: `elev_m, elev_min_m, elev_max_m, slope_deg, aspect (N..NW), dom_terrain, fracs{type:share}, tree_frac`. Client copies these into `G.terrainParcels[pid]` (same shape fetchEnhancedKG used to fill).
  * footprints: `footprint_id` (= bevdirect `id`), rest as before.
  * landuse: `{code:"52", area_sqm, geometry}` — the former `/api/viewport-landuse` payload now rides along; `/api/viewport-landuse` still works (served from the same cached build).
  * `ready:false` ⇒ client re-fetches after `retry_after_s` (existing `loadTileResilient`).
  * Cached **24 h max** (`api_cache`, key `vp:v1:i:j`). `X-Cache: HIT|MISS|MISS-SHARED|WARM`.
* `GET /api/lucky` → `{gemeinde_code, name, lon, lat, enhanced, warm, kgs[]}` — a Gemeinde whose KGs are currently cached (prefer srtm grid25). Replaces the client-side lucky logic.
* `GET /api/warm/status` → prewarm plan + queue (ops / DEV).
* `GET /api/kg-geo/{kg}` → admin row + neighbours (`{kg, neighbours[], gemeinde}`) from the embedded table.
* `GET /api/municipality?lon&lat` → bevdirect `/municipality` (Gemeinde + KG at point). Replaces `CAD/search/municipalities?contains_lon…` and `CAD/spatial/point` (KG at camera).
* `GET /api/municipalities?q=` / `?list=all` / `?state=&format=geojson` → umfeld (picker, Statistik Austria polygons) — same shapes as before (`{data:[…]}` / GeoJSON).
* `GET /api/parcel-context?pid&lon&lat&area_sqm&building_count&footprint_area_sqm&landuse_areas=52:917,41:344&kg=` → umfeld `/context` (24 h cache). Replaces `CAD/land_prices/parcel/{pid}` (→ `data.land_price`) and `CAD/osm/parcel/{pid}` (→ `data.osm`); also carries `natura2000`, `toponyms`, `legal`.
* `GET /api/osm-lines?west&south&east&north&cat=road,rail,water` → umfeld `/osm/geometry?bbox` (24 h). Replaces `CAD/osm/geometry?kg=…` (`cat=water_parcels` is gone — water share of a parcel now comes from bevdirect `landuse_areas` NS 60–64).
* `GET /api/n2k?west&south&east&north` → N2K sites intersecting the bbox with geometry (from umfeld site list + `/natura2000/site/{code}?geometry=1`, 24 h). Replaces `CAD/natura2000/kg/{kg}` + `/site/`.
* `GET /api/enhanced-kgs` → same shape as before (`{kgs:[{kg_code,kg_name,gemeinde_code,gemeinde_name,lon,lat,v2}]}`), built from srtm `/kgs` (v2 = grid25).
* `GET /api/lidar/kg/{code}` → same shape (`terrain, buildings[], top_trees[], top_objects[], product_version`) but `parcels:[]` is always empty (per-parcel terrain now rides in `/api/viewport`). buildings from `/buildings/bbox`, top_trees `/trees/bbox?min_height=25`, top_objects `/landmarks/bbox`, terrain `/kg/{code}`.
* `GET /api/trees` → rows `{lon,lat,h_m,crown_d_m}` (**no parcel_id** — client assigns `pid` by point-in-parcel against `G.parcelPolys`).
* `GET /api/buildings` → rows `{lon,lat,max_height_m,mean_height_m,stories_est,roof_type}` (**no footprint_id** — client indexes them into the centroid grid `G.lidarBuildingIdx`; `lidarForFootprint` falls back to `findLidarBuilding`).
* `GET /api/forest-value` additionally accepts `west,south,east,north` (parcel bbox) — stand facts from `/landscape?bbox&include=trees`.
* `GET /api/similar` → candidates from our own cached viewport tiles around the point (explored area only), scored with the enrichment fields.
* `/api/cadastre/*` generic proxy: only umfeld-compatible, non-cadastre paths remain (`/lookup`, `/search/municipalities`, `/search/address_osm`, `/toponyms/*`, `/natura2000/point|site|search`, `/osm/point`, `/legal/*`, `/land_prices/point|gemeinde|…`). Cadastre paths (`/spatial/*`, `/export/*`, `/search/parcel|ez|kg`, `/parcels/*`, `/osm/parcel`, `/land_prices/parcel`, `/natura2000/kg`) answer 410.
* Hillshade proxy unchanged in URL; upstream host changed.

## Caching & warming policy (24 h max for anything cadastre-derived)
* `api_cache` TTL for viewport cells, EZ, parcel lookups: **24 h** (was 6 h). Janitor prunes hourly. Context/N2K/OSM: 24 h. Landscape: 24 h.
* `kg_warm` table (migration 0xx): `kg_code PK, warmed_at, expires_at (=+24 h), cells, parcels, reason ('daily'|'neighbour'|'session')`.
* **Daily plan** (`warm.go`): 100 KGs/day in **5 geographic patches** (~20 KGs each: seed Gemeinde preferring srtm grid25 + neighbouring KGs by bbox adjacency), one patch every ~4.8 h, 1 worker, ~1 s pause between cells, yields to foreground traffic. Patches rotate so a lucky player can roam. Persisted as `warm-plan:<date>` in api_cache so a restart resumes.
* **Neighbour warming**: first viewport miss in a KG enqueues the KG's remaining cells + its adjacent KGs (≤ 6) at low priority (debounced 24 h via kg_warm).
* **Session warming**: `POST /api/session/create` enqueues the Gemeinde's KGs at medium priority (replaces the old upstream `POST /prewarm`).
* Lucky picks only Gemeinden whose KGs are all warm & not expiring within 2 h.

## Loading screen (client)
Steps: Session → Landschaft (srtm: `/api/lidar/kg` terrain + hillshade tiles + trees — fast) → Kataster (BEV live, lazy: only the centre cell blocks, max ~12 s; progress "Zelle 1/4", subline tells whether it came from cache or is being assembled live from BEV tiles) → Arten & Schätze → Bereit. Game screen opens as soon as the centre cell is in (or the budget is over); the remaining cells stream in with `#map-loading`.

## Known limitations (2026-10-03)
* **Stale cell vs live building_count.** A cadastre cell (`vp:v1:i:j`) is cached up to 24 h; bevdirect's
  own cell cache is 6 h and `/ez` / `/parcel` are assembled from fresher tiles. So a parcel can show e.g.
  `building_count 0 / total_building_area 881` in our cached viewport/look rows while `/ez` already says
  `building_count 2` for the same parcel (seen on 90107-2019/2). Prices, claims and the popup use the cell
  row; the EZ block in inspect uses the live `/ez` answer. Expected to converge within 24 h; the
  `watchAssembly()` purge does not apply (bevdirect carries no assembly tag). Deliberately not fixed —
  re-fetching cells on every mismatch would defeat the 24 h cache that keeps BEV tile load low.
* **Agent endpoints on cold cells** answer `202 {status:"pending", retry_after_s}` + `Retry-After`
  (`relayCellStatus`, agent.go); while the cadastre breaker is open they answer the breaker's
  `503 {status:"down", service:"cadastre", retry_after_s}` with `X-Upstream: down`.
* **Sibling overload** (farm `/api/schlaege` under cold-cell bursts) is relayed as
  `503 {status:"busy", retry_after_s}` + `X-Upstream: busy`; the farm host is limited to 3 concurrent
  proxy calls (`hostSlots`, siblings.go). `/api/metrics` counts both down and busy under
  `upstream_down_503`, not under our 5xx.
* **Session create** no longer touches the cadastre: ~20–100 ms warm, ~0.6 s when the Gemeinde's
  settlement centre is not memoised yet (one umfeld `/search/address_osm` call, 2.5 s cap, cached 24 h).
  N2K treasures and parcel-based treasure placement run in goroutines (SSE `treasures_updated`).

## 2026-10-04 — NE cells (observed layer) become the parcel enrichment source
srtm-lidar-at (product v2.4) and umfeld-at jointly publish **NE cells**: H3 res-12 statistics per cell,
same ids on both sides — umfeld `/api/v1/ne/{kg}` = *declared* land use from BEV (frozen `ne-cells-2`,
epoch 2026-03, manifest `/api/v1/ne/manifest`), srtm `/api/v1/cells` = *observed* (cover groups, canopy,
LiDAR heights, NDVI, change, terrain, `consistency` verdict, every tree apex ≥ 3 m with species/vitality,
every structure with type/heights). Contracts: umfeld `docs/ne-cells.md` (public rendering
`/api/v1/docs/ne.md`), srtm `/api/v1/docs/llm.txt#cells`, `/api/v1/cells/dict`.

What changed here (`srv/necells.go`):
* `buildCell` fetches the NE columns document for the grid cell (`/cells?bbox&format=columns&centres=1&layers=obs,trees,structures`,
  30–100 ms, 0.05–1 MB, cached 24 h as `ne:v1:i:j`) in parallel with bevdirect and the heightfield.
  `neEnrichParcel` (cells whose centre is inside the polygon, nearest centre ≤ 20 m for sub-cell
  parcels) fills elevation/slope/aspect/`fracs`/`dom_terrain`/`tree_frac` **and** a new `ne{}` block
  (cover, canopy, heights, trees on the parcel incl. species/vitality, structures, consistency shares,
  `verdict`, phenology, NDVI, change, ALS years). Footprints get `ne{h_max_m,h_robust_m,stories_est,type,…}`
  from the structure centroid inside the ring. The heightfield remains the fallback for KGs < v2.4 and
  for parcels with < 30 % of their expected cells in coverage.
* `/api/trees` and `/api/buildings` are served from NE rows for aligned cells of processed KGs (partial
  cells merge legacy rows for the uncovered part); `GET /api/ne` exposes heat-layer columns.
* Warming prefers v2.4: registry `v24` flag, `neAdoptKGs` purges + re-warms a KG once per product
  generation, plan seeds v2.4 Gemeinden first, `/api/lucky` prefers them, `/api/warm/status.v24_kgs`.
* Client popup "👁 Beobachtet" (verdict, Bäume, Bauwerke, Satellit, Veränderung), species-aware tree
  sprites, NE attribution row; agent inspect `terrain.observed` + narration ("Observation vs cadastre: FOREST LOSS …").
* Reporting back: `tools/ne-report` builds the frozen `ne_cells` container from our bevdirect v0.2.1 cells
  and posts epoch reports to umfeld `/ne/{kg}/report` (needs `ne-peer.key`; without it reports are stored
  locally). 63330 reproduces the contract digest `594025647ec3557e` bit-exactly (v0.2.1 ≡ v0.2.0).
* bevdirect-serve upgraded 2b7e725 (dev) → release **v0.2.1** (`bevdirect_version` + `coord_decimals:7`
  in every document, tile sweeper `-tile-ttl 24h`).
* 2026-10-04 (later): bevdirect-serve went public as **github.com/raffopenssh/vtcseamless** (MIT, preset
  `bevdirect/`); upgraded to **v0.3.0** via the public `bootstrap.sh` (no token; release asset 403 → built
  from source, same version string). Output byte-identical to v0.2.1 (63330 digest `594025647ec3557e`
  reproduced). install.sh rewrote the unit — `-cells 120 -prefetch 0` tuning restored by hand. NE reports
  now POST to umfeld `/contrib/api/v1/ne/{kg}/report` (was `/k/…`); peer token installed (`ne-peer.key`),
  first report per KG became the new baseline (POST 200, compared=false). Licence page / Impressum
  paragraph rewritten to the wording agreed with the maintainer (VTC §2.3, tileset URL
  `…/at.gv.bev.kataster/tiles/…`, open-source assembler, 6 h cells / 24 h tiles).

Known limitation: NE coverage is per KG (32 of 7 850 on 2026-10-04, growing); a cell straddling
processed and unprocessed KGs shows both enrichment kinds side by side (`kgs[].ne`).

Follow-up (same day, afternoon): verification under load — 129/159 cell builds since the deploy
`ne=true`, 26 477 parcels NE-enriched, non-bevdirect overhead ≈ 200 ms per ready build on both
paths, heightfield path intact for v2.3 KGs (Wien Josefstadt cell: 3 525 hf-enriched / 0 ne),
srv RSS 124 MB, heap 54 MB, no panics. Added: 👁 heat overlay (`/api/ne`, consistency / canopy),
`parcel_claims.ne_verdict` (migration 016) with the **Spurenleser** quest (+60 XP on a discrepant
claim) and the Wiederbewaldung bonus (Naturschutz on `forest_loss` = 200 XP), xbrowser scenes
`ne-*`. Bug fixed on the way: `/api/ne` emitted `[]uint8` columns, which Go JSON-encodes as base64
(`neheat:v2`, client `&v=2`). Upstream notes: `docs/upstream-notes-ne.md`.

## 2026-10-06 — bevdirect-serve v0.3.1 (tiles in RAM only)

Upgraded via `bootstrap.sh` (release binary). BEV `.pbf` tiles are no longer written to disk:
`/opt/bevdirect/bevcache` (564 MB) deleted, `bevcache-prune.timer`/`.service` removed, `-cache`
dropped from the unit, `-tile-cache-mb 1024` LRU + `MemoryMax=3G` / `GOMEMLIMIT=2560MiB` as shipped.
Unit re-tuned after install.sh overwrote it (`User=exedev`, `-cells 120 -prefetch 0`). Cell output
unchanged; v0.3.1 is `bev_equivalent_tags` on the NE server → no re-baselining, report source string
becomes `bevdirect@v0.3.1` automatically. `/api/metrics` → `bevdirect.tile_cache{}`. Our side keeps
no raw tiles (api_cache = assembled cells only; `tools/ne-report` uses the systemd instance and
removes its work dir).
