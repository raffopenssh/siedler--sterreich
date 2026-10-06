#!/usr/bin/env bash
# run.sh — NE epoch reports for a list of KGs (driver used by ne-report.timer).
#
#   tools/ne-report/run.sh                 # KGs = today's rotation from localhost:8000/api/contrib/plan (quarterly sweep of all v2.4 KGs)
#   tools/ne-report/run.sh 05007 63330     # explicit KGs
#   FORCE=1 tools/ne-report/run.sh 05007   # ignore the 7-day skip
#
# Each KG: head → fetch bevdirect cells (≤ 2 in flight) → ne_cells build → report → data/ne-reports/KG.<date>.json
# → POST to umfeld when a peer token exists (ne-peer.key or $NE_PEER_TOKEN), otherwise a logged no-op.
# KGs with a report younger than 7 days are skipped (ne_report.py --max-age-days).
set -uo pipefail
cd "$(dirname "$0")"
HERE=$(pwd)
PY="$HERE/.venv/bin/python"
if [ ! -x "$PY" ]; then
  echo "ne-report: venv missing — run tools/ne-report/setup.sh" >&2
  exit 2
fi

# Fallback (only when the game API is unreachable): a tiny v2.4 sample, never the whole universe.
FALLBACK_KGS="05007 06030 63330"

if [ $# -gt 0 ]; then
  KGS="$*"
else
  # Today's contrib rotation (srv/contrib.go): every v2.4 KG is assigned one day of the
  # quarter by hash, so the whole universe (~1 400 KGs) is reported once a quarter at
  # ~16 KGs a night; the list includes the two previous nights for catch-up (ne_report.py
  # skips reports < 7 d old). Never use v24_kgs from /api/warm/status — that is all of them.
  # The plan is empty (source:"none") for a few seconds while the srtm KG registry does a
  # full refresh (hourly check) — retry before falling back to the sample.
  for try in 1 2 3 4; do
    PLAN=$(curl -s --max-time 10 "${SIEDLER_API:-http://localhost:8000}/api/contrib/plan")
    case "$PLAN" in *'"source":"registry"'*) break;; esac
    echo "ne-report: contrib plan not ready (try $try) — waiting 30 s" >&2
    sleep 30
  done
  KGS=$(PLAN="$PLAN" python3 -c '
import json, os, sys
try:
    d = json.loads(os.environ.get("PLAN") or "")
except Exception as e:
    print("plan: not json: %s" % e, file=sys.stderr); sys.exit(0)
v = d.get("kgs")
if isinstance(v, list) and v:
    print(" ".join(str(k).zfill(5) for k in v))
    print("ne-report: contrib plan %s day %s/%s: %s today, %s cheap (warm < 24 h), %s incl. catch-up, universe %s, reported this quarter %s"
          % (d.get("quarter"), d.get("day"), d.get("days"), len(d.get("today") or []), len(d.get("cheap") or []),
             len(v), d.get("universe"), d.get("reported_quarter")), file=sys.stderr)
' 2>/tmp/ne-report-plan.err)
  [ -s /tmp/ne-report-plan.err ] && cat /tmp/ne-report-plan.err >&2
  if [ -n "$KGS" ]; then
    echo "ne-report: KGs from /api/contrib/plan: $KGS" >&2
  else
    KGS="$FALLBACK_KGS"
    echo "ne-report: /api/contrib/plan unavailable — using the small fallback sample" >&2
  fi
fi

# Token: ne-peer.key in the repo root wins, else the environment. ne_report.py logs a clear
# "POST skipped — no peer token" line when neither exists.
EXTRA=()
[ "${FORCE:-0}" = "1" ] && EXTRA+=(--force)
# shellcheck disable=SC2086
exec "$PY" "$HERE/ne_report.py" "${EXTRA[@]}" $KGS
