#!/usr/bin/env bash
# run.sh — NE epoch reports for a list of KGs (driver used by ne-report.timer).
#
#   tools/ne-report/run.sh                 # KGs = `v24_kgs` from localhost:8000/api/warm/status, else the v2.4 fallback list
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

# Default v2.4 KG list (used until /api/warm/status exposes `v24_kgs`).
FALLBACK_KGS="01205 01209 01512 01609 03030 03113 03134 03136 04304 05007 05023 06030 06101 06107 06205 09002 09008 09025 09045 09061 11039 12134"

if [ $# -gt 0 ]; then
  KGS="$*"
else
  KGS=$(curl -s --max-time 10 "${SIEDLER_API:-http://localhost:8000}/api/warm/status" \
    | python3 -c 'import json,sys
try:
    d = json.load(sys.stdin)
except Exception:
    d = {}
v = d.get("v24_kgs")
print(" ".join(str(k).zfill(5) for k in v) if isinstance(v, list) and v else "")' 2>/dev/null)
  if [ -n "$KGS" ]; then
    echo "ne-report: KGs from /api/warm/status v24_kgs: $KGS" >&2
  else
    KGS="$FALLBACK_KGS"
    echo "ne-report: /api/warm/status has no v24_kgs — using the built-in v2.4 list" >&2
  fi
fi

# Token: ne-peer.key in the repo root wins, else the environment. ne_report.py logs a clear
# "POST skipped — no peer token" line when neither exists.
EXTRA=()
[ "${FORCE:-0}" = "1" ] && EXTRA+=(--force)
# shellcheck disable=SC2086
exec "$PY" "$HERE/ne_report.py" "${EXTRA[@]}" $KGS
