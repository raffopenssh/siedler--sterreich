# Ops, QA tooling, housekeeping

## Services on this VM (2 vCPU, 8 GB)

| unit | what | notes |
|---|---|---|
| `srv` | `./siedler` on :8000 | `sudo systemctl restart srv`; `journalctl -u srv -f`. Maintenance: `touch MAINTENANCE` (bypass cookie `siedler_dev=1` / `?dev=1`). |
| `bevdirect-serve` | cadastre assembler :8787 (/opt/bevdirect) | `User=exedev`, `-cells 120 -prefetch 0`, `MemoryMax=3G`, `GOMEMLIMIT=2560MiB`. Upgrade recipe: providers.md. |
| `ne-report.timer/service` | nightly NE report 01:00 Vienna | contrib.md. Units in `tools/ne-report/`, installed by `INSTALL_UNITS=1 tools/ne-report/setup.sh`. |
| `xb-report` | serves `tools/xbrowser/out/report.html` on :8765 | |

Live: `https://siedler-oesterreich.exe.xyz:8000/`. DB `./db.sqlite3`.

## Health & metrics

`GET /api/metrics` (route p50/p95, cache hit ratio, bevdirect health + `version_mismatch`, `tiles{}`,
`upstream_down_503`, `kg_universe.alert`), `GET /api/warm/status` (plan, queue, tier, boost, `contrib{stash…}`,
`v24_kgs`), `GET /api/upstreams` (breakers), `GET /api/contrib/plan`, bevdirect `/health`. Browser:
`DEV.cells()/warm()/lucky()/timing()/upstreams()`. Load: `tools/soak.sh` (`USERS=20 SECONDS_=30`; the box
saturates around 50 concurrent users).

Owner alerts: `sendOwnerMail` (`SIEDLER_ALERT_MAIL`) — one mail per fingerprint (KG universe drift).

## Disk hygiene

- Cadastre rule: providers.md § Cadastre-data hygiene (audit commands there).
- `db.sqlite3` has `auto_vacuum=INCREMENTAL`; the hourly janitor prunes expired api_cache rows and runs
  `PRAGMA incremental_vacuum`, so the file tracks the live cache (~1.2 GB = one day of warmed cells + NE docs).
- journald capped at 100 M (`/etc/systemd/journald.conf.d/size.conf`).
- Big regenerable users: `~/.cache/go-build` (`go clean -cache`), `~/.cache/ms-playwright`, `/tmp`,
  `tools/xbrowser/out`.

## Cross-browser QA harness (`tools/xbrowser/`, `tools/xb.sh`)

Walks every app state (welcome, invite, picker, loading, all zooms, every popup, EZ, Chronik, Herald, treasures,
giant trees, Naturschutz/Naturwald/Schlag/field mocks, station, flow, similar, search, chat, rules, mobile sheet,
chrome off, GPS, NE, deal, smash, attention-budget scenes) in Chromium, Firefox and WebKit, desktop 1440×900 +
phone 390×844 @2x. Flags JS errors, missing fonts, clipped text, off-screen chrome, dead canvas animations,
pixel Δ vs Chromium, and (with `--i18n`) language leaks. Scenes: `tools/xbrowser/scenes.mjs` — **add one per new
popup/overlay/feature**. Report `https://siedler-oesterreich.exe.xyz:8765/report.html`. `tools/xb.sh --quick`
(webkit+chromium core), `--engines= --form= --only=`, `--i18n [--lang=de]`. xbrowser turns relief off after
the game opens and only exercises it in the `relief` scene.

## i18n checks (before committing UI text)

`node tools/i18n/audit.js srv/static/game.js` (static) · `tools/xb.sh --i18n` (runtime, every scene) ·
`DEV.i18n()` in the game. Details and fixing recipe: `tools/i18n/README.md`. Bump `i18n.js?v=` when the
dictionary changes.

## Perf drills

`python3 tools/perf_wheel.py <cdp-port> <cpu-throttle> [lon lat zoom] [auto|low|high]`,
`tools/cpuprof.py <port> "<js>" [rate]`; see frontend.md § Performance tier.

## Screenshots

`docs/screenshot-recipes.md`; hi-res via browser `emulate_custom` DPR 2 or `emulate_device`. Glitch log
`docs/glitches.md` (append, mark ✅ when fixed).
