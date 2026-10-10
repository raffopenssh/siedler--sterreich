# Task: NE epoch reports from our bevdirect cells (siedler → umfeld-at)

Context: umfeld-at.exe.xyz publishes "NE cells" (declared land-use statistics per H3 res-12 cell,
derived from BEV cadastre). Peers that run their own bevdirect-serve (we do: http://127.0.0.1:8787,
now v0.2.1, which is declared cell-equivalent to the pinned baseline bevdirect@v0.2.0) are expected
to act as *observers*: build the same cells from their bevdirect output with the frozen python
reference package `ne_cells` and POST an *epoch report* (digests only) to
`POST https://umfeld-at.exe.xyz/api/v1/ne/{kg}/report`. The report lets umfeld detect cadastre
change per res-10 chunk. Full contract: /tmp/ne/umfeld-ne.md (also /tmp/cpa/docs/ne-cells.md).
Reference package source: /tmp/cpa/ne_cells (python; pinned shapely==2.1.2 h3==4.5.0) and
/tmp/cpa/pyproject.toml, tests /tmp/cpa/tests_py if present. Read `python3 -m ne_cells --help`,
algo.py, pack.py, __main__.py to learn the exact CLI (build --bevdirect cell.json … --input-bbox …,
report --observer …).

Goal: a reproducible pipeline under /home/exedev/siedler/tools/ne-report/ that, for a KG code:
1. reads the KG's `input_bbox` from `GET https://umfeld-at.exe.xyz/api/v1/ne/{kg}/head`
   (field lu.header.input_bbox; 404 → KG not built yet, skip);
2. fetches every bevdirect 0.02° cell intersecting input_bbox:
   `GET http://127.0.0.1:8787/viewport?west=i*0.02&south=j*0.02&east=(i+1)*0.02&north=(j+1)*0.02&layers=parcels,footprints,landuse&wait=20`
   — grid i=floor(lon/0.02), j=floor(lat/0.02); a cell may answer ready:false/pending → wait
   retry_after_s and re-GET (never use a non-ready document). Save each as cell_{i}_{j}.json.
   Be gentle: bevdirect is also serving the live game — ≤ 2 cells in flight, and it keeps only
   ~120 cells in RAM. Don't hammer kataster.bev.gv.at: a KG is ~10–30 cells.
3. `python3 -m ne_cells build --kg KG --epoch 2026-10 --bevdirect cell_*.json --input-bbox W,S,E,N -o out/KG.nec`
   (source is derived from bevdirect_version in the documents → bevdirect@v0.2.1; the manifest's
   frozen.bev_equivalent_tags maps it to the v0.2.0 baseline, so that's correct — do NOT pass --source).
4. `python3 -m ne_cells report out/KG.nec --observer siedler-oesterreich` → JSON report;
   store it at /home/exedev/siedler/data/ne-reports/KG.<date>.json (gitignored) and, if the env var
   NE_PEER_TOKEN is set (read from /home/exedev/siedler/ne-peer.key if present, else env), POST it
   with `Authorization: Bearer $TOKEN` to https://umfeld-at.exe.xyz/api/v1/ne/KG/report and log the
   answer (identical_source, compared, chunks_same/changed). We currently have NO token — build and
   test everything else; the POST step must be a clean no-op with a clear log line when unset.
   Note: unauthenticated POST answers 404 by design.
5. A driver `tools/ne-report/run.sh [KG…]` that defaults to the KGs listed by
   `curl -s localhost:8000/api/warm/status` field `v24_kgs` if present (I'm adding it; until then
   accept the list on the command line and fall back to these v2.4 KGs: 01205 01209 01512 01609 03030
   03113 03134 03136 04304 05007 05023 06030 06101 06107 06205 09002 09008 09025 09045 09061 11039 12134),
   skipping KGs with a report younger than 7 days. Plus a systemd service+timer (`ne-report.timer`,
   daily ~03:30, Nice=15, after bevdirect-serve) — install & enable via sudo. Use a venv at
   /home/exedev/siedler/tools/ne-report/.venv (gitignored) with the pinned deps; the install step
   should be in a `setup.sh`.

Validate on one small v2.4 KG first (e.g. 05007 Gerhaus or 06030 Aderklaa) — check the build
succeeds, cells_n matches umfeld's lu.cells_n for that KG from /ne/manifest (border cells are
emitted per touching KG so it should match exactly or very nearly; report what you see), and
that `report` produces digests. Then run 63330 (Kohlschwarz) if time permits and compare the whole-build
digest with the contract's bev digest `594025647ec3557e` (built from v0.2.0; v0.2.1 should be
byte-identical). Report all digests back to me.

Constraints: Go repo conventions in /home/exedev/siedler/AGENTS.md (don't touch srv/ or game.js —
I'm editing those concurrently). Commit your work in git with a good message (only your files:
tools/ne-report/**, the systemd unit files kept in tools/ne-report/, .gitignore lines). Short
summary of findings in docs/ne-report.md (how to run, what the digests were, whether cells_n matched).
