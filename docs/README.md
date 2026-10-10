# docs/ — read what the task needs, nothing more

| read when you touch… | file |
|---|---|
| any upstream call, bevdirect, umfeld, srtm, siblings, breaker/202, KG registry, **cadastre data hygiene** | [providers.md](providers.md) |
| `/api/viewport`, cells, cell store, loading, warming, lucky, search | [cadastre-cells.md](cadastre-cells.md) |
| NE observed layer (`ne{}`, trees/buildings from NE, heat overlay, Spurenleser) | [ne.md](ne.md) |
| the nightly contrib report, plan, stash, schedule | [contrib.md](contrib.md) · pipeline runbook [ne-report.md](ne-report.md) |
| game.js: state, rendering, perf, popups, Herald/attention budget, overlays, DEV helpers | [frontend.md](frontend.md) |
| DB, endpoints, SSE, pricing, harvest/regrowth, timber, water, treasures, quests, EZ | [mechanics.md](mechanics.md) |
| agent endpoints, inspect, similar, openapi, `/llm/ahead` | [agent-surface.md](agent-surface.md) |
| systemd units, metrics, disk, xbrowser, i18n checks, perf drills | [ops.md](ops.md) · [../tools/i18n/README.md](../tools/i18n/README.md) |
| why a rule exists / what broke before | [lessons.md](lessons.md) · open cosmetic issues [glitches.md](glitches.md) |
| past providers (the only place) | [migration-2026-10.md](migration-2026-10.md) |
| licence basis for BEV tiles | [licences/README.md](licences/README.md) |
| screenshots for decks | [screenshot-recipes.md](screenshot-recipes.md) |

Convention: docs describe the **current** state. History goes to `lessons.md` (rule + why) or
`migration-2026-10.md` (providers). When you change behaviour, update the one topic file that owns it and, only if
a hard rule or the mental model changed, `AGENTS.md`.
