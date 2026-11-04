#!/usr/bin/env python3
"""Pull umfeld's per-KG report history (metadata only: observer, received_at, source, cells_n, digest —
no chunk maps, no cadastre content) into data/contrib-history.json for coverage audits.
Usage: tools/contrib-audit/pull_history.py [--workers 4]   (token: ne-peer.key)"""
import json, os, sys, time, gzip, concurrent.futures as cf, urllib.request
ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
U = os.environ.get("UMFELD", "https://umfeld-at.exe.xyz")
T = open(os.path.join(ROOT, "ne-peer.key")).read().strip()
OUT = os.path.join(ROOT, "data", "contrib-history.json")
kgs = [k["kg_code"] for k in json.load(gzip.open(os.path.join(ROOT, "srv/data/admin.json.gz")))["kgs"]]
hist = json.load(open(OUT)) if os.path.exists(OUT) else {}
if "--refresh" not in sys.argv:
    kgs = [k for k in kgs if k not in hist]

def one(kg):
    for t in range(4):
        try:
            r = urllib.request.Request(f"{U}/contrib/api/v1/ne/{kg}/report", headers={"Authorization": "Bearer " + T})
            with urllib.request.urlopen(r, timeout=120) as f:
                d = json.load(f)
            return kg, [{"at": x["received_at"], "observer": x["report"].get("observer"), "token": x.get("token"),
                         "source": x["report"].get("source"), "cells_n": x["report"].get("cells_n"),
                         "chunks": len(x["report"].get("chunks") or {}), "digest": x["report"].get("digest"),
                         "bbox": x["report"].get("bbox")} for x in d.get("reports", [])]
        except urllib.error.HTTPError as e:
            if e.code == 404:
                return kg, []
            time.sleep(2 + 3 * t)
        except Exception:
            time.sleep(2 + 3 * t)
    return kg, None

n = 0; t0 = time.time()
with cf.ThreadPoolExecutor(int(sys.argv[sys.argv.index("--workers") + 1]) if "--workers" in sys.argv else 4) as ex:
    for kg, rep in ex.map(one, kgs):
        n += 1
        if rep is not None:
            hist[kg] = rep
        if n % 200 == 0:
            json.dump(hist, open(OUT + ".tmp", "w")); os.replace(OUT + ".tmp", OUT)
            print(f"{n}/{len(kgs)} {time.time()-t0:.0f}s", file=sys.stderr, flush=True)
json.dump(hist, open(OUT + ".tmp", "w")); os.replace(OUT + ".tmp", OUT)
print(f"done {len(hist)} KGs", file=sys.stderr)
