package srv

// /llm/ahead — the roadmap we ask our sibling data services to implement,
// plus a live conformance harness their agents can test against.
//
// One table (aheadItems) is the single source of truth: it renders the
// markdown to-do list (GET /llm/ahead), the machine-readable manifest
// (?format=json) and drives the checks (GET /llm/ahead/check/{service}).
// Checks are tiny, bounded, cached and singleflighted, so an agent looping
// "implement → curl check → fix" costs the upstreams almost nothing.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Fixture: KG 63307 Gaisfeld (Stmk, Gemeinde 61611 Krottendorf-Gaisfeld) —
// covered by cadastre, srtm-lidar (v2 product), holzeinschlag and farm data.
const (
	fxKG       = "63307"
	fxGem      = "61611"
	fxBBoxQ    = "west=15.205&south=47.015&east=15.215&north=47.025" // ~0.8 km²
	fxBBoxCSV  = "15.205,47.015,15.215,47.025"
	fxPoly     = `{"type":"Polygon","coordinates":[[[15.207,47.017],[15.210,47.017],[15.210,47.019],[15.207,47.019],[15.207,47.017]]]}`
	fxNoDataKG = "00000"
	fxPtQ      = "lon=15.2105&lat=47.0195" // inside fxBBox, ~400 m from gw station gw:356899
)

type aheadService struct {
	Slug, Base, Repo string
}

var aheadServices = []aheadService{
	{"cadastre", "https://cadastre-process-api.exe.xyz", "raffopenssh/cadastre-process-api"},
	{"srtm", "https://srtm-lidar-at.exe.xyz:8000", "raffopenssh/srtm-lidar-at"},
	{"holz", "https://holzeinschlag-at.exe.xyz", "raffopenssh/holzeinschlag-austria"},
	{"farm", "https://farm-subsidies-austria.exe.xyz", "raffopenssh/farm-subsidies"},
	{"gw", "https://groundwater-at.exe.xyz", "groundwater-at (GW Power)"},
}

// aheadCheck is a declarative HTTP probe. Paths are relative to the service
// base. Has entries are dotted JSON paths ("parcels.0.landuse_areas"); a "|"
// separates alternatives (any one suffices). Status empty = 200 only.
type aheadCheck struct {
	Method, Path, Body string
	Headers            map[string]string
	Status             []int
	MaxMs              int
	MaxBytes           int
	Has                []string
	HeaderHas          map[string]string // header must contain value (case-insensitive)
	Custom             func(res *probeRes) error
}

type aheadItem struct {
	ID, Prio, Title string
	Services        []string // "*" = all
	Spec            string   // terse contract
	Why             string   // one line: what it unlocks in the game
	Check           *aheadCheck
	Manual          bool // no automatic check
}

// aheadUsed: items the game actually consumes today → where. Rendered as [x]
// in the roadmap; ?unused=1 hides them so an implementing agent only reads
// what still matters to us (green in the harness ≠ used in the game).
var aheadUsed = map[string]string{
	"ALL-4":  "server: prewarmMunicipality() on POST /api/session/create (cadastre + srtm)",
	"CAD-1":  "server: GET /api/viewport-landuse → game.js loadViewportLanduse() (replaces whole-KG landuse export)",
	"CAD-2":  "game.js: extractLuCode() prefers dominant_ns; popup lists landuse_areas in m²; price uses it",
	"HOLZ-2": "server: timberStatePrices() in timber.go (Holzernte prices)",
	"FARM-2": "server: GET /api/schlaege → game.js loadSchlaege(): real crop textures + 'Feld' popup row",
	"ALL-1":  "server: llmKG() in dossier.go → GET /api/dossier/{kg} (gw + holz + farm /llm/kg in parallel)",
	"ALL-1b": "server: llmGet() negative-caches the 404 no_data contract for 1 h (dossier.go)",
	"GW-1":   "server: GET /api/water/point, wellQuote() → POST /api/dig-well prices the Brunnen by depth_to_gw_m_est (water.go)",
	"GW-2":   "server: GET /api/water/points (bbox) + /api/water/station/{id} (history) — Messstellen as landmarks",
	"GW-3":   "server: droughtFrom() in dossier.go — season_profile + now.status → drought level 0..3 scales harvest yield",
	"GW-4":   "server: now block → droughtState.status/sigma in /api/dossier + /api/drought; wellProtection() halves when very_low",
	"GW-5":   "server: GET /api/water/protection (bbox) + inWaterProtection() → Naturschutz XP ×1.5 inside a Wasserschutzgebiet",
	"GW-6":   "server: GET /api/water/flowpath (24 h cache, 3-decimal quantized) — Wassertropfen-Reise",
	"GW-7":   "server: GET /api/water/gwi (24 h) for the municipality picker tint",
	"GW-8":   "server: stationOnParcel() on POST /api/claim-parcel (gw_station hint) → +80 XP Pegelwart; GET /api/water/parcel/{pid}",
	"HOLZ-1": "server: forestBlock() in dossier.go → 'Wald' tab of the Gemeinde-Chronik (24-year loss/harvest/CO₂ timeline)",
	"FARM-1": "server: farmBlock() + subsidyCoins() — Förderung paid on every harvest and once per cycle on meadows (eur_per_ha_median, ×1.3 organic)",
}

var aheadItems = []aheadItem{
	// ---- cross-cutting: the sibling spec nobody implements yet ----
	{ID: "ALL-1", Prio: "P1", Services: []string{"*"}, Title: "GET /llm/kg/{kg_code} (sibling spec)",
		Spec: "Per cadastre spec (cadastre /api/v1/docs/llm.txt?section=integration). JSON: service, dataset, kg_code, gemeinde_code, granularity (gemeinde|point|parcel), as_of, updated_at, source, license, unit_glossary, metrics{flat snake_case numbers}, history[{as_of,...}]. Unknown KG → 404 {kg_code,error:\"no_data\"}. CORS *. ETag. <300 ms warm.",
		Why:  "One KG dossier call at session start replaces 6-8 proxies with 4 pending semantics.",
		Check: &aheadCheck{Path: "/llm/kg/" + fxKG, MaxMs: 3000, Has: []string{"service", "kg_code", "granularity", "metrics", "as_of"},
			HeaderHas: map[string]string{"Access-Control-Allow-Origin": "*"}}},
	{ID: "ALL-1b", Prio: "P1", Services: []string{"*"}, Title: "/llm/kg 404 contract",
		Spec:  "GET /llm/kg/00000 → HTTP 404, body {\"kg_code\":\"00000\",\"error\":\"no_data\"}. Never 500, never empty 200.",
		Why:   "Lets us distinguish 'no data' from 'pending' without heuristics.",
		Check: &aheadCheck{Path: "/llm/kg/" + fxNoDataKG, Status: []int{404}, MaxMs: 3000, Has: []string{"error"}}},
	{ID: "ALL-2", Prio: "P1", Services: []string{"*"}, Title: "GET /llm/manifest.json",
		Spec:  "{service, finest_granularity, join_keys[], kg_endpoint:\"/llm/kg/{kg_code}\", metrics_schema{key:type}, kg_count, covered_kgs_url?, updated_at}. Optional GET /llm/kgs?codes=a,b (≤500) → {results:[...]}.",
		Why:   "We crawl coverage once instead of probing per KG.",
		Check: &aheadCheck{Path: "/llm/manifest.json", MaxMs: 3000, Has: []string{"service", "kg_endpoint", "kg_count"}}},
	{ID: "ALL-3", Prio: "P2", Services: []string{"*"}, Title: "GET /llm.txt at root",
		Spec: "text/plain, ≤8 KB, lists /llm/ endpoints + base URL + license + rate limits. Must mention '/llm/kg/'.",
		Why:  "Agents (ours and yours) discover the contract without reading source.",
		Check: &aheadCheck{Path: "/llm.txt", MaxMs: 3000, MaxBytes: 16384, Custom: func(r *probeRes) error {
			if !bytes.Contains(r.Body, []byte("/llm/kg/")) {
				return fmt.Errorf("does not mention /llm/kg/")
			}
			return nil
		}}},
	{ID: "ALL-4", Prio: "P2", Services: []string{"cadastre", "srtm"}, Title: "POST /api/v1/prewarm?kgs=a,b — explicit warm-up hint",
		Spec:  "Idempotent, deduped, low-priority. Returns ≤500 ms with 202 {queued:[..],already_warm:[..]} (200 if all warm). ≤50 KGs per call. Fired once per game session create.",
		Why:   "Kills loading-screen 202 waits; both of you lazily pull Zenodo.",
		Check: &aheadCheck{Method: "POST", Path: "/api/v1/prewarm?kgs=" + fxKG, Status: []int{200, 202}, MaxMs: 2000}},
	{ID: "ALL-5", Prio: "P3", Services: []string{"holz", "farm", "gw"}, Title: "ETag/304 + gzip on static JSON",
		Spec: "Every /data/* and /llm/* JSON: ETag (or Last-Modified), If-None-Match → 304, Content-Encoding gzip when requested, Cache-Control max-age≥3600. CORS *.",
		Why:  "700 KB catalog every session → 0 bytes when unchanged.",
		Check: &aheadCheck{Path: "/llm/manifest.json", MaxMs: 3000, Custom: func(r *probeRes) error {
			if r.Header.Get("ETag") == "" && r.Header.Get("Last-Modified") == "" {
				return fmt.Errorf("no ETag/Last-Modified")
			}
			if !r.Uncompressed && r.Header.Get("Content-Encoding") == "" {
				return fmt.Errorf("not gzip-encoded")
			}
			return nil
		}}},

	// ---- cadastre ----
	{ID: "CAD-1", Prio: "P1", Services: []string{"cadastre"}, Title: "GET /api/v1/spatial/landuse?west&south&east&north — R-tree landuse polygons",
		Spec:  "Same contract as /spatial/parcels: {landuse:[{kg_code, code (NS 40-97), area_sqm, geometry(Polygon|MultiPolygon, ring[0]=exterior)}], count, limit, ready, truncated, warming?}. Clipped to bbox or whole polygons ≤ limit. ≤150 ms warm.",
		Why:   "Backdrop today streams the whole-KG landuse export (~7 MB); viewport slice is ~50 KB.",
		Check: &aheadCheck{Path: "/api/v1/spatial/landuse?" + fxBBoxQ, MaxMs: 5000, Has: []string{"landuse.0.code", "landuse.0.geometry", "ready"}}},
	{ID: "CAD-2", Prio: "P1", Services: []string{"cadastre"}, Title: "landuse_areas on /spatial/parcels rows",
		Spec:  "Each parcel row gains landuse_areas:{\"48\":12000.5,\"56\":450.0} (m² from polygon geometry, same source as landuse_area_breakdown) and dominant_ns:\"48\" (largest area). Default on; ?attrs=min to drop.",
		Why:   "Replaces symbol-count guessing (nsWeight hack) for terrain fill + price; lets us drop the landuse layer entirely.",
		Check: &aheadCheck{Path: "/api/v1/spatial/parcels?" + fxBBoxQ, MaxMs: 5000, Has: []string{"parcels.0.landuse_areas|parcels.0.dominant_ns"}}},
	{ID: "CAD-3", Prio: "P2", Services: []string{"cadastre"}, Title: "?tolerance_m= geometry simplification on /spatial/parcels|footprints",
		Spec:  "Douglas-Peucker (or VW) with tolerance in metres, 0 = exact. Response echoes tolerance_m; keep ring closure, drop rings whose area < tolerance². Suggested client mapping: zoom13→8 m, z15→2 m, z17+→0.",
		Why:   "16 km² Alm polygons with thousands of vertices at zoom 13 kill canvas FPS in dense towns.",
		Check: &aheadCheck{Path: "/api/v1/spatial/parcels?" + fxBBoxQ + "&tolerance_m=10", MaxMs: 5000, Has: []string{"tolerance_m"}}},
	{ID: "CAD-4", Prio: "P2", Services: []string{"cadastre"}, Title: "?include=land_prices,osm,n2k enrichment on /spatial/parcels rows",
		Spec: "Opt-in comma list. land_prices → {buy_total_blended_eur, class_source}; osm → {dist_road_m, remoteness}; n2k → in_natura2000:bool, n2k_site_codes[]. Unknown include → 400. Row cost ≤ +200 B.",
		Why:  "Price heat-map, remoteness quests and N2K badges without one lazy call per popup.",
		Check: &aheadCheck{Path: "/api/v1/spatial/parcels?" + fxBBoxQ + "&include=land_prices,osm,n2k", MaxMs: 6000,
			Has: []string{"parcels.0.land_prices|parcels.0.osm|parcels.0.in_natura2000"}}},
	{ID: "CAD-5", Prio: "P2", Services: []string{"cadastre"}, Title: "assembly version on viewport responses",
		Spec:  "Add assembly:{\"63307\":{version:\"tile_partition_v1\",reprocessed_at:\"2026-08-06T..\"}} per KG touched to /spatial/parcels, /spatial/footprints and /export/geojson (as header X-Assembly-Version for GeoJSON).",
		Why:   "Lets us invalidate our 6 h viewport cache exactly when you reprocess a KG, instead of purging everything.",
		Check: &aheadCheck{Path: "/api/v1/spatial/parcels?" + fxBBoxQ, MaxMs: 5000, Has: []string{"assembly"}}},

	// ---- srtm-lidar ----
	{ID: "LID-1", Prio: "P1", Services: []string{"srtm"}, Title: "/api/v1/query/parcels?bbox= carries the slim fields, served from index",
		Spec: "Row fields: parcel_id, fracs{type:frac ≥0.02}, tree_h{mean,max}, dom_terrain (dominant natural cover), auto_class, auto_subclass, slope_deg, elev_m, aspect_deg, top_trees[{lon,lat,h,rf_conf}] (h≥25). Answered from the R-tree/SQLite index, never from the KG JSON; ≤500 ms warm, ready:false + retry_after_s when cold (never hang).",
		Why:  "Deletes our 1 MB-per-KG lidar-slim proxy; enhanced data loads per viewport tile like the cadastre.",
		Check: &aheadCheck{Path: "/api/v1/query/parcels?bbox=" + fxBBoxCSV + "&limit=20", MaxMs: 4000,
			Has: []string{"parcels.0.fracs|data.parcels.0.fracs|results.0.fracs", "parcels.0.tree_h|data.parcels.0.tree_h|results.0.tree_h"}}},
	{ID: "LID-2", Prio: "P1", Services: []string{"srtm"}, Title: "GET /api/v1/trees/bbox — tree apices from a precomputed index",
		Spec:  "Params west,south,east,north, min_height (default 20), limit (≤2000). {trees:[{lon,lat,h_m,crown_d_m?,species_hint?,parcel_id?}], count, truncated, ready}. Backed by a pre-extracted apex R-tree (not the light GPKG). ≤500 ms warm.",
		Why:   "Giant-tree gameplay + timber estimate stop depending on the 2.5 s POST /v3/trees budget.",
		Check: &aheadCheck{Path: "/api/v1/trees/bbox?" + fxBBoxQ + "&min_height=20&limit=50", MaxMs: 4000, Has: []string{"trees", "ready|count"}}},
	{ID: "LID-3", Prio: "P2", Services: []string{"srtm"}, Title: "footprint_id on buildings",
		Spec:  "Every building object (KG JSON buildings[], /query/buildings, /kg/X/buildings) carries cadastre footprint_id (from /api/v1/spatial/footprints) next to the address building_id. null when unmatched; matching by ≥50 % footprint overlap.",
		Why:   "We match heights by centroid grid today — fragile in terraces and courtyards.",
		Check: &aheadCheck{Path: "/api/v1/query/buildings?bbox=" + fxBBoxCSV + "&limit=5", MaxMs: 4000, Has: []string{"buildings.0.footprint_id|data.buildings.0.footprint_id|results.0.footprint_id"}}},
	{ID: "LID-4", Prio: "P2", Services: []string{"srtm"}, Title: "Static hillshade / nDSM XYZ tiles",
		Spec:  "GET /tiles/{hillshade|ndsm}/{z}/{x}/{y}.png (z 12-17, 256 px, WebMercator) pre-rendered from the products; 204 for no-data tiles; Cache-Control 1 y; CORS *.",
		Why:   "Real relief under the pixel art; overlay endpoints are POST-and-render and we had to block them.",
		Check: &aheadCheck{Path: "/tiles/hillshade/14/8882/5773.png", Status: []int{200, 204}, MaxMs: 4000, HeaderHas: map[string]string{"Content-Type": "image/"}}},
	{ID: "LID-5", Prio: "P2", Services: []string{"srtm"}, Title: "POST /api/v1/processing/queue low-priority hint (dedupe, instant)",
		Spec: "Body {kgs:[..], priority:\"low\", source:\"siedler\"}. Unauthenticated when priority=low (today: 401). Skips processed KGs, dedupes queued ones, returns ≤1 s with 202 {queued[], skipped[], position}. Hard cap 20 KGs/call, 100/h per source.",
		Why:  "Coverage is 33 % of KGs; queueing where players actually are grows it where it matters.",
		Check: &aheadCheck{Method: "POST", Path: "/api/v1/processing/queue", Body: `{"kgs":["` + fxKG + `"],"priority":"low","source":"siedler-ahead-check"}`,
			Headers: map[string]string{"Content-Type": "application/json"}, Status: []int{200, 202}, MaxMs: 3000}},
	{ID: "LID-6", Prio: "P3", Services: []string{"srtm"}, Title: "Light artefact split for cold start", Manual: true,
		Spec: "Publish parcels+buildings+tree_apices as a separate ≤5 MB Zenodo artefact per KG so a cold KG answers LID-1/LID-2 in ≤5 s (cadastre cold path is ~2 s). Keep the full GPKG for /v3.",
		Why:  "223 s cold start makes 'Auf Glück' unplayable outside already-warm KGs."},

	// ---- holzeinschlag ----
	{ID: "HOLZ-1", Prio: "P1", Services: []string{"holz"}, Title: "/llm/kg/{kg} = municipal forest timeline",
		Spec:  "granularity gemeinde. metrics: forest_area_ha, loss_ha_2024, loss_total_ha, harvest_efm, harvest_value_eur, co2_t, net_flux_tco2e_ha, price_spruce_eur_efm. history: one row per year 2001-2024 with the same keys (null where series absent). kg→gemeinde via cadastre /lookup, cached.",
		Why:   "Gives every forest parcel a 24-year context line and a real CO₂ number for Naturwald.",
		Check: &aheadCheck{Path: "/llm/kg/" + fxKG, MaxMs: 3000, Has: []string{"metrics.harvest_efm", "history.0.as_of"}}},
	{ID: "HOLZ-2", Prio: "P1", Services: []string{"holz"}, Title: "GET /data/prices/state/{1-9}.json — slim regional prices",
		Spec:  "≤10 KB: {state, state_name, as_of, series:{LK_BLFIM2b:{name,unit,latest,latest_date,yearly_avg:{\"2025\":..}}, LK_BLLA3aplus, LK_BLKI2aplus, LK_BLBU3plus, LK_ISFI_FMO, LK_BHH, LK_BHW}}. State code = Bundesland digit as in KG code prefix (6 = Stmk). Regenerated by the weekly cron. ETag.",
		Why:   "We pull 736 KB to read 7 numbers.",
		Check: &aheadCheck{Path: "/data/prices/state/6.json", MaxMs: 3000, MaxBytes: 20480, Has: []string{"series.LK_BLFIM2b.latest"}}},
	{ID: "HOLZ-3", Prio: "P2", Services: []string{"holz"}, Title: "POST /api/plot-context?fast=1 — plot-only, cached, concurrent",
		Spec: "fast=1 skips rings + municipality zonal stats: returns input, plot{forest_share_2000_pct, loss_ha_by_year, loss_total, net_flux_tco2e_ha, gross_emissions, gross_removals}, municipality_codes[] only, and echoes fast:true. Target ≤2 s for ≤5 ha, ≥4 concurrent, cache keyed by geometry hash (24 h). Full mode unchanged.",
		Why:  "10-60 s single-threaded is unusable in-game; the plot part is a 30 m raster read.",
		Check: &aheadCheck{Method: "POST", Path: "/api/plot-context?fast=1", Body: fxPoly, Headers: map[string]string{"Content-Type": "application/json"},
			Status: []int{200}, MaxMs: 3000, Has: []string{"plot", "fast"}}},
	{ID: "HOLZ-4", Prio: "P2", Services: []string{"holz"}, Title: "GET /api/stands?west&south&east&north — Waldkarte stand layer (API-only)",
		Spec:  "{stands:[{id, type: nadel|laub|misch, conifer_share?, age_class?, crown_cover_pct?, geometry}], as_of, source, ready, truncated}. ≤300 ms; limit 2000; license carried.",
		Why:   "Replaces our elevation-based species guess → real Holzernte prices and Naturwald rendering.",
		Check: &aheadCheck{Path: "/api/stands?" + fxBBoxQ, MaxMs: 4000, Has: []string{"stands"}}},

	// ---- farm subsidies ----
	{ID: "FARM-1", Prio: "P1", Services: []string{"farm"}, Title: "/llm/kg/{kg} = municipal subsidy profile (aggregate only)",
		Spec:  "granularity gemeinde, no recipient data. metrics: recipients_n, total_eur, eur_per_ha_median, eur_per_recipient_median, organic_share, archetype_mix{archetype:share}, top_measures[{code,name,share}]. history per year. kg→gemeinde via cadastre /lookup.",
		Why:   "'Förderung' income mechanic on farmland + Bio-Bergbauer quests, GDPR-free.",
		Check: &aheadCheck{Path: "/llm/kg/" + fxKG, MaxMs: 4000, Has: []string{"metrics.eur_per_ha_median|metrics.total_eur"}}},
	{ID: "FARM-2", Prio: "P1", Services: []string{"farm"}, Title: "GET /api/schlaege?west&south&east&north — INVEKOS field polygons",
		Spec:  "{fields:[{id, snar_code, snar_name, crop_group (getreide|mais|gruenland|wein|obst|alm|brache|sonst), area_ha, organic:bool?, geometry}], year, count, truncated, ready}. Open national INVEKOS Schläge data, WGS84, limit 3000, ≤300 ms from an R-tree; join to parcel_id optional (POST cadastre /spatial/points).",
		Why:   "Real crop per field replaces hash-based field textures; field cycle uses true phenology. Biggest visual win available.",
		Check: &aheadCheck{Path: "/api/schlaege?" + fxBBoxQ, MaxMs: 5000, Has: []string{"fields", "year"}}},
	{ID: "FARM-3", Prio: "P2", Services: []string{"farm"}, Title: "CORS + /llm.txt",
		Spec:  "Access-Control-Allow-Origin: * on all read-only JSON; root /llm.txt documenting /api/* and /llm/*.",
		Why:   "Today no llm.txt, no CORS: unusable from a browser game.",
		Check: &aheadCheck{Path: "/llm/manifest.json", MaxMs: 4000, HeaderHas: map[string]string{"Access-Control-Allow-Origin": "*"}}},
	{ID: "FARM-4", Prio: "P3", Services: []string{"farm"}, Title: "GET /api/hofstellen?west&south&east&north — farmsteads",
		Spec:  "{points:[{id (hashed betr_id), lon, lat, organic?, size_class (s|m|l), parcel_id?}], year}. Aggregated/hashed, no names.",
		Why:   "Render the real farmstead as the EZ 'home'.",
		Check: &aheadCheck{Path: "/api/hofstellen?" + fxBBoxQ, MaxMs: 5000, Has: []string{"points"}}},

	// ---- groundwater (GW Power) — already ships /llm/kg, /llm/kgs, manifest, llm.txt (ALL-1..3 green).
	// What follows moves it from "KG dossier" to parcel-scale gameplay: wells, drought seasons, water landmarks.
	{ID: "GW-1", Prio: "P1", Services: []string{"gw"}, Title: "GET /llm/point?lon=&lat= — water context at a coordinate (parcel scale)",
		Spec:  "Same IDW as the KG centroid calc, evaluated at the point. {lon,lat,kg_code, metrics:{gwi, gwi_category, gwi_q_*, gwi_gw_trend, gwi_no3, depth_to_gw_m_est, depth_confidence (high|med|low), aquifer_type}, nearest_gw_station:{id,name,distance_m,gw_level_m,gw_trend_m_per_decade,parcel_id?}, nearest_no3_station:{id,distance_m,no3_mg_l}, nearest_river:{name,distance_m,reach_id,glacier_fed:bool}, groundwater_body:{gwk_id,name,abstraction_intensity_pct}}. depth_to_gw = IDW station level (m a.s.l.) − DEM at point; null + confidence low when nearest station > 5 km. Quantize input to 4 decimals for cache hits. ≤150 ms. 404 no_data outside coverage.",
		Why:   "KGs span 0.5–30 km; a well/irrigation mechanic needs the value under *this* parcel. 'Brunnen graben' costs by depth, pays by gwi.",
		Check: &aheadCheck{Path: "/llm/point?" + fxPtQ, MaxMs: 3000, Has: []string{"metrics.gwi", "nearest_gw_station"}}},
	{ID: "GW-2", Prio: "P1", Services: []string{"gw"}, Title: "GET /llm/points?west&south&east&north — stations/plants/sites in a bbox, no history",
		Spec:  "R-tree over the four point datasets. Params: categories=groundwater_station,nitrate_station,power_plant,water_quality_site (default all), limit ≤500, history=0|1 (default 0). {points:[{id,category,name,lon,lat,kg_code,parcel_id?,metrics{…latest only},history_url:\"/llm/point/{id}\"}], count, truncated}. ≤100 ms. CORS *.",
		Why:   "Stations become discoverable map landmarks (Messstelle, Brunnen, Kraftwerk) with a real 30-year chart on tap — today only the points snapped into each visible KG are reachable and each costs a 9 KB dossier.",
		Check: &aheadCheck{Path: "/llm/points?" + fxBBoxQ, MaxMs: 3000, Has: []string{"points.0.category", "count"}}},
	{ID: "GW-3", Prio: "P1", Services: []string{"gw"}, Title: "drought block on /llm/kg — event calendar + seasonal profile",
		Spec:  "drought:{source, as_of, p_drought_year (share of years since 2012 with ≥1 month CDI ≥ warning), worst_year, season_profile:[12 × mean CDI class per calendar month], events:[{year,start_month,end_month,cdi_max,class:watch|warning|alert}]} from the monthly Copernicus EDO CDI grid (you already hold the annual mean/max). Extend history to the latest complete year and add precip_mm + gw_level_anomaly_m per year where the station/precip series allow. Null-safe; never drop existing keys.",
		Why:   "Real, local drought seasons drive an in-game event: fields yield less, the well runs dry, the Naturschutz meadow survives. p_drought_year is the dice, season_profile the calendar.",
		Check: &aheadCheck{Path: "/llm/kg/" + fxKG, MaxMs: 3000, Has: []string{"drought.p_drought_year|drought.events", "drought.season_profile"}}},
	{ID: "GW-4", Prio: "P2", Services: []string{"gw"}, Title: "now block — latest measured state (eHYD daily), refreshed daily",
		Spec:  "now:{as_of (date of newest observation), gw_level_anomaly_sigma (mean over stations within the IDW radius: latest level vs. that calendar month's 1991-2020 mean, in σ), gw_percentile_of_month (0-100), n_stations, trend_30d_cm, no3_latest_mg_l?, status: normal|low|very_low|high}. Source eHYD current data (or the GeoSphere/BML public feeds); one daily cron, served from a file; Cache-Control 1 h. Also on GET /llm/point.",
		Why:   "A 'Grundwasser heute' weather bar: the game world follows the real aquifer under the player. Zero cost per request.",
		Check: &aheadCheck{Path: "/llm/kg/" + fxKG, MaxMs: 3000, Has: []string{"now.gw_level_anomaly_sigma|now.status"}}},
	{ID: "GW-5", Prio: "P2", Services: []string{"gw"}, Title: "GET /llm/protection?west&south&east&north — Wasserschutz-/Schongebiete polygons",
		Spec:  "{zones:[{id, name, type: schutzgebiet|schongebiet, zone: I|II|III|null, state, source, geometry (Polygon|MultiPolygon, WGS84, ring[0] exterior)}], count, ready, truncated}. Sources: Länder WIS / INSPIRE AM (Area Management) datasets; carry the per-state license string. ≤200 ms; limit 500. Plus per-KG metric water_protection_share_pct on /llm/kg.",
		Why:   "A real reason a meadow must stay a meadow: conversion surcharge / manure ban inside zone II, bonus XP for Naturschutz there. Also the nitrate mechanic gets a map.",
		Check: &aheadCheck{Path: "/llm/protection?" + fxBBoxQ, MaxMs: 4000, Has: []string{"zones", "ready|count"}}},
	{ID: "GW-6", Prio: "P2", Services: []string{"gw"}, Title: "GET /llm/flowpath?lon=&lat= — downstream reach chain to the border",
		Spec:  "Walk the MERIT-Basins reach graph downstream from the nearest reach. {start:{reach_id,river,distance_m}, reaches:[{reach_id, river, length_km, glacier_fed:bool, gauge?:{hzb,name,flow_mean_m3s,flow_trend_pct_per_decade}}], total_km, exit:{river, border_point:[lon,lat], sea: black_sea|north_sea}, geometry: LineString simplified ≤3 KB, catchment_geometry_url (the containing verified catchment as GeoJSON)}. ≤300 ms (precompute per reach).",
		Why:   "'Wassertropfen-Reise' quest: follow the rain from your parcel to the Danube, with every real gauge en route as a checkpoint; catchment outline as a map overlay.",
		Check: &aheadCheck{Path: "/llm/flowpath?" + fxPtQ, MaxMs: 4000, Has: []string{"reaches", "total_km"}}},
	{ID: "GW-7", Prio: "P2", Services: []string{"gw"}, Title: "GET /llm/gwi.json + ?fields= slimming",
		Spec:  "/llm/gwi.json → {as_of, categories:{0:good,1:watch,2:stressed}, kgs:{\"63307\":[0.53,2], …}} for all 7,850 KGs, ≤150 KB gzip, ETag, Cache-Control 1 d. On /llm/kg and /llm/kgs: ?fields=metrics,points,drought (comma list of top-level keys; unit_glossary only when asked) and ?history=0.",
		Why:   "Colour the municipality picker by water stress in one cached file; per-session KG dossiers drop from 9 KB (3 KB glossary each) to ~1 KB.",
		Check: &aheadCheck{Path: "/llm/gwi.json", MaxMs: 3000, MaxBytes: 1 << 20, Has: []string{"kgs", "as_of"}}},
	{ID: "GW-8", Prio: "P3", Services: []string{"gw"}, Title: "parcel_id on every point + GET /llm/parcel/{parcel_id}",
		Spec:  "Snap all points to parcels (today only 'when confident'); expose parcel_id → {points[], water_body, point-context (= GW-1 at the parcel centroid)}. 404 no_data when the parcel has no snapped point and no coverage.",
		Why:   "Buying the parcel with the Messstelle on it is a collectible ('Pegelwart' badge); one call from the parcel popup.",
		Check: &aheadCheck{Path: "/llm/parcel/63307-133/5", MaxMs: 3000, Has: []string{"points|metrics"}}},
}

func aheadServiceByName(slug string) *aheadService {
	for i := range aheadServices {
		if aheadServices[i].Slug == slug {
			return &aheadServices[i]
		}
	}
	return nil
}

func itemAppliesTo(it aheadItem, slug string) bool {
	for _, s := range it.Services {
		if s == "*" || s == slug {
			return true
		}
	}
	return false
}

// ---- probe engine ----

type probeRes struct {
	Status       int
	Header       http.Header
	Body         []byte
	Uncompressed bool
	Ms           int64
}

type checkResult struct {
	ID     string `json:"id"`
	Prio   string `json:"prio"`
	Status string `json:"status"` // pass | fail | manual
	Ms     int64  `json:"ms,omitempty"`
	HTTP   int    `json:"http,omitempty"`
	Detail string `json:"detail,omitempty"`
	URL    string `json:"url,omitempty"`
}

var aheadProbeClient = &http.Client{
	Timeout: 10 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse // an auth redirect is a fail, not a pass
	},
	Transport: &http.Transport{MaxIdleConnsPerHost: 4, IdleConnTimeout: 30 * time.Second, ForceAttemptHTTP2: true},
}

func runProbe(ctx context.Context, base string, c *aheadCheck) (*probeRes, error) {
	m := c.Method
	if m == "" {
		m = http.MethodGet
	}
	var body io.Reader
	if c.Body != "" {
		body = strings.NewReader(c.Body)
	}
	to := time.Duration(c.MaxMs)*time.Millisecond + 2*time.Second
	if to > 10*time.Second {
		to = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, to)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, m, base+c.Path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "siedler-ahead-check/1 (+https://siedler-oesterreich.exe.xyz:8000/llm/ahead)")
	req.Header.Set("Accept", "application/json, text/plain;q=0.9, */*;q=0.5")
	for k, v := range c.Headers {
		req.Header.Set(k, v)
	}
	t0 := time.Now()
	resp, err := aheadProbeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	return &probeRes{Status: resp.StatusCode, Header: resp.Header, Body: b, Uncompressed: resp.Uncompressed, Ms: time.Since(t0).Milliseconds()}, nil
}

// jsonPath walks "a.b.0.c" in decoded JSON. Returns (value, ok).
func jsonPath(v any, path string) (any, bool) {
	for _, seg := range strings.Split(path, ".") {
		switch t := v.(type) {
		case map[string]any:
			nv, ok := t[seg]
			if !ok {
				return nil, false
			}
			v = nv
		case []any:
			i, err := strconv.Atoi(seg)
			if err != nil || i < 0 || i >= len(t) {
				return nil, false
			}
			v = t[i]
		default:
			return nil, false
		}
	}
	return v, true
}

func evalCheck(base string, it aheadItem) checkResult {
	c := it.Check
	cr := checkResult{ID: it.ID, Prio: it.Prio}
	if c == nil || it.Manual {
		cr.Status = "manual"
		return cr
	}
	cr.URL = base + c.Path
	res, err := runProbe(context.Background(), base, c)
	if err != nil {
		cr.Status, cr.Detail = "fail", "request: "+trimErr(err)
		return cr
	}
	cr.Ms, cr.HTTP = res.Ms, res.Status
	want := c.Status
	if len(want) == 0 {
		want = []int{200}
	}
	okStatus := false
	for _, s := range want {
		okStatus = okStatus || s == res.Status
	}
	if !okStatus {
		cr.Status = "fail"
		cr.Detail = fmt.Sprintf("HTTP %d (want %v)", res.Status, want)
		if res.Status >= 300 && res.Status < 400 {
			cr.Detail += " — redirect; is the port public (no proxy auth)?"
		}
		return cr
	}
	if c.MaxBytes > 0 && len(res.Body) > c.MaxBytes {
		cr.Status, cr.Detail = "fail", fmt.Sprintf("%d B > max %d B", len(res.Body), c.MaxBytes)
		return cr
	}
	for k, v := range c.HeaderHas {
		if !strings.Contains(strings.ToLower(res.Header.Get(k)), strings.ToLower(v)) {
			cr.Status, cr.Detail = "fail", fmt.Sprintf("header %s missing/≠ %q (got %q)", k, v, res.Header.Get(k))
			return cr
		}
	}
	if len(c.Has) > 0 {
		var doc any
		if err := json.Unmarshal(res.Body, &doc); err != nil {
			cr.Status, cr.Detail = "fail", "body is not JSON"
			return cr
		}
		for _, alt := range c.Has {
			found := false
			for _, p := range strings.Split(alt, "|") {
				if _, ok := jsonPath(doc, p); ok {
					found = true
					break
				}
			}
			if !found {
				cr.Status, cr.Detail = "fail", "missing JSON field: "+alt
				return cr
			}
		}
	}
	if c.Custom != nil {
		if err := c.Custom(res); err != nil {
			cr.Status, cr.Detail = "fail", err.Error()
			return cr
		}
	}
	if c.MaxMs > 0 && res.Ms > int64(c.MaxMs) {
		cr.Status, cr.Detail = "fail", fmt.Sprintf("%d ms > budget %d ms (warm)", res.Ms, c.MaxMs)
		return cr
	}
	cr.Status = "pass"
	return cr
}

func trimErr(err error) string {
	s := err.Error()
	if i := strings.LastIndex(s, ": "); i > 0 && len(s)-i < 80 {
		s = s[i+2:]
	}
	if len(s) > 120 {
		s = s[:120]
	}
	return s
}

// ---- report cache / orchestration ----

type aheadReport struct {
	Service   string        `json:"service"`
	Base      string        `json:"base"`
	At        time.Time     `json:"at"`
	Pass      int           `json:"pass"`
	Fail      int           `json:"fail"`
	Manual    int           `json:"manual"`
	Score     string        `json:"score"` // "3/12"
	Results   []checkResult `json:"results"`
	NextSteps []string      `json:"next_steps,omitempty"`
	TookMs    int64         `json:"took_ms"`
	Cached    bool          `json:"cached"`
}

var aheadCache struct {
	sync.Mutex
	m map[string]*aheadReport
}

const aheadCacheTTL = 60 * time.Second

// aheadSlots bounds concurrent probing per upstream (1) and globally (2).
var aheadSlots = struct {
	global chan struct{}
	mu     sync.Mutex
	per    map[string]chan struct{}
}{global: make(chan struct{}, 2), per: map[string]chan struct{}{}}

func aheadSlot(host string) chan struct{} {
	aheadSlots.mu.Lock()
	defer aheadSlots.mu.Unlock()
	c, ok := aheadSlots.per[host]
	if !ok {
		c = make(chan struct{}, 1)
		aheadSlots.per[host] = c
	}
	return c
}

func (s *Server) runAheadReport(svc *aheadService, base string, only string, force bool) *aheadReport {
	key := svc.Slug + "|" + base + "|" + only
	aheadCache.Lock()
	if aheadCache.m == nil {
		aheadCache.m = map[string]*aheadReport{}
	}
	if r, ok := aheadCache.m[key]; ok && !force && time.Since(r.At) < aheadCacheTTL {
		aheadCache.Unlock()
		cp := *r
		cp.Cached = true
		return &cp
	}
	aheadCache.Unlock()

	v, _, _ := s.sf.Do("ahead:"+key, func() (any, error) {
		host := base
		if u, err := url.Parse(base); err == nil {
			host = u.Host
		}
		aheadSlots.global <- struct{}{}
		defer func() { <-aheadSlots.global }()
		slot := aheadSlot(host)
		slot <- struct{}{}
		defer func() { <-slot }()

		t0 := time.Now()
		rep := &aheadReport{Service: svc.Slug, Base: base, At: t0}
		type job struct {
			i  int
			it aheadItem
		}
		var jobs []job
		for _, it := range aheadItems {
			if !itemAppliesTo(it, svc.Slug) {
				continue
			}
			if only != "" && !strings.EqualFold(only, it.ID) {
				continue
			}
			jobs = append(jobs, job{len(jobs), it})
		}
		rep.Results = make([]checkResult, len(jobs))
		// 3 probes in flight per upstream: gentle but not serial.
		sem := make(chan struct{}, 3)
		var wg sync.WaitGroup
		for _, j := range jobs {
			wg.Add(1)
			go func(j job) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				rep.Results[j.i] = evalCheck(base, j.it)
			}(j)
		}
		wg.Wait()
		for _, r := range rep.Results {
			switch r.Status {
			case "pass":
				rep.Pass++
			case "fail":
				rep.Fail++
				if r.Prio == "P1" && len(rep.NextSteps) < 3 {
					rep.NextSteps = append(rep.NextSteps, r.ID+": "+r.Detail+" → "+s.aheadURL()+"?service="+svc.Slug+"&item="+r.ID)
				}
			default:
				rep.Manual++
			}
		}
		rep.Score = fmt.Sprintf("%d/%d", rep.Pass, rep.Pass+rep.Fail)
		rep.TookMs = time.Since(t0).Milliseconds()
		aheadCache.Lock()
		aheadCache.m[key] = rep
		aheadCache.Unlock()
		return rep, nil
	})
	return v.(*aheadReport)
}

func (s *Server) aheadURL() string { return "https://siedler-oesterreich.exe.xyz:8000/llm/ahead" }

// allowedAheadBase restricts ?base= overrides to sibling hosts (SSRF guard).
func allowedAheadBase(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "", false
	}
	h := u.Hostname()
	if !(strings.HasSuffix(h, ".exe.xyz") || strings.HasSuffix(h, ".exe.dev")) {
		return "", false
	}
	return u.Scheme + "://" + u.Host, true
}

// ---- HTTP ----

// GET /llm/ahead            markdown roadmap (?service=, ?item=, ?format=json)
// GET /llm/ahead/check/{service}   run harness (?item=, ?base=, ?force=1, ?format=md)
// GET /llm/ahead/status     all services, cached matrix
func (s *Server) handleLLMAhead(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "public, max-age=300")
	svcF := r.URL.Query().Get("service")
	itemF := r.URL.Query().Get("item")
	unusedOnly := r.URL.Query().Get("unused") == "1" || r.URL.Query().Get("used") == "0"
	if r.URL.Query().Get("format") == "json" || strings.HasPrefix(r.Header.Get("Accept"), "application/json") {
		type jItem struct {
			ID       string   `json:"id"`
			Prio     string   `json:"prio"`
			Services []string `json:"services"`
			Title    string   `json:"title"`
			Spec     string   `json:"spec"`
			Why      string   `json:"why"`
			Check    string   `json:"check,omitempty"`
			Manual   bool     `json:"manual,omitempty"`
			Used     string   `json:"used_by_game,omitempty"`
		}
		out := struct {
			Version  string         `json:"version"`
			Services []aheadService `json:"services"`
			Fixture  map[string]any `json:"fixture"`
			Harness  string         `json:"harness"`
			Items    []jItem        `json:"items"`
		}{Version: "2026-09-25", Services: aheadServices, Harness: s.aheadURL() + "/check/{service}?item=&base=&force=1",
			Fixture: map[string]any{"kg_code": fxKG, "gemeinde_code": fxGem, "bbox": fxBBoxCSV, "no_data_kg": fxNoDataKG}}
		for _, it := range aheadItems {
			if svcF != "" && !itemAppliesTo(it, svcF) {
				continue
			}
			if itemF != "" && !strings.EqualFold(itemF, it.ID) {
				continue
			}
			if unusedOnly && aheadUsed[it.ID] != "" {
				continue
			}
			ji := jItem{ID: it.ID, Prio: it.Prio, Services: it.Services, Title: it.Title, Spec: it.Spec, Why: it.Why, Manual: it.Manual, Used: aheadUsed[it.ID]}
			if it.Check != nil {
				m := it.Check.Method
				if m == "" {
					m = "GET"
				}
				ji.Check = m + " {base}" + it.Check.Path
			}
			out.Items = append(out.Items, ji)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Write(renderAheadMarkdown(s.aheadURL(), svcF, itemF, unusedOnly))
}

func renderAheadMarkdown(self, svcF, itemF string, unusedOnly bool) []byte {
	var b strings.Builder
	if itemF == "" {
		fmt.Fprintf(&b, "# Siedler Österreich → sibling services: AHEAD list\n\n")
		fmt.Fprintf(&b, "Consumer: https://siedler-oesterreich.exe.xyz:8000 (browser game on live Austrian cadastre). Terse by design.\n")
		fmt.Fprintf(&b, "Machine form: %s?format=json   Filter: ?service=cadastre|srtm|holz|farm|gw  ?item=ID  ?unused=1 (only items the game does not consume yet — start here)\n", self)
		fmt.Fprintf(&b, "Checkbox = **used by the game** (where: 'used:' line), not 'implemented upstream' — that is what %s/check/<service> tells you.\n\n", self)
		fmt.Fprintf(&b, "## Protocol (for the implementing agent)\n")
		fmt.Fprintf(&b, "1. `curl -s %s/check/<service>` → JSON pass/fail per item, P1 first in next_steps.\n", self)
		fmt.Fprintf(&b, "2. Implement one item. Deploy. `curl -s '%s/check/<service>?item=<ID>&force=1'` (cached 60 s otherwise; `&base=https://<host>:<port>` for a public staging instance).\n", self)
		fmt.Fprintf(&b, "3. Green → next item by priority. Done → reply with the check URL; nothing else needed.\n")
		fmt.Fprintf(&b, "Rules: additive changes only; never break existing fields; 202/`ready:false` + `retry_after_s` when cold, never hang; CORS `*`; ETag; codes are 5-char strings.\n")
		fmt.Fprintf(&b, "Fixture used by every check: kg_code %s (Gaisfeld, Stmk; gemeinde %s), bbox %s, no-data kg %s. Warm these.\n", fxKG, fxGem, fxBBoxCSV, fxNoDataKG)
		fmt.Fprintf(&b, "Sibling spec reference: https://cadastre-process-api.exe.xyz/api/v1/docs/llm.txt?section=integration\n\n")
	}
	for _, svc := range aheadServices {
		if svcF != "" && svcF != svc.Slug {
			continue
		}
		any := false
		for _, it := range aheadItems {
			if !itemAppliesTo(it, svc.Slug) || (itemF != "" && !strings.EqualFold(itemF, it.ID)) {
				continue
			}
			used := aheadUsed[it.ID]
			if unusedOnly && used != "" {
				continue
			}
			mark := " "
			if used != "" {
				mark = "x"
			}
			if !any {
				fmt.Fprintf(&b, "## %s — %s (%s)\ncheck: %s/check/%s\n\n", svc.Slug, svc.Base, svc.Repo, self, svc.Slug)
				any = true
			}
			// shared items: print once in full under the first service, short ref afterwards
			if shared := it.Services[0] == "*" || len(it.Services) > 1; shared {
				if svc.Slug != firstServiceFor(it) {
					fmt.Fprintf(&b, "- [%s] **%s** %s %s → see under %s\n", mark, it.ID, it.Prio, it.Title, firstServiceFor(it))
					continue
				}
			}
			fmt.Fprintf(&b, "- [%s] **%s** %s — %s\n  - spec: %s\n  - why: %s\n", mark, it.ID, it.Prio, it.Title, it.Spec, it.Why)
			if used != "" {
				fmt.Fprintf(&b, "  - used: %s\n", used)
			}
			if it.Check != nil && !it.Manual {
				m := it.Check.Method
				if m == "" {
					m = "GET"
				}
				fmt.Fprintf(&b, "  - check: `%s %s`", m, it.Check.Path)
				if it.Check.MaxMs > 0 {
					fmt.Fprintf(&b, " ≤%d ms", it.Check.MaxMs)
				}
				if len(it.Check.Has) > 0 {
					fmt.Fprintf(&b, ", json has %s", strings.Join(it.Check.Has, ", "))
				}
				if len(it.Check.Status) > 0 {
					fmt.Fprintf(&b, ", status %v", it.Check.Status)
				}
				b.WriteString("\n")
			} else {
				b.WriteString("  - check: manual (report in reply)\n")
			}
		}
		b.WriteString("\n")
	}
	if itemF == "" {
		fmt.Fprintf(&b, "## Priority key\nP1 = unblocks a game feature now · P2 = removes a hack/latency · P3 = nice to have.\nOrder of work per service: all P1 (ALL-* first — they are ~50 lines each), then P2.\n")
	}
	return []byte(b.String())
}

func firstServiceFor(it aheadItem) string {
	if len(it.Services) == 1 && it.Services[0] == "*" {
		return aheadServices[0].Slug
	}
	best, bi := "", 1<<30
	for _, s := range it.Services {
		for i, svc := range aheadServices {
			if svc.Slug == s && i < bi {
				best, bi = s, i
			}
		}
	}
	return best
}

func (s *Server) handleLLMAheadCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "no-store")
	svc := aheadServiceByName(r.PathValue("service"))
	if svc == nil {
		http.Error(w, `{"error":"unknown service; use cadastre|srtm|holz|farm"}`, 404)
		return
	}
	base := svc.Base
	if bo := r.URL.Query().Get("base"); bo != "" {
		b, ok := allowedAheadBase(bo)
		if !ok {
			http.Error(w, `{"error":"base must be http(s)://*.exe.xyz[:port]"}`, 400)
			return
		}
		base = b
	}
	rep := s.runAheadReport(svc, base, r.URL.Query().Get("item"), r.URL.Query().Get("force") == "1")
	if r.URL.Query().Get("format") == "md" {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		fmt.Fprintf(w, "# %s @ %s — %s pass (%d manual) %s\n\n", rep.Service, rep.Base, rep.Score, rep.Manual, map[bool]string{true: "(cached)", false: ""}[rep.Cached])
		for _, c := range rep.Results {
			mark := map[string]string{"pass": "x", "fail": " ", "manual": "?"}[c.Status]
			fmt.Fprintf(w, "- [%s] %s %s", mark, c.ID, c.Prio)
			if c.Status == "fail" {
				fmt.Fprintf(w, " — %s", c.Detail)
			}
			if c.Ms > 0 {
				fmt.Fprintf(w, " (%d ms)", c.Ms)
			}
			fmt.Fprintln(w)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rep)
}

func (s *Server) handleLLMAheadStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "no-store")
	force := r.URL.Query().Get("force") == "1"
	type row struct {
		Service string            `json:"service"`
		Score   string            `json:"score"`
		At      time.Time         `json:"at"`
		Items   map[string]string `json:"items"`
	}
	out := make([]row, 0, len(aheadServices))
	var wg sync.WaitGroup
	var mu sync.Mutex
	for i := range aheadServices {
		wg.Add(1)
		go func(svc *aheadService) {
			defer wg.Done()
			rep := s.runAheadReport(svc, svc.Base, "", force)
			m := map[string]string{}
			for _, c := range rep.Results {
				m[c.ID] = c.Status
			}
			mu.Lock()
			out = append(out, row{svc.Slug, rep.Score, rep.At, m})
			mu.Unlock()
		}(&aheadServices[i])
	}
	wg.Wait()
	sort.Slice(out, func(i, j int) bool { return out[i].Service < out[j].Service })
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"at": time.Now().UTC(), "services": out, "roadmap": s.aheadURL()})
}
