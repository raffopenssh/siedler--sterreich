#!/usr/bin/env bash
# Cross-browser QA walk. Examples:
#   tools/xb.sh                              # full matrix (3 engines × desktop/mobile), ~10 min
#   tools/xb.sh --engines=webkit --form=mobile --only=trees,nature
#   tools/xb.sh --quick                      # webkit+chromium desktop, core scenes
# Report: https://siedler-oesterreich.exe.xyz:8765/report.html
set -euo pipefail
cd "$(dirname "$0")/xbrowser"
[ -d node_modules ] || npm i --silent
[ -d ~/.cache/ms-playwright ] || npx playwright install chromium firefox webkit
args=("$@")
if [[ " $* " == *" --quick "* ]]; then
  args=(--engines=webkit,chromium --form=desktop --only=game-z16,popup,trees,nature,treasure,flow,dossier)
fi
exec node xb.mjs "${args[@]}"
