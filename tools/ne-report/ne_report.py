#!/usr/bin/env python3
"""ne_report.py — NE epoch report for one KG from our local bevdirect-serve cells.

Pipeline (contract: docs/ne-report.md, umfeld's docs/ne-cells.md):
  1. GET {umfeld}/api/v1/ne/{kg}/head          → lu.header.input_bbox = the KG's viewport (404 → KG not built yet, skip)
  2. the ALIGNED 0.02° cells covering that viewport (vtcseamless.bevdirect.cells_for: ix=floor(lon/0.02)),
     one GET {bev}/viewport per cell with west=ix*0.02 … north=(iy+1)*0.02 (BevDirect.cell; ≤ 2 in flight,
     pending/retry_after_s honoured, never a truncated/non-ready document). Never a free or multi-cell
     viewport: bevdirect-serve's compose keeps ONE truncated copy of a parcel wider than cell + pad, so a
     stitched viewport is never equal to the operator's aligned-cell build (vtcseamless DEPLOY.md v0.3.3).
  3. python -m ne_cells build --kg KG --epoch E --bevdirect cell_*.json --input-bbox <UNION OF THE CELLS> -o KG.nec
     The input bbox is the union of the aligned cells, not the raw viewport: the report then has a stable
     bbox per cell block, which is what the operator's coverage rule compares (identical bbox → most
     cells_n wins; a lossy build is flagged change_suspect:"coverage_lossy" and its diffs are ignored).
     Source is derived from bevdirect_version in the documents — never pass --source.
     A report is only built from ≥ 1 full aligned cell; a viewport that cannot be covered is skipped.
  4. ne_cells.change.epoch_report_ap(KG.nec) (= `ne_cells report` + chunks_ap: per chunk the digest of its rows with
     the register bytes gk/n_parc zeroed, change protocol ≥ vtcseamless-py 0.2.0) → data/ne-reports/KG.<date>.json
  5. POST {umfeld}/contrib/api/v1/ne/{kg}/report with `Authorization: Bearer $NE_PEER_TOKEN` if a token is configured,
     else a logged no-op. Step 0 first: the header alone (no chunks{}/chunks_ap{}) — umfeld dedupes it on token +
     source class + algo + digest + epoch (bbox-independent) and answers unchanged:true when our last report of the KG
     had the same digest, so an unchanged re-pass never sends the chunk list; otherwise the full report follows. When the answer lists want_chunks (chunks whose statistics are new to the server — every
     chunk once for the baseline, then only changed ones) the stripped rows of exactly those chunks are posted from
     RAM as NECH bodies ≤ 1.5 MB to POST …/ne/{kg}/chunks?observer=<label> (register bytes zeroed, no K rows, nothing
     new on disk); meta.chunks{posted,stored,seen,rejected,deltas}. One POST per build: when our newest accepted
     POST of the KG (meta files) carried this very digest < 20 h ago (umfeld's same-pass window, in which a repeat
     by the same observer confirms nothing) nothing is sent at all — not even the header (--repost overrides). The answer's coverage{} / change_suspect is logged and stored; chunks_changed from an
     answer that carries change_suspect is never surfaced as change (`change` in the meta is then null).

Run through the venv: tools/ne-report/.venv/bin/python tools/ne-report/ne_report.py 05007
"""
import argparse
import concurrent.futures as cf
import datetime as dt
import glob
import gzip
import json
import os
import shutil
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

from vtcseamless.bevdirect import BevDirect, PendingError, ServerError, cell_bbox, cells_for
from ne_cells.change import chunk_rows, epoch_report_ap, pack_chunk_rows
from ne_cells.pack import MAGIC_LU, unpack_sections

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.abspath(os.path.join(HERE, "..", ".."))
GRID = 0.02
LAYERS = ("parcels", "footprints", "landuse")
UA = "siedler-oesterreich ne-report/1 (+https://siedler-oesterreich.exe.xyz)"


def log(msg):
    print(time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()), msg, file=sys.stderr, flush=True)


def http_get(url, headers=None, timeout=90):
    """→ (status, body_bytes, headers). HTTPError is returned, not raised; URLError is raised."""
    req = urllib.request.Request(url, headers={"User-Agent": UA, "Accept": "application/json", **(headers or {})})
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            return r.status, r.read(), dict(r.headers)
    except urllib.error.HTTPError as e:
        return e.code, e.read(), dict(e.headers)


# ---------------------------------------------------------------- 1. input_bbox from umfeld

def umfeld_head(umfeld, kg):
    """None when the KG is not built (404). Otherwise the /head document."""
    url = f"{umfeld}/api/v1/ne/{kg}/head"
    for attempt in range(4):
        try:
            st, body, _ = http_get(url)
        except urllib.error.URLError as e:
            log(f"{kg}: head: {e}; retry")
            time.sleep(5 * (attempt + 1))
            continue
        if st == 404:
            return None
        if st == 200:
            return json.loads(body)
        log(f"{kg}: head HTTP {st}: {body[:200]!r}; retry")
        time.sleep(5 * (attempt + 1))
    raise SystemExit(f"{kg}: /head unreachable")


# ---------------------------------------------------------------- 2. bevdirect cells (aligned only)

def grid_cells(bbox):
    """Aligned 0.02° cells covering the viewport bbox, row-major (j, i) — vtcseamless' rule
    (a bbox ending exactly on a grid line does not include the next cell)."""
    return sorted(cells_for(*[float(v) for v in bbox]), key=lambda c: (c[1], c[0]))


def cells_union(cells):
    """(W, S, E, N) of the union of aligned cells — the --input-bbox of the build and the bbox the
    report carries. Exact multiples of 0.02° rounded to 2 dp, so identical cell blocks give an
    identical bbox on every run and on every peer."""
    i0, i1 = min(c[0] for c in cells), max(c[0] for c in cells)
    j0, j1 = min(c[1] for c in cells), max(c[1] for c in cells)
    return (round(i0 * GRID, 2), round(j0 * GRID, 2), round((i1 + 1) * GRID, 2), round((j1 + 1) * GRID, 2))


def full_cells_in(bbox, cells):
    """Cells of `cells` that lie entirely inside bbox (viewport contains the whole aligned cell)."""
    W, S, E, N = [float(v) for v in bbox]
    eps = 1e-9
    return [(i, j) for i, j in cells
            if i * GRID >= W - eps and (i + 1) * GRID <= E + eps and j * GRID >= S - eps and (j + 1) * GRID <= N + eps]


def stash_cell(siedler, i, j):
    """The raw bevdirect document of an aligned cell from the game server's RAM stash
    (GET /api/contrib/cell?i&j, srv/contrib_stash.go): the 23:00 contrib prewarm already made
    bevdirect assemble tonight's cells, so re-using the byte-identical answer saves one full
    assembly (1–2.5 s CPU) per cell at night. None when not stashed or the server is down."""
    if not siedler:
        return None
    try:
        req = urllib.request.Request(f"{siedler}/api/contrib/cell?i={i}&j={j}",
                                     headers={"Accept": "application/json", "Accept-Encoding": "gzip"})
        with urllib.request.urlopen(req, timeout=30) as r:
            raw = r.read()
            if r.headers.get("Content-Encoding") == "gzip":
                raw = gzip.decompress(raw)
            doc = json.loads(raw)
    except Exception:
        return None
    if not doc.get("ready") or doc.get("pending") or doc.get("truncated"):
        return None
    for layer in ("parcels", "footprints", "landuse"):
        if doc.get(layer) is None:
            doc[layer] = []
    return doc


def fetch_cell(bev, i, j, path, max_total_s=900, siedler=None):
    """GET one aligned cell — the game server's RAM stash first (stash_cell), else through
    vtcseamless' BevDirect.cell (loops on ready:false/pending with retry_after_s, normalises null
    layers to [] for bevdirect < v0.3.3). Never writes a non-ready or truncated document."""
    t0 = time.time()
    errors = 0
    doc = stash_cell(siedler, i, j)
    src = "stash" if doc is not None else "bevdirect"
    while doc is None:
        try:
            doc = bev.cell(i, j, deadline_s=max_total_s)
            break
        except PendingError as e:
            raise RuntimeError(f"cell {i}_{j}: still pending after {max_total_s}s") from e
        except ServerError as e:
            errors += 1
            if errors > 6:
                raise RuntimeError(f"cell {i}_{j}: {e}")
            log(f"cell {i}_{j}: {e}; backoff")
            time.sleep(10 * errors)
    if doc.get("truncated"):
        raise RuntimeError(f"cell {i}_{j}: truncated:true — not a valid NE input")
    if not doc.get("ready") or doc.get("pending"):
        raise RuntimeError(f"cell {i}_{j}: not ready — not a valid NE input")
    W, S, E, N = cell_bbox(i, j)
    with open(path + ".tmp", "w", encoding="utf-8") as f:
        json.dump(doc, f, separators=(",", ":"), ensure_ascii=False)
    os.replace(path + ".tmp", path)
    return dict(i=i, j=j, bbox=[W, S, E, N], parcels=len(doc.get("parcels") or []), footprints=len(doc.get("footprints") or []),
                landuse=len(doc.get("landuse") or []), incomplete=sum(1 for p in doc.get("parcels") or [] if p.get("complete") is False),
                bevdirect_version=doc.get("bevdirect_version"), coord_decimals=doc.get("coord_decimals"),
                ms=doc.get("query_time_ms"), s=round(time.time() - t0, 1), src=src)


def fetch_cells(bev, cells, workdir, inflight=2, siedler=None):
    os.makedirs(workdir, exist_ok=True)
    out = []
    with cf.ThreadPoolExecutor(max_workers=inflight) as ex:
        futs = {ex.submit(fetch_cell, bev, i, j, os.path.join(workdir, f"cell_{i}_{j}.json"), 900, siedler): (i, j) for i, j in cells}
        for fut in cf.as_completed(futs):
            r = fut.result()          # raises on a failed cell → whole KG aborts (never build on a partial set)
            log(f"  cell {r['i']}_{r['j']}: parcels={r['parcels']} (truncated at pad {r['incomplete']}) footprints={r['footprints']} "
                f"landuse={r['landuse']} bevdirect={r['bevdirect_version']} ({r['s']}s, {r['src']})")
            out.append(r)
    return sorted(out, key=lambda r: (r["j"], r["i"]))


# ---------------------------------------------------------------- 3/4. ne_cells build + report

def run_ne_cells(args, capture_stdout=False):
    cmd = [sys.executable, "-m", "ne_cells", *args]
    p = subprocess.run(cmd, capture_output=True, text=True)
    if p.returncode != 0:
        raise RuntimeError(f"ne_cells {args[0]} failed ({p.returncode}): {p.stderr.strip()[-2000:]}")
    return p.stdout if capture_stdout else p.stderr


# ---------------------------------------------------------------- 5. POST

def load_token(path):
    if path and os.path.isfile(path):
        with open(path) as f:
            t = f.read().strip()
        if t:
            return t, path
    t = os.environ.get("NE_PEER_TOKEN", "").strip()
    return (t, "env NE_PEER_TOKEN") if t else (None, None)


def post_report(umfeld, kg, report_bytes, token, prefix="/contrib"):
    # Contributor surface: POST {umfeld}/contrib/api/v1/ne/{kg}/report (was /k/…; the old path still
    # answers for now). /head stays on the public /api/v1. First report per KG after a bevdirect
    # upgrade is stored as the new baseline (baseline:"this_report"), chunks_changed from the 2nd run on.
    url = f"{umfeld}{prefix}/api/v1/ne/{kg}/report"
    req = urllib.request.Request(url, data=report_bytes, method="POST",
                                 headers={"User-Agent": UA, "Accept": "application/json",
                                          "Content-Type": "application/json", "Authorization": "Bearer " + token})
    try:
        with urllib.request.urlopen(req, timeout=120) as r:
            return r.status, r.read()
    except urllib.error.HTTPError as e:
        return e.code, e.read()


CHUNKS_BODY_MAX = 1_500_000   # umfeld's limit is 2 MB per POST …/chunks


def split_chunk_bodies(rows, limit=CHUNKS_BODY_MAX):
    """Chunk-row dicts whose packed NECH size stays under `limit` (order = chunk id)."""
    out, cur, size = [], {}, 6
    for c in sorted(rows):
        n = 12 + len(rows[c])
        if cur and size + n > limit:
            out.append(cur)
            cur, size = {}, 6
        cur[c] = rows[c]
        size += n
    if cur:
        out.append(cur)
    return out


def post_chunks(umfeld, kg, observer, body, token, prefix="/contrib"):
    url = f"{umfeld}{prefix}/api/v1/ne/{kg}/chunks?observer={urllib.parse.quote(observer)}"
    req = urllib.request.Request(url, data=body, method="POST",
                                 headers={"User-Agent": UA, "Accept": "application/json",
                                          "Content-Type": "application/octet-stream", "Authorization": "Bearer " + token})
    for attempt in range(4):
        try:
            with urllib.request.urlopen(req, timeout=180) as r:
                return r.status, r.read()
        except urllib.error.HTTPError as e:
            if e.code == 429 and attempt < 3:
                time.sleep(max(1, int(e.headers.get("Retry-After") or 2)))
                continue
            return e.code, e.read()
    return 0, b""


def send_wanted_chunks(a, kg, section, want, token):
    """Change protocol step 2: the stripped rows of exactly the chunks umfeld asked for, from the section in RAM."""
    rows = chunk_rows(section.raw, section.header["cells_n"], want)
    missing = sorted(set(want) - set(rows))
    if missing:
        log(f"{kg}: want_chunks lists {len(missing)} chunks we have no rows for (ignored): {missing[:5]}")
    tot = dict(wanted=len(want), posted=len(rows), bodies=0, stored=0, seen=0, rejected=0, deltas=0, http=[])
    for part in split_chunk_bodies(rows):
        st, body = post_chunks(a.umfeld, kg, a.observer, pack_chunk_rows(part), token, a.contrib_prefix)
        tot["bodies"] += 1
        tot["http"].append(st)
        try:
            ans = json.loads(body)
        except Exception:
            ans = {}
        if st != 200 or not isinstance(ans, dict):
            log(f"{kg}: POST chunks ({len(part)} chunks) HTTP {st}: {body[:300].decode('utf-8', 'replace')}")
            continue
        for k in ("stored", "seen", "rejected"):
            tot[k] += int(ans.get(k) or 0)
        tot["deltas"] += len(ans.get("deltas") or [])
    log(f"{kg}: chunks wanted={tot['wanted']} posted={tot['posted']} in {tot['bodies']} bodies → "
        f"stored={tot['stored']} seen={tot['seen']} rejected={tot['rejected']} deltas={tot['deltas']}")
    return tot


def classify_change(kg, ans):
    """Read the /contrib answer: coverage{} (the operator's per-bbox rule: identical bbox → most cells_n
    wins) and change_suspect. Logged in full; `chunks_changed` is surfaced as change ONLY when the answer
    carries no change_suspect — a suspect report (e.g. "coverage_lossy": cells_n below the best build
    seen for this bbox) is an artefact of the input, never land-use change. Returns the meta `change`
    block (None = nothing to surface)."""
    changed = ans.get("chunks_changed") or []
    since = ans.get("since_last") or {}
    since_changed = since.get("changed") if isinstance(since, dict) else None
    cov = ans.get("coverage")
    suspect = ans.get("change_suspect")
    log(f"{kg}: POST 200: baseline={ans.get('baseline')} identical_source={ans.get('identical_source')} compared={ans.get('compared')} "
        f"chunks_same={ans.get('chunks_same')} chunks_changed={len(changed)} since_last_changed={len(since_changed) if isinstance(since_changed, list) else since_changed} "
        f"chunks_unknown_to_us={len(ans.get('chunks_unknown_to_us') or [])} our_digest={ans.get('our_digest')}")
    log(f"{kg}: coverage={json.dumps(cov, sort_keys=True) if cov is not None else 'n/a'} change_suspect={json.dumps(suspect) if suspect else 'none'}")
    if suspect:
        log(f"{kg}: change NOT surfaced — report flagged change_suspect={json.dumps(suspect)} "
            f"({len(changed)} chunks_changed / {len(since_changed) if isinstance(since_changed, list) else 0} since_last ignored)")
        return dict(surfaced=False, reason=suspect, coverage=cov, ignored_chunks=len(changed))
    if ans.get("unchanged") is True:
        # The operator's verdict: nothing moved since the last report of this source/bbox. chunks_changed
        # then lists diffs against first-seen digests of other reports (other bbox/source) — not change.
        log(f"{kg}: no change — unchanged since {ans.get('unchanged_since')} ({len(changed)} first-seen diffs not surfaced)")
        return dict(surfaced=False, reason="unchanged", unchanged_since=ans.get("unchanged_since"), coverage=cov)
    if changed or (isinstance(since_changed, list) and since_changed):
        log(f"{kg}: CHANGE: {len(changed)} chunks vs baseline, {len(since_changed) if isinstance(since_changed, list) else 0} since last report "
            f"(coverage={json.dumps(cov, sort_keys=True) if cov is not None else 'n/a'})")
        return dict(surfaced=True, chunks_changed=changed, since_last_changed=since_changed, coverage=cov)
    return dict(surfaced=False, reason="no change", coverage=cov)


# ---------------------------------------------------------------- driver

def latest_report(outdir, kg):
    files = sorted(glob.glob(os.path.join(outdir, f"{kg}.????-??-??.json")))
    return files[-1] if files else None


SAME_PASS_H = 20   # umfeld neSamePassWindow: a re-post by the same observer < 20 h later is not a sighting


def last_post(outdir, kg):
    """(digest, posted_at epoch s) of our newest *accepted* POST of this KG from the meta files, else None."""
    for f in sorted(glob.glob(os.path.join(outdir, f"{kg}.????-??-??.meta.json")), reverse=True):
        try:
            with open(f) as fh:
                m = json.load(fh)
        except Exception:
            continue
        post = m.get("post") or {}
        if post.get("status") == "skipped" and post.get("reason") == "same_pass":
            prev = post.get("previous") or {}
            if prev.get("digest") and prev.get("posted_at"):
                return prev["digest"], float(prev["posted_at"])
            continue
        if post.get("status") == 200 and (m.get("report") or {}).get("digest"):
            return m["report"]["digest"], float(post.get("posted_at") or os.path.getmtime(f))
    return None


def process_kg(a, kg):
    kg = str(kg).zfill(5)
    today = dt.date.today().isoformat()
    os.makedirs(a.out, exist_ok=True)

    prev = latest_report(a.out, kg)
    if prev and not a.force:
        age = (time.time() - os.path.getmtime(prev)) / 86400
        if age < a.max_age_days:
            log(f"{kg}: skip — report {os.path.basename(prev)} is {age:.1f} d old (< {a.max_age_days} d)")
            return "skipped"

    head = umfeld_head(a.umfeld, kg)
    if head is None:
        log(f"{kg}: skip — umfeld /head 404 (KG not built yet)")
        return "not_built"
    lu = head.get("lu") or {}
    hdr = lu.get("header") or {}
    ibox = hdr.get("input_bbox")
    if not ibox or len(ibox) != 4:
        log(f"{kg}: skip — /head has no lu.header.input_bbox")
        return "no_bbox"
    umfeld_lu_n, umfeld_lu_digest = hdr.get("cells_n"), hdr.get("digest")
    bev = head.get("bev") or {}
    bev_hdr = bev.get("header") or {}
    # Viewport → aligned cells → the union of those cells is the build's input bbox (stable per cell block).
    cells = grid_cells(ibox)
    if not cells:
        log(f"{kg}: skip — viewport {ibox} covers no aligned cell")
        return "no_cells"
    ubox = cells_union(cells)
    bbox_s = ",".join(f"{v:.2f}" for v in ubox)
    full = full_cells_in(ibox, cells)
    log(f"{kg}: viewport={[float(v) for v in ibox]} → {len(cells)} aligned cells "
        f"(i {cells[0][0]}..{cells[-1][0]}, j {cells[0][1]}..{cells[-1][1]}; {len(full)} fully inside the viewport) "
        f"input_bbox={bbox_s}; umfeld lu cells_n={umfeld_lu_n} digest={umfeld_lu_digest}"
        + (f"; bev cells_n={bev_hdr.get('cells_n')} digest={bev_hdr.get('digest')} source={bev_hdr.get('source')}" if bev_hdr else ""))
    if len(full) < a.min_full_cells:
        log(f"{kg}: skip — viewport contains {len(full)} full aligned cell(s) < --min-full-cells {a.min_full_cells} "
            "(one single-cell report is worth more to the change signal than partial ones)")
        return "viewport_too_small"
    if len(cells) > a.max_cells:
        log(f"{kg}: skip — {len(cells)} cells exceeds --max-cells {a.max_cells}")
        return "too_big"

    work = os.path.join(a.work, kg)
    if os.path.isdir(work):
        shutil.rmtree(work)
    t0 = time.time()
    fetched = fetch_cells(a.bevc, cells, work, inflight=a.inflight, siedler=a.siedler or None)
    if len(fetched) != len(cells):                       # belt and braces: fetch_cells raises on any failed cell
        raise RuntimeError(f"{kg}: {len(fetched)} of {len(cells)} aligned cells fetched — refusing a partial build")
    log(f"{kg}: fetched {len(fetched)} cells in {time.time() - t0:.0f}s "
        f"(bevdirect {sorted({r['bevdirect_version'] for r in fetched})}, {sum(r['incomplete'] for r in fetched)} parcel copies truncated at the pad, "
        f"{sum(1 for r in fetched if r['src'] == 'stash')} from the game server's stash)")

    nec = os.path.join(a.out, "nec", f"{kg}.{a.epoch}.nec")
    os.makedirs(os.path.dirname(nec), exist_ok=True)
    build_args = ["build", "--kg", kg, "--epoch", a.epoch, "--input-bbox", bbox_s, "-o", nec]
    for i, j in cells:
        build_args += ["--bevdirect", os.path.join(work, f"cell_{i}_{j}.json")]
    t0 = time.time()
    build_out = run_ne_cells(build_args)
    summary = json.loads(build_out.strip().splitlines()[-1])
    log(f"{kg}: build ok in {time.time() - t0:.0f}s: source={summary['source']} cells={summary['cells']} kcells={summary['kcells']} "
        f"digest={summary['digest']} (umfeld lu cells_n={umfeld_lu_n}, Δ={summary['cells'] - umfeld_lu_n if umfeld_lu_n is not None else '?'})")

    with open(nec, "rb") as f:
        section = [sec for sec in unpack_sections(f.read()) if sec.magic == MAGIC_LU][0]
    report = epoch_report_ap(section, a.observer)   # `ne_cells report` + chunks_ap / ap_version
    report_s = json.dumps(report, separators=(",", ":"))
    report_path = os.path.join(a.out, f"{kg}.{today}.json")
    with open(report_path + ".tmp", "w") as f:
        f.write(report_s)
    os.replace(report_path + ".tmp", report_path)
    meta = dict(kg=kg, date=today, epoch=a.epoch, viewport_bbox=[float(v) for v in ibox], input_bbox=list(ubox),
                input_rule="union-of-aligned-cells", cells=[dict(i=i, j=j) for i, j in cells], full_cells=len(full), fetched=fetched,
                build=summary, umfeld=dict(lu_cells_n=umfeld_lu_n, lu_digest=umfeld_lu_digest, lu_source=hdr.get("source"),
                                           bev_cells_n=bev_hdr.get("cells_n"), bev_digest=bev_hdr.get("digest"), bev_source=bev_hdr.get("source")),
                report=dict(digest=report["digest"], cells_n=report["cells_n"], chunks=len(report["chunks"]), chunks_total=report["chunks_total"],
                            chunks_ap=len(report.get("chunks_ap") or {}), ap_version=report.get("ap_version")))
    log(f"{kg}: report → {report_path}: digest={report['digest']} cells_n={report['cells_n']} chunks={len(report['chunks'])}/{report['chunks_total']} "
        f"chunks_ap={len(report.get('chunks_ap') or {})}")

    token, token_src = load_token(a.token_file)
    lp = last_post(a.out, kg)
    same_pass = lp and lp[0] == report["digest"] and (time.time() - lp[1]) < SAME_PASS_H * 3600
    if not token:
        log(f"{kg}: POST skipped — no peer token ({a.token_file} absent and NE_PEER_TOKEN unset)")
        meta["post"] = dict(status="skipped", reason="no token")
    elif same_pass and not a.repost:
        # Same build, same observer, same pass: umfeld counts a sighting by the same observer only
        # ≥ 20 h after the previous one, so re-posting this digest now confirms nothing and would only
        # resend the chunk list. One POST per build; --repost overrides (protocol QA only).
        age_h = (time.time() - lp[1]) / 3600
        log(f"{kg}: POST skipped — same build (digest {report['digest']}) already reported {age_h:.1f} h ago "
            f"(< {SAME_PASS_H} h same-pass window); --repost to send anyway")
        meta["post"] = dict(status="skipped", reason="same_pass", previous=dict(digest=lp[0], posted_at=lp[1]),
                            age_h=round(age_h, 2))
    else:
        # Change protocol step 0 (umfeld 2026-10-08): a header-only report (no chunks{}/chunks_ap{}) is
        # deduped on token + source class + algo + digest + epoch, bbox-independent. `unchanged:true` =
        # our last report of this KG had the same digest → the chunk list (≈ 40–400 KB) is not sent at
        # all. Any other answer (changed, first report, old server → 4xx) falls through to the full POST.
        head = {k: v for k, v in report.items() if k not in ("chunks", "chunks_ap")}
        st, body = post_report(a.umfeld, kg, json.dumps(head, separators=(",", ":")).encode(), token, a.contrib_prefix)
        try:
            ans = json.loads(body)
        except Exception:
            ans = dict(raw=body[:500].decode("utf-8", "replace"))
        meta["post_head"] = dict(status=st, answer=ans)
        if st == 200 and ans.get("unchanged") is True:
            log(f"{kg}: header-only report: unchanged since {ans.get('unchanged_since')} — chunk list not sent")
            meta["post"] = dict(status=st, answer=ans, token_source=token_src, header_only=True, posted_at=time.time())
        else:
            if st == 202 and same_pass:
                # Should never happen: we reported this digest < 20 h ago and umfeld still wants chunks.
                log(f"{kg}: WARNING header probe answered 202 need={ans.get('need')} for a build reported "
                    f"{(time.time() - lp[1]) / 3600:.1f} h ago — step 0 dedupe miss on umfeld's side?")
                meta["step0_miss"] = True
            log(f"{kg}: header-only report HTTP {st}: unchanged={ans.get('unchanged')} — sending the full report")
            st, body = post_report(a.umfeld, kg, report_s.encode(), token, a.contrib_prefix)
            try:
                ans = json.loads(body)
            except Exception:
                ans = dict(raw=body[:500].decode("utf-8", "replace"))
            meta["post"] = dict(status=st, answer=ans, token_source=token_src, posted_at=time.time())
        if st == 200:
            meta["change"] = classify_change(kg, ans)
            want = ans.get("want_chunks") or []
            if want:
                meta["chunks"] = send_wanted_chunks(a, kg, section, want, token)
            else:
                log(f"{kg}: want_chunks empty — nothing to upload")
        else:
            log(f"{kg}: POST HTTP {st}: {json.dumps(ans)[:400]}")
    with open(os.path.join(a.out, f"{kg}.{today}.meta.json"), "w") as f:
        json.dump(meta, f, indent=1)
    if not a.keep_cells:
        shutil.rmtree(work, ignore_errors=True)
    return "ok"


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("kgs", nargs="+", help="5-digit KG codes")
    ap.add_argument("--epoch", default=dt.date.today().strftime("%Y-%m"), help="ISO month (default: current)")
    ap.add_argument("--observer", default="siedler-oesterreich")
    ap.add_argument("--bev", default=os.environ.get("BEV_API", "http://127.0.0.1:8787"))
    ap.add_argument("--siedler", default=os.environ.get("SIEDLER_API", "http://localhost:8000"),
                    help="game server whose RAM stash of raw cells is tried before bevdirect ('' = off)")
    ap.add_argument("--umfeld", default=os.environ.get("UMFELD_API", "https://umfeld-at.exe.xyz"))
    ap.add_argument("--contrib-prefix", default=os.environ.get("UMFELD_CONTRIB_PREFIX", "/contrib"),
                    help="path prefix of umfeld's contributor API for the report POST (default /contrib)")
    ap.add_argument("--out", default=os.path.join(REPO, "data", "ne-reports"))
    ap.add_argument("--work", default=os.path.join(REPO, "data", "ne-reports", "work"))
    ap.add_argument("--token-file", default=os.path.join(REPO, "ne-peer.key"))
    ap.add_argument("--inflight", type=int, default=2, help="concurrent bevdirect requests (default 2 — it serves the live game)")
    ap.add_argument("--max-cells", type=int, default=600, help="sanity guard only: refuse KGs needing more bevdirect cells than this (largest KG in the admin table needs 345, with the 0.004° pad; every KG must be reportable)")
    ap.add_argument("--min-full-cells", type=int, default=1,
                    help="skip a KG whose viewport (umfeld input_bbox) contains fewer fully covered aligned cells than this; "
                         "0 = any viewport (the build input is always whole aligned cells, see docstring)")
    ap.add_argument("--max-age-days", type=float, default=7, help="skip a KG with a report younger than this")
    ap.add_argument("--force", action="store_true",
                    help="ignore --max-age-days (rebuild); the POST is still skipped when the same digest was posted < 20 h ago")
    ap.add_argument("--repost", action="store_true",
                    help="POST even if we reported this very digest < 20 h ago (umfeld counts it as no sighting; protocol QA only)")
    ap.add_argument("--keep-cells", action="store_true", help="keep the fetched cell_*.json under --work")
    ap.add_argument("--pause", type=float, default=5, help="seconds between KGs")
    a = ap.parse_args(argv)
    # No disk cache in the client: the fetched documents live only under --work (pruned ≤ 24 h by run.sh).
    a.bevc = BevDirect(a.bev, cache_dir=None, deadline_s=900, log=log)
    try:
        log(f"bevdirect-serve {a.bev}: {a.bevc.version}")
    except ServerError as e:
        log(f"bevdirect-serve {a.bev}: {e}")

    results = {}
    for n, kg in enumerate(a.kgs):
        try:
            results[kg] = process_kg(a, kg)
        except Exception as e:  # one KG failing must not stop the run
            log(f"{kg}: FAILED: {e}")
            results[kg] = "failed"
        if n + 1 < len(a.kgs) and results[kg] == "ok":
            time.sleep(a.pause)
    log("done: " + " ".join(f"{k}={v}" for k, v in results.items()))
    return 1 if any(v == "failed" for v in results.values()) else 0


if __name__ == "__main__":
    sys.exit(main())
