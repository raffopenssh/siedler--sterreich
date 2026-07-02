# Handover: "Enhanced Mode" for LiDAR-processed KGs

**Goal (user request):** For KGs that are already processed by the srtm-lidar API (and road/OSM-covered by the cadastre API), add an *enhanced mode* to the game: real terrain, tallest objects/trees (after applying quality flags), OSM roads/rail/water, per-parcel land prices, Natura-2000 overlays (with extra treasures there), improved sprites, and a GPS "show my location" button on mobile. "Feeling lucky" start should prefer enhanced KGs, updating dynamically as the two data services converge. **Keep everything fast and fluid** — this is a canvas game; never block the render loop on network.

Read `AGENTS.md` first — it documents the whole architecture (Go server `srv/server.go`, single-file frontend `srv/static/game.js` ~4050 lines, SQLite/sqlc, SSE, cadastre proxy with 1h cache).

## Verified API facts (all tested 2026-07-02)

### srtm-lidar API — `https://srtm-lidar-at.exe.xyz:8000/api/v1` (docs: `/api/v1/docs/llm.txt`)
- **1411 of 8440 KGs processed** (`/api/v1/index/status` → `processed_count`; growing continuously).
- `GET /api/v1/query?bbox=9,46,18,49.5&processed_only=true&limit=1000` → **0.3s**, paginated envelope `{total,offset,limit,results}`. Each result: `kg_code, kg_name, gemeinde_code, gemeinde_name, centroid_lat/lon, min/max_lon/lat, elevation_min/max_m, terrain_class, dominant_type, tree_count, building_* stats, processed:1`. Use this to build the enhanced-KG list.
- `GET /api/v1/query?kg=63314` → single-KG index record, ~150ms. Includes `processed`, `gemeinde_code`, landcover breakdown, `_links`.
- `GET /api/v1/kg/63314` → **full KG JSON, ~4MB, 0.4s**. Keys: `parcels.details[]` (per-parcel: `elevation_m, elevation_min/max_m, slope_mean_deg, aspect_dominant, terrain_class, vertex_heights[], area_summary (type→area/fraction), top_objs, top_trees, dominant_type, forested_fraction, ndsm_max_m`), `building_footprints.details[]` (per-building: `max_height_m, mean_height_m, stories_est, roof_type_hint (flat|pitched), centroid, footprint_area_sqm`), `top_10_trees[]`, `top_10_objects[]`, `top_by_type`, `terrain` (KG-level), `tree_stats`. **This one file powers most of enhanced mode.** Proxy + cache it server-side (4MB → ~fine gzipped; consider stripping `vertex_heights` server-side to shrink, they're the bulk of the payload).
- `GET /api/v1/query?segments=true&object_type=tree,roof&bbox=...&sort=height_max_m&limit=50` → ~2s. Top-50-per-type-per-KG segments. Alternative to full JSON for "tallest objects".
- `GET /api/v1/flags?kg=63314&limit=...` → **0.35s**. Quality flags. IMPORTANT: tallest "trees" are often masts/errors — e.g. KG 63314 top tree claims 213m, flagged `tree_height_implausible` (critical). **Filter any top-tree/object whose obj_ref appears in flags with severity high/critical before displaying.** Flag obj_refs look like `63314:top_tree:0` (rank N matches `top_10_trees` index).
- Overlays (`/dtm/overlay` etc.) take **45–60s** — TOO SLOW for gameplay. Do NOT use live. `/elevation` POST also ~45s. **Use only the pre-computed KG JSON + index queries.** (Optionally: server could warm/cache hillshade PNGs in background per session KG and deliver later via SSE, but treat as stretch goal.)
- Processing queue API exists (`POST /api/v1/processing/queue`) — optional stretch: enqueue the current session's KGs so they become enhanced later.

### cadastre API — `https://cadastre-process-api.exe.xyz/api/v1` (docs: `/api/v1/docs/llm.txt`)
Already proxied at `/api/cadastre/` with 1h SQLite cache (`handleCadastreProxy` in server.go).
- **OSM geometry**: `GET /osm/geometry?kg=63314&cat=road,water,rail` → **0.2s, ~290KB** (828 features, line chunks ≤24 vertices with `cat, fclass, name, ref, major` props). Perfect for drawing real roads/rivers/rail. Also `cat=transit,place` for stops/villages.
- **OSM per-parcel metrics**: `GET /osm/parcel/{parcel_id}` → dist_road_m, remoteness 0-100, etc. (enrichment also auto-appears in /search/parcel rows when warmed).
- **Land prices**: `GET /land_prices/parcel/{parcel_id}` → `buy_eur_per_sqm, buy_total_eur, rent_eur_per_year, class (bauland_built|bauland_zoned|ackerland|gruenland|wald|other), confidence`. Fast (<100ms). Batch: `POST /land_prices/batch` max 1000 `parcel_ids` — use batch for visible parcels.
- **Natura 2000**: `GET /natura2000/kg/{kg}` → **80ms**, `inside_count, inside_sites[] (sitecode, sitename, habitats[], site_type_label), near_only_sites[]`. `GET /natura2000/site/{code}?geometry=1` → polygon for drawing overlay. `GET /natura2000/site_parcels/{code}?limit=..` → parcels inside (60ms). DO NOT use `/query?gemeinde=X&has_natura2000=true` with gemeinde *names* — one test took 2m16s (cold); prefer the natura2000/* endpoints or gemeinde_code.

## Current code anchors (game.js)
- Global `G` state ~line 122; `render()` ~1469 (draw order documented in AGENTS.md); `fetchKGPolygonsBlocking/fetchKGPolygons` ~1243/1283 (per-KG parallel loads — add enhanced fetches here); `startGameWithLoading()` ~1040 (loading pipeline with progress steps); `startLucky()` ~359 (picks random muni from `/search/municipalities?list=all` — change to prefer enhanced); `showParcelPopup()` ~3589 (parcel info panel — add elevation/slope/price/N2K/remoteness rows); `calcPrice()` ~3798; treasure sprites ~2675-3170; building footprints drawn flat-ish at ~1646 (`drawBuildingFootprints`, roofOff currently from screen area only); zoom controls + `#btn-gearth` in index.html ~300.
- Server: `handleKGData` (paginated KG layer proxy, 1130), `handleCadastreProxy` (1203), `generateTreasures` (1429, deterministic offsets around center — currently a grid-ish pattern), `handleCreateSession` (288).
- Treasures table has `species_*` columns (migration 006). Treasure gen is server-side at session create.

## Implementation plan (single new conversation, all-in-one)

### 1. Server: enhanced-KG registry + lidar proxy
- Add `const lidarAPI = "https://srtm-lidar-at.exe.xyz:8000/api/v1"`.
- New endpoint `GET /api/lidar/` generic proxy (same pattern as cadastre proxy, 1h cache in `api_cache`). For `/api/lidar/kg/{code}` strip `vertex_heights` from parcels/buildings before caching (cuts ~4MB → likely <1MB; verify).
- New endpoint `GET /api/enhanced-kgs` → server fetches `query?bbox=<austria>&processed_only=true&limit=1000` (paginate to total), caches 15 min, returns compact list `[{kg_code, gemeinde_code, gemeinde_name, centroid_lon, centroid_lat}]`. This is the "dynamically converging" source for feeling-lucky and for badging KGs as enhanced.
- Extend `generateTreasures`: accept KG codes / N2K info. At session create, query `natura2000/kg/{kg}`; if sites inside, fetch a few `site_parcels` and place **extra rare-species treasures at those parcel coords** (higher value). Keep it non-blocking-ish (small requests, they're fast; or do it in a goroutine + SSE `treasures_updated` event).

### 2. Feeling lucky prefers enhanced
- In `startLucky()`: fetch `/api/enhanced-kgs`, pick a random *gemeinde* from it (dedupe by gemeinde_code), ~90% of the time; fall back to full list otherwise/on error. Show "✨ Enhanced" hint on the loading screen.

### 3. Frontend enhanced-mode data load (fast & fluid rules)
- After the normal KG polygon load, for each loaded KG check membership in enhanced set (`/api/enhanced-kgs` fetched once, kept in `G.enhancedKGs` Set, refreshed every ~10 min). For enhanced KGs, fire **background** fetches (never block loading screen past current steps; update via `render()` when they arrive):
  - `/api/lidar/kg/{kg}` → store per-parcel map `G.lidarParcels[parcel_id] = {elev, slope, aspect, terrain_class, dominant_type, forested_fraction, area_summary}`; per-building heights map `G.lidarBuildings` keyed by rounded centroid (match to cadastre footprints by nearest centroid, they align); `G.topTrees[kg]`, `G.topObjects[kg]` (AFTER filtering by flags).
  - `/api/lidar/flags?kg={kg}&limit=500` → build a Set of flagged obj_refs with severity high/critical; drop matching top_trees/top_objs entries.
  - `/api/cadastre/osm/geometry?kg={kg}&cat=road,water,rail` → `G.osmLines[kg]`.
  - `/api/cadastre/natura2000/kg/{kg}` (+ `site/{code}?geometry=1` for inside sites) → `G.n2kSites` polygons.
- Land prices: on parcel select, `GET /api/cadastre/land_prices/parcel/{id}` (lazy, cached). Optionally batch-prefetch for the selected EZ.
- Perf guardrails: precompute screen-projected paths only in draw fns as today; cull by bbox as existing code does; cap OSM line drawing below zoom 15 to `major` roads only; don't re-parse JSON per frame — parse once into typed arrays where hot (OSM lines: flatten to Float64Array pairs per feature).

### 4. Rendering (Settlers look, improved sprites)
Add to `render()` in this z-order:
- **OSM water lines** (blue, width by fclass) after landuse polys, **roads** (dirt-brown pixel style; major roads wider with lighter center line; rail as dark line + cross ties at zoom≥16) before parcel polys or just after — pick what looks right.
- **Terrain feel without hillshade**: per-parcel elevation tint — use `G.lidarParcels[pid].elev` normalized against KG `elevation_min/max_m` to subtly darken/lighten parcel fill (cheap, no rasters). Optionally slope hatching for `terrain_class` rugged parcels.
- **Building heights**: in `drawBuildingFootprints`, if lidar building match exists, scale `roofOff` by `stories_est` (e.g. 3px/story clamped 2–16) and use `roof_type_hint` — pitched → draw a ridge line/gradient, flat → plain top. This alone massively improves the look.
- **Tallest-tree landmarks**: draw special big tree sprites (new sprite: bigger canopy, trunk, subtle sway animation using existing `Date.now()` pattern) at `top_trees` coords (flag-filtered, height ≤60m), with a small "🌲 42m" label at zoom≥16. Same for tallest object/roof: a small banner marker.
- **Natura 2000 overlay**: translucent green-hatched polygon + dashed border, name label at zoom≥15, habitat emoji badges (🦋 meadow, 💧 floodplain...). Toggleable via a small layer button next to zoom controls.
- **Enhanced badge**: HUD chip "✨ Enhanced Gelände" when camera is over an enhanced KG.
- New sprites: keep pixel-art style (see existing drawTree/drawChestTreasure). Add: tall-tree variant, ridge-roofed building rendering, road texture, N2K rare-species treasure glow (gold ring pulse).

### 5. Parcel popup enrichment
Add rows (only when data present): `Höhe` (elevation_m + range), `Hang` (slope° + aspect octant arrow), `Bewuchs` (dominant_type German + forested %), `Marktwert` (land_prices buy_total_eur, styled — and show game-price vs market-value comparison for flavor), `Natura 2000` (site name + habitats badges), `Lage` (remoteness score → "Abgeschieden/Zentral", dist to road). German labels throughout.

### 6. GPS location on mobile
- Button `📍` in zoom controls (index.html ~line 300). On click: `navigator.geolocation.watchPosition` (needs HTTPS — fine via exe.dev proxy). Draw pulsing blue dot + accuracy circle at `toScreen(lon,lat)`; first fix → `flyTo(lon,lat,17)` if inside Austria bbox (46.3–49.1/9.5–17.2), else toast "Außerhalb Österreichs". Toggle off on second click (clearWatch). Show only if `'geolocation' in navigator`.

### 7. Migrations/queries
Likely NONE needed (treasures reuse species columns; add `treasure_type='n2k_species'` or reuse `species` with higher value). If storing enhanced-KG cache in DB, reuse `api_cache` table via existing queries.

### 8. Mobile-first polish (IMPORTANT — game must work nicely on mobile)
Existing mobile support: bottom-sheet sidebar, touch pan/pinch-zoom (see style.css media queries + touch handlers in initGameInput/initPicker). All new UI must respect that:
- New layer-toggle + GPS buttons join the existing `.zoom-controls` stack — keep tap targets ≥40px, don't overlap the bottom-sheet sidebar or the minimap; on small screens (`@media (max-width:700px)`) verify placement.
- Parcel popup gains several new rows — on mobile it must stay scrollable/compact: hide less-important rows (Lage/remoteness) behind a "Mehr ▸" expander or cap popup max-height with overflow-y auto.
- N2K/enhanced HUD chip: small, top-center, must not block the search bar on narrow screens.
- Perf on phones is the binding constraint: OSM lines + per-parcel elevation tint + taller buildings all add draw cost. Use `devicePixelRatio` capping (max 2), cull aggressively, majors-only roads below zoom 15, and skip elevation tint below zoom 15. Test with browser tool `emulate_device` (e.g. iPhone/Pixel preset) for BOTH layout and fluidity (screencast a pan/zoom to check).
- Geolocation is the flagship mobile feature (walk around your Gemeinde and see where you stand) — make the pulsing dot + follow-mode (re-center on watchPosition updates until user pans manually) work well; battery-friendly: `enableHighAccuracy:false` initially.
- Touch: new interactive map elements (top-tree markers, N2K badges, treasures) need generous hit radii (~24px) in onGameClick.

### 9. Testing & deploy
- `go build -o siedler ./cmd/srv/ && sudo systemctl restart srv`; check `journalctl -u srv -f`.
- Browser-tool test: start lucky game → confirm it lands in an enhanced gemeinde (e.g. Köflach/Gradenberg 63314 area, Graz KGs 63101+, Stallhofen 63301 are processed); verify roads/water render, popup shows Höhe/Marktwert, top-tree sprites appear (and the 213m flagged fake tree does NOT), N2K overlay near Mur river KGs, GPS button appears with mobile emulation (`emulate_device`).
- Perf: `profile_metrics` / just eyeball — pan/zoom must stay smooth with OSM lines on (cull + major-only at low zoom).
- Commit with good messages; update AGENTS.md (new endpoints, new G fields, enhanced-mode section).

## Gotchas
- lidar API docs base says `:8000` — plain `https://srtm-lidar-at.exe.xyz/...` also works; keep `:8000` as documented.
- cadastre llm.txt at `http://` 301-redirects; use `https://`.
- Full-KG JSON `top_trees.height_m` can be garbage (masts) — always flag-filter + clamp ≤60m.
- `/api/v1/query?kg=` (index record) vs `/api/v1/kg/{code}` (full 4MB JSON) — different endpoints.
- Don't call lidar overlay/elevation/segment endpoints during gameplay (45–60s).
- Existing `render()` is called on demand (no rAF loop) except treasure animation via `setInterval` at game.js:4037 — new animated sprites piggyback on that.
- Price calc duplicated Go+JS — market value from land_prices is display-only, do NOT change game economy pricing unless desired (could add a small price modifier for N2K parcels later).
