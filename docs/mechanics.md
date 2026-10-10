# Game mechanics, API, database

## Database (SQLite, WAL, busy_timeout 5 s, `SetMaxOpenConns(8)`)

Tables: `players` (coins start 10000, xp, level, rejoin_token, agent), `game_sessions`, `session_players`,
`parcel_claims` (hashed parcel/EZ, kg_code, area, landuse, converted_to, purchase_price, harvested_at, harvests,
well_at, well_depth_m, ne_verdict), `treasures`, `challenges`, `chat_messages`, `offers`, `api_cache` (hourly
`cacheJanitor` prunes expired rows + `PRAGMA incremental_vacuum`), `kg_warm`, `parcel_harvest_state`.

- `s.Q` is `Store` (`srv/cachestore.go`) wrapping sqlc: cadastre keys (`vp:v1:`, `parcel:v1:`, `ez:v1:`) are
  routed to the RAM-only `cadastreRAM` (`srv/ramcache.go`) and never reach SQLite; other api_cache bodies > 2 KB are stored gzipped (magic
  detected on read) — never read `api_cache.data` with raw SQL expecting JSON (column is `cache_key`, `data`,
  `fetched_at`, `expires_at`, `etag`).
- Migration: `db/migrations/NNN-name.sql` ending with `INSERT OR IGNORE INTO migrations (migration_number,
  migration_name) VALUES (NNN, 'NNN-name');` (auto-applied on startup). Query: edit `db/queries/game.sql`
  (`-- name: X :one|:many|:exec`) → `go generate ./db/...` → `s.Q.X(ctx, …)`.
- No cadastre ids in the DB: HMAC `parcel_hash`/`ez_hash` (`srv/parcelhash.go`).

## API endpoints

**Auth.** `POST /api/register` (returns `rejoin_token` once; 409 carries `suggested`), `GET /api/suggest-name`.
Mutating calls need `X-Player-Token` (`authPlayer`; client `api()` sends `G.playerToken`). `Player.RejoinToken`
is `json:"-"`.

**Session.** `POST /api/session/create|join`, `GET /api/session/{id}`,
`/players|parcels|treasures|challenges|biodiversity|chat|offers|harvests`, `/events` (SSE).

**Actions.** `POST /api/claim-parcel` (parcel_id, kg_code, gnr, ez, area_sqm, landuse, building_count,
total_building_area, tall_tree_*, gw_station, lon, lat, crop_group), `/claim-ez` (20 % off, ≤ 100),
`/convert-parcel` (biodiversity|forest|wildforest), `/sell-parcel` (60 % × regrowth), `/harvest-parcel`,
`/harvest-forest`, `/dig-well`, `/claim-treasure`, `/complete-challenge`, `/offer-parcel`, `/offer-respond`,
`/smash-label`, chat + rules + block/report.

**Geo/data proxies.** `/api/viewport`, `/api/viewport-landuse`, `/api/lucky`, `/api/warm/status`,
`/api/kg-geo/{kg}`, `/api/municipality?lon&lat`, `/api/municipalities?q=|list=all|state=&format=geojson`,
`/api/search-index`, `/api/parcel-find`, `/api/parcel-context`, `/api/osm-lines?…&cat=road,rail,water`,
`/api/n2k`, `/api/enhanced-kgs`, `/api/lidar/kg/{code}`, `/api/lidar/*` (slow paths blocked), `/api/trees`,
`/api/buildings`, `/api/landmarks`, `/api/landscape`, `/api/ne`, `/api/similar`, `/api/forest-value`,
`/api/building-info`, `/api/kg-summary/{code}`, `/api/schlaege`, `/api/hofstellen`, `/api/water/*`,
`/api/well-quote`, `/api/field-economy`, `/api/dossier/{kg}`, `/api/tiles/hillshade/{z}/{x}/{y}`, `/api/umfeld/*`
(only `umfeldPublicPrefixes`; anything else 404), `/api/licenses`, `/api/contrib/plan|stats|cell`.

**Ops.** `GET /api/metrics` (route p50/p95, cache hit ratio, bevdirect health, tiles, upstream_down_503,
kg_universe.alert), `GET /api/warm/status`, `GET /api/upstreams` (breakers), `POST /api/warm/run-plan|trim`.

**SSE** `s.broadcast(sessionID, data)` → client `handleEvent(d)`: `parcel_claimed`, `parcel_converted`,
`parcel_sold`, `parcel_harvested` (`forest`, `meadow`, `drought`), `ez_claimed`, `treasure_claimed`,
`treasures_updated`, `well_dug`, `player_joined`, `challenge_completed`,
`offer_made|accepted|rejected|funds_needed`, `chat`, `chat_mode`, `chat_hidden`, `chat_refresh`.

## Pricing

`calculatePrice(areaSqm, landuse, buildingCount, totalBuildingArea)` (server.go), mirrored by JS `calcPrice()`
— keep in sync. Base €/m² per NS code `nsBasePrice` = `NS_TABLE.price` (Gebäude 0.5 … Wald 0.2 … Straße 0.1 …
Gewässer 0.05, Fels 0.03; unknown 0.15). Density: built-up ratio > 0.3 = 2×, 0.05–0.3 = 1–2×, none = 0.5×.
Clamped 10–5000 coins. Claim price = `calculatePrice × regen` (below).

## Harvest state & regrowth value (`srv/harveststate.go`)

- **Harvest state outlives ownership.** `parcel_harvest_state(session_id, parcel_hash, kind forest|field|meadow,
  crop_group, harvested_at, harvests)`. Written on every harvest (`recordHarvestState`) and on sale, seeded into
  the next claim (`inheritedHarvest` → `seedInheritedHarvest`, also `/api/claim-ez`; stale rows — forest
  > 510 min, field > 4 h — ignored). Client `G.harvestStates`; **`harvestOf(pid, claim)`** is the single
  accessor (`fieldStage`, `forestStage(hv)`, overlays, popup). Offers transfer the claim row in place.
- **Value follows the stand/crop.** `regenFactor(kind, harvestedAt, pid, crop, now)`: forest
  `0.4 + 0.6·min(1, min/510)`, crop field `0.75 + 0.25·progress`, meadows/others 1. Sell =
  `sellPrice(purchase, regen)` = 60 % × regen. Client mirror `regenOf(p, claim)` / `regenLabel` / `sellQuote`
  (popup row **Wert** `#pp-regen`; `REGEN_FLOOR_*` must match). Agent inspect `game.regrowth{}`.
- **Per-crop cycles.** `cropCycle(crop, kind)` / JS `cropCycleS`: Getreide 60 min, Mais 90, Feldfrucht 45, Obst
  180, Wein 240; hash kinds 60/90/150; meadow Förderung `fieldCycle` 60; timber `forestFullValueMin` 510. Phase
  offset hash-based (mod 60 min) so neighbours ripen at different times.
- Selling a converted parcel drops the protection and takes the conversion XP back.

## Forest plots — Holzernte & Naturwald (`srv/timber.go`)

`claimIsForest` (NS 56 or tree cover ≥ 50 % on non-crop) swaps the generic buttons for:
- **🪓 Holzernte** `POST /api/harvest-forest`: coins = net timber value / `eurPerCoin` (10); stand regrows
  (`forestPhase()`/`forestStage()`: Schlag < 40 min → Jungwuchs < 90 → Stangenholz < 150 → Baumholz, value
  50 → 100 % by 510 min). Quest "Holzknecht". Pays the cached popup estimate (`timber:<pid>`).
- **🌳 Naturwald** `POST /api/convert-parcel` `wildforest`: permanent, Baumholz only, XP `120 + min(180,
  Vfm/10)`, counts toward the 30 % bio target. Quest "Waldhüter".

Estimate `GET /api/forest-value?parcel_id&kg&area&lu&session_id&west&south&east&north` (singleflight per parcel,
cached 1 h / 10 min heuristic): `standFacts` (cell row enrichment + srtm `/landscape?bbox&include=trees`; NS 56
defaults 85 % canopy, h 18 m; species mix by elevation) → stock `Vfm/ha ≈ 0.9·h_mean^1.95` × canopy ha, `Efm =
0.8·Vfm`, CO₂ ≈ 0.9 t/Vfm → holz `plotHistory()` (`estimate.history`) → `timberStatePrices()` (`kgState()` maps
KG prefix → Bundesland; Wien → NÖ) → harvest cost 28/36/45 €/Efm by slope + 150 € fixed.

## Fields, water, Chronik

- Field cycle 60 min (`FIELD_CYCLE_S`); `doHarvest()` sends `crop_group` (server `fieldPhaseAtCrop`,
  `cropMeadow`). Harvest payout = crop × drought factor (+ well protection) + Förderung (`subsidyCoins` floor
  5🪙); Naturschutz × 1.5 XP in a Wasserschutzgebiet; +80 XP Pegelwart when a gauge sits on the parcel
  (`stationOnParcel(pid, lon, lat)`: cached cell + gw `/llm/points` + PIP, 1.5 s budget).
- `srv/water.go`: well quote / `POST /api/dig-well` / `/api/field-economy`; `handleWaterFlowpath` prewarms the
  KGs under the reach chain.
- Chronik: `GET /api/dossier/{kg}` (providers.md) → HUD chip, `#dossier-popup`, sidebar `#sb-chronik`.

## Treasures, giant trees, quests

- `srv/treasures.go`: N2K species treasures + parcel treasures placed in goroutines after session create
  (prefers parcels ≤ 3 ha, dedupes positions), `treasures_updated` SSE; roaming species (Durchzügler) have a
  45 min lifetime and are never compass targets afterwards. Rarity `TREASURE_RARITY`.
- Giant trees unlock after the first treasure (`treasures_found` on `GET /api/player/{id}`); bonus XP on claim
  from `tall_tree_count/max_h` (cap +300).
- Quests: `generateChallenges`/`backfillChallenges`, `autoCompleteChallenges` swept on every `GET …/challenges`;
  NE quest "Spurenleser" (ne.md), "Holzknecht", "Waldhüter", "Naturschützer", "Schatzsucher". Client side:
  frontend.md § Quests → Herald.
- Schildersturm coins: `smashCoins` server mirror of `smashCoinsFor`; ≥ 300 ms between smashes (429), 400
  coins/player/UTC day (in-memory guard).

## EZ (Einlagezahl)

Land-register folio grouping parcels under one owner. Client `G.ezIndex["kg-EZnnn"]`; selecting shows
count/area/ownership; bulk buy via `/api/claim-ez`. Agent inspect reads the live bevdirect `/ez` for the folio.
