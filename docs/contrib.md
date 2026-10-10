# Contrib — the nightly NE epoch report (siedler → umfeld-at)

We run our own cadastre assembler (bevdirect-serve), so we act as an **observer** for umfeld's NE cells: each
night we rebuild the declared-land-use cells of ~200 KGs from our bevdirect output with the frozen python
reference `ne_cells` and POST digests (never geometry) to umfeld, which detects cadastre change per res-10
chunk. Pipeline runbook: [ne-report.md](ne-report.md). Public counter: `GET /api/contrib/stats` → the
„Beitrag zum Nutzungsmonitoring“ callout on impressum/imprint.

## The loop (Europe/Vienna wall clock, `contribLoc`, DST-safe)

```
22:00  warmContribRun (srv/contrib.go)   → builds tonight's cells into api_cache + RAM stash (≤ 200 KGs × ~25 s ≈ 85 min)
01:00  ne-report.timer → run.sh          → GET /api/contrib/plan?night=1 → ne_report.py per KG, cells from the stash
≤ 04:30 done (goal 05:30).               Nothing contrib-related runs by day.
```

- `contribWarmHour` 22, `contribNightHour` 1 (keep in sync with `tools/ne-report/ne-report.timer`). A restart
  between 22:00 and 01:00 re-runs the prewarm after 90 s (`contribPrewarmPending`). Prewarm is independent of
  the warm activity tier, cap 200, reason `contrib`, prio 2, paced `warmContribPause` 1.5 s/cell (≈ ½ duty on
  bevdirect).
- The service unit has `Nice=15`, `CPUQuota=100%` (one of the 2 cores), `CPUWeight=30`, `TimeoutStartSec=8h`;
  ~34 s CPU + ~10 s overhead per KG.

## Plan (`GET /api/contrib/plan`, `srv/contrib.go`)

`?night=1` = the plan the next run uses, `?kg=` its due day. Answer: `{quarter, day, days, today[], fill[],
cheap[], rolling[], small, ahead_days, kgs[], universe, per_day_avg, night_min, night_max, reported_quarter,
left_quarter, catch_up_days, source}`.

- **Universe = the whole admin table (7 850 KGs)** — umfeld has an lu digest for every KG, enhanced or not.
- Each KG gets a day of the quarter by `hash(quarter, kg)` (~85/day); the quarter is the **change resolution**
  of the chunk protocol (first report baselines a KG's chunks, later ones upload only changed ones).
- A night is filled to `night_min` = **200** (`contribNightMin`): today + catch-up (`contribCatchUpDays` 3), then
  **every `cheap[]` KG** (warmed < 24 h, uncapped — so the KGs the 22:00 prewarm built *are* the fill of the
  night that follows), then unreported KGs in due order (`fill[]`, overdue first); once the quarter's
  unreported KGs are exhausted the **rolling sweep** re-reports the least recently reported KGs ≥ 7 d old.
  `night_max` 200 caps only KGs needing fresh tiles (≈ 0.45 GB/night).
- **Small KGs** (`small`): a KG whose umfeld viewport holds no whole aligned cell (or whose `/head` is 404) is
  what `ne_report.py --min-full-cells 1` skips as `viewport_too_small` (~30 %). `contribBBoxSmall` mirrors the
  rule on the cached head (`ne-head:v1:`, 7 d), the plan leaves them out (`contribSmallKGs`, one api_cache
  query) and the prewarm skips them on first contact (`contribKGSmall`).
- Contrib jobs warm the cells of **umfeld's declared viewport** (`/ne/{kg}/head` `input_bbox`,
  `contribKGCells`), wider than the admin bbox — exactly what the report reads. Cached cells missing from the
  stash are rebuilt (`enqueueWarmOpt` force skips the freshness guard).
- `reported_quarter`/`left_quarter` come from `data/ne-reports/KG.<date>.json` (`contribLastReport`). The plan
  answers `source:"none"` for seconds during an srtm registry full refresh; run.sh retries 4× 30 s before using
  the fallback sample. The prewarm writes `kg_warm` rows → `/api/lucky` sees contrib KGs as warm.

## RAM stash (`srv/contrib_stash.go`)

bevdirect assembles a cell from tiles on every `/viewport` (1–2.5 s CPU). `buildCell` keeps the **raw**
bevdirect document (same query as vtcseamless' `BevDirect.cell`, byte-identical → same digest) of every ready,
untruncated aligned cell built by a contrib job (and of any cell holding a parcel of tonight's KGs) **in memory
only** — gzip, ≤ 768 MB LRU, expires with the cell ≤ 24 h, lost on restart, never on disk. `ne_report.py` reads
`GET /api/contrib/cell?i&j` (loopback only, `--siedler`) before falling back to bevdirect. `/api/warm/status` →
`contrib{stash{cells,docs,mb,put,hits,misses,evicted}, next_night, tz}`.

## Report rules (what makes digests comparable)

- **Aligned cells only**, `--input-bbox` = union of those cells (stable bbox per cell block → umfeld's coverage
  rule `coverage{bbox, cells_n, best_cells_n, status}` can compare builds; `change_suspect:"coverage_lossy"` =
  lossy build, its diffs are never surfaced — `meta.change.surfaced`).
- `--min-full-cells 1`: KGs whose domain contains no whole aligned cell are skipped (`0` = report every KG).
- Never build on `ready:false` or `truncated:true`.
- **Never feed `v24_kgs` or the whole universe to run.sh** — the plan is the only KG source.
- Same-pass guard: one POST per build; a digest reported < 20 h ago is not re-posted (`--repost` overrides).

## Checking a night

```bash
journalctl -u ne-report --since <date>      # every cell "(stash)", "fetched … N from the game server's stash" = N,
                                            # the "done:" line without viewport_too_small
systemctl show ne-report -p CPUUsageNSec -p ExecMainStartTimestamp -p ExecMainExitTimestamp
curl -s localhost:8000/api/warm/status | jq .contrib
curl -s 'localhost:8000/api/contrib/plan?night=1' | jq '{night_min,kgs:(.kgs|length),cheap:(.cheap|length),small}'
```

QA of one KG: `POST /api/warm/run-plan?kg=NNNNN` (ahead token) = one contrib job, then
`tools/ne-report/.venv/bin/python tools/ne-report/ne_report.py NNNNN --force --pause 0` should log every cell
`(stash)`. Verify after a run: `meta.post.answer.want_chunks` present, `meta.chunks.stored > 0` on the baseline
pass, `unchanged:true` + `header_only:true` on an unchanged re-run.
