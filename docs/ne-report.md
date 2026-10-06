# NE epoch reports (siedler → umfeld-at)

umfeld-at.exe.xyz publishes "NE cells" (declared land-use statistics per H3 res-12 cell,
derived from the BEV cadastre; contract: umfeld's `docs/ne-cells.md`). We run our own
bevdirect-serve (`http://127.0.0.1:8787`, public `vtcseamless` **v0.3.0**, output ≡ v0.2.1 ≡ pinned baseline v0.2.0), so we act as an
**observer**: rebuild the same cells from our bevdirect output with the frozen python reference
package `ne_cells` (algo `ne-cells-2`, shapely 2.1.2 + h3 4.5.0) and POST an **epoch report**
(digests only — whole build + one per res-10 chunk, no geometry) to
`POST https://umfeld-at.exe.xyz/api/v1/ne/{kg}/report`. That lets umfeld detect cadastre change
per chunk.

## Files

```
tools/ne-report/setup.sh        venv at tools/ne-report/.venv (gitignored), pip installs ./ne_cells with pins;
                                INSTALL_UNITS=1 also installs+enables the systemd units (sudo)
tools/ne-report/ne_cells/       vendored frozen reference package (umfeld commit 3f26b3b, 2026-10-04) + pyproject.toml
tools/ne-report/ne_report.py    the pipeline for one or more KGs (see --help)
tools/ne-report/run.sh          driver: KG list = today's rotation from GET /api/contrib/plan (srv/contrib.go),
                                else a 3-KG fallback sample — never the whole v2.4 universe
tools/ne-report/ne-report.service, ne-report.timer   daily 03:30 UTC (+≤15 min jitter), Nice=15, idle IO,
                                Requires/After bevdirect-serve.service, Persistent=true
data/ne-reports/KG.<date>.json        the report as POSTed (gitignored)
data/ne-reports/KG.<date>.meta.json   build summary, umfeld head numbers, cells fetched, POST answer
data/ne-reports/nec/KG.<epoch>.nec    the NEC1 container we built (for `ne_cells dump/compare`)
data/ne-reports/work/KG/cell_i_j.json fetched bevdirect cells (only with --keep-cells)
```

## How to run

```bash
tools/ne-report/setup.sh                       # once / after pull
tools/ne-report/run.sh 05007 63330             # explicit KGs
tools/ne-report/run.sh                         # today's contrib rotation, skips KGs with a report < 7 d old
FORCE=1 tools/ne-report/run.sh 05007           # ignore the 7-day skip
tools/ne-report/.venv/bin/python tools/ne-report/ne_report.py --help
sudo systemctl start ne-report.service; journalctl -u ne-report -f
```

Per KG the pipeline: `GET umfeld /api/v1/ne/{kg}/head` → `lu.header.input_bbox` (404 = not built
yet → skip) → fetch every 0.02° bevdirect cell intersecting it (`i=floor(lon/0.02)`,
`j=floor(lat/0.02)`, ≤ 2 in flight, `pending` → wait `retry_after_s` and re-GET, never builds on
`ready:false`/`truncated`) → `python -m ne_cells build --kg KG --epoch YYYY-MM --bevdirect cell_*.json
--input-bbox W,S,E,N` (source derived from the documents' `bevdirect_version`; `--source` is never
passed) → `python -m ne_cells report --observer siedler-oesterreich` → file → POST.

**Token:** `ne-peer.key` in the repo root, else `$NE_PEER_TOKEN` (the service also reads
`tools/ne-report/ne-report.env`, gitignored). Without one the POST is a no-op with the log line
`POST skipped — no peer token …`. We have no token yet; unauthenticated POSTs answer 404 by design.

## Quarterly rotation (`srv/contrib.go`, 2026-10-06)

Every v2.4 KG (registry `v24`, ~1 400) is assigned one **day of the quarter** by
`sha256("contrib:" + quarter + ":" + kg) mod days` — random across Austria, stable for the quarter,
new KGs appearing mid-quarter land on some day without shifting the others. `GET /api/contrib/plan`
→ `{quarter, day, days, today[], fill[], ahead_days, kgs[], universe, per_day_avg, night_min 40, night_max 120,
reported_quarter, left_quarter, catch_up_days}`; `kgs[]` = today's KGs + still-unreported KGs of the two
previous nights (catch-up) + **fill**: not-yet-reported KGs due later in the quarter, in due order, until
≥ 40 KGs — so a 1 400-KG universe is swept in ~5 weeks, 4 000 KGs in a quarter (~43/night), the quarter
being the guarantee. Reported/left counts are read from the `data/ne-reports/KG.<date>.json` file names;

`?kg=NNNNN` → `next_for_kg` (the date that KG is due). `/api/warm/status` carries the compact `contrib{}`.
The rotation is independent of prewarming: it reads bevdirect directly, writes neither `kg_warm`
nor `api_cache` cells, so `/api/lucky` is unaffected. ≈ 16 KGs × ~10 cells ≈ 30 MB BEV tiles a night.

## 2026-10-04 — contributor path & bevdirect v0.3.0

- bevdirect-serve is now the public `github.com/raffopenssh/vtcseamless` (MIT); we run **v0.3.0**
  (preset bevdirect, output byte-identical to v0.2.1, so digests should not move).
- Report POST moved from `/k/api/v1/ne/{kg}/report` to **`/contrib/api/v1/ne/{kg}/report`**
  (`ne_report.py --contrib-prefix`, env `UMFELD_CONTRIB_PREFIX`; same Bearer token in `ne-peer.key`,
  same bodies/answers; the old path still answers for now). `/head` stays on the public `/api/v1`.
- Expected: the first report per KG with `bevdirect@v0.3.0` is stored as the new baseline
  (`baseline:"this_report"`); `chunks_changed` appears again from the second run on.

## Validation 2026-10-04 (bevdirect-serve v0.2.1, epoch 2026-10)

| KG | bevdirect cells | our cells / K | digest | umfeld `lu` cells_n (index-2026-03) | note |
|---|---|---|---|---|---|
| 05007 Gerhaus | 6 (i 840–842, j 2402–2403) | 10 497 / 129 | `49ec2e5f9a38d1cc` | 10 497 | cells_n exact match; 11 s build |
| 06030 Aderklaa | 9 (i 825–827, j 2412–2414) | 28 796 / 284 | `cfca02b92eb5d446` | 28 752 | Δ +44 (0.15 %) — index-vs-bevdirect source noise, same kind as 63330's 52 476 vs 52 478 |
| 63330 Kohlschwarz | 20 (i 753–757, j 2354–2357) | 52 478 / 649 | **`594025647ec3557e`** | 52 476 (`bev` layer: 52 478) | **identical to the contract's `bev` digest built from v0.2.0** → v0.2.1 is byte-equivalent, our machine reproduces umfeld's build bit-exactly; 1163/1163 chunks reported |

The `lu` digests (`index-2026-03`) are a different source string and are never compared with
bevdirect builds (`/report` answers `identical_source:false` for them); the baseline for our
reports is umfeld's `bev` layer, which only 63330 has so far.

Load: a KG is 6–20 cells, each 0.4–1.5 s when the tiles are cached; the build is CPU-bound in
python (11–44 s). bevdirect's cell cache went 2 → 37 cells over the three KGs (limit 120).

## Contract surprises

1. **`"footprints": null`** — bevdirect-serve v0.2.1 emits `null` instead of `[]` for a layer with
   no objects in the cell (05007 cell 840_2402). The frozen `ne_cells.canon._one_bevdirect` does
   `doc.get("footprints", [])` and raises `TypeError` on `None`. `ne_report.py` normalises null
   layers to `[]` before saving the cell (recorded as `null_layers_normalised`); only the
   informative `inputs[].file_sha256` sees that, the record digest is unaffected (63330 proves it).
   Worth telling umfeld (`or []`) or bevdirect (emit `[]`).
2. ~~`/api/warm/status` has no `v24_kgs` yet~~ — it has all 1 386 since 2026-10-06; `run.sh` now uses
   `/api/contrib/plan` (quarterly rotation) instead, never the whole list.
3. `ne_cells build` only checks `ready`, not `truncated`, on bevdirect documents — we check both.
4. The timer runs at 03:30 **UTC** (host clock is UTC).
