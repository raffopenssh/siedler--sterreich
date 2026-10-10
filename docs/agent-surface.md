# Agent surface (LLM players)

- `GET /` serves `/llm/game` markdown to agent UAs (`agentUASubstrings`) and `Accept: text/markdown`;
  `Link: rel=alternate|help|service-desc` headers; `/llms.txt`, `/openapi.json` (hand-written 3.1 — **keep in
  sync** when agent endpoints change; `discover_test.go` checks it stays valid JSON), `/agents` +
  `/api/agents/leaderboard` (protected ha per `players.agent`, 60 s cache).
- `GET /api/agent/look`, `/api/agent/municipality`, `POST /api/agent/claim` (`agentSeen` gate).
  `fetchAgentParcels` reads cached cells (`parcelsNear`), applies the `landuse` filter locally via
  `dominantNS()`. Look rows carry NE fields (`ne_verdict, ne_canopy, ne_tree_n, ne_structures_n,
  bonus_spurenleser_xp`).
- `GET /api/agent/inspect?session_id&player_id&parcel_id[&lon&lat]` (`agent_inspect.go`): parallel fan-out under
  `inspectBudget` 4.5 s — parcel row from the cell (`lookupParcel`), folio via bevdirect `/ez`, umfeld
  `/context`, srtm `/landscape` + `/buildings/bbox`, farm Schläge/Hofstellen, gw point + stations + well quote,
  Chronik, cached timber, similar. Blocks past budget → `{pending:true}` in `pending[]`; vanished upstream
  blocks listed in `missing`. `game.actions[]`, `game.regrowth{}`, `terrain.observed`, `text` (`inspectNarrate`).
- Relays: cold cell → `202 {status:"pending", retry_after_s, …}` + `Retry-After` (`relayCellStatus`);
  breaker open → `503 {status:"down", service:"cadastre"}` + `X-Upstream: down`. Every answer carries the
  BEV `notice` (`agentAttribution`, also `/api/similar`).
- `/api/similar?parcel_id&lon&lat&area&bcount&barea&lu&limit` (`similar.go`): candidates from our cached cells
  (`parcelsNear`), scored 0..1 on size ratio, landuse composition, built density, terrain histogram;
  `source:"cells"`, cached 1 h (`similar:v6:`). Feeds inspect `similar` (top 8).
- `/llm/ahead` (`llmahead.go`, token `X-Ahead-Token` from `SIEDLER_AHEAD_TOKEN` / `./ahead.key`, else 404):
  roadmap for the sibling services + conformance harness `/llm/ahead/check/{service}`. Checkbox = consumed by
  us (`aheadUsed` map — **add an entry when you start consuming an item**); GONE items target endpoints that no
  longer exist. Never link it from public docs.
- Multi-player QA: register a 2nd player via `POST /api/register`, join with `invite_code`, drive with curl
  (`X-Player-Token`); `curl -N /api/session/{id}/events` shows SSE.
