# NE epoch reports (siedler → umfeld-at)

umfeld-at.exe.xyz publishes "NE cells" (declared land-use statistics per H3 res-12 cell,
derived from the BEV cadastre; contract: umfeld's `docs/ne-cells.md`). We run our own
bevdirect-serve (`http://127.0.0.1:8787`, public `vtcseamless` **v0.3.3**, output ≡ v0.2.1 ≡ pinned baseline v0.2.0), so we act as an
**observer**: rebuild the same cells from our bevdirect output with the frozen python reference
package `ne_cells` (algo `ne-cells-2`, shapely 2.1.2 + h3 4.5.0) and POST an **epoch report**
(digests only — whole build + one per res-10 chunk, no geometry) to
`POST https://umfeld-at.exe.xyz/api/v1/ne/{kg}/report`. That lets umfeld detect cadastre change
per chunk.

## Files

```
tools/ne-report/setup.sh        venv at tools/ne-report/.venv (gitignored), pip installs vtcseamless-py (0.2.0, commit 0ccea07,
                                bevdirect client `cells_for`/`BevDirect.cell`) + ./ne_cells with pins;
                                INSTALL_UNITS=1 also installs+enables the systemd units (sudo)
tools/ne-report/ne_cells/       vendored frozen reference package (= vtcseamless-py 0.2.0's copy; algo/pack byte-identical
                                to umfeld commit 3f26b3b, 2026-10-04) + change.py (NOT frozen: ap digests,
                                chunk rows, NECH packing — change protocol) + pyproject.toml
tools/ne-report/ne_report.py    the pipeline for one or more KGs (see --help)
tools/ne-report/run.sh          driver: KG list = today's rotation from GET /api/contrib/plan (srv/contrib.go),
                                else a 3-KG fallback sample — never the whole v2.4 universe
tools/ne-report/ne-report.service, ne-report.timer   daily 01:00 Europe/Vienna (+≤15 min jitter), Nice=15,
                                CPUQuota=100%, idle IO, Requires/After bevdirect-serve.service, Persistent=true
data/ne-reports/KG.<date>.json        the report as POSTed (gitignored)
data/ne-reports/KG.<date>.meta.json   build summary, umfeld head numbers, cells fetched, POST answer
data/ne-reports/nec/KG.<epoch>.nec    the NEC1 container we built — derived statistics, fine on disk; pruned > 24 h by run.sh
                                (only needed for `ne_cells dump/compare`)
data/ne-reports/work/KG/cell_i_j.json raw bevdirect cells — scratch, wiped after each KG (kept only with --keep-cells,
                                then pruned > 24 h by run.sh); never commit, never copy elsewhere
```

## How to run

```bash
tools/ne-report/setup.sh                       # once / after pull
tools/ne-report/run.sh 05007 63330             # explicit KGs
tools/ne-report/run.sh                         # tonight's contrib plan (/api/contrib/plan?night=1), skips KGs with a report < 7 d old
FORCE=1 tools/ne-report/run.sh 05007           # ignore the 7-day skip
tools/ne-report/.venv/bin/python tools/ne-report/ne_report.py --help
sudo systemctl start ne-report.service; journalctl -u ne-report -f
```

Per KG the pipeline: `GET umfeld /api/v1/ne/{kg}/head` → `lu.header.input_bbox` = the KG's viewport
(404 = not built yet → skip) → the **aligned** 0.02° cells covering it (`vtcseamless.bevdirect.cells_for`,
`ix=floor(lon/0.02)`), one `GET /viewport?west=ix*0.02&south=iy*0.02&east=+0.02&north=+0.02` per cell
(`BevDirect.cell`, ≤ 2 in flight, `pending` → wait `retry_after_s` and re-GET, never builds on
`ready:false`/`truncated`) → `python -m ne_cells build --kg KG --epoch YYYY-MM --bevdirect cell_*.json
--input-bbox <union of the aligned cells>` (source derived from the documents' `bevdirect_version`;
`--source` is never passed) → `python -m ne_cells report --observer siedler-oesterreich` → file → POST.

**Why aligned cells and the cell union (2026-10-08).** Every "changed" chunk we had produced before
(678 `since_last` diffs, e.g. 67407, 65509, 51222, 51240) was an artefact: a later, wider viewport
contained the whole of a parcel an earlier, smaller one held truncated (`complete:false` at the fetched
tile edge), so the chunk digest differed. And bevdirect-serve's multi-cell `/viewport` keeps one
truncated copy of parcels wider than cell + pad (0.028°), so a stitched viewport never equals the
operator's aligned-cell build. With `--input-bbox` = union of the aligned cells the report has a
stable bbox per cell block and the operator's coverage rule can compare it: identical bbox → most
`cells_n` wins (`coverage{bbox, cells_n, best_cells_n, status:"best"|…}`); a lossy build is flagged
`change_suspect:"coverage_lossy"` and its diffs are ignored. `ne_report.py` logs `coverage` /
`change_suspect` and surfaces change (`meta.change.surfaced:true`) only when the answer has no
`change_suspect` and is not `unchanged:true`. A KG whose viewport contains no whole aligned cell is
skipped (`viewport_too_small`, `--min-full-cells 1`; ~55 % of the KGs reported so far — pass `0` to
report every KG): one single-cell report is worth more to the change signal than many partial ones.
Historic reports are not touched — the operator re-classifies them on read. Meta files now carry
`viewport_bbox` (umfeld's), `input_bbox` (the union), `full_cells`, `fetched[].incomplete`, `change{}`.

**Token:** `ne-peer.key` in the repo root, else `$NE_PEER_TOKEN` (the service also reads
`tools/ne-report/ne-report.env`, gitignored). Without one the POST is a no-op with the log line
`POST skipped — no peer token …`. Token in place since 2026-10-06 (token name `siedler-oesterreich`, POST → 200); unauthenticated POSTs answer 404 by design.

## Rotation, schedule, stash

The KG list, the 22:00 prewarm → 01:00 report schedule, the RAM stash and the plan fields are documented in
[contrib.md](contrib.md) (`srv/contrib.go`). `run.sh` only ever takes its KGs from `GET /api/contrib/plan?night=1`
(or the command line); never the whole v2.4 universe.

## 2026-10-04 — contributor path & bevdirect v0.3.0

- bevdirect-serve is now the public `github.com/raffopenssh/vtcseamless` (MIT); we run **v0.3.0**
  (preset bevdirect, output byte-identical to v0.2.1, so digests should not move).
- Report POST moved from `/k/api/v1/ne/{kg}/report` to **`/contrib/api/v1/ne/{kg}/report`**
  (`ne_report.py --contrib-prefix`, env `UMFELD_CONTRIB_PREFIX`; same Bearer token in `ne-peer.key`,
  same bodies/answers; the old path still answers for now). `/head` stays on the public `/api/v1`.
- Expected: the first report per KG with `bevdirect@v0.3.0` is stored as the new baseline
  (`baseline:"this_report"`); `chunks_changed` appears again from the second run on.

## Change protocol (vtcseamless-py 0.2.0, 2026-10-08)

umfeld's NE change protocol is live and backwards compatible (spec: umfeld `/api/v1/docs/llm.txt` § NE cells).
Same token, same report POST; two additions in `ne_report.py`:

1. The report is built in-process with `ne_cells.change.epoch_report_ap` (= `ne_cells report` +
   `chunks_ap{chunk: digest_ap}` + `ap_version:"ap-1"`): per res-10 chunk the sha256[:16] of its LU rows with the
   two register-derived bytes (`gk`, `n_parc`) zeroed and K rows dropped — so a split/merge that leaves the
   statistics untouched never counts as change.
2. When the answer lists `want_chunks[]` (chunks whose statistics are new to the server: **every chunk once** for
   the baseline, afterwards only changed ones) `send_wanted_chunks` posts the stripped rows of exactly those chunks
   from the NEC1 section already in RAM to `POST /contrib/api/v1/ne/{kg}/chunks?observer=siedler-oesterreich`
   (binary `NECH` bodies, split ≤ 1.5 MB, 429 honoured). Register bytes, K rows and inputs never leave us; nothing
   new is written to disk. `meta.chunks{wanted, posted, bodies, stored, seen, rejected, deltas, http[]}`,
   log line `chunks wanted=… → stored=…`. Public result: `GET umfeld /api/v1/ne/stats?kg=<kg>` and `chg` on
   `/api/v1/ne/cell/{h3}`.

3. **Step 0, header first** (umfeld 2026-10-08): the report is first POSTed *without* `chunks{}`/`chunks_ap{}`
   (~300 B). umfeld dedupes a header-only report on token + source class + algo + digest + epoch — bbox-independent —
   and answers `unchanged:true` when our last report of that KG carried the same digest; the chunk list (40–400 KB)
   is then not sent at all (`meta.post.header_only:true`, log "header-only report: unchanged"). Any other answer
   (changed digest, first report, old server → 4xx) is followed by the full report as before. `meta.post_head{}`
   keeps the step-0 answer. Full reports with `chunks{}` behave unchanged. Protocol: umfeld `/api/v1/docs/ne-change.md`.

Verify after a run: `meta.post.answer.want_chunks` present, `meta.chunks.stored > 0` on the first (baseline) pass,
`unchanged:true` + `header_only:true` (no chunk list sent) on an unchanged re-run.

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

1. ~~**`"footprints": null`**~~ — bevdirect-serve ≤ v0.3.2 emitted `null` instead of `[]` for a layer with
   no objects in the cell (05007 cell 840_2402); the frozen `ne_cells.canon` crashed on `None`.
   Fixed in bevdirect-serve **v0.3.3** (`[]`); vtcseamless-py's client also normalises older servers.
2. ~~`/api/warm/status` has no `v24_kgs` yet~~ — it has all 1 386 since 2026-10-06; `run.sh` now uses
   `/api/contrib/plan` (quarterly rotation) instead, never the whole list.
3. `ne_cells build` only checks `ready`, not `truncated`, on bevdirect documents — we check both.
4. The timer runs on the **Europe/Vienna** wall clock (01:00); the host clock is UTC — `journalctl` shows UTC.
