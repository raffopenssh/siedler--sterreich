# NE cells — the observed layer (srtm v2.4) in the game

**NE cell** = H3 res-12 cell (~307 m²) with *observed* land cover (LiDAR/satellite) for KGs whose srtm product
is **v2.4**; umfeld holds the *declared* twin (cadastre statistics per cell). The game shows observed vs
declared, and every night we report our own cadastre digests back to umfeld (see [contrib.md](contrib.md)).

## Fetch & cache (`srv/necells.go`)

- srtm `/api/v1/cells?bbox&format=columns&centres=1&layers=obs,trees,structures`, one fetch per 0.02° cell
  (`ne:v1:i:j`, 24 h, hot LRU 24 parsed). Per cell: cover[9 groups], canopy, LiDAR heights, NDVI, change,
  terrain, `consistency` (observed vs declared), **every tree apex ≥ 3 m** (species, vitality), **every
  structure** (type, heights).
- 404 = KG not processed (negative 1 h, `retry_after_s`); `meta.partial` for mixed cells; `meta.kgs_missing`
  (union) + `kgs_pending` (poll after `meta.retry_after_s`) + `kgs_not_processed`. A 200 cell with pending KGs
  is cached only until the hint (30 min–6 h), a 404 `status:"pending"` likewise (≥ 5 min). Relayed as
  `ne.kgs_pending` on `/api/viewport` and `/api/ne`.
- `neEnrichParcel` fills the legacy terrain fields **and** `ne{}` (`neParcel`): `ne{cells, cover{geb,bau,acker,
  gruen,wald,wasser,verkehr,alpen,sonst}, canopy, h_max_m, tree_n, tree_h_max_m, trees_tall, species{},
  vitality{}, structures_n, structures_cover, structure_h_max_m, structure_types{}, consistency{code:share},
  verdict, phenology, ndvi, dh_m, forest_loss_year, als_years, epoch}`. `neGroupLC` maps the 9 groups to
  roof/parking/crop/grass/tree/water/road/rock/bare_soil.
- Parcel `verdict` only flags *big* changes: land-use ≥ 35 % & ≥ 3 cells, built ≥ 15 % & ≥ 2 cells —
  **`vp:v1` cells must be purged when this rule changes**.
- `neReadyKGSet()` = registry `v24`; `neConfirmedKGSet()` = minus cells observed without NE
  (`srv/neobserved.go`, see cadastre-cells.md § Lucky).

## Derived endpoints

- `/api/trees` for an aligned cell of a v2.4 KG = every NE apex ≥ 4 m (`neTreeMinH`; ≤ `neTreeLimit` 6000 per
  cell, **thinned spatially** — k-th tallest per H3 cell first, so hedgerows/orchards survive; tallest first;
  cache `trees:ne:v2`) with `species`, `vitality`, `source:"ne-cells"`. Unprocessed KGs only have the legacy
  ≥ 20 m index. Client draws ≥ 10 m below z16, ≥ 6 m below z17, everything at street level.
- `/api/buildings` every structure (`type`, `area_m2`, `dh_m`; walls/fences dropped); partial cells merge legacy
  rows for the uncovered part (`legacy_rows`).
- `GET /api/ne?west..north&v=2` → heat columns (lon/lat, cover, canopy, h_max, consistency, phenology) as plain
  int arrays (never `[]uint8` — Go would base64 it; cache `neheat:v2`).
- `/api/kg-summary/{code}` carries `ne{cells, coverage, cover{}, canopy, tree_n, trees_tall, tree_h_max_m,
  structures_n, consistency{}, discrepant, forest_loss, forest_loss_last, species{}, structure_types{}, epoch}`
  = `neKGSummary(kg)` over cached NE cells (no fetch).

## Client

- `apexVariant()` uses species/vitality (dead/declining → snag); `lidarForFootprint()` prefers `props.ne`;
  popup `neRows()` ("👁 Beobachtet" verdict `NE_VERDICT`, Bäume, Bauwerke, Satellit, Veränderung);
  `#map-attrib-ne` row via `noteNE()`; `DEV.ne(pid?)`.
- **Heat overlay** `#btn-ne` (pixel eye in the zoom column, hidden until `noteNE` sees NE data; cycles off →
  `consistency` → `canopy`): `setNEHeat(mode)` → `loadNEHeat(c)` per cell → `G.neHeat[key]` typed arrays →
  `drawNEHeat()` in the cached base layer (H3-res-12-sized hexes, part of `baseSignature`). Consistency paints
  only discrepant cells (`NE_HEAT_COL`), canopy a teal wash; cached radial splat sprites (`neSplat`). Minimap:
  one dot per discrepant parcel. `DEV.neHeat(mode?)`.
- KG card **👁 Spähbericht** (`kgObservedHTML`: cover bar + 4 stat tiles; the Abweichung tile toggles the
  overlay). Picker: `G.neKGs` / `enhancedGemeinden[].ne` → green ring + "👁 beobachtet" badge,
  legend `#pick-ne-legend`.

## Mechanic

- `parcel_claims.ne_verdict` (migration 016) = `neVerdictOf(pid,lon,lat)` from the cached cell at claim time
  (client `doClaim` sends `lon/lat`). `neDiscrepant(verdict)` (not consistent/unknown) → **+60 XP Spurenleser**
  on claim (`ne_bonus_xp`, `ne_verdict` in the answer) and quest **"Spurenleser"** (`challenge_type: observe`,
  hidden until `G.neCells>0`); `convert biodiversity` on a `forest_loss` claim pays 200 XP (`ne_restore`).
- Agent: look rows carry `ne_verdict, ne_canopy, ne_tree_n, ne_structures_n, bonus_spurenleser_xp` (`neBonusXP`);
  inspect `terrain.observed` + narration, `game.actions[].bonus_spurenleser_xp`.
- xbrowser scenes `ne-heat`, `ne-heat-canopy`, `ne-popup`, `ne-quest`, `ne-back` (Kohlschwarz 63330).
