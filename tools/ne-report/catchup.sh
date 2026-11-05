#!/bin/bash
# catchup.sh — daytime catch-up round: report every KG umfeld has never seen (never_seen_kgs of
# /api/contrib/plan, largest first). Exits 0 with nothing to do once the list is empty; the timer
# (ne-report-catchup.timer, 08:00 Vienna) can then be disabled. Stops at 18:30 Vienna (run.sh deadline)
# so it never overlaps the 20:00 prewarm / 23:00 nightly run. Skips the day if another reporter runs.
set -u
HERE=$(cd "$(dirname "$0")" && pwd)
for u in ne-report ne-report-extra ne-report-80110; do
  systemctl is-active -q "$u" && { echo "catchup: $u is running — skipping today"; exit 0; }
done
mapfile -t KGS < <(curl -sf --max-time 60 "${SIEDLER_API:-http://localhost:8000}/api/contrib/plan" \
  | python3 -c 'import json,sys; [print(k) for k in json.load(sys.stdin).get("never_seen_kgs") or []]')
if [ ${#KGS[@]} -eq 0 ]; then echo "catchup: umfeld has seen every KG — nothing to do (disable ne-report-catchup.timer)"; exit 0; fi
echo "catchup: ${#KGS[@]} never-seen KGs"
export DEADLINE_H=18 DEADLINE_M=30 DEADLINE_END=2200
exec "$HERE/run.sh" "${KGS[@]}"
