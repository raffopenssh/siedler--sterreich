# Frontend — `srv/static/game.js` (~11k lines), `index.html`, `style.css`

Single-file vanilla-JS canvas game. Settlers IV pixel art; fonts `Press Start 2P` (headers) / `VT323` (body);
CSS `:root` palette (`--gold`, `--green`, `--panel`, `--bg`); all UI German + runtime EN (`tr()`, see
`tools/i18n/README.md`); `toast(msg,'ok'|'err')`. Static assets `?v=` immutable 1 y — **bump `?v=` in
index.html whenever game.js/style.css/i18n.js change.**

## State

```js
G.player, G.session, G.cam {lon,lat,zoom 13–20}, G.sel, G.ezHighlight {kg,ez}
G.parcelPolys        // parcel Features from /api/viewport (polygon geometry)
G.parcels            // point Features synthesised from parcelPolys (same properties object)
G.buildingFootprints // DKM NS-41 building polygons (footprint_id)
G.landusePolys       // landuse polygons {landuse_code, area_sqm}
G.terrainParcels[pid]  // per-parcel terrain from the cell row (elev, slope, aspect, dom, fracs)
G.ezIndex            // "kg_code-EZnnn" → [features]
G.claimed, G.treasures, G.challenges, G.harvestStates
G.kgsLoaded (Set), G.kgNames, G.terrainKGs, G.enhancedKGs, G.neKGs
G.vpTiles / G.polyIds / G.fpIds / G.luIds   // dedup: cell keys, parcel ids, footprint ids, landuse keys
G.cellState          // per cell: state (loading|pending|ready|down|error|gaveup), xcache, ms — DEV.cells()
```

Screen flow: `welcome` → `pick` (municipality picker) → `loading` → `game` (lobby bypassed,
`startSinglePlayer()`). Loading sequence: [cadastre-cells.md § Client loading](cadastre-cells.md).
Camera in the URL: `syncViewHash()` keeps `#v=lon,lat,zoom`; `startGameWithLoading` applies it — no storage.

## Map rendering (pure canvas 2D)

`toScreen(lon,lat)`, `toGeo(x,y)`, `mapScale() = 2^(zoom-14)*25000`. `render()` order: grass pattern → landuse
polygons → parcel polygons (terrain colour) → point parcels → forest sprites → building footprints →
treasures → EZ highlight → selection → scale bar. Living overlays (nature reserves, forest, treasures, quest
ping, deal FX) draw above the cached base layer (`invalidateBase()`).

- **MultiPolygon.** Parcels with detached parts are MultiPolygon (common and *big* — Almen split by ridges).
  Never index `geometry.coordinates[0]` or bail on `type !== 'Polygon'`; use `geomAllRings(g)`,
  `biggestRing(g)`, `isAreaGeom(g)`, `pipGeom(lon,lat,g)`, `pipRings`, `featureLonLat(f)`. Hit-test **all rings
  even-odd**. Footprints are single Polygons. Go side: `geomRings`, `pipRingsGo`, `bboxOfRaw`.
- **Landuse codes** (BEV Nutzungssymbole): `NS_TABLE` is the single source of truth (code →
  `{abbr,name,terrain,price}`); `LANDUSE_TERRAIN`, `LANDUSE_NAMES`, `ABBR_MAP`, `LANDUSE_POLY_COLORS` derive from
  it. Only 26 codes exist (40,41,42,48,52–65,72,83,84,87,88,92,95,96); never match on label text. 48 =
  Äcker/Wiesen/Weiden, 83 = Gebäudenebenflächen, roads 95, rail 92, parking 42. `extractLuCode()` prefers
  `dominant_ns`, `getLanduseName()` lists shares; `landuse_summary` counts are a fallback weighted by `nsWeight()`;
  server mirror `dominantNS()` (agent.go). Water share = `waterFraction()` from NS 59/60 areas.
- Per-parcel fill prefers `dom_terrain` (`DOM_TERRAIN`, `IMPERVIOUS_DOM` fallback) over cadastre landuse.
  Elevation tint ≥ z15, slope hatching ≥ z16.5. Relief: `drawRelief()` composites hillshade tiles, deliberately
  faint (α 0.34 → 0.10); client `TILEQ` ≤ 8 in flight, centre-first, drops tiles that left `realView()`, fetches
  nothing while the camera moves (`reliefCamSettled` 220 ms), honours Retry-After. `DEV.relief(bool)`.
- **Textures/markers.** `drawTreasure()` (rarity ring `TREASURE_RARITY`, `treasureHitRadius()` ≥ 22 px),
  `drawFieldPattern()` (NS 48, kind from `fieldKindFor`/hash), crop/meadow sprites ≥ z16. Canvas type scale
  `MAP_FONT` (`label` 14px VT323, `small` 11px, `pixel` 9px Press Start 2P) — never ad-hoc sizes. Visual
  restraint: field textures α 0.5/0.35, Wasserschutz = blue wash + dashed edge; colour + sprites carry the map.
- **Austrian border.** `srv/static/austria.json` → `G.atBorder`; `drawForeignShading()`, `drawAustriaBorderLine()`,
  minimap, `updateAbroadBadge()`, `tilesInAustria()`. `insideAustria()` returns **true** while loading.
- **Enhanced badge** `#enhanced-badge`: shown while ≥ 60 % of visible parcel area carries enrichment
  (`enhancedViewShare()`, hysteresis < 50 %).
- Giant trees: hidden until first treasure (`G.tallUnlocked`), then golden hint trees (`G.tallRevealed`);
  claiming a parcel with giant trees awards bonus XP (capped +300). Labels via `labelSlotFree()` +
  `flushTreeLabels()`. KG names: `kgName(kg)`/`ensureKGName(kg)` — never print a bare KG code.

## Performance tier (`PERF`)

Measured, not guessed: `perfNoteBuild(ms)` (CPU time of every completed base build, EMA) and `perfNoteFrame(ms)`.
Build EMA > 450 ms or sustained frames > 34 ms → **low tier** (`perfSlow()`): static nature/forest overlays,
treasure loop 10 fps, flat footprints below z17, crop sprites only ≥ z17, thinned forest filler, halved NE apex
budget. ≤ 2 cores / ≤ 2 GB / reduced-motion start low; recovery after 10 cheap builds. `DEV.perf()`,
`DEV.perf('low'|'high'|null)`, `?perf=low`.

Rules that keep it smooth: never rebuild the base while a zoom gesture is live (`zooming` in `drawCachedBase`);
minimap base keyed on *data*, slid affinely during motion (`MINI.baseW`); living overlays visit
`statefulPolys(claimMap)`, never all polygons; per-feature memos (`f._ter`, `f._veg`, `t._mw`); layout reads cached
400 ms; toponym placement reused ≤ 150 ms while moving; heavy base steps are generators yielding every 64–512
features; each pump slice ends with a 1 px `drawImage` into `_flushCtx` so deferred raster is paid inside the
slice. Wheel: `ZOOM_LEAD` 1.25, reversal drops the queued lead (`smoothZoomBy`), `zoomStep` dt cap 400 ms.
Drills: `python3 tools/perf_wheel.py <cdp-port> <cpu-throttle> [lon lat zoom] [auto|low|high]`,
`tools/cpuprof.py <port> "<js>" [rate]`; port via `ss -ltnp | grep headless-shell`. `DEV.goto` hangs under
`Profiler.start` — set `G.cam` + `loadMoreParcels()` directly in drills.

## Popups, hints, attention budget

- **Parcel popup** (`#parcel-popup`, `showParcelPopup`): `.pp-head` (title + `.pp-sub` KG ▸ / EZ ▸ links) →
  `.pp-body` (only scroller) → `#pp-actions` **pinned** action bar. Essentials grid (Fläche, Nutzung top-2,
  game rows, Besitzer, Preis), the rest folded in `.pp-sec` sections (`det`, `bldg`, `env`) with one-line
  `.pp-sec-sum`; defaults `G.ppSec`, toggles persist in `localStorage siedler_ppsec`. Action bar: `.pp-act-main`
  = one headline action + icon-only 🔍 similar button (`setSimilarBtn`), `.pp-act-row` = secondary. xbrowser
  scenes `popup-actions`, `popup-details`.
- **Hints vs. popups.** Canvas beacons (`edgePoint` → `avoidObstacles`) are clamped to `hudSafeInsets()` and
  steered around open popups (`_hudObstacles`). Desktop popups are corner-anchored, `placePopupClear(id,lon,lat)`
  toggles `.flip`; right-anchored panels use `right:60px`. Phones: every popup is a sheet (grabber, swipe →
  `.peek` → close; `initPopupSheets`). Call `invalidateHudInsets()` after moving a popup programmatically.
- **Attention budget** — the player's attention is a budget; nothing unsolicited spends it while he is busy:
  - **Herald**: unsolicited speech (`hint`, fresh-player `quest`, `completed`) goes through
    `Herald.enqueue({key, lines, mode, autoHide, compact, prio, expires, onShow})` and appears only when
    `Herald.calm()` (no popup/modal, camera settled ≥ 1.2 s, no zoom ease/drag/flow, quiet gap `QUIET` 14 s /
    `QUIET_USER` 30 s). Hints are compact (one-liner, 9 s) and expire unseen after 90 s; completions (prio 2) are
    coalesced. Only the intro and user-initiated briefings (`brief`) use the typewriter. Hint texts ≤ 2
    sentences. `DEV.herald('queue')`, `DEV.herald('hint', key)`.
  - **Toasts**: `toast(msg, type, {quiet:true})` = chatter, shown only when nothing else is live, never queued.
    Queue ≤ 4 (`TOAST_QMAX`), entries > 12 s dropped, `opts.key` replaces a twin, phones ≤ 2 at once.
  - **Beacons** fade while the camera moves; hand-over gap grows 5 → 25 s (`BEACON.rounds`).
  - **Treasures zoomed out** (< `TREASURE_CLUSTER_Z` 15.5): calm sprites, clusters within `TREASURE_CLUSTER_PX`
    36 px merge into one marker (`×N`); tap = `openTreasureCluster` zooms until they part.
  - **Zoom column**: `+`/`−`/`#btn-tools` at rest; Earth, share, N2K, Flurnamen, NE eye, GPS in `#zc-tools`
    (`.zoom-controls.open`; gold dot `has-active` marks non-default modes). Chips fold to icons after 8–14 s.
  - xbrowser scenes `treasure-cluster`, `tools-tray`, `tools-tray-closed`, `herald-compact`.

## Quests → Herald

Built-in quests: `generateChallenges` + `backfillChallenges` (+ `questProgressFor`/`questSatisfied`/
`questProgress`). `GET /api/session/{id}/challenges` returns live `progress/goal`. Tapping a quest →
`Herald.brief(id)` → `questBriefing(c)` + gold `#herald-act` → `questPing(lon,lat,zoom)` (`drawQuestPing`).
QA: `await DEV.quest()`, `DEV.quest(id|title)`, `DEV.quest(id, true)`.

## Living overlays

- **Nature reserves** (`converted_to==='biodiversity'`): `drawNatureReserves(ctx, claimMap)`; `natureScene(f)` =
  hash-stable scene (≤ 2600 samples, `vnoise` clumps, edge-distance succession zones), items with priority `pr`
  drawn when `pr < frac`. Wind `windAt()`, pixel unit `u` via `wildPx()`. Fauna `drawNatureFauna`.
  `natureAnimLevel()` 0/1/2; `NATURE.quality` self-tunes at ~9 ms/frame; rAF only while `NATURE.onScreen>0`.
  `drawSporadicHabitat()` gives ~14 % of ordinary parcels a hive/nest box.
- **Forest** (`drawForestOverlay()` → `forestScene(f,'wild'|'schlag')`): old growth, gaps, Waldmantel; stumps,
  slash, Holzpolter, Überhälter; regrowth items with `birth`; species tables `F_SPECIES_LOW/MID/HIGH`, `fTree()`;
  below z15 plain sprites. Popup `forestPopupRows()`, `G.forestValues[pid]`. `DEV.timber(pid, force)`.
- **Fields/farms/water**: `parcelSchlag(p)`, `fieldKindFor()` (`CROP_GROUPS`), popup `#pp-crop`;
  `drawHofstellen()` (z ≥ 15), `hofOnParcel(pid)`; Brunnen (`wellButtonHTML`, `doDigWell`, `drawWells` z ≥ 16),
  Messstellen (`loadGwPoints` → `drawGwStations` z ≥ 15, `openStation`), Wasserschutzgebiete
  (`loadWaterProtection` → `drawWaterProtection`, gated by `G.n2kVisible`), `fieldEconomy(pid)` → `#pp-eco`,
  `harvestToast`. HUD chip `#water-chip` (`updateWaterChip`), Chronik `#dossier-popup` (`openDossier(kg, tab)`,
  `pxChart()`, `segBar()`). Picker tint `loadPickerGwi` → `gwiTint()`.
- **Wassertropfen-Reise**: `startFlow(lon,lat)` → `G.flow`; MERIT path snapped onto drawn OSM watercourses
  (`refineFlow`, `snapToRiver` ≤ 250 m); `flowAnimLoop()` moves the camera (20 s + 1.5 s/km, ≤ 150 s, zoom 15);
  `#flow-chip`, `clearFlow()`. `DEV.flow(lon,lat)`, `DEV.flowInfo()`.
- **Deal ceremony** (`dealFX(kind, f, coins, hold?)` → `G.fx`, drawn by `drawDealFX`): buy = coins arc from the
  purse (`hudCoinPoint()`), gold outline, banner pole; sell = banner yanked, `VERKAUFT` sign, coins back. Purse
  ticks via `tweenCoins`. `DEV.deal('buy'|'sell', hold, pid?)`, xbrowser `deal-buy`, `deal-sell`.
- **Schildersturm** (hidden, `SMASH`): decorative canvas labels go through `smashDraw(ctx, id, bx, by, w, h,
  paint)`; a precise tap (checked in `onGameClick` after every real tappable, before footprints/parcels) →
  `smashHit` debris + coins `smashCoinsFor(visible_ms)` = `max(1, round(12·e^(−ms/6000)))` (server mirror
  `smashCoins`). `POST /api/smash-label` (≥ 300 ms apart, 400 coins/player/day). Labels regrow after 45–60 s or
  on a big camera move. Only labels drawn live in `renderNow` are smashable — never register from the cached
  base layer. `DEV.smash()`, xbrowser `smash`.
- **EZ**: `G.ezIndex["kg-EZnnn"]` built incrementally by `buildEZIndex()`; `drawEZHighlight()` pulses gold; bulk
  buy via `/api/claim-ez`.

## DEV helpers (`window.DEV`)

Rejoin directly: `/?lang=de&dev=1&pid=<id>&pname=<name>&rejoin=<token>&sid=<session>#v=lon,lat,zoom`
(`dev=1` skips the loading dwell).

```js
await DEV.goto(15.5205, 48.3955, 17.5)   // camera + load cells + wait idle
await DEV.parcel('12105-68/3')           // select + popup (exact id)
DEV.ez('12105', 430); DEV.kg('12105'); DEV.tree(0); DEV.trees('locked'|'hint'|'revealed')
DEV.cells(); DEV.warm(); DEV.lucky(); DEV.timing(); DEV.upstreams(); DEV.state()
DEV.chrome(false); DEV.sidebar(false); DEV.freeze(); DEV.closeAll(); DEV.i18n()
DEV.apex(pid); DEV.bldg(); DEV.hof(); DEV.kgName(kg); DEV.relief(); DEV.ne(pid); DEV.neHeat(mode)
DEV.quest('Schatzsucher', true); DEV.herald('queue'); DEV.perf(); DEV.deal('buy'); DEV.smash()
DEV.dossier; DEV.station; DEV.water(); DEV.flow(lon,lat); DEV.timber(pid, true); DEV.find('68/3')
```

Hi-res captures: `emulate_custom` DPR 2 (1920×1080) or `emulate_device` phone. Screenshot recipes:
`docs/screenshot-recipes.md`; glitch log `docs/glitches.md`.
