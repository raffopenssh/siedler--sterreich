"""Low-memory, bit-identical re-implementation of ne_cells.algo.build_cells + __main__._finish.

The frozen reference (ne_cells, algo ne-cells-2) keeps every res-12 cell polygon, every (object ∩ cell)
piece and every output row of the whole KG in Python objects at once. For 80110 (Sölden, 1.55 M res-12
cells, 345 aligned cells) that is > 5 GB and the reporter is OOM-killed. This module computes the SAME
record bytes (same digest, same header, same stats) by walking the KG in blocks of one res-7 parent
(≤ 16 807 res-12 cells): per block the cell polygons, STRtree, accumulators and rows exist, are packed into
the record block and dropped.

Why the bytes are identical (keep this true — ALGO stays ne-cells-2, ne_cells/ is untouched):
* every per-cell quantity is a function of the objects whose geometry really intersects the cell (the
  reference gates every effect on `_area(x) > 0`), and we test exactly the same (object, cell) pairs —
  the block bbox prefilter only skips objects whose envelope misses every cell envelope of the block;
* the only order-dependent float is `line` (Σ boundary length in parcel order) and the orientation
  sums (footprint order): we visit candidates in ascending P/F index, i.e. the reference order;
* unions are WKB-sorted in the reference, so list order never matters there;
* records are emitted in ascending cell id, blocks in ascending res-7 parent — children of one parent are
  contiguous in int order, so the concatenation IS `sorted(cells)`; K records are sorted at the end.

QA: `python ne_lowmem.py --check KG…` builds a KG both ways and compares the .nec bytes (tools/ne-report/
README section in docs/ne-report.md § Memory).
"""
import collections
import math
import os
import time

import h3
import numpy as np
import shapely
from shapely import affinity
from shapely.strtree import STRtree

from ne_cells import algo
from ne_cells.algo import (ALGO, CELL_PAD_DEG, COV_UNKNOWN, FLAG_DECLARED_UNKNOWN, FLAG_NORMALISED, H3_RES,
                           INPUT_PAD_DEG, INPUT_RULE, K, K_RES, MIN_LAND, NONE8, NONE16, BuildResult, _area,
                           _in_domain, _lower_median, _pct, _union_sorted, cell_affine, cell_polygon,
                           default_input_bbox, footprint_axis_deg, q255)
from ne_cells.ns_groups import GROUP_ORDER, group_of
from ne_cells.pack import K_REC, LU_REC, MAGIC_LU, h3_parent, lu_header, pack_section, unpack_sections

BLOCK_RES = 7            # one block = the res-12 cells of one res-7 parent (≤ 7^5 = 16 807 cells, ~5 km²)


def _parents(cids: np.ndarray, res: int) -> np.ndarray:
    """Vectorised pack.h3_parent for an array of res-12 ids."""
    mask = 0
    for d in range(res + 1, 16):
        mask |= 0x7 << ((15 - d) * 3)
    return (cids & ~np.uint64(0xF << 52)) | np.uint64(res << 52) | np.uint64(mask)


def _row(c, d, cp, st):
    """One LU row — the per-cell block of algo.build_cells, verbatim."""
    cell_a = cp.area
    land_g = _union_sorted(d["land"])
    land_a = _area(land_g)
    land = land_a / cell_a
    if land < MIN_LAND:
        st["slivers_omitted"] += 1
        return None
    own_a = _area(_union_sorted(d["own"]))
    kg_share = own_a / cell_a
    if q255(kg_share) == 0:
        st["no_own_share"] += 1
        return None
    geb_g = _union_sorted(d["fp"] + d["lu"].get("geb", []))
    geb_a = _area(geb_g)
    shares = [0.0] * 9
    shares[0] = geb_a / cell_a
    for gi, grp in enumerate(GROUP_ORDER):
        if gi == 0:
            continue
        u = _union_sorted(d["lu"].get(grp, []))
        if u is None:
            continue
        if geb_g is not None:
            u = u.difference(geb_g)
        shares[gi] = _area(u) / cell_a
    tot = sum(shares)
    flags = 0
    cov = tot / land if land > 0 else 0.0
    if tot > land and tot > 0:
        shares = [s * land / tot for s in shares]
        if tot - land >= 1.0 / 255:
            flags |= FLAG_NORMALISED
            st["normalised"] += 1
    if cov < COV_UNKNOWN:
        flags |= FLAG_DECLARED_UNKNOWN
        st["declared_unknown"] += 1
    lu = [q255(s) for s in shares]
    bld_cover = q255(_area(_union_sorted(d["fp"])) / cell_a)
    coh, axis = NONE8, NONE8
    if d["bld_n"] >= K:
        xs = sum(math.cos(math.radians(2 * a)) for a in d["bld_or"])
        ys = sum(math.sin(math.radians(2 * a)) for a in d["bld_or"])
        coh = q255(math.hypot(xs, ys) / len(d["bld_or"]))
        axis = int(((math.degrees(math.atan2(ys, xs)) / 2.0) % 180.0) / 2.0) % 90
    land_m2 = affinity.affine_transform(land_g, cell_affine(c)).area if land_g is not None else 0.0
    line = int(d["line"] / (land_m2 / 1e4) + 0.5) if land_m2 > 50 else 0
    return dict(kg_share=q255(kg_share), land=q255(land), cov=q255(min(cov, 1.0)), flags=flags,
                lu=lu, bld_cover=bld_cover, bld_n=min(NONE16 - 1, d["bld_n"]),
                gk=q255(_area(_union_sorted(d["g"])) / land_a) if land_a > 0 else 0,
                n_parc=min(255, len(d["ids"])), line_m_ha=min(NONE16 - 1, line),
                orient_coh=coh, orient_axis=axis)


def build_packed(kg: str, parcels, footprints, landuse, coverage_bbox=None, input_bbox=None, log=None):
    """≡ pack_records(*build_cells(...)) → (raw record bytes, cells_n, kcells_n, stats, input_bbox)."""
    kg = str(kg).zfill(5)
    # ── dedupe (tile copies) and sort — verbatim ───────────────────────────────────────
    copies = collections.defaultdict(list)
    for p in parcels:
        copies[p["id"]].append(p)
    byid = {}
    for pid, cs in copies.items():
        complete = [p for p in cs if p.get("complete", True)]
        if complete:
            byid[pid] = max(complete, key=lambda p: (p["geometry"].area, p["geometry"].wkb))
        else:
            u = _union_sorted([p["geometry"] for p in cs])
            q = dict(max(cs, key=lambda p: (p["geometry"].area, p["geometry"].wkb)))
            q["geometry"] = u
            byid[pid] = q
    del copies
    P = sorted(byid.values(), key=lambda p: p["id"])
    del byid
    F = sorted({f["geometry"].wkb: f for f in footprints}.values(), key=lambda f: f["geometry"].wkb)
    L = sorted({(l["ns"], l["geometry"].wkb): l for l in landuse}.values(), key=lambda l: (l["ns"], l["geometry"].wkb))
    own = [p for p in P if p["kg"] == kg]
    if not own:
        return b"", 0, 0, dict(parcels=0, footprints=len(F), landuse=len(L), cells=0, kcells=0), input_bbox
    if input_bbox is None:
        input_bbox = default_input_bbox([p["geometry"] for p in own])
    input_bbox = tuple(float(v) for v in input_bbox)
    if coverage_bbox is None:
        coverage_bbox = input_bbox
    n_in = (len(P), len(F), len(L))
    P, F, L = _in_domain(P, input_bbox), _in_domain(F, input_bbox), _in_domain(L, input_bbox)
    own = [p for p in P if p["kg"] == kg]
    own_union = _union_sorted([p["geometry"] for p in own])
    # ── candidate cells as uint64, blocked by res-7 parent ────────────────────────────
    cand = np.unique(np.fromiter((h3.str_to_int(c) for c in h3.geo_to_cells(shapely.buffer(own_union, CELL_PAD_DEG), H3_RES)),
                                 dtype=np.uint64))
    par = _parents(cand, BLOCK_RES)
    assert np.all(np.diff(par.astype(np.int64)) >= 0), "res-12 ids of one parent must be contiguous in int order"
    bounds = np.flatnonzero(np.diff(par)) + 1
    starts = np.concatenate(([0], bounds))
    ends = np.concatenate((bounds, [len(cand)]))
    del par
    own_prepared = shapely.prepared.prep(own_union)
    W, S, E, N = input_bbox
    ptree = STRtree([p["geometry"] for p in P])
    ftree = STRtree([f["geometry"] for f in F])
    ltree = STRtree([l["geometry"] for l in L])
    lgrp = [group_of(l["ns"]) for l in L]
    kacc = collections.defaultdict(lambda: dict(ids={}, ez=set(), bld=[]))
    kparents = set()
    st = collections.Counter()
    cells_outside = 0
    out = bytearray()
    cells_n = 0
    t0 = time.time()
    for bi, (s0, s1) in enumerate(zip(starts, ends)):
        ids = [int(x) for x in cand[s0:s1]]
        keep, polys = [], []
        for cid in ids:
            c = h3.int_to_str(cid)
            cp = cell_polygon(c)
            if not own_prepared.intersects(cp):
                continue
            b = cp.bounds
            if not (b[0] >= W and b[1] >= S and b[2] <= E and b[3] <= N):
                cells_outside += 1
                continue
            keep.append(c)
            polys.append(cp)
        if not keep:
            continue
        keepset = set(keep)
        cellpoly = dict(zip(keep, polys))
        tree = STRtree(polys)
        bx = shapely.box(min(p.bounds[0] for p in polys), min(p.bounds[1] for p in polys),
                         max(p.bounds[2] for p in polys), max(p.bounds[3] for p in polys))
        acc = {c: dict(land=[], own=[], g=[], ids=set(), line=0.0, lu=collections.defaultdict(list), fp=[],
                       bld_n=0, bld_or=[]) for c in keep}

        def hits(g):
            return sorted(int(i) for i in tree.query(g))

        for pi in sorted(int(i) for i in ptree.query(bx)):
            p = P[pi]
            g = p["geometry"]
            for i in hits(g):
                c = keep[i]
                cp = cellpoly[c]
                x = g.intersection(cp)
                if x.is_empty or _area(x) <= 0:
                    continue
                d = acc[c]
                d["land"].append(x)
                d["ids"].add(p["id"])
                if p["kg"] == kg:
                    d["own"].append(x)
                if p["status"] == "G":
                    d["g"].append(x)
                b = g.boundary.intersection(cp)
                if not b.is_empty:
                    d["line"] += affinity.affine_transform(b, cell_affine(c)).length / 2.0
                kp = h3.cell_to_parent(c, K_RES)
                kd = kacc[kp]
                kd["ids"][p["id"]] = int(p["area_sqm"] + 0.5)
                if p["ez"]:
                    kd["ez"].add(p["ez"])
        for fi in sorted(int(i) for i in ftree.query(bx)):
            g = F[fi]["geometry"]
            for i in hits(g):
                c = keep[i]
                x = g.intersection(cellpoly[c])
                if not x.is_empty and _area(x) > 0:
                    acc[c]["fp"].append(x)
            ctr = g.centroid
            c0 = h3.latlng_to_cell(ctr.y, ctr.x, H3_RES)
            if c0 in keepset:
                gm = affinity.affine_transform(g, cell_affine(c0))
                acc[c0]["bld_n"] += 1
                acc[c0]["bld_or"].append(footprint_axis_deg(gm))
                kacc[h3.cell_to_parent(c0, K_RES)]["bld"].append(int(gm.area + 0.5))
        for li in sorted(int(i) for i in ltree.query(bx)):
            grp = lgrp[li]
            g = L[li]["geometry"]
            for i in hits(g):
                c = keep[i]
                x = g.intersection(cellpoly[c])
                if not x.is_empty and _area(x) > 0:
                    acc[c]["lu"][grp].append(x)
        for c in keep:                       # keep is ascending (cand sorted, filter preserves order)
            r = _row(c, acc[c], cellpoly[c], st)
            if r is None:
                continue
            out += LU_REC.pack(int(c, 16), r["kg_share"], r["land"], r["cov"], r["flags"], bytes(r["lu"]),
                               r["bld_cover"], r["bld_n"], r["gk"], r["n_parc"], r["line_m_ha"],
                               r["orient_coh"], r["orient_axis"])
            cells_n += 1
            kparents.add(h3.cell_to_parent(c, K_RES))
        del acc, tree, cellpoly, polys, keep, keepset
        if log and (bi % 20 == 19 or bi + 1 == len(starts)):
            log(f"{kg}: block {bi + 1}/{len(starts)}: {cells_n} rows so far ({time.time() - t0:.0f}s)")
    # ── res-10 k-layer — verbatim ──────────────────────────────────────────────────────
    kcells_n = 0
    for kp in sorted(kparents):
        kd = kacc.get(kp)
        if not kd or len(kd["ids"]) < K:
            continue
        if coverage_bbox is not None:
            W, S, E, N = coverage_bbox
            x0, y0, x1, y1 = cell_polygon(kp).bounds
            if x0 < W or y0 < S or x1 > E or y1 > N:
                st["kcells_withheld_coverage"] += 1
                continue
        areas = sorted(kd["ids"].values())
        n = len(areas)
        ezn = len(kd["ez"])
        blds = sorted(kd["bld"])
        out += K_REC.pack(int(kp, 16), min(NONE16 - 1, n), _pct(areas, 0.1), _lower_median(areas), _pct(areas, 0.9),
                          min(NONE16 - 1, ezn), min(254, (n * 10) // ezn) if ezn else NONE8,
                          min(NONE16 - 1, len(blds)), min(NONE16 - 1, _lower_median(blds)) if len(blds) >= K else NONE16)
        kcells_n += 1
    st.update(dict(parcels=len(P), own_parcels=len(own), footprints=len(F), landuse=len(L),
                   parcels_outside_domain=n_in[0] - len(P), footprints_outside_domain=n_in[1] - len(F),
                   landuse_outside_domain=n_in[2] - len(L), cells_outside_domain=cells_outside,
                   cells=cells_n, kcells=kcells_n))
    return bytes(out), cells_n, kcells_n, dict(st), input_bbox


def finish(kg, epoch, source, P, F, L, prov, output, bbox, input_bbox, t0, log=None):
    """≡ ne_cells.__main__._finish (same header, same file, same summary) with the blocked build."""
    raw, cells_n, kcells_n, stats, ibox = build_packed(kg, P, F, L, coverage_bbox=bbox, input_bbox=input_bbox, log=log)
    hdr = lu_header(str(kg).zfill(5), epoch, source, ALGO, H3_RES, K_RES, K, cells_n, kcells_n, stats,
                    input_bbox=ibox, coverage_bbox=bbox or ibox, input_pad_deg=INPUT_PAD_DEG,
                    input_rule=INPUT_RULE, inputs=prov)
    blob = pack_section(MAGIC_LU, hdr, raw)
    tmp = output + ".tmp"
    with open(tmp, "wb") as f:
        f.write(blob)
    os.replace(tmp, output)
    sec = unpack_sections(blob)[0]
    return dict(kg=str(kg).zfill(5), algo=ALGO, source=source, epoch=epoch, input_bbox=list(ibox or ()),
                cells=cells_n, kcells=kcells_n, raw_bytes=len(raw),
                file_bytes=len(blob), digest=sec.header["digest"], seconds=round(time.time() - t0, 1),
                stats=stats)


def check(kg, P, F, L, input_bbox):
    """QA: reference build vs blocked build on the same canonical records → (equal, ref_digest, new_digest)."""
    from ne_cells.pack import digest_of, pack_records
    res = algo.build_cells(kg, P, F, L, input_bbox=input_bbox)
    ref = pack_records(res.cells, res.kcells)
    raw, cn, kn, stats, _ = build_packed(kg, P, F, L, input_bbox=input_bbox)
    same = ref == raw and (cn, kn) == (len(res.cells), len(res.kcells)) and dict(res.stats) == stats
    if dict(res.stats) != stats:
        print("stats differ:", res.stats, stats)
    return same, digest_of(ref), digest_of(raw)


def _main():
    """QA: ne_lowmem.py KG… — fetch the KG's cells like ne_report (stash first), build both ways, compare bytes."""
    import sys
    sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
    import ne_report as nr
    from ne_cells import canon
    from vtcseamless.bevdirect import BevDirect
    bev = BevDirect(os.environ.get("BEV_API", "http://127.0.0.1:8787"), cache_dir=None, deadline_s=900, log=nr.log)
    ok = True
    for kg in sys.argv[1:]:
        kg = kg.zfill(5)
        head = nr.umfeld_head(os.environ.get("UMFELD_API", "https://umfeld-at.exe.xyz"), kg)
        ibox = (((head or {}).get("lu") or {}).get("header") or {}).get("input_bbox")
        cells = nr.grid_cells(ibox)
        ubox = nr.cells_union(cells)
        _, docs = nr.fetch_cells(bev, cells, inflight=2, siedler=os.environ.get("SIEDLER_API", "http://localhost:8000"))
        P, F, L = [], [], []
        for key in cells:
            p, f, l = canon._one_bevdirect(docs[key])
            P += p; F += f; L += l
        del docs
        t0 = time.time()
        same, dref, dnew = check(kg, P, F, L, tuple(float(f"{v:.2f}") for v in ubox))
        nr.log(f"{kg}: {len(cells)} cells, reference {dref} vs lowmem {dnew}: {'IDENTICAL' if same else 'DIFFERENT'} ({time.time() - t0:.0f}s)")
        ok &= same
    sys.exit(0 if ok else 1)


if __name__ == "__main__":
    _main()
