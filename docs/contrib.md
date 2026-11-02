# Contrib — the nightly NE epoch report (siedler → umfeld-at)

We run our own cadastre assembler (bevdirect-serve), so we act as an **observer** for umfeld's NE cells: each
night we rebuild the declared-land-use cells of ~390 KGs from our bevdirect output with the frozen python
reference `ne_cells` and POST digests (never geometry) to umfeld, which detects cadastre change per res-10
chunk. Pipeline runbook: [ne-report.md](ne-report.md). Public counter: `GET /api/contrib/stats` → the
„Beitrag zum Nutzungsmonitoring“ callout on impressum/imprint.

## The loop (Europe/Vienna wall clock, `contribLoc`, DST-safe)

```
~20:00 warmContribRun (srv/contrib.go)   → builds tonight's cells into api_cache + RAM stash (≤ 440 KGs × ~18 s ≈ 2 h 15)
23:00  ne-report.timer → run.sh          → GET /api/contrib/plan?night=1 → ne_report.py per KG, cells from the stash
≤ 02:30 done (2026-11-02: 293 KGs in 2 h 10 → 440 ≈ 3.3 h); run.sh starts no batch after 04:30 Vienna (`DEADLINE_H/M`), leftovers are next night's overdue.               Nothing contrib-related runs by day.
```

- **Wave** (`contribWave`): start time (20:00 ± 25 min) and volume (340–440 KGs) drift per night — two
  incommensurate sines over the night number + a per-date hash jitter, deterministic per night so plan, prewarm
  and report agree (`night_min`/`night_max` = tonight's volume; `/api/warm/status` → `contrib.wave{night, volume,
  shift_min, prewarm_at}`). The per-cell pause is jittered ±40 % too. Mean ≈ 390/night → universe in ~20 nights (monthly sweep with margin).
- `contribWarmHour` 20, `contribNightHour` 23 (keep in sync with `tools/ne-report/ne-report.timer`). A restart
  between 20:00 and 23:00 re-runs the prewarm after 90 s (`contribPrewarmPending`). `nextContribNight` has a 3 h grace: a run asking `?night=1` just after the timer fired gets the slot that just started, not tomorrow's. Prewarm is independent of
  the warm activity tier, cap 440, reason `contrib`, prio 2, paced `warmContribPause` 1 s/cell.
- The service unit has `Nice=15`, `CPUQuota=100%` (one of the 2 cores), `CPUWeight=30`, `TimeoutStartSec=8h`;
  ~34 s CPU + ~10 s overhead per KG.

## Plan (`GET /api/contrib/plan`, `srv/contrib.go`)

`?night=1` = the plan the next run uses, `?kg=` its due day. Answer: `{quarter, day, days, today[], fill[],
cheap[], rolling[], small, ahead_days, kgs[], universe, per_day_avg, night_min, night_max, reported_quarter,
left_quarter, catch_up_days, source}`.

- **Universe = the whole admin table (7 850 KGs)** — umfeld has an lu digest for every KG, enhanced or not.
- Each KG gets a day of the quarter by `hash(quarter, kg)` (~85/day); the quarter is the **change resolution**
  of the chunk protocol (first report baselines a KG's chunks, later ones upload only changed ones).
- A night is filled to `night_min` = **440** (`contribNightMin`): today + catch-up (`contribCatchUpDays` 3), then
  **every `cheap[]` KG** (warmed < 24 h, uncapped — so the KGs the 20:00 prewarm built *are* the fill of the
  night that follows), then unreported KGs in due order (`fill[]`, overdue first); once the quarter's
  unreported KGs are exhausted the **rolling sweep** re-reports the least recently reported KGs ≥ 7 d old.
  `night_max` 440 caps only KGs needing fresh tiles (≈ 1 GB/night) → whole universe every ~20 nights.
- **No KG is skipped as small or big.** `ne_report.py --min-full-cells 0` (default) reports a KG even when
  umfeld's viewport holds no whole aligned cell (the build input is whole aligned cells either way, so the digest
  is as comparable as any other); `--max-cells 600` is a sanity guard only (largest KG 345). `small` in the plan is
  the number of KGs *deferred*: ne_report writes `KG.<date>.skip` (outcome only, no cadastre content) when it cannot
  report a KG (`not_built`, `no_bbox`, `no_cells`, `too_big`, `failed`) and the plan leaves such a KG out for 7 d
  (`contribSkippedRecently`), so an unreportable KG never holds a slot night after night. Before 2026-10-14 ~30 %
  of the universe was left out as `viewport_too_small`.
- Contrib jobs warm the cells of **umfeld's declared viewport** (`/ne/{kg}/head` `input_bbox`,
  `contribKGCells`), wider than the admin bbox — exactly what the report reads. Cached cells missing from the
  stash are rebuilt (`enqueueWarmOpt` force skips the freshness guard).
- **Revisit hints** (`srv/contrib_revisit.go`, umfeld `GET /contrib/api/v1/ne/revisit?observer=…`, peer token,
  fetched ≤ 1×/10 min): `pending[]` (chunk versions sighted once — our sighting confirms) then `stale[]`
  (baseline only), then **`no_log[]`** (KGs without any chunk log — their declared layer on umfeld is still the March index; one
  chunk-bearing report covering the KG's universe bbox gives a baseline and makes its chunks authoritative, umfeld
  `ne-change.md` §4b) go to the **head of the night**, ≤ ⅓ of `night_min` (umfeld caps each list at 50),
  re-reported regardless of age (`revisit[]`, `revisit_pending`, `revisit_stale`, `revisit_no_log`; run.sh →
  `--revisit`). `obs_on_index[]` is for NEO1 observers, not us. Every revisit KG is POSTed as a **full report**
  (no header-only step 0, [ne-report.md](ne-report.md) § step 0): a header answered `unchanged` confirms but does
  not count as a chunk-bearing report, so a `stale`/`no_log` KG would stay on the list forever.
  Umfeld (2026-10-27, §4b) treats every logged chunk as authoritative for its cell set: index cells our build no
  longer holds are served from the neighbour KG's rows or, orphaned, as `flags.superseded` fallbacks (bit 7 —
  ignore when diffing against our build; `header.view.hidden` counts them). Reporting both sides of a moved
  boundary clears them on its own — nothing to flag or delete. Every entry carries a `bbox`
  (`"w,s,e,n"`, the layout the pending chunk versions were built with); the plan passes it as
  `revisit_bbox{kg: bbox}`, run.sh as `--revisit KG=w:s:e:n`, and the KG is then **built with exactly that
  bbox** — `contribKGCells` warms its cells and `ne_report.py` uses it as `input_bbox` instead of the
  head-viewport cell union (`meta.input_rule:"umfeld-revisit-bbox"`). Any other layout moves chunk digests:
  umfeld answers `change_suspect:"layout_change"`, issues no `want_chunks` and confirms nothing (2026-10-23:
  84014/85205 with our wider bbox → 0 confirmed; 73510 with the peer's bbox → 98/98). KGs we pick ourselves
  keep the cell-union rule (umfeld `docs/ne-change.md` §5b).
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
- `--min-full-cells 0`: every KG is reported, whatever its viewport (whole aligned cells are built either way).
- Never build on `ready:false` or `truncated:true`.
- **Never feed `v24_kgs` or the whole universe to run.sh** — the plan is the only KG source.
- **Unchanged tiles → no build** ([ne-report.md](ne-report.md) § Unchanged tiles): `tiles.hash` of tonight's stash
  docs equal to our previous report's → that report is re-sent (epoch updated) without `ne_cells build`; a full
  build at least every 28 d per KG. `--no-skip` for QA.
- Same-pass guard: one POST per build; a digest reported < 20 h ago is not re-posted (`--repost` overrides).
- **QC provenance** (`tiles{}`/`chunks_tiles{}` from the documents' `tiles[]`, bevdirect ≥ v0.3.4): see [ne-report.md](ne-report.md) § Quality control. Stash docs without `tiles[]` are not used.

## Checking a night

```bash
journalctl -u ne-report --since <date>      # every cell "(stash)", "fetched … N from the game server's stash" = N,
                                            # the "done:" line all ok
systemctl show ne-report -p CPUUsageNSec -p ExecMainStartTimestamp -p ExecMainExitTimestamp
curl -s localhost:8000/api/warm/status | jq .contrib
curl -s 'localhost:8000/api/contrib/plan?night=1' | jq '{night_min,kgs:(.kgs|length),cheap:(.cheap|length),small}'
```

QA of one KG: `POST /api/warm/run-plan?kg=NNNNN` (ahead token) = one contrib job, then
`tools/ne-report/.venv/bin/python tools/ne-report/ne_report.py NNNNN --force --pause 0` should log every cell
`(stash)`. Verify after a run: `meta.post.answer.want_chunks` present, `meta.chunks.stored > 0` on the baseline
pass, `unchanged:true` + `header_only:true` on an unchanged re-run.
