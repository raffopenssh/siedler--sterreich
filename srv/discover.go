package srv

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"
)

// Agent discovery: how a non-browser client finds /llm/game without a human
// pointing at it.
//
//   - GET /          with Accept: text/markdown|text/plain (preferred over
//     text/html) or a known agent/CLI User-Agent → the /llm/game markdown
//     itself (200, Vary: Accept, User-Agent). Browsers still get index.html.
//   - Every HTML/markdown response carries `Link: </llm/game>; rel="alternate"`.
//   - GET /llm/game  from a browser (Accept text/html, no text/markdown) →
//     an HTML wrapper with Open Graph tags around the same markdown, so the
//     link unfurls on Discord/Slack/X. `?format=md` forces markdown.
//   - GET /openapi.json describes the agent surface for tool generators.
//   - Unmatched /api/* → JSON 404 pointing at the docs.

var agentUASubstrings = []string{
	// LLM user-driven fetchers
	"Claude-User", "ClaudeBot", "anthropic-ai", "ChatGPT-User", "GPTBot", "OAI-SearchBot",
	"Perplexity", "cohere-ai", "Google-Extended", "Applebot-Extended", "Meta-ExternalAgent",
	"Bytespider", "Amazonbot", "YouBot", "Diffbot", "mistral", "DuckAssistBot",
	// CLI / library clients — a human at a terminal wants the text edition too
	"curl/", "Wget/", "HTTPie", "python-requests", "python-httpx", "aiohttp", "Go-http-client",
	"node-fetch", "undici", "axios", "okhttp", "Java/", "libwww",
}

func prefersMarkdown(r *http.Request) bool {
	acc := strings.ToLower(r.Header.Get("Accept"))
	if acc == "" || acc == "*/*" {
		return true // no opinion → text (browsers always send text/html)
	}
	// crude q-less ordering: whichever appears first wins
	iHTML := strings.Index(acc, "text/html")
	iMD := strings.Index(acc, "text/markdown")
	iTxt := strings.Index(acc, "text/plain")
	if iMD >= 0 && (iHTML < 0 || iMD < iHTML) {
		return true
	}
	if iTxt >= 0 && iHTML < 0 {
		return true
	}
	return false
}

func isAgentUA(r *http.Request) bool {
	ua := r.UserAgent()
	for _, s := range agentUASubstrings {
		if strings.Contains(ua, s) {
			return true
		}
	}
	return false
}

// wantsAgentEdition reports whether GET / should answer with the text edition.
func wantsAgentEdition(r *http.Request) bool {
	if r.URL.Query().Get("format") == "html" {
		return false
	}
	acc := strings.ToLower(r.Header.Get("Accept"))
	if strings.Contains(acc, "text/html") && !strings.Contains(acc, "text/markdown") {
		// A real browser (or an HTML-converting fetch tool) – keep the game.
		// Agent UAs override: their HTML would be thrown away anyway.
		return isAgentUA(r)
	}
	return isAgentUA(r) || prefersMarkdown(r)
}

func setDiscoveryHeaders(w http.ResponseWriter) {
	w.Header().Add("Link", `<`+siteURL+`/llm/game>; rel="alternate"; type="text/markdown"; title="Agent edition"`)
	w.Header().Add("Link", `<`+siteURL+`/llms.txt>; rel="help"; type="text/plain"`)
	w.Header().Add("Link", `<`+siteURL+`/openapi.json>; rel="service-desc"; type="application/openapi+json"`)
	w.Header().Add("Vary", "Accept")
	w.Header().Add("Vary", "User-Agent")
}

func (s *Server) llmGameStats() (players, agents, sessions int64) {
	s.DB.QueryRow("SELECT COUNT(*) FROM players WHERE agent != ''").Scan(&agents)
	s.DB.QueryRow("SELECT COUNT(*) FROM players").Scan(&players)
	s.DB.QueryRow("SELECT COUNT(*) FROM game_sessions").Scan(&sessions)
	return
}

func (s *Server) llmGameMarkdown() string {
	p, a, se := s.llmGameStats()
	return fmt.Sprintf(llmGameMD, siteURL, p, a, se, len(quickPhrases))
}

func (s *Server) writeLLMGameMarkdown(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=600")
	fmt.Fprint(w, s.llmGameMarkdown())
}

// handleLLMGame serves markdown to agents and an OG-tagged HTML wrapper to browsers.
func (s *Server) handleLLMGame(w http.ResponseWriter, r *http.Request) {
	setDiscoveryHeaders(w)
	q := r.URL.Query().Get("format")
	acc := strings.ToLower(r.Header.Get("Accept"))
	browser := q != "md" && (q == "html" || (strings.Contains(acc, "text/html") && !strings.Contains(acc, "text/markdown") && !isAgentUA(r)))
	if !browser {
		s.writeLLMGameMarkdown(w)
		return
	}
	p, a, se := s.llmGameStats()
	desc := fmt.Sprintf("Text-only edition of Siedler Österreich for LLM agents: register → session → look → act over real Austrian cadastre, LiDAR forests and groundwater data. %d players (%d agents), %d sessions so far.", p, a, se)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=600")
	fmt.Fprintf(w, `<!doctype html>
<html lang="en"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Siedler Österreich — how to play as an agent</title>
<meta name="description" content="%[1]s">
<meta name="robots" content="index, follow">
<link rel="canonical" href="%[2]s/llm/game">
<link rel="alternate" type="text/markdown" href="%[2]s/llm/game?format=md" title="Markdown">
<link rel="alternate" type="application/openapi+json" href="%[2]s/openapi.json" title="OpenAPI">
<meta property="og:type" content="article"><meta property="og:site_name" content="Siedler Österreich">
<meta property="og:title" content="🤖 Siedler Österreich — play it as an LLM agent">
<meta property="og:description" content="%[1]s">
<meta property="og:url" content="%[2]s/llm/game">
<meta property="og:image" content="%[2]s/static/og-image.png">
<meta property="og:image:width" content="1200"><meta property="og:image:height" content="630">
<meta name="twitter:card" content="summary_large_image">
<meta name="twitter:title" content="🤖 Siedler Österreich — play it as an LLM agent">
<meta name="twitter:description" content="%[1]s">
<meta name="twitter:image" content="%[2]s/static/og-image.png">
<style>
body{margin:0;background:#1a1408;color:#e8dbb5;font:16px/1.45 ui-monospace,Menlo,Consolas,monospace}
header{padding:14px 20px;border-bottom:2px solid #6b5530;background:#2a2010;display:flex;gap:16px;align-items:center;flex-wrap:wrap}
header a{color:#f0c860;text-decoration:none}header a:hover{text-decoration:underline}
header .sp{flex:1}header code{background:#1a1408;padding:3px 8px;border:1px solid #6b5530;border-radius:4px;color:#e8dbb5;user-select:all}
pre{white-space:pre-wrap;word-wrap:break-word;max-width:900px;margin:0 auto;padding:24px 20px 60px}
footer{padding:14px 20px;border-top:1px solid #3a3018;color:#8a7e5a;font-size:13px;text-align:center}footer a{color:#8a7e5a}
</style></head><body>
<header><strong>🏰 Siedler Österreich</strong> · <span style="color:#8a7e5a">agent edition</span><span class="sp"></span>
<code>curl %[2]s/llm/game</code>
<a href="%[2]s/llm/game?format=md">raw .md</a> <a href="%[2]s/openapi.json">openapi.json</a> <a href="%[2]s/llms.txt">llms.txt</a> <a href="%[2]s/">▶ play in the browser</a></header>
<pre>%[3]s</pre>
<footer><a href="/impressum">Impressum &amp; data sources</a> · <a href="/datenschutz">Datenschutz</a> · <a href="https://github.com/raffopenssh/siedler--sterreich" rel="noopener">GitHub</a></footer>
</body></html>`, html.EscapeString(desc), siteURL, html.EscapeString(s.llmGameMarkdown()))
}

// handleAPINotFound is the /api/ catch-all: a JSON 404 that points to the docs.
func (s *Server) handleAPINotFound(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(404)
	json.NewEncoder(w).Encode(map[string]any{
		"error":   "no such endpoint: " + r.Method + " " + r.URL.Path,
		"docs":    siteURL + "/llm/game",
		"openapi": siteURL + "/openapi.json",
	})
}

// handleOpenAPI: a hand-written, deliberately small OpenAPI 3.1 document for
// the agent-facing surface. Tool generators (OpenAI Actions, LangChain
// OpenAPI toolkit, MCP openapi bridges) can consume it directly.
func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/openapi+json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Write([]byte(openAPIJSON))
}

var openAPIJSON = strings.ReplaceAll(`{
  "openapi": "3.1.0",
  "info": {
    "title": "Siedler Österreich — agent API",
    "version": "1.1.0",
    "summary": "Play a land game on real Austrian open data as an LLM agent: cadastral parcels assembled live from the BEV map tiles, LiDAR terrain + tree heights, INVEKOS crops, groundwater gauges, timber prices, real land values.",
    "description": "Text-only edition of the browser game. Flow: register once → pick a municipality → create/join a session → look → inspect (one-call data dossier of a parcel) → act (claim, convert, harvest, chat quick phrases). Full playbook with ground rules and rate limits: SITE/llm/game. Prices/ownership are game fiction; data © BEV CC BY 4.0 (cadastre assembled live from the Katastralmappe vector tiles, cached ≤ 24 h, show the notice field with it), © OSM ODbL, © AMA CC BY 4.0 (see /impressum, /lizenzen). Cadastre cells still being assembled answer 202 + retry_after_s — never 'no parcel'.",
    "termsOfService": "SITE/llm/game#0-ground-rules-we-made-promises-to-humans--you-inherit-them",
    "contact": {"url": "SITE/impressum"},
    "license": {"name": "Game data: see attribution field in every response; machine-readable catalogue at /api/licenses (incl. cadastre assembler vtcseamless, MIT)", "url": "SITE/licenses"}
  },
  "servers": [{"url": "SITE"}],
  "externalDocs": {"description": "How to play as an agent (markdown)", "url": "SITE/llm/game"},
  "components": {
    "securitySchemes": {
      "playerToken": {"type": "apiKey", "in": "header", "name": "X-Player-Token", "description": "rejoin_token returned once by POST /api/register"}
    },
    "schemas": {
      "Error": {"type": "object", "properties": {"error": {"type": "string"}, "docs": {"type": "string"}, "suggested": {"type": "string", "description": "409 on register: a free name to retry with"}}},
      "Player": {"type": "object", "properties": {"id": {"type": "string"}, "name": {"type": "string", "description": "prefixed with 🤖 for agents"}, "coins": {"type": "integer"}, "xp": {"type": "integer"}, "level": {"type": "integer"}}},
      "Session": {"type": "object", "properties": {"id": {"type": "string"}, "invite_code": {"type": "string"}, "municipality_code": {"type": "string"}, "municipality_name": {"type": "string"}, "center_lon": {"type": "number"}, "center_lat": {"type": "number"}}},
      "Parcel": {"type": "object", "properties": {"parcel_id": {"type": "string", "description": "KG-GNR, e.g. 61611-123/4 (Bauflächen have a leading dot: 90107-.10/22)"}, "kg_code": {"type": "string"}, "kg_name": {"type": "string"}, "gnr": {"type": "string"}, "ez": {"type": "string", "description": "land-register folio number; POST /api/claim-ez buys the folio's parcels you saw"}, "building_count": {"type": "integer"}, "building_area_sqm": {"type": "number"}, "lon": {"type": "number"}, "lat": {"type": "number"}, "landuse": {"type": "string", "description": "BEV NS code: 41 buildings, 48 fields/meadows, 52 gardens, 54 alpine pasture, 56 forest, 59/60 water"}, "landuse_name": {"type": "string"}, "area_sqm": {"type": "number"}, "price": {"type": "integer"}, "owner": {"type": ["string", "null"]}, "converted_to": {"type": ["string", "null"]}, "distance_m": {"type": "number"}, "map_url": {"type": "string"}, "ne_verdict": {"type": "string", "description": "observed layer (NE cells, KGs on srtm v2.4): consistent|forest_loss|forest_gain|sealed_new|structure_new|green_new|unknown; absent where not observed. Only sizeable changes are flagged: land-use codes need ≥ 35 % of the parcel and ≥ 3 cells (~900 m²), sealed_new/structure_new ≥ 15 % and ≥ 2 cells"}, "ne_canopy": {"type": "number"}, "ne_tree_n": {"type": "integer"}, "ne_structures_n": {"type": "integer"}, "bonus_spurenleser_xp": {"type": "integer", "description": "extra XP on claim when the observation contradicts the declared land use"}}},
      "Inspect": {"type": "object", "description": "Blocks that missed the time budget are {pending:true} and listed in pending[]; call again after retry_after_s. missing lists figures the data tiers provide only at KG/bbox resolution. notice is the BEV attribution to show with cadastre data.", "properties": {"text": {"type": "string", "description": "one-paragraph narration of all blocks"}, "notice": {"type": "string"}, "parcel": {"$ref": "#/components/schemas/Parcel"}, "cadastre": {"type": "object", "description": "kg_code, kg_name, gemeinde, gemeinde_code, district, state, gnr, ez, rstatus, dominant_ns, landuse_areas{ns:m²} (= landuse_summary), footprint_count, complete, parts, osm{dist_road_m,road_name,road_fclass,dist_major_road_m,major_road_ref,dist_rail_m,dist_transit_m,transit_name,dist_train_station_m,dist_water_m,water_name,dist_settlement_m,settlement_name,remoteness,…}, in_natura2000, natura2000_sites, in_protected_area, protected_area_sites, legal_refs{scope:\"kg\",total_refs,refs[],detail}, notice, source"}, "terrain": {"type": "object", "description": "resolution (parcel: N NE cells H3 res 12 | parcel 25 m heightfield | kg/bbox aggregate), observed{cells,cover{geb,bau,acker,gruen,wald,wasser,verkehr,alpen,sonst},canopy,h_max_m,tree_n,tree_h_max_m,trees_tall,species{},vitality{},structures_n,structures_cover,structure_types{},consistency{code:share},verdict,phenology,ndvi,dh_m,forest_loss_year,als_years,epoch} (srtm NE cells where the KG is v2.4 — verdict compares observation with the declared land use: consistent|forest_loss|forest_gain|sealed_new|structure_new|green_new|unknown — flagged only for sizeable changes: land-use ≥ 35 % & ≥ 3 cells, built ≥ 15 % & ≥ 2 cells; consistency{} keeps the raw shares), elevation_m (min/max), slope_deg, aspect, terrain_class, dominant_cover, cover_fractions{tree,grass,…}, forested_fraction, tree_height_m{mean,max}, canopy_max_m, tallest_trees[{height_m,crown_d_m,lon,lat}] (on the parcel), landmarks[], forest_loss{resolution:\"kg\",loss_by_year_px,loss_total_px,loss_since_2020_px,last_loss_year} (Hansen, KG level); available:false when the KG has no landscape product"}, "missing": {"type": "object", "additionalProperties": {"type": "string"}, "description": "block.field → why it is absent (e.g. per-parcel Hansen loss, per-parcel legal refs, folio addresses, no 25 m grid for this KG)"}, "forest": {"type": "object", "description": "NS 56 only: standing_stock_vfm, harvestable_efm, co2_stored_t, species_shares, timber_gross_eur, harvest_cost_eur, timber_net_eur, harvest_coins, naturwald_xp, prices{source,date}, history"}, "field": {"type": "object", "description": "schlag{crop,crop_group,area_ha,organic,year} from AMA INVEKOS (null if none), field_kind, nearest_farmstead{distance_m,size_class,organic}"}, "buildings": {"type": "object", "description": "count, footprint_area_sqm, footprints[{footprint_id,area_sqm,ns_code,length_m,width_m,orientation_deg,size_class,shape_class,lidar{max_height_m,mean_height_m,stories_est,roof_type,ground_m,match_m}}] — lidar = nearest measured building point ≤ 20 m"}, "water": {"type": "object", "description": "gwi, gwi_category, aquifer, depth_to_groundwater_m_est, depth_confidence, nitrate_mg_l, gw_trend_m_per_decade, status_now, stations_on_parcel[], pegelwart_bonus_xp, nearest_gw_station{name,distance_m,gw_level_m,trend_m_per_decade,detail}, water_protection{type,zone}|null, well{depth_m,price_coins,protection}"}, "market": {"type": "object", "description": "class, buy_eur_per_sqm, buy_total_eur, building_value_eur, combined_value_eur, trend_cagr_gemeinde, confidence, baseline_source, year (Statistik Austria model for this point + our area figures, not an appraisal)"}, "toponyms": {"type": "array", "items": {"type": "object", "properties": {"name": {"type": "string"}, "kind": {"type": "string"}, "distance_m": {"type": "number"}}}}, "ez": {"type": "object", "description": "folio parcels within the BEV tiles fetched around this parcel (partial:true, scope): parcel_count, total_area_sqm, unclaimed_count, unclaimed_area_sqm, bulk_price_coins (20 % off), parcel_ids, landuse_breakdown[{code,name,share,area_sqm}] — no addresses, no owners"}, "chronik": {"type": "object", "description": "drought{level,label,status,sigma}, yield_factor, well_protection, subsidy_coins_per_ha, detail=/api/dossier/{kg}"}, "similar": {"type": "object", "description": "scope:\"vicinity\" — up to 8 look-alike parcels from the cadastre cells we already hold around the point (radius_m 3000, cells, candidates): results[{parcel_id,score,parts{size,landuse,built,terrain},distance_m,area_sqm,lon,lat,kg_code,gnr,ez,building_count,landuse_summary,elev,slope,aspect,forest_frac,dom}], ref{…}, lidar_terms, source:\"cells\"; more via GET /api/similar?parcel_id=&lon=&lat=&area=&lu=&radius=&limit="}, "game": {"type": "object", "description": "price_coins, owner, converted_to, actions[{action,call,cost_coins|coins|xp,bonus,…}], session_biodiversity_pct"}, "pending": {"type": "array", "items": {"type": "string"}}, "retry_after_s": {"type": "integer"}, "attribution": {"type": "object"}}},
      "Look": {"type": "object", "description": "parcels come from the 0.02° cadastre cells around the point (nearest first, ≤ 9 cells). A cell still being assembled from the BEV tiles answers 202 — retry in a few seconds.", "properties": {"text": {"type": "string", "description": "Herald-style narration of the same content"}, "center": {"type": "object"}, "session": {"type": "object", "description": "id, municipality, invite_code, view_url, biodiversity_pct"}, "actions": {"type": "object", "description": "the calls available from here"}, "parcels": {"type": "array", "items": {"$ref": "#/components/schemas/Parcel"}}, "treasures": {"type": "array", "items": {"type": "object"}}, "players": {"type": "array", "items": {"$ref": "#/components/schemas/Player"}}, "quests": {"type": "array", "items": {"type": "object"}}, "drought": {"type": "object", "properties": {"level": {"type": "integer"}, "label": {"type": "string"}, "yield_factor": {"type": "number"}}}, "me": {"$ref": "#/components/schemas/Player"}, "view_url": {"type": "string"}, "attribution": {"type": "object", "description": "licences + disclaimer; attribution.notice is the BEV notice to show with the parcels"}}}
    },
    "responses": {
      "Err": {"description": "error", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Error"}}}},
      "TooMany": {"description": "rate limited; honour Retry-After", "headers": {"Retry-After": {"schema": {"type": "integer"}}}, "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Error"}}}},
      "Pending": {"description": "cadastre cell still being assembled from the BEV tiles — repeat the identical request after Retry-After / retry_after_s; never 'no parcel here'", "headers": {"Retry-After": {"schema": {"type": "integer"}}}, "content": {"application/json": {"schema": {"type": "object", "properties": {"status": {"type": "string", "enum": ["pending"]}, "retry_after_s": {"type": "number"}, "note": {"type": "string"}, "center": {"type": "object"}, "parcel_id": {"type": "string"}}}}}},
      "Down": {"description": "data service unavailable (circuit breaker) or busy; retry after Retry-After", "headers": {"Retry-After": {"schema": {"type": "integer"}}, "X-Upstream": {"schema": {"type": "string", "enum": ["down", "busy"]}}}, "content": {"application/json": {"schema": {"type": "object", "properties": {"error": {"type": "string"}, "status": {"type": "string", "enum": ["down", "busy"]}, "service": {"type": "string"}, "retry_after_s": {"type": "number"}}}}}}
    }
  },
  "paths": {
    "/api/register": {"post": {"operationId": "register", "summary": "Create a player (once). Keep rejoin_token — it is shown exactly once.", "requestBody": {"required": true, "content": {"application/json": {"schema": {"type": "object", "required": ["name", "agent"], "properties": {"name": {"type": "string", "description": "pseudonym, no real person's name"}, "agent": {"type": "string", "description": "short model label, e.g. claude-opus-4"}}}}}}, "responses": {"200": {"description": "ok", "content": {"application/json": {"schema": {"type": "object", "properties": {"player": {"$ref": "#/components/schemas/Player"}, "rejoin_token": {"type": "string"}}}}}}, "409": {"$ref": "#/components/responses/Err"}, "429": {"$ref": "#/components/responses/TooMany"}}}},
    "/api/suggest-name": {"get": {"operationId": "suggestName", "summary": "A guaranteed-free pseudonym", "responses": {"200": {"description": "ok", "content": {"application/json": {"schema": {"type": "object", "properties": {"name": {"type": "string"}}}}}}}}},
    "/api/agent/municipality": {"get": {"operationId": "findMunicipality", "summary": "Search a municipality by name (Statistik Austria register via umfeld), or ?random=1 for a Gemeinde whose cadastre cells are already warm", "parameters": [{"name": "q", "in": "query", "schema": {"type": "string"}}, {"name": "random", "in": "query", "schema": {"type": "integer", "enum": [1]}}], "responses": {"200": {"description": "ok", "content": {"application/json": {"schema": {"type": "object", "properties": {"municipalities": {"type": "array", "items": {"type": "object", "properties": {"municipality_code": {"type": "string"}, "municipality_name": {"type": "string"}, "center_lon": {"type": "number"}, "center_lat": {"type": "number"}, "state": {"type": "string"}, "district": {"type": "string"}}}}, "next": {"type": "string"}, "attribution": {"type": "object", "description": "licences + disclaimer + notice"}}}}}}}}},
    "/api/session/create": {"post": {"operationId": "createSession", "security": [{"playerToken": []}], "summary": "Open a session in a municipality", "requestBody": {"required": true, "content": {"application/json": {"schema": {"type": "object", "required": ["player_id", "name", "municipality_code", "municipality_name", "center_lon", "center_lat"], "properties": {"player_id": {"type": "string"}, "name": {"type": "string"}, "municipality_code": {"type": "string"}, "municipality_name": {"type": "string"}, "center_lon": {"type": "number"}, "center_lat": {"type": "number"}}}}}}, "responses": {"200": {"description": "ok", "content": {"application/json": {"schema": {"type": "object", "properties": {"session": {"$ref": "#/components/schemas/Session"}}}}}}, "401": {"$ref": "#/components/responses/Err"}, "429": {"$ref": "#/components/responses/TooMany"}}}},
    "/api/session/join": {"post": {"operationId": "joinSession", "security": [{"playerToken": []}], "summary": "Join a human's session by invite code", "requestBody": {"required": true, "content": {"application/json": {"schema": {"type": "object", "required": ["player_id", "invite_code"], "properties": {"player_id": {"type": "string"}, "invite_code": {"type": "string"}}}}}}, "responses": {"200": {"description": "ok"}, "404": {"$ref": "#/components/responses/Err"}}}},
    "/api/agent/look": {"get": {"operationId": "look", "security": [{"playerToken": []}], "summary": "See parcels (from the cadastre cell around the point), treasures, players, quests and drought state around a point", "parameters": [{"name": "session_id", "in": "query", "required": true, "schema": {"type": "string"}}, {"name": "player_id", "in": "query", "schema": {"type": "string"}}, {"name": "lon", "in": "query", "required": true, "schema": {"type": "number"}}, {"name": "lat", "in": "query", "required": true, "schema": {"type": "number"}}, {"name": "radius", "in": "query", "schema": {"type": "number", "default": 300, "description": "metres"}}, {"name": "limit", "in": "query", "schema": {"type": "integer", "default": 25}}, {"name": "landuse", "in": "query", "schema": {"type": "string", "description": "BEV NS code filter, e.g. 56"}}, {"name": "unclaimed", "in": "query", "schema": {"type": "integer", "enum": [1]}}], "responses": {"200": {"description": "ok", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Look"}}}}, "202": {"$ref": "#/components/responses/Pending"}, "404": {"$ref": "#/components/responses/Err"}, "429": {"$ref": "#/components/responses/TooMany"}, "503": {"$ref": "#/components/responses/Down"}}}},
    "/api/agent/inspect": {"get": {"operationId": "inspect", "security": [{"playerToken": []}], "summary": "Full data dossier of one parcel (~1–3 s): cadastre + folio (as far as fetched), OSM proximity, Natura 2000, LiDAR terrain/trees (KG-level forest loss), timber value, INVEKOS crop, building heights, groundwater/protection, market value, toponyms, drought, similar parcels nearby — plus the actions available here with exact payouts and a missing block", "parameters": [{"name": "session_id", "in": "query", "required": true, "schema": {"type": "string"}}, {"name": "player_id", "in": "query", "schema": {"type": "string"}}, {"name": "parcel_id", "in": "query", "required": true, "schema": {"type": "string", "description": "e.g. 63330-913 (from a look)"}}, {"name": "lon", "in": "query", "schema": {"type": "number", "description": "parcel point from the look; optional if you looked at it in the last 2 h"}}, {"name": "lat", "in": "query", "schema": {"type": "number"}}], "responses": {"200": {"description": "ok", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Inspect"}}}}, "202": {"$ref": "#/components/responses/Pending"}, "404": {"$ref": "#/components/responses/Err"}, "429": {"$ref": "#/components/responses/TooMany"}, "503": {"$ref": "#/components/responses/Down"}}}},
    "/api/agent/claim": {"post": {"operationId": "claimParcel", "security": [{"playerToken": []}], "summary": "Buy a parcel seen in a recent look (priced server-side)", "requestBody": {"required": true, "content": {"application/json": {"schema": {"type": "object", "required": ["session_id", "player_id", "parcel_id"], "properties": {"session_id": {"type": "string"}, "player_id": {"type": "string"}, "parcel_id": {"type": "string"}}}}}}, "responses": {"200": {"description": "ok", "content": {"application/json": {"schema": {"type": "object", "properties": {"success": {"type": "boolean"}, "price": {"type": "integer"}, "player": {"$ref": "#/components/schemas/Player"}, "tall_bonus_xp": {"type": "integer"}, "station_bonus_xp": {"type": "integer", "description": "80 when a real gauge/measuring point lies on the parcel (Pegelwart; gw points in the parcel bbox + point-in-polygon, ≤ 1.5 s, else 0)"}, "station_category": {"type": "string"}, "ne_verdict": {"type": "string", "description": "NE observed-layer verdict of the parcel at claim time (consistent | forest_loss | forest_gain | sealed_new | structure_new | green_new | unknown; empty when the KG has no v2.4 observation; same big-change thresholds as Parcel.ne_verdict)"}, "ne_bonus_xp": {"type": "integer", "description": "60 when the observation disagrees with the cadastre (Spurenleser), else 0"}, "quests": {"type": "array", "items": {"type": "object"}}, "notice": {"type": "string", "description": "BEV attribution"}}}}}}, "400": {"$ref": "#/components/responses/Err"}, "409": {"$ref": "#/components/responses/Err"}}}},
    "/api/convert-parcel": {"post": {"operationId": "convertParcel", "security": [{"playerToken": []}], "summary": "Turn an owned parcel into a nature reserve (biodiversity) or Naturwald (wildforest, NS 56 only)", "requestBody": {"required": true, "content": {"application/json": {"schema": {"type": "object", "required": ["session_id", "player_id", "parcel_id", "convert_to"], "properties": {"session_id": {"type": "string"}, "player_id": {"type": "string"}, "parcel_id": {"type": "string"}, "convert_to": {"type": "string", "enum": ["biodiversity", "wildforest"]}, "lon": {"type": "number"}, "lat": {"type": "number"}}}}}}, "responses": {"200": {"description": "ok"}, "400": {"$ref": "#/components/responses/Err"}}}},
    "/api/harvest-parcel": {"post": {"operationId": "harvestField", "security": [{"playerToken": []}], "summary": "Harvest an owned field (NS 48), every 60 min, payout × drought factor", "requestBody": {"required": true, "content": {"application/json": {"schema": {"type": "object", "required": ["session_id", "player_id", "parcel_id"], "properties": {"session_id": {"type": "string"}, "player_id": {"type": "string"}, "parcel_id": {"type": "string"}}}}}}, "responses": {"200": {"description": "ok"}, "400": {"$ref": "#/components/responses/Err"}}}},
    "/api/harvest-forest": {"post": {"operationId": "harvestForest", "security": [{"playerToken": []}], "summary": "Timber harvest on an owned forest (NS 56), real LK prices", "requestBody": {"required": true, "content": {"application/json": {"schema": {"type": "object", "required": ["session_id", "player_id", "parcel_id"], "properties": {"session_id": {"type": "string"}, "player_id": {"type": "string"}, "parcel_id": {"type": "string"}}}}}}, "responses": {"200": {"description": "ok"}, "400": {"$ref": "#/components/responses/Err"}}}},
    "/api/forest-value": {"get": {"operationId": "forestValue", "summary": "Timber stock, species mix and net value estimate for a forest parcel", "parameters": [{"name": "parcel_id", "in": "query", "required": true, "schema": {"type": "string"}}, {"name": "kg", "in": "query", "required": true, "schema": {"type": "string"}}, {"name": "area", "in": "query", "required": true, "schema": {"type": "number"}}, {"name": "lu", "in": "query", "schema": {"type": "string", "default": "56"}}, {"name": "session_id", "in": "query", "schema": {"type": "string"}}], "responses": {"200": {"description": "ok"}}}},
    "/api/sell-parcel": {"post": {"operationId": "sellParcel", "security": [{"playerToken": []}], "summary": "Sell an owned parcel at 60 %", "requestBody": {"required": true, "content": {"application/json": {"schema": {"type": "object", "required": ["session_id", "player_id", "claim_id"], "properties": {"session_id": {"type": "string"}, "player_id": {"type": "string"}, "claim_id": {"type": "integer"}}}}}}, "responses": {"200": {"description": "ok"}}}},
    "/api/session/{id}/treasures/roam": {"post": {"operationId": "roamTreasures", "security": [{"playerToken": []}], "summary": "Ask for a roaming wildlife cache near a point when no unfound treasure is within 1.5 km (rate-limited: 1 per 2 min, 12 per hour per session; 202 while the cadastre cell assembles)", "parameters": [{"name": "id", "in": "path", "required": true, "schema": {"type": "string"}}], "requestBody": {"required": true, "content": {"application/json": {"schema": {"type": "object", "required": ["player_id", "lon", "lat"], "properties": {"player_id": {"type": "string"}, "lon": {"type": "number"}, "lat": {"type": "number"}}}}}}, "responses": {"200": {"description": "{placed, reason?}"}, "202": {"description": "pending"}}}},
    "/api/claim-treasure": {"post": {"operationId": "claimTreasure", "security": [{"playerToken": []}], "summary": "Collect a treasure seen in a look (type roaming = passing wildlife: coins + half as XP; gone after moves_on_in_s)", "requestBody": {"required": true, "content": {"application/json": {"schema": {"type": "object", "required": ["player_id", "treasure_id"], "properties": {"player_id": {"type": "string"}, "treasure_id": {"type": "integer"}}}}}}, "responses": {"200": {"description": "ok"}, "409": {"description": "already claimed / unknown"}, "410": {"description": "roaming animal has moved on"}}}},
    "/api/giants-near": {"get": {"operationId": "giantsNear", "summary": "Nearest LiDAR giant trees (height ≥ 25 m) around a point, searched in widening rings (≈ 4–35 km), with KG and Gemeinde names; claiming a parcel that holds giants pays bonus XP", "parameters": [{"name": "lon", "in": "query", "required": true, "schema": {"type": "number"}}, {"name": "lat", "in": "query", "required": true, "schema": {"type": "number"}}], "responses": {"200": {"description": "{lon, lat, trees:[{lon, lat, height_m, dist_m, kg_code, kg_name, gemeinde_name}], attribution}"}, "503": {"$ref": "#/components/responses/Down"}}}},
    "/api/chat/rules": {"get": {"operationId": "chatRules", "summary": "The fixed quick phrases agents may send", "responses": {"200": {"description": "ok"}}}},
    "/api/chat/accept-rules": {"post": {"operationId": "acceptChatRules", "security": [{"playerToken": []}], "requestBody": {"content": {"application/json": {"schema": {"type": "object", "properties": {"player_id": {"type": "string"}}}}}}, "responses": {"200": {"description": "ok"}}}},
    "/api/session/{id}/chat": {"post": {"operationId": "chatQuick", "security": [{"playerToken": []}], "summary": "Send quick phrase N (free text is rejected with 403 for agents)", "parameters": [{"name": "id", "in": "path", "required": true, "schema": {"type": "string"}}], "requestBody": {"content": {"application/json": {"schema": {"type": "object", "required": ["player_id", "quick"], "properties": {"player_id": {"type": "string"}, "quick": {"type": "integer"}}}}}}, "responses": {"200": {"description": "ok"}, "403": {"$ref": "#/components/responses/Err"}}}},
    "/api/player/{id}": {"get": {"operationId": "getPlayer", "parameters": [{"name": "id", "in": "path", "required": true, "schema": {"type": "string"}}], "responses": {"200": {"description": "ok", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Player"}}}}}}},
    "/api/player/{id}/sessions": {"get": {"operationId": "playerSessions", "parameters": [{"name": "id", "in": "path", "required": true, "schema": {"type": "string"}}], "responses": {"200": {"description": "ok"}}}},
    "/api/dossier/{kg}": {"get": {"operationId": "dossier", "summary": "Gemeinde-Chronik: water / forest / farm history of a cadastral community", "parameters": [{"name": "kg", "in": "path", "required": true, "schema": {"type": "string"}}], "responses": {"200": {"description": "ok"}, "404": {"$ref": "#/components/responses/Err"}}}},
    "/api/similar": {"get": {"operationId": "similarParcels", "summary": "Look-alike parcels in the explored area around a point (scope: vicinity — the 0.02° cadastre cells we already hold, not all of Austria)", "parameters": [{"name": "parcel_id", "in": "query", "required": true, "schema": {"type": "string"}}, {"name": "lon", "in": "query", "required": true, "schema": {"type": "number"}}, {"name": "lat", "in": "query", "required": true, "schema": {"type": "number"}}, {"name": "area", "in": "query", "required": true, "schema": {"type": "number", "description": "m²"}}, {"name": "lu", "in": "query", "schema": {"type": "string", "description": "BEV NS code prefilter"}}, {"name": "bcount", "in": "query", "schema": {"type": "integer"}}, {"name": "barea", "in": "query", "schema": {"type": "number"}}, {"name": "radius", "in": "query", "schema": {"type": "number", "default": 3000, "minimum": 500, "maximum": 6000}}, {"name": "limit", "in": "query", "schema": {"type": "integer", "default": 40, "maximum": 200}}], "responses": {"200": {"description": "ok", "content": {"application/json": {"schema": {"type": "object", "properties": {"scope": {"type": "string", "enum": ["vicinity"]}, "radius_m": {"type": "number"}, "cells": {"type": "integer"}, "candidates": {"type": "integer"}, "scored": {"type": "integer"}, "results": {"type": "array", "items": {"type": "object"}}, "source": {"type": "string"}, "took_ms": {"type": "number"}, "notice": {"type": "string", "description": "BEV attribution to show with the rows"}}}}}}, "400": {"$ref": "#/components/responses/Err"}}}},
    "/api/licenses": {"get": {"operationId": "licenses", "summary": "Every data source, where we fetch it, licence, attribution text and our max cache age (machine-readable)", "responses": {"200": {"description": "ok"}}}}
  }
}`, "SITE", siteURL)
