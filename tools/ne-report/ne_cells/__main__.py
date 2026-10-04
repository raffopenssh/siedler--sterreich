"""CLI.
  python3 -m ne_cells fetch --api https://umfeld-at.exe.xyz --token $TOKEN --kg 63330 -o inputs/   (strict, waits for ready)
  python3 -m ne_cells build --kg 63330 --epoch 2026-03 --source index-2026-03 \
      --parcels parcels.json --footprints footprints.json --landuse landuse.json [--input-bbox W,S,E,N] -o 63330.nec
  python3 -m ne_cells build --kg 63330 --epoch 2026-10 --bevdirect cell1.json [--bevdirect cell2.json …] -o today.nec
      (source = bevdirect@<bevdirect_version> read from the documents; --source bevdirect@<tag> to override)
  python3 -m ne_cells dump 63330.nec            → JSON (header + cells + kcells) on stdout
  python3 -m ne_cells compare a.nec b.nec [--tol 8]
  python3 -m ne_cells report today.nec --observer siedler-vm-1 [--bbox W,S,E,N]   → epoch report JSON (digests only)
"""
import argparse
import hashlib
import json
import os
import sys
import time

from . import canon
from .algo import ALGO, H3_RES, K_RES, K, INPUT_PAD_DEG, INPUT_RULE, build_cells
from .pack import MAGIC_LU, lu_header, pack_records, pack_section, unpack_sections, epoch_report


def _load(path):
    with open(path) as f:
        return json.load(f)


_ROW_KEYS = {"parcels": "parcel_id", "footprints": "footprint_id", "landuse": None}


def content_sha256(doc) -> str:
    """Provenance hash over CANONICAL content, so two strict fetches of the same rows compare equal
    regardless of fetched_at / row order / whitespace: for each layer list (parcels, footprints,
    landuse) the rows are serialised as canonical JSON (sorted keys, compact), sorted, and hashed as
    "<layer>\n" + rows joined by "\n". Document-level keys (fetched_at, bbox, …) are NOT included."""
    h = hashlib.sha256()
    for key in ("parcels", "footprints", "landuse"):
        rows = doc.get(key) if isinstance(doc, dict) else None
        if not isinstance(rows, list):
            continue
        h.update((key + "\n").encode())
        for r in sorted(json.dumps(r, sort_keys=True, separators=(",", ":"), ensure_ascii=True) for r in rows):
            h.update(r.encode())
            h.update(b"\n")
    return h.hexdigest()[:16]


def _file_prov(path, rows_key=None, doc=None):
    with open(path, "rb") as f:
        b = f.read()
    if doc is None:
        doc = json.loads(b)
    # sha256 = canonical content (see content_sha256); file_sha256 = the bytes as fetched (informative).
    d = dict(file=os.path.basename(path), sha256=content_sha256(doc), file_sha256=hashlib.sha256(b).hexdigest()[:16],
             bytes=len(b), hash_rule="content-sorted-rows-1")
    if isinstance(doc, dict):
        for k in ("ready", "truncated", "source", "bevdirect_version", "fetched_at", "bbox", "strict", "endpoint"):
            if k in doc:
                d[k] = doc[k]
        if rows_key and isinstance(doc.get(rows_key), list):
            d["rows"] = len(doc[rows_key])
        for k in ("parcels", "footprints", "landuse"):
            if rows_key is None and isinstance(doc.get(k), list):
                d[k] = len(doc[k])
    return d


def cmd_build(a):
    t0 = time.time()
    bbox = tuple(map(float, a.bbox.split(","))) if a.bbox else None
    ibox = tuple(map(float, a.input_bbox.split(","))) if a.input_bbox else None
    prov = []
    if a.bevdirect:
        docs = [_load(x) for x in a.bevdirect]
        for x, d in zip(a.bevdirect, docs):
            if not d.get("ready", True):
                raise SystemExit(f"{x}: ready:false — refusing to build on a partial fetch")
            prov.append(_file_prov(x, None, d))
        P, F, L = canon.from_bevdirect(docs)
        source = a.source or canon.bevdirect_source(docs)
        if not source.startswith("bevdirect@"):
            raise SystemExit("--source for bevdirect input must be bevdirect@<tag>")
    else:
        if not a.source:
            raise SystemExit("--source required for index input (e.g. index-2026-03)")
        source = a.source
        docs = {}
        for key, path in (("parcels", a.parcels), ("footprints", a.footprints), ("landuse", a.landuse)):
            docs[key] = _load(path)
            if not docs[key].get("ready", True) or docs[key].get("truncated"):
                raise SystemExit(f"{path}: ready:false or truncated — refusing to build on a partial input (use `ne_cells fetch`)")
            prov.append(_file_prov(path, key, docs[key]))
        P = canon.parcels_from_index(docs["parcels"]["parcels"])
        F = canon.footprints_from_index(docs["footprints"]["footprints"])
        L = canon.landuse_from_index(docs["landuse"]["landuse"])
    res = build_cells(a.kg, P, F, L, coverage_bbox=bbox, input_bbox=ibox)
    raw = pack_records(res.cells, res.kcells)
    hdr = lu_header(a.kg, a.epoch, source, ALGO, H3_RES, K_RES, K, len(res.cells), len(res.kcells), res.stats,
                    input_bbox=res.input_bbox, coverage_bbox=bbox or res.input_bbox, input_pad_deg=INPUT_PAD_DEG,
                    input_rule=INPUT_RULE, inputs=prov)
    blob = pack_section(MAGIC_LU, hdr, raw)
    with open(a.output, "wb") as f:
        f.write(blob)
    sec = unpack_sections(blob)[0]
    print(json.dumps(dict(kg=res.kg, algo=ALGO, source=source, epoch=a.epoch, input_bbox=list(res.input_bbox or ()),
                          cells=len(res.cells), kcells=len(res.kcells), raw_bytes=len(raw),
                          file_bytes=len(blob), digest=sec.header["digest"], seconds=round(time.time() - t0, 1),
                          stats=res.stats)), file=sys.stderr)


def cmd_fetch(a):
    """Fetch the three index layers for a KG's input domain from the private API, STRICT: every
    sub-box must answer ready:true (202 → wait + retry) and truncated:false. Output = the three
    documents `build` expects, merged over sub-boxes and de-duplicated, plus inputs.json."""
    import urllib.request
    import urllib.parse
    api = a.api.rstrip("/")
    hdrs = {"Authorization": "Bearer " + a.token, "Accept": "application/json"}

    def get(path, params):
        url = f"{api}{path}?{urllib.parse.urlencode(params)}"
        for attempt in range(200):
            req = urllib.request.Request(url, headers=hdrs)
            try:
                with urllib.request.urlopen(req, timeout=300) as r:
                    return json.loads(r.read()), r.status
            except urllib.error.HTTPError as e:
                if e.code == 202:
                    ra = int(e.headers.get("Retry-After") or 5)
                    body = json.loads(e.read() or b"{}")
                    print(f"  pending {body.get('kgs_pending')} — retry in {ra}s", file=sys.stderr)
                    time.sleep(min(ra, 30))
                    continue
                raise
        raise SystemExit("gave up waiting for ready")

    kg = str(a.kg).zfill(5)
    step = a.step
    layers = dict(parcels=("/k/api/v1/spatial/parcels", "parcels", "parcel_id"),
                  footprints=("/k/api/v1/spatial/footprints", "footprints", "footprint_id"),
                  landuse=("/k/api/v1/spatial/landuse", "landuse", None))

    def fetch_layer(name, W, S, E, N):
        path, key, idk = layers[name]
        # ONE call per layer (strict ⇒ limit up to 100 000): /spatial/landuse answers from the candidate
        # KGs' files and neighbouring files hold differently-truncated copies of the same piece, so a
        # sub-box grid would make the row set grid-dependent. `step` only splits when a call truncates.
        rows, seen, calls = [], set(), 0
        import math
        nx, ny = max(1, math.ceil((E - W) / step - 1e-9)), max(1, math.ceil((N - S) / step - 1e-9))
        for iy in range(ny):
            for ix in range(nx):
                x0, y0 = W + ix * (E - W) / nx, S + iy * (N - S) / ny
                x1, y1 = W + (ix + 1) * (E - W) / nx, S + (iy + 1) * (N - S) / ny
                sub = dict(west=f"{x0:.5f}", south=f"{y0:.5f}", east=f"{x1:.5f}", north=f"{y1:.5f}",
                           limit=100000, strict=1, geometry=1)
                if name == "landuse":
                    sub["wait"] = 120
                if name == "parcels":
                    sub["attrs"] = "min"
                d, _ = get(path, sub)
                calls += 1
                if not d.get("ready") or d.get("truncated"):
                    raise SystemExit(f"{name} {sub}: ready={d.get('ready')} truncated={d.get('truncated')} — the domain exceeds one call; this KG needs a per-KG export input (open an issue)")
                for r in d[key]:
                    k = r[idk] if idk else (r.get("kg_code"), r.get("code"), json.dumps(r.get("geometry"), sort_keys=True, separators=(",", ":")))
                    if k in seen:
                        continue
                    seen.add(k)
                    rows.append(r)
        return rows, calls

    # 1. the input domain is defined by the OWN parcels' canonical bounds (algo.default_input_bbox), not by
    #    the kg table's bbox (legacy rows are off by up to 0.01°): preliminary parcel fetch with a wide pad.
    from .algo import default_input_bbox
    if a.bbox:
        W, S, E, N = map(float, a.bbox.split(","))
    else:
        d, _ = get("/k/api/v1/search/kg", dict(q=kg))
        row = next(r for r in d["data"] if r["kg_code"] == kg)
        b = row["bbox"]
        W, S, E, N = b["min_lon"], b["min_lat"], b["max_lon"], b["max_lat"]
    pre = (W - 0.02, S - 0.02, E + 0.02, N + 0.02)
    prows, _ = fetch_layer("parcels", *pre)
    own = [p["geometry"] for p in canon.parcels_from_index(prows) if p["kg"] == kg]
    if not own:
        raise SystemExit(f"no parcels of KG {kg} in {pre}")
    W, S, E, N = default_input_bbox(own, a.pad)
    if W < pre[0] or S < pre[1] or E > pre[2] or N > pre[3]:
        raise SystemExit(f"input bbox {W},{S},{E},{N} exceeds the preliminary fetch {pre} — pass --bbox")
    print(f"input bbox (bbox(own parcels) + {a.pad}°, 4 dp outward): {W},{S},{E},{N}", file=sys.stderr)
    os.makedirs(a.output, exist_ok=True)
    manifest = dict(api=api, kg=kg, input_bbox=[W, S, E, N], input_pad_deg=a.pad, step_deg=step, strict=True,
                    fetched_at=time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()), layers={})
    # fetch a 0.002° margin around the domain: the algo keeps only objects intersecting input_bbox, and
    # an object merely TOUCHING the domain edge must not depend on the server's R-tree float rounding.
    m = 0.002
    manifest["fetch_margin_deg"] = m
    for name, (path, key, idk) in layers.items():
        rows, calls = fetch_layer(name, W - m, S - m, E + m, N + m)
        doc = {key: rows, "input_bbox": [W, S, E, N], "ready": True, "truncated": False, "strict": True, "endpoint": path,
               "bbox": dict(west=W, south=S, east=E, north=N), "source": "index", "fetched_at": manifest["fetched_at"],
               "sub_boxes": calls}
        out = os.path.join(a.output, f"{name}.json")
        with open(out, "w") as f:
            json.dump(doc, f, separators=(",", ":"))
        manifest["layers"][name] = _file_prov(out, key, doc)
        print(f"  {name}: {len(rows)} rows over {calls} sub-boxes → {out}", file=sys.stderr)
    with open(os.path.join(a.output, "inputs.json"), "w") as f:
        json.dump(manifest, f, indent=1)
    print(json.dumps(manifest), file=sys.stderr)


def cmd_dump(a):
    with open(a.file, "rb") as f:
        secs = unpack_sections(f.read())
    out = []
    for s in secs:
        d = dict(header=s.header)
        if s.magic == MAGIC_LU:
            cells, kcells = s.records()
            d["cells"], d["kcells"] = cells, kcells
        else:
            d["raw_bytes"] = len(s.raw)
        out.append(d)
    json.dump(out if len(out) > 1 else out[0], sys.stdout, separators=(",", ":"))


def cmd_report(a):
    """Epoch report for a build: chunk digests only. POST it to /k/api/v1/ne/{kg}/report."""
    with open(a.file, "rb") as f:
        secs = [s for s in unpack_sections(f.read()) if s.magic == MAGIC_LU]
    bbox = tuple(map(float, a.bbox.split(","))) if a.bbox else None
    json.dump(epoch_report(secs[0], a.observer, bbox), sys.stdout, separators=(",", ":"))


def cmd_compare(a):
    def lu_of(path):
        with open(path, "rb") as f:
            for s in unpack_sections(f.read()):
                if s.magic == MAGIC_LU:
                    return s
        raise SystemExit(f"{path}: no NEC1 section")
    A, B = lu_of(a.a), lu_of(a.b)
    ca, _ = A.records()
    cb, _ = B.records()
    same = exact = 0
    diffs = []
    for c in sorted(set(ca) | set(cb)):
        x, y = ca.get(c), cb.get(c)
        if x is None or y is None:
            diffs.append((c, "missing"))
            continue
        if x == y:
            exact += 1
            same += 1
            continue
        worst = max(abs(p - q) for p, q in zip(x["lu"], y["lu"]))
        if worst <= a.tol and abs(x["bld_cover"] - y["bld_cover"]) <= a.tol:
            same += 1
        else:
            diffs.append((c, worst))
    print(json.dumps(dict(digest_a=A.header["digest"], digest_b=B.header["digest"], identical_digest=A.header["digest"] == B.header["digest"],
                          cells_a=len(ca), cells_b=len(cb), exact=exact, within_tol=same, differing=len(diffs),
                          sample=diffs[:20])))


def main(argv=None):
    ap = argparse.ArgumentParser(prog="ne_cells")
    sp = ap.add_subparsers(dest="cmd", required=True)
    b = sp.add_parser("build")
    b.add_argument("--kg", required=True)
    b.add_argument("--epoch", required=True, help="ISO month, e.g. 2026-03")
    b.add_argument("--source", help="index-2026-03 | bevdirect@<tag> (default for bevdirect input: from the documents)")
    b.add_argument("--parcels"); b.add_argument("--footprints"); b.add_argument("--landuse")
    b.add_argument("--bevdirect", action="append")
    b.add_argument("--bbox", help="coverage bbox for K cells (default = input bbox)")
    b.add_argument("--input-bbox", help="input domain W,S,E,N (default: bbox(own parcels)+0.004°, 4 dp outward; header input_bbox)")
    b.add_argument("-o", "--output", required=True)
    b.set_defaults(fn=cmd_build)
    fe = sp.add_parser("fetch")
    fe.add_argument("--api", default="https://umfeld-at.exe.xyz"); fe.add_argument("--token", default=os.environ.get("CADASTRE_TOKEN", ""))
    fe.add_argument("--kg", required=True); fe.add_argument("--bbox", help="KG bbox override W,S,E,N (unpadded)")
    fe.add_argument("--pad", type=float, default=INPUT_PAD_DEG); fe.add_argument("--step", type=float, default=1.0, help="sub-box size; default = one call per layer")
    fe.add_argument("-o", "--output", required=True, help="directory")
    fe.set_defaults(fn=cmd_fetch)
    d = sp.add_parser("dump"); d.add_argument("file"); d.set_defaults(fn=cmd_dump)
    r = sp.add_parser("report"); r.add_argument("file"); r.add_argument("--observer", required=True)
    r.add_argument("--bbox"); r.set_defaults(fn=cmd_report)
    c = sp.add_parser("compare"); c.add_argument("a"); c.add_argument("b"); c.add_argument("--tol", type=int, default=8)
    c.set_defaults(fn=cmd_compare)
    a = ap.parse_args(argv)
    a.fn(a)


if __name__ == "__main__":
    main()
