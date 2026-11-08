#!/usr/bin/env python3
"""Prune data/ne-reports: digest-only reports (KG.<date>.json + .meta.json) accumulate ~40 MB/night
(1.3 GB after a month on a 20 GB disk, 2026-11-08). Per KG we keep what ne_report.py / contrib.go read:

  - the newest KEEP report dates (latest_report, last_post, contribLastReport);
  - the newest *full* build (build.digest set, not build.skipped — last_full_build_age_d);
  - the newest POST that carried the chunk list (chunk_list_sent);
  - anything younger than MIN_AGE_D days;
  - KG.<date>.skip markers younger than SKIP_AGE_D (contribSkippedRecently uses 7 d).

Usage: prune.py [--dry-run] [--dir DIR]
"""
import argparse, glob, json, os, re, sys, time

KEEP, MIN_AGE_D, SKIP_AGE_D = 2, 7, 30
RX = re.compile(r"^(\d{5})\.(\d{4}-\d{2}-\d{2})\.(json|meta\.json|skip)$")


def meta(path):
    try:
        with open(path) as f:
            return json.load(f)
    except Exception:
        return {}


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--dir", default=os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "data", "ne-reports"))
    ap.add_argument("--dry-run", action="store_true")
    a = ap.parse_args()
    now = time.time()
    cut = time.strftime("%Y-%m-%d", time.gmtime(now - MIN_AGE_D * 86400))
    skipcut = time.strftime("%Y-%m-%d", time.gmtime(now - SKIP_AGE_D * 86400))
    by_kg = {}
    skips = []
    for name in os.listdir(a.dir):
        m = RX.match(name)
        if not m:
            continue
        kg, date, kind = m.groups()
        if kind == "skip":
            if date < skipcut:
                skips.append(name)
            continue
        by_kg.setdefault(kg, set()).add(date)
    doomed, freed = list(skips), sum(os.path.getsize(os.path.join(a.dir, n)) for n in skips)
    for kg, dates in by_kg.items():
        dates = sorted(dates, reverse=True)
        keep = set(dates[:KEEP]) | {d for d in dates if d >= cut}
        full = chunks = None
        for d in dates:
            m = meta(os.path.join(a.dir, f"{kg}.{d}.meta.json"))
            b = m.get("build") or {}
            if full is None and b.get("digest") and not b.get("skipped"):
                full = d
            post = m.get("post") or {}
            ans = post.get("answer") or {}
            if chunks is None and post.get("status") == 200 and not post.get("header_only") and isinstance(ans, dict) \
                    and any(k in ans for k in ("want_ap", "chunks_held", "chunks_reported")):
                chunks = d
            if full and chunks:
                break
        keep |= {d for d in (full, chunks) if d}
        for d in dates:
            if d in keep:
                continue
            for suf in (".json", ".meta.json"):
                p = os.path.join(a.dir, f"{kg}.{d}{suf}")
                if os.path.exists(p):
                    freed += os.path.getsize(p)
                    doomed.append(os.path.basename(p))
    for n in doomed:
        if not a.dry_run:
            os.remove(os.path.join(a.dir, n))
    print(f"ne-reports prune: {'would remove' if a.dry_run else 'removed'} {len(doomed)} files, {freed / 1e6:.0f} MB "
          f"({len(by_kg)} KGs)", file=sys.stderr)


if __name__ == "__main__":
    main()
