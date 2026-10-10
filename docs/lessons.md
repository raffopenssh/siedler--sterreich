# Lessons learned — things that went wrong once; never reintroduce

Each entry: what happened → the rule that now exists. Code and other docs describe the current state only;
this file is where the "why" lives.

## Cadastre / cells
- **Stitched viewports produced phantom change.** 678 "changed" NE chunks were artefacts: a wider viewport held
  a parcel whole that a narrower one held truncated; bevdirect's multi-cell `/viewport` also keeps one truncated
  copy of parcels wider than cell + pad. → Reports use **aligned cells only** with `--input-bbox` = cell union.
- **`ready:false` treated as empty** blanked maps. → Never treat 202 / `ready:false` / `pending:true` as "no
  data", never cache it.
- **Polygon loading gated on zoom/span** (in device pixels) hid parcels. → Never gate on `viewBounds()`.
- **`geometry.coordinates[0]`** dropped Alm parts; MultiPolygons are common and big. → `geomAllRings` & co.
- **Registry said v2.4, `/cells` had nothing** (61 of 354 flagged KGs; two lucky players landed on empty NE).
  → `ne-obs` notes; lucky uses `neConfirmedKGSet()`; `spawnNEOK`.
- **Re-fetching cells on `/ez` mismatch** would defeat the cache that keeps BEV tile load low. → Accepted
  24 h staleness (`building_count` may differ from live).

## Warming / load
- **Whole v2.4 universe kept warm** (1 300+ KGs) with nobody playing → 72-min RX spike, GBs/day of tiles.
  → Activity tiers; idle = every 5th patch; never keep the universe warm.
- **Contrib fill drew unwarmed KGs first** → ~416 KGs/night, 1 577 live cell builds, 128 too-small KGs, 5.3 h,
  done 07:22. → `cheap[]` (prewarmed) KGs are the fill; small KGs skipped from the plan; cap 200 on fresh tiles.
- **Night run re-assembled every cell the prewarm had just built** (bevdirect 140 % + python 70 % on 2 vCPUs for
  2 h). → RAM stash of raw bevdirect docs, `ne_report.py --siedler` reads it.
- **Contrib running by day** competed with players. → 22:00 prewarm, 01:00 report, nothing by day;
  `CPUQuota=100%` + `Nice=15`.
- **bevdirect install.sh overwrites the unit** (root, 160 cells, prefetch) and does not restart. → Restore unit
  settings after every upgrade (providers.md).
- **Session create waited on umfeld** (3.8–6.3 s under load). → Never touch upstreams on the create path;
  `prememoSettlements`, treasures in goroutines.
- **Farm bursts** of cold cells 503'd and tripped the breaker. → `hostSlots` 3 per farm host, busy ≠ down.
- **srtm hillshade 429s** at z10–11. → Tile gate ≤ 8 in flight, z < 12 as 204, 1 y cache with revalidation,
  client fetches nothing while the camera moves.

## Frontend
- **"Ruckelig in Wien"**: the base layer was rebuilt every frame during the zoom ease and the minimap re-keyed
  on camera. → Never rebuild the base while zooming; data-keyed minimap; generators + `_flushCtx`; `PERF` tier.
- **Screen too busy** (hints, toasts, beacons, treasure sparkle all at once). → Attention budget: Herald queue
  with calm gate, quiet toasts, beacon fade, treasure clusters, folded tool tray.
- **Labels unreadable / overlapping** (dark-on-green, 9 landmark banners on one Stift). → pill-backed tags,
  `labelSlotFree()`, landmark banner layer removed.
- **Session centre 2 km from the village** (Gemeinde centroid). → `settlementCenter()` snaps to the OSM place.
- **`stories_est` from srtm** was ridge height / 3. → storeys = `mean_height_m / 2.9`.
- **Smashable labels registered from the cached base layer** had wrong coordinates. → Only live `renderNow`
  labels register.

## Data hygiene / legal
- **`.nec` containers were never pruned** (1 216 files, 829 > 24 h, found 2026-10-10). → run.sh prunes them;
  derived NE statistics may live on disk, raw/assembled cadastre may not (providers.md § hygiene).
- **`footprints: null`** from bevdirect ≤ v0.3.2 crashed the frozen `ne_cells.canon`. → fixed upstream in
  v0.3.3; client normalises older servers.
- **`ne_cells build` ignores `truncated`.** → our driver checks `ready` *and* `truncated`.

- **2026-10-10 — cadastre cells were on disk.** `api_cache` held 2 931 `vp:v1` rows (810 MB gzipped) with
  raw parcel ids, NS codes and geometry, plus the `parcel:v1:<id>`/`ez:v1` code paths — a rule-2 violation
  hiding behind "≤ 24 h". Fix: `srv/ramcache.go` + prefix routing in `Store`; boot purges strays and
  re-queues previously warm KGs. Lesson: a TTL is not a storage tier — route by key prefix at the store, not
  by discipline at each call site. Second pass the same day found `similar:v7` (77 rows with parcel ids),
  `bldg-info:v2` and `timber:<hash>` (parcel_id in the body) on the same path → added to `ramPrefixes`.
