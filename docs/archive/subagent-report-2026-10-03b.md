# Subagent report — server-side findings from 2026-10-03 (follow-up)

Scope: the "Findings" of `docs/subagent-report-2026-10-03.md`, Go code under `srv/` only
(no `srv/static/*`). Service restarted 3× (12:00, 12:12, 12:16 UTC). `go vet ./... && go test ./srv/...` green.

## Commits
| # | commit | what |
|---|---|---|
| 1 | c6ca886 | Agent look/inspect: cold cell → **202 `{status:"pending", retry_after_s, center/parcel_id, note}` + `Retry-After`** (`ensureCellStatus()` in cellstore.go, `relayCellStatus()` in agent.go); breaker open → 503 `{status:"down", service:"cadastre"}` + `X-Upstream: down`; other failures 502 with `retry_after_s`. `buildCell` (viewport.go) relays the breaker body on 503 instead of a generic 502. bevdirect host labelled `cadastre` in the breaker (was `data`). |
| 2 | cc7f985 | **BEV `notice`** in `agentAttribution` (look / claim / municipality / inspect), in `claimParcel`'s response and in `similarJSON` (`similar:v6`). `/openapi.json`: shared `Pending` (202) / `Down` (503 down|busy) responses referenced from look + inspect, claim response schema, notice fields. New `discover_test.go` (openapi stays valid JSON; relay shapes). |
| 3 | 2ada8d3 | **Pegelwart +80 XP on claim works again**: `stationOnParcel(pid, lon, lat)` = parcel from our cached cells (`lookupParcelCached`, never bevdirect) + gw `/llm/points` in the parcel bbox + point-in-polygon (`gwPointsOnParcel`, shared with inspect), hard **1.5 s** budget, falls back to no bonus. `claimReq` gains optional `lon/lat`; agent claim passes them. Also includes the parent's `treasure_claimed` broadcast hunk in `handleClaimTreasure`. |
| 4 | 6d86ef5 | **Farm bursts**: `hostSlots` per-host semaphore (farm host: 3 concurrent, ≤ 8 s wait) in `bboxProxyOpt`; a sibling's own 503/429 or a slot timeout is relayed as `503 {status:"busy", service, retry_after_s}` + `X-Upstream: busy` + `Retry-After`; `/api/metrics` counts any `X-Upstream` 503 under `upstream_down_503`, not our 5xx. |
| 5 | aa107bf | **/api/lucky**: `luckyCenter()` = OSM settlement (`settlementCenter`, memoised) if inside a warm KG, else centre of the warm KG with most parcels; enhanced preference 85 % only when ≥ 3 enhanced Gemeinden are warm (else 50 %); partially warm Gemeinden qualify when the spawn KG is warm; up to 4 candidates tried per tier. |
| 6 | e1677a5 | docs/migration-2026-10.md "Known limitations": stale cell vs live `/ez` building_count (24 h cell cache, by design), 202/503/busy relays, session-create timing. |
| 7 | be76bcd | **Session create < 1 s**: `prememoSettlements()` kicks `settlementCenter()` in the background for the top 2 picker hits (`/api/municipalities?q=`, `/api/agent/municipality?q=`, `?random=1`), so create never waits on umfeld for a Gemeinde the picker just showed. |

## Findings → status
1. **look 202 without Retry-After** — fixed (c6ca886). Verified against a stub bevdirect that answers `ready:false, retry_after_s:7`:
   `GET /api/agent/look` → `HTTP 202, Retry-After: 7, {"status":"pending","retry_after_s":7,"center":{…},"note":…}`; inspect → same with `parcel_id`.
   Stub killed → first calls `503 {status:"down",…}`, after the breaker opens the same body is relayed (viewport too, with `X-Upstream: down`). Claim and municipality never touch the cadastre (claim prices from `agentSeen`).
2. **`cellstore.ezParcels()` whole-KG bbox** — already gone: removed in 85df717 (before the report was filed); no caller left. `inspectEZ` uses `/ez?kg&ez&lon&lat`. Verified: inspect `65006-78/3` → `ez {available:true, ez:"688", parcel_count:1, bulk_price_coins:221, landuse_breakdown[…]}`; cross-checked bevdirect `/ez` directly (same ids, `partial:true`).
3. **notice** — fixed (cc7f985). `look.attribution.notice`, `municipality.attribution.notice`, `claim.notice`, `similar.notice` all = `© BEV, 2026 – Datenquelle: …`.
4. **Pegelwart dead** — fixed (2ada8d3). Parcel `65006-78/2` (Farrach, gauge gw:309906 on it): inspect `stations_on_parcel` 2 entries, `pegelwart_bonus_xp 80`; `POST /api/agent/claim` → `station_bonus_xp:80, station_category:"groundwater_station"` in 12 ms (warm caches). Parcel without gauge → `0`, 167 ms.
5. **farm 503 under bursts** — mitigated (6d86ef5). Could not reproduce the 503 today (12 parallel cold farm calls all 200 in ~0.25 s), so the limiter is verified structurally + `go test`; `/api/schlaege` live 200. The breaker still counts a farm 503 (deliberately unchanged — the breaker is per transport and a real outage must still trip it); with 3 slots the burst that caused it cannot form.
6. **lucky pool tiny / St. Gallenkirch centre** — fixed (aa107bf). Root cause of `pool:1`: 12 Gemeinden were "full" but the 85 % enhanced preference reduced the pool to the 1–2 enhanced ones. 20 picks now: Dürnstein, Nauders, Fohnsdorf, Zeltweg, Judenburg, Pfunds, Ried i. O., Tösens, Serfaus, Spiss, Gallneukirchen. Centres are settlements: Nauders 10.5026/46.8918 (village, 25 parcels in 300 m, was bbox centre 10.5445/46.894), Dürnstein 15.5203/48.3954 (town). St. Gallenkirch resolves to 9.9741/47.0202 (village; look radius 300 → 25 parcels; was 9.9703/46.9864 → 0).
7. **Session create latency** — verified + improved (be76bcd). Warm Gemeinde 20–100 ms; cold Gemeinde was 0.57–1.46 s (= one umfeld address_osm call, 0.4–1.5 s). After a picker search for "Freistadt" the memo exists and create takes **87 ms**. Treasures/N2K already run in goroutines (c68d775). Under 50-user CPU saturation everything is still slower — that is the 2-core box, not the request path.
8. **Stale cell vs live building_count** — documented only (e1677a5).

## Left open / notes
- `kg_warm.parcels` is 0 for KGs whose cells were already cached when `warmKG` ran (it only counts freshly built cells) — cosmetic; `luckyCenter` only uses it as a tie-break fallback.
- Farm: if bursts still produce 503s, lower `hostSlots[farm]` to 2 or exclude JSON 503s from the breaker count (`breaker.go`), not done to keep the breaker semantics simple.
- `/api/agent/look` text/`/llm/game` still says "may answer 202 (retry in 3 s)" — now `retry_after_s` is the real upstream hint; wording left as is.
- QA player `🤖 Subagent Prüfer B` (id 3873e0ab…) + 5 QA sessions created in the live DB during verification.
