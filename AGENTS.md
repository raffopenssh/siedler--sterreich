# Siedler Österreich — Agent Guide

Multiplayer browser game on real Austrian cadastre data: explore, claim parcels, convert land to nature
reserves. Settlers IV pixel-art look. Go `net/http` + SQLite (sqlc) backend, single-file vanilla-JS canvas
frontend, no frameworks. We also run a public-good side job: every night we report cadastre-change digests of
~200 KGs to umfeld-at ("contrib"). Live `https://siedler-oesterreich.exe.xyz:8000/`.

This file = rules + mental model + map. Details live in `docs/` — **read the one topic file your task touches**
(index: `docs/README.md`), not all of them.

## Commands

```bash
go build -o siedler ./cmd/srv/ && sudo systemctl restart srv     # build + deploy (:8000)
journalctl -u srv -f                                              # logs
go generate ./db/...                                              # after editing db/queries/*.sql
go build ./... && go vet ./srv/... && go test ./srv/...
node tools/i18n/audit.js srv/static/game.js; tools/xb.sh --quick # UI checks (docs/ops.md)
```

## Hard rules (violating one of these has bitten us before — `docs/lessons.md`)

**Data & legal**
1. The browser never talks to a data host; every upstream call goes through our proxies via
   `upstreamGet`/`upstreamClient` (never `http.Get`, never hand-set `Accept-Encoding`).
2. **Kataster-derived data is RAM-only on our side** (raw BEV tiles, bevdirect `/viewport|/parcel|/ez` docs,
   assembled cells, NEC1 containers): ≤ 24 h in memory, nothing on disk, nothing in git, nothing in `/tmp`
   when you finish. `vp:v1` cells and `parcel:v1`/`ez:v1` look-ups live in `cadastreRAM` (`srv/ramcache.go`) — `Store`
   routes those key prefixes away from SQLite; `.nec`/work files (derived statistics) are pruned > 24 h
   (`docs/providers.md` § hygiene). NE cells (srtm), heightfields, hillshade, umfeld context are not cadastre and may be cached.
3. The DB stores **no cadastre ids** — HMAC `parcel_hash`/`ez_hash` only (`srv/parcelhash.go`).
4. Every cadastre-derived answer carries the BEV `notice`; legal pages (impressum/imprint/datenschutz/privacy,
   `/api/licenses`) must describe what the code actually does.
5. Never persist or re-feed the whole KG universe into warming or contrib (`v24_kgs` → run.sh is forbidden);
   the contrib plan and the activity-tiered warm plan are the only KG sources.

**Cells & upstreams**
6. The 0.02° **aligned cell** is the unit of everything cadastre (`floor(lon/0.02), floor(lat/0.02)`); anything
   reproducible (NE reports) uses one `/viewport` per aligned cell, never a stitched bbox.
7. `ready:false` / `pending:true` / HTTP 202 means **still assembling, never "no data"** — relay with
   `Retry-After`, never cache, never show an empty map for it.
8. Prefer bbox/cell endpoints over per-id loops; quantise anything that fires on every pan or the cache never
   hits; anything > 3 s per call is out. Session create never touches an upstream.
9. KG codes are 5-digit strings with a leading zero; compare with `unpadKG`. Never print a bare KG code to a
   player (`kgName`/`ensureKGName`).

**Frontend**
10. Bilingual DE/EN is mandatory: German is canonical in markup/code/server; EN via `srv/static/i18n.js`
    (`tr()` on the *final* string, `I18N_EXACT`/`I18N_RX`). Run the three checks in `tools/i18n/README.md`
    before committing UI text; add an xbrowser scene for every new popup/overlay.
11. **Bump `?v=` in index.html whenever game.js / style.css / i18n.js change.**
12. Geometry is MultiPolygon-aware everywhere (`geomAllRings`, `pipGeom`, `featureLonLat`; never
    `coordinates[0]`). Never gate polygon loading on zoom/span. `insideAustria()` is true while loading.
13. Never rebuild the cached base layer during a live zoom gesture; living overlays visit `statefulPolys`, not
    all polygons; canvas text uses `MAP_FONT` sizes only.
14. Unsolicited UI (hints, completions, chatter toasts) goes through the attention budget (`Herald.enqueue`,
    `toast(…,{quiet:true})`) — never straight to the screen.
15. Server/client mirrors must stay in sync: `calculatePrice`↔`calcPrice`, `regenFactor`↔`regenOf`,
    `cropCycle`↔`cropCycleS`, `smashCoins`↔`smashCoinsFor`, `dominantNS`↔`extractLuCode`, `/openapi.json`↔agent
    endpoints, `contribNightHour`↔`ne-report.timer`, `bevdirectVersion`↔impressum/imprint.

**Process**
16. Docs describe the current state only; history goes to `docs/lessons.md` / `docs/migration-2026-10.md`.
    Update the topic doc that owns the behaviour you changed; touch this file only for rules/mental model.
17. Commit with good messages before returning to the user. Delete debugging dumps first.

## Mental model

```
 browser (game.js) ── /api/* only ──► our Go server (srv/) ──┬─► bevdirect-serve :8787  (cadastre: BEV tiles → cells, RAM)
                                                            ├─► umfeld-at                (context, declared NE twin, contrib sink)
                                                            ├─► srtm-lidar-at            (NE observed cells, landscape, hillshade)
                                                            └─► holz / farm / gw         (timber, INVEKOS, water)
```

- **Cell = unit.** `/api/viewport` builds one aligned cell: bevdirect doc + srtm NE cell + heightfield in
  parallel (`buildCell`), cached only when ready (≤ 24 h, key `vp:v1:i:j`), served pre-gzipped from a hot LRU.
  Every server-side parcel look-up (agent, similar, lucky interest, parcel-find, treasures) reads cached cells
  (`srv/cellstore.go`), never the upstream directly. Cold cell → 202 + `Retry-After`.
- **Warming** keeps a rotating ~100 KGs/day of cells warm (activity-tiered: idle = 1/5 of that), plus neighbours
  of what players load and the Gemeinde of each new session. `kg_warm` says what is warm; `/api/lucky` spawns
  only in warm, srtm-enhanced, NE-*confirmed* Gemeinden at the most visually interesting spot.
- **NE (observed) vs cadastre (declared).** srtm v2.4 KGs carry per-H3-cell observed land cover, every tree
  apex and every structure; the game shows discrepancies (verdict, heat overlay, Spurenleser XP). Registry
  (`srv/kgregistry.go`) + universe check (`srv/kguniverse.go`) decide which KGs are v2.4; adoption purges and
  re-warms them.
- **Contrib (nightly, Vienna time).** 22:00 the server prewarms tonight's ~200 KGs (plan from
  `/api/contrib/plan`, whole 7 850-KG universe over a quarter, small KGs skipped) and keeps the *raw* bevdirect
  docs in a RAM stash; 01:00 `ne-report.timer` rebuilds the declared cells with the frozen `ne_cells` package from
  the stash and POSTs digests + changed chunk statistics to umfeld. Nothing contrib-related runs by day.
- **Resilience.** Per-host circuit breaker (`srv/breaker.go`) answers synthetic 503 `status:"down"` in ~0 ms;
  `status:"busy"` for rate limits; client layers reschedule, only a cadastre outage toasts.
- **Game state** lives in SQLite keyed by hashes; the client maps hashes back from loaded geometry
  (`resolveClaims`). Real-time via SSE `s.broadcast`.
- **Frontend** is one canvas: cached base layer (landuse → parcels → footprints) + living overlays (nature,
  forest, treasures, deal FX) + HUD; `PERF` tier measured from real build/frame times; `#v=lon,lat,zoom` in the
  URL is the only persisted camera.

## Repo map

```
cmd/srv/main.go           entrypoint, flags
srv/server.go             routes (Serve()), game actions, SSE, cachedFetch, middleware
srv/viewport.go           /api/viewport cell build + hot LRU         srv/cellstore.go  cached-cell readers
srv/upstreams.go          provider URLs, 0.02° grid, admin table     srv/breaker.go, upstream_pending.go
srv/warm*.go              warming (plan, tiers, boost, spread)       srv/lucky*.go, neobserved.go  spawn picking
srv/necells.go            NE cells fetch/enrich, /api/ne, trees/buildings from NE
srv/landscape.go          srtm public-tier adapters, enhanced-kgs    srv/kgregistry.go, kguniverse.go
srv/contrib.go            nightly plan + 22:00 prewarm               srv/contrib_stash.go, contrib_stats.go
srv/siblings.go           farm/holz/gw proxies, hostSlots            srv/water.go, dossier.go, timber.go
srv/harveststate.go       regrowth value                             srv/treasures.go, smash.go, similar.go
srv/agent*.go, discover.go, llmahead.go, leaderboard.go   agent surface
srv/search.go, tiles.go, metrics.go, parcelhash.go, licenses*.go, cachestore.go, safety.go, ratelimit.go
srv/static/game.js        entire frontend (~11k lines)   index.html all screens   style.css   i18n.js
srv/data/admin.json.gz    7 850 KGs (embedded)           data/*.seed.json.gz  registry/universe mirrors
db/migrations/NNN-*.sql   auto-applied; db/queries/game.sql → sqlc → db/dbgen/
tools/ne-report/          contrib pipeline (python venv, systemd units)   tools/xbrowser, tools/i18n, tools/soak.sh
```

## Workflows

- **New feature**: migration → sqlc query → handler (register in `Serve()`) → game.js → index.html (+ i18n
  entries, xbrowser scene) → bump `?v=` → build, restart, check in the browser → update the topic doc → commit.
- **New map layer**: server proxy via `bboxProxy`/`bboxLayer` (quantised key, relay `ready:false`), client
  loader via `loadBboxLayer`/`loadPointLayer` from `loadFastLayers(c)`, `G.x`, `drawX(ctx)` at the right z-order
  in `render()`, `DEV.x()`.
- **New proxy**: `s.cachedFetch(w, key, fetch)` — singleflight, `X-Cache`, never caches errors/202; fetch
  closures use `context.Background()`.
- **bevdirect upgrade / contrib change / warming change**: read `docs/providers.md` / `docs/contrib.md` /
  `docs/cadastre-cells.md` first — each has the exact recipe and the numbers.
- **QA**: `DEV.*` helpers (`docs/frontend.md`), rejoin URL
  `/?lang=de&dev=1&pid=&pname=&rejoin=&sid=#v=lon,lat,zoom`, `tools/xb.sh`, `tools/soak.sh`, outage drill =
  stop `bevdirect-serve`.

## Style

Settlers IV pixel art; `Press Start 2P` headers / `VT323` body; CSS `:root` palette; German UI + `tr()`;
`toast(msg,'ok'|'err')`; phones get bottom sheets; visual restraint — colour + sprites carry the map, patterns
are hints. Communicate with brevity in commits and docs.
