#!/usr/bin/env bash
# tools/soak.sh — load test Siedler Österreich with simulated players.
#
# Usage:
#   tools/soak.sh                       # 50 users, 60 s against http://localhost:8000
#   BASE=http://localhost:8000 USERS=20 SECONDS=30 tools/soak.sh
#   tools/soak.sh -v                    # extra flags are passed to soakload (see tools/soakload/main.go)
#
# Environment:
#   BASE      server base URL           (default http://localhost:8000)
#   USERS     concurrent virtual users  (default 50)
#   SECONDS_  run length in seconds     (default 60)  — also accepts DURATION
#   COOKIE    cookie sent with each request (default siedler_dev=1 = maintenance bypass)
#
# What it does: picks one cold Gemeinde (random not-warm KG from
# srv/data/admin.json.gz, verified with /api/kg-geo, ≤ 9 distinct cells), one
# warm Gemeinde (/api/lucky) and one more lucky pick; each virtual user
# registers, creates a session and loops over grid-aligned /api/viewport cells
# plus trees/buildings/osm-lines/n2k/schlaege, session treasures/parcels, and
# an occasional claim-parcel + /api/similar. Prints a per-role/per-endpoint
# table and asserts: viewport p95 < 1.5 s on HIT/WARM cells, zero 5xx
# (breaker 503s reported separately, fail only > 1 %), bevdirect-serve memory
# < MemoryMax, api_cache ≤ 1.5 GB and growth ≤ 400 MB. Exit 1 on failure.
#
# No external dependency: builds tools/soakload (Go) into ./tools/soakload/soakload.
set -euo pipefail
cd "$(dirname "$0")/.."
BASE="${BASE:-http://localhost:8000}"
USERS="${USERS:-50}"
SECS="${SECONDS_:-${DURATION:-60}}"
COOKIE="${COOKIE:-siedler_dev=1}"
BIN=tools/soakload/soakload
echo "building $BIN ..."
go build -o "$BIN" ./tools/soakload
exec "$BIN" -base "$BASE" -users "$USERS" -seconds "$SECS" -cookie "$COOKIE" -admin srv/data/admin.json.gz "$@"
