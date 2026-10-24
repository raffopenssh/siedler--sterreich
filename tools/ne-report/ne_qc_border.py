#!/usr/bin/env python3
"""ne_qc_border.py — rebuild one KG exactly as ne_report.py does (RAM only) and compare it with
umfeld's live NEC1 container: cell set, chunk set, kg_share at the KG border, rows outside
umfeld's viewport. Answers "are our border rows contract-conform?" (ne-cells: a border cell is
identical across KGs except kg_share; chunks only where cells intersect the KG).

  tools/ne-report/.venv/bin/python tools/ne-report/ne_qc_border.py 80004

Raw bevdirect documents stay in RAM; nothing is written."""
import collections
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import h3                                     # noqa: E402
import urllib.request                         # noqa: E402
import ne_report as R                         # noqa: E402
from ne_cells.algo import build_cells         # noqa: E402
from ne_cells.canon import from_bevdirect     # noqa: E402
from ne_cells.pack import unpack_sections, MAGIC_LU   # noqa: E402
from vtcseamless.bevdirect import BevDirect   # noqa: E402

FIELDS = ("land", "lu", "cov", "bld_n", "bld_cover", "line_m_ha", "flags")


def main(kg, umfeld=os.environ.get("UMFELD_API", "https://umfeld-at.exe.xyz"),
         bev=os.environ.get("BEV_API", "http://127.0.0.1:8787"), siedler=os.environ.get("SIEDLER_API", "http://localhost:8000")):
    kg = kg.zfill(5)
    head = R.umfeld_head(umfeld, kg)
    if not head:
        raise SystemExit(f"{kg}: not built at umfeld")
    vb = head["lu"]["header"]["input_bbox"]
    cells = R.grid_cells(vb)
    ibox = R.cells_union(cells)
    print(f"{kg}: umfeld viewport {vb} → {len(cells)} aligned cells, input_bbox {ibox}")
    _, docs = R.fetch_cells(BevDirect(bev, cache_dir=None, deadline_s=900, log=R.log), cells, 2, siedler)
    P, F, L = from_bevdirect([docs[c] for c in cells])
    own = [p for p in P if p["kg"] == kg]
    print(f"parcels {len(P)} own (bevdirect kg_code == {kg}) {len(own)}; "
          f"own parcels outside umfeld viewport: {sum(1 for p in own if not p['geometry'].intersects(__import__('shapely').box(*vb)))}")
    ours = build_cells(kg, P, F, L, input_bbox=ibox).cells
    buf = urllib.request.urlopen(f"{umfeld}/api/v1/ne/{kg}?layers=lu", timeout=180).read()
    sec = [s for s in unpack_sections(buf) if s.magic == MAGIC_LU][0]
    U, _ = sec.records()
    h = sec.header
    print(f"umfeld: source={h.get('source')} epoch={h.get('epoch')} cells_n={h.get('cells_n')}; ours cells {len(ours)}")
    both = set(ours) & set(U)
    print(f"cells: ours-only {len(set(ours) - set(U))}, umfeld-only {len(set(U) - set(ours))}")
    co = {h3.cell_to_parent(c, 10) for c in ours}
    cu = {h3.cell_to_parent(c, 10) for c in U}
    print(f"chunks: ours {len(co)} umfeld {len(cu)} ours-only {len(co - cu)} umfeld-only {len(cu - co)}")
    W, S, E, N = [float(v) for v in vb]
    out = [c for c in ours if not (S <= h3.cell_to_latlng(c)[0] <= N and W <= h3.cell_to_latlng(c)[1] <= E)]
    print(f"our rows centred outside umfeld's viewport: {len(out)}")
    ks = collections.Counter(r["kg_share"] for r in ours.values())
    print(f"kg_share histogram (ours): 255={ks[255]} 250..254={sum(ks[k] for k in range(250, 255))} <250={sum(v for k, v in ks.items() if k < 250)}")
    kd = [(c, U[c]["kg_share"], ours[c]["kg_share"]) for c in both if abs(U[c]["kg_share"] - ours[c]["kg_share"]) > 10]
    print(f"border rows with |kg_share diff| > 10/255 vs umfeld: {len(kd)} {sorted(kd)[:8]}")
    full = [(c, U[c]["kg_share"]) for c in both if ours[c]["kg_share"] == 255 and U[c]["kg_share"] < 200]
    print(f"rows ours kg_share=255 while umfeld < 200 (the peer note's symptom): {len(full)} {sorted(full)[:8]}")
    od = sum(1 for c in both if any(ours[c][f] != U[c][f] for f in FIELDS))
    print(f"rows differing in non-share fields (source/epoch drift, expected): {od}")


if __name__ == "__main__":
    main(*sys.argv[1:])
