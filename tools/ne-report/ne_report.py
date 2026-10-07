#!/usr/bin/env python3
"""ne_report.py — NE epoch report for one KG from our local bevdirect-serve cells.

Pipeline (contract: docs/ne-report.md, umfeld's docs/ne-cells.md):
  1. GET {umfeld}/api/v1/ne/{kg}/head          → lu.header.input_bbox  (404 → KG not built yet, skip)
  2. GET {bev}/viewport for every 0.02° cell intersecting input_bbox (≤ 2 in flight, honour pending/retry_after_s)
  3. python -m ne_cells build --kg KG --epoch E --bevdirect cell_*.json --input-bbox W,S,E,N -o KG.nec
     (source derived from bevdirect_version in the documents — never pass --source)
  4. python -m ne_cells report KG.nec --observer siedler-oesterreich  → data/ne-reports/KG.<date>.json
  5. POST {umfeld}/contrib/api/v1/ne/{kg}/report with `Authorization: Bearer $NE_PEER_TOKEN` if a token is configured, else a logged no-op.

Run through the venv: tools/ne-report/.venv/bin/python tools/ne-report/ne_report.py 05007
"""
import argparse
import concurrent.futures as cf
import datetime as dt
import glob
import json
import math
import os
import shutil
import subprocess
import sys
import time
import urllib.error
import urllib.request

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.abspath(os.path.join(HERE, "..", ".."))
GRID = 0.02
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


# ---------------------------------------------------------------- 2. bevdirect cells

def grid_cells(bbox):
    W, S, E, N = bbox
    i0, i1 = math.floor(W / GRID), math.floor(E / GRID)
    j0, j1 = math.floor(S / GRID), math.floor(N / GRID)
    return [(i, j) for j in range(j0, j1 + 1) for i in range(i0, i1 + 1)]


def fetch_cell(bev, i, j, path, wait=20, max_total_s=900):
    """GET one 0.02° cell, re-GET while bevdirect answers ready:false/pending. Never writes a non-ready doc."""
    url = (f"{bev}/viewport?west={i*GRID:.2f}&south={j*GRID:.2f}&east={(i+1)*GRID:.2f}&north={(j+1)*GRID:.2f}"
           f"&layers=parcels,footprints,landuse&wait={wait}")
    t0 = time.time()
    errors = 0
    while True:
        try:
            st, body, _ = http_get(url, timeout=wait + 40)
        except (urllib.error.URLError, TimeoutError, OSError) as e:
            errors += 1
            if errors > 6:
                raise RuntimeError(f"cell {i}_{j}: {e}")
            log(f"cell {i}_{j}: {e}; backoff")
            time.sleep(10 * errors)
            continue
        if st != 200:
            errors += 1
            if errors > 6:
                raise RuntimeError(f"cell {i}_{j}: HTTP {st} {body[:200]!r}")
            log(f"cell {i}_{j}: HTTP {st}; backoff")
            time.sleep(10 * errors)
            continue
        doc = json.loads(body)
        if doc.get("ready") is True and not doc.get("pending") and not doc.get("truncated"):
            # bevdirect-serve v0.2.1 emits `"footprints": null` for a cell without buildings; the frozen
            # ne_cells.canon does `doc.get("footprints", [])` and crashes on None. Normalise null layer
            # lists to [] (only `inputs[].file_sha256` — informative provenance — sees the difference).
            nulls = [k for k in ("parcels", "footprints", "landuse") if doc.get(k) is None]
            if nulls:
                for k in nulls:
                    doc[k] = []
                doc["null_layers_normalised"] = nulls
                body = json.dumps(doc, separators=(",", ":"), ensure_ascii=False).encode()
            with open(path + ".tmp", "wb") as f:
                f.write(body)
            os.replace(path + ".tmp", path)
            return dict(i=i, j=j, parcels=len(doc.get("parcels") or []), footprints=len(doc.get("footprints") or []),
                        landuse=len(doc.get("landuse") or []), bevdirect_version=doc.get("bevdirect_version"),
                        coord_decimals=doc.get("coord_decimals"), ms=doc.get("query_time_ms"), s=round(time.time() - t0, 1))
        if doc.get("truncated"):
            raise RuntimeError(f"cell {i}_{j}: truncated:true — not a valid NE input")
        ra = float(doc.get("retry_after_s") or 5)
        if time.time() - t0 > max_total_s:
            raise RuntimeError(f"cell {i}_{j}: still pending after {max_total_s}s")
        log(f"cell {i}_{j}: pending, retry in {ra:.0f}s")
        time.sleep(min(max(ra, 1), 60))


def fetch_cells(bev, cells, workdir, inflight=2):
    os.makedirs(workdir, exist_ok=True)
    out = []
    with cf.ThreadPoolExecutor(max_workers=inflight) as ex:
        futs = {ex.submit(fetch_cell, bev, i, j, os.path.join(workdir, f"cell_{i}_{j}.json")): (i, j) for i, j in cells}
        for fut in cf.as_completed(futs):
            r = fut.result()          # raises on a failed cell → whole KG aborts (never build on a partial set)
            log(f"  cell {r['i']}_{r['j']}: parcels={r['parcels']} footprints={r['footprints']} landuse={r['landuse']} "
                f"bevdirect={r['bevdirect_version']} ({r['s']}s)")
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


# ---------------------------------------------------------------- driver

def latest_report(outdir, kg):
    files = sorted(glob.glob(os.path.join(outdir, f"{kg}.????-??-??.json")))
    return files[-1] if files else None


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
    bbox_s = ",".join(repr(float(v)) for v in ibox)
    umfeld_lu_n, umfeld_lu_digest = hdr.get("cells_n"), hdr.get("digest")
    bev = head.get("bev") or {}
    bev_hdr = bev.get("header") or {}
    cells = grid_cells(ibox)
    log(f"{kg}: input_bbox={bbox_s} → {len(cells)} bevdirect cells "
        f"(i {cells[0][0]}..{cells[-1][0]}, j {cells[0][1]}..{cells[-1][1]}); umfeld lu cells_n={umfeld_lu_n} digest={umfeld_lu_digest}"
        + (f"; bev cells_n={bev_hdr.get('cells_n')} digest={bev_hdr.get('digest')} source={bev_hdr.get('source')}" if bev_hdr else ""))
    if len(cells) > a.max_cells:
        log(f"{kg}: skip — {len(cells)} cells exceeds --max-cells {a.max_cells}")
        return "too_big"

    work = os.path.join(a.work, kg)
    if os.path.isdir(work):
        shutil.rmtree(work)
    t0 = time.time()
    fetched = fetch_cells(a.bev, cells, work, inflight=a.inflight)
    log(f"{kg}: fetched {len(fetched)} cells in {time.time() - t0:.0f}s")

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

    report_s = run_ne_cells(["report", nec, "--observer", a.observer], capture_stdout=True)
    report = json.loads(report_s)
    report_path = os.path.join(a.out, f"{kg}.{today}.json")
    with open(report_path + ".tmp", "w") as f:
        f.write(report_s)
    os.replace(report_path + ".tmp", report_path)
    meta = dict(kg=kg, date=today, epoch=a.epoch, input_bbox=ibox, cells=[dict(i=i, j=j) for i, j in cells], fetched=fetched,
                build=summary, umfeld=dict(lu_cells_n=umfeld_lu_n, lu_digest=umfeld_lu_digest, lu_source=hdr.get("source"),
                                           bev_cells_n=bev_hdr.get("cells_n"), bev_digest=bev_hdr.get("digest"), bev_source=bev_hdr.get("source")),
                report=dict(digest=report["digest"], cells_n=report["cells_n"], chunks=len(report["chunks"]), chunks_total=report["chunks_total"]))
    log(f"{kg}: report → {report_path}: digest={report['digest']} cells_n={report['cells_n']} chunks={len(report['chunks'])}/{report['chunks_total']}")

    token, token_src = load_token(a.token_file)
    if not token:
        log(f"{kg}: POST skipped — no peer token ({a.token_file} absent and NE_PEER_TOKEN unset)")
        meta["post"] = dict(status="skipped", reason="no token")
    else:
        st, body = post_report(a.umfeld, kg, report_s.encode(), token, a.contrib_prefix)
        try:
            ans = json.loads(body)
        except Exception:
            ans = dict(raw=body[:500].decode("utf-8", "replace"))
        meta["post"] = dict(status=st, answer=ans, token_source=token_src)
        if st == 200:
            log(f"{kg}: POST 200: identical_source={ans.get('identical_source')} compared={ans.get('compared')} "
                f"chunks_same={ans.get('chunks_same')} chunks_changed={ans.get('chunks_changed')} "
                f"chunks_unknown_to_us={ans.get('chunks_unknown_to_us')} our_digest={ans.get('our_digest')}")
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
    ap.add_argument("--umfeld", default=os.environ.get("UMFELD_API", "https://umfeld-at.exe.xyz"))
    ap.add_argument("--contrib-prefix", default=os.environ.get("UMFELD_CONTRIB_PREFIX", "/contrib"),
                    help="path prefix of umfeld's contributor API for the report POST (default /contrib)")
    ap.add_argument("--out", default=os.path.join(REPO, "data", "ne-reports"))
    ap.add_argument("--work", default=os.path.join(REPO, "data", "ne-reports", "work"))
    ap.add_argument("--token-file", default=os.path.join(REPO, "ne-peer.key"))
    ap.add_argument("--inflight", type=int, default=2, help="concurrent bevdirect requests (default 2 — it serves the live game)")
    ap.add_argument("--max-cells", type=int, default=600, help="sanity guard only: refuse KGs needing more bevdirect cells than this (largest KG in the admin table needs 345, with the 0.004° pad; every KG must be reportable)")
    ap.add_argument("--max-age-days", type=float, default=7, help="skip a KG with a report younger than this")
    ap.add_argument("--force", action="store_true", help="ignore --max-age-days")
    ap.add_argument("--keep-cells", action="store_true", help="keep the fetched cell_*.json under --work")
    ap.add_argument("--pause", type=float, default=5, help="seconds between KGs")
    a = ap.parse_args(argv)

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
