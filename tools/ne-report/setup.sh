#!/usr/bin/env bash
# setup.sh — create the venv for the NE epoch-report pipeline and install the frozen
# `ne_cells` reference package (vendored in ./ne_cells, pinned shapely==2.1.2 h3==4.5.0;
# bit-identity of the digests depends on exactly these wheels — do not upgrade).
#
#   tools/ne-report/setup.sh            # idempotent; re-run after `git pull`
#
# Optional: INSTALL_UNITS=1 also installs + enables the systemd service/timer (needs sudo).
set -euo pipefail
cd "$(dirname "$0")"
HERE=$(pwd)

if [ ! -x .venv/bin/python ]; then
  python3 -m venv .venv
fi
.venv/bin/pip install --quiet --upgrade pip
# vtcseamless-py (MIT) — the bevdirect-serve client on the 0.02° grid (`cells_for`, `BevDirect.cell`);
# pinned to a tag. It ships its own copy of ne_cells (algo/pack byte-identical to ours), which the
# vendored package below overwrites so the frozen reference in ./ne_cells stays authoritative.
.venv/bin/pip install --quiet --no-deps "git+https://github.com/raffopenssh/vtcseamless-py@${VTCSEAMLESS_PY_TAG:-v0.1.1}"
# `pip install .` honours the pins in pyproject.toml (shapely==2.1.2, h3==4.5.0).
.venv/bin/pip install --quiet "$HERE"
.venv/bin/python - <<'EOF'
import shapely, h3, ne_cells, vtcseamless
from vtcseamless.bevdirect import cells_for, BevDirect
assert cells_for(13.001, 47.001, 13.019, 47.019) == [(650, 2350)]
assert shapely.__version__ == "2.1.2", shapely.__version__
assert h3.__version__ == "4.5.0", h3.__version__
print(f"ok: ne_cells {ne_cells.ALGO}, vtcseamless-py {getattr(vtcseamless, '__version__', '?')}, shapely {shapely.__version__} (GEOS {shapely.geos_version_string}), h3 {h3.__version__}")
EOF

if [ "${INSTALL_UNITS:-0}" = "1" ]; then
  sudo install -m 0644 "$HERE/ne-report.service" "$HERE/ne-report.timer" /etc/systemd/system/
  sudo systemctl daemon-reload
  sudo systemctl enable --now ne-report.timer
  systemctl list-timers ne-report.timer --no-pager
fi
