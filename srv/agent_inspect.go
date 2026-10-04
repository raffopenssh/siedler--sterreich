package srv

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"srv.exe.dev/db/dbgen"
)

// GET /api/agent/inspect?session_id=&player_id=&parcel_id=[&lon=&lat=]
//
// The "Grundbuch-Dossier" of one parcel: everything the data services know
// about it, gathered in parallel under one time budget. Since the Oct-2026
// provider migration (docs/migration-2026-10.md) nothing is keyed by parcel
// id upstream any more:
//
//   - the parcel row comes from the cadastre cell we already hold (cellstore:
//     lookupParcel → bevdirect-serve, live from the BEV vector tiles);
//   - context (OSM proximity, Natura 2000, land price, toponyms, legal) is one
//     umfeld /context call for the parcel's point + our own area figures;
//   - terrain / trees / buildings come from the srtm public tier by bbox
//     (/landscape, /buildings/bbox) plus the per-parcel heightfield enrichment
//     that already rides in the viewport cell;
//   - water is gw /llm/point + stations in the parcel bbox, forest the cached
//     timber estimate, chronik the KG dossier, similar the vicinity search.
//
// Blocks that no longer exist upstream (per-parcel Hansen forest loss, legal
// references per parcel, folio addresses, LiDAR auto_class) are listed in
// `missing`; forest loss is reported at KG resolution instead. Blocks that
// miss the budget come back as {"pending":true}; every fetch is cached, so
// calling again a few seconds later fills them in.
//
// lon/lat are optional: without them we use the position from the agent's
// last look (agentSeen) or scan the KG's cached cells. No database lookup is
// keyed by parcel id — ownership is resolved from the session's claim list.
const inspectBudget = 4500 * time.Millisecond

type inspectBlock struct {
	name string
	fn   func() any
}

func (s *Server) handleAgentInspect(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	qs := r.URL.Query()
	pid := strings.TrimSpace(qs.Get("parcel_id"))
	if pid == "" || len(pid) > 32 || !strings.Contains(pid, "-") {
		jsonErr(w, "parcel_id required (e.g. 63335-391 — take it from a look)", 400)
		return
	}
	sess, err := s.Q.GetSession(ctx, qs.Get("session_id"))
	if err != nil {
		jsonErr(w, "session not found — create one with POST /api/session/create (see /llm/game)", 404)
		return
	}
	var me *dbgen.Player
	if p := qs.Get("player_id"); p != "" {
		if pl, ok := s.authPlayer(r, p); ok {
			me = &pl
		}
	}
	lon, _ := strconv.ParseFloat(qs.Get("lon"), 64)
	lat, _ := strconv.ParseFloat(qs.Get("lat"), 64)
	if (lon == 0 || lat == 0) && !validKG(kgCodeOf(pid)) {
		jsonErr(w, "unknown KG in parcel_id — pass lon&lat as well (both come from a look)", 400)
		return
	}

	// 1. Base row from the cadastre cell (never a per-parcel upstream index).
	bp, st, cs := s.inspectBaseParcel(pid, lon, lat)
	if bp == nil {
		if st == 202 || st == 502 || st == 503 {
			relayCellStatus(w, cs, map[string]any{"parcel_id": pid})
			return
		}
		jsonErr(w, "parcel not found in the cadastre cells we hold — take parcel_id (+ lon, lat) from a recent look", 404)
		return
	}
	lon, lat = bp.Lon, bp.Lat
	kg := bp.KG
	p := parcelFromRow(bevParcelRow(bp, lon, lat), sess)
	claims, _ := s.Q.GetSessionParcels(ctx, sess.ID)
	var myClaim *dbgen.ParcelClaim
	if c := sessionClaimFor(claims, pid); c != nil {
		myClaim = c
		if pl, err := s.Q.GetPlayerByID(ctx, c.PlayerID); err == nil {
			n := pl.Name
			p.Owner = &n
		}
		p.OwnerID, p.ConvertedTo, p.ClaimID = c.PlayerID, c.ConvertedTo, c.ID
		p.Price = int(c.PurchasePrice)
	}
	agentSeen.Store(pid, agentSeenEntry{p, time.Now().Add(2 * time.Hour)})
	fps := s.footprintsOf(bp)

	// 2. Everything else in parallel.
	var ctxBlock map[string]any // umfeld /context, shared by cadastre/market/toponyms
	var ctxOnce sync.Once
	contextOf := func() map[string]any {
		ctxOnce.Do(func() { ctxBlock = s.inspectContext(pid, bp, fps) })
		return ctxBlock
	}
	blocks := []inspectBlock{
		{"terrain", func() any { return s.inspectTerrain(bp) }},
		{"ez", func() any { return s.inspectEZ(claims, kg, p.Ez, lon, lat) }},
		{"buildings", func() any { return s.inspectBuildings(bp, fps) }},
		{"field", func() any { return s.inspectField(lon, lat) }},
		{"water", func() any { return s.inspectWater(bp) }},
		{"market", func() any { return inspectMarket(contextOf()) }},
		{"toponyms", func() any { return inspectToponyms(contextOf()) }},
		{"context", func() any { return contextOf() }},
		{"chronik", func() any { return s.inspectChronik(kg) }},
		{"similar", func() any {
			return s.inspectSimilar(ctx, pid, lon, lat, p.AreaSqm, p.BuildingCount, p.BuildingArea, p.Landuse)
		}},
	}
	if p.Landuse == "56" {
		blocks = append(blocks, inspectBlock{"forest", func() any { return s.inspectForest(kg, pid, p.AreaSqm, p.Landuse, lon, lat) }})
	}
	results := make([]any, len(blocks))
	var mu sync.Mutex
	done := make(chan int, len(blocks))
	for i, b := range blocks {
		go func(i int, b inspectBlock) {
			defer func() {
				if rec := recover(); rec != nil {
					mu.Lock()
					results[i] = map[string]any{"error": "internal"}
					mu.Unlock()
				}
				done <- i
			}()
			v := b.fn()
			mu.Lock()
			results[i] = v
			mu.Unlock()
		}(i, b)
	}
	deadline := time.After(inspectBudget)
	finished := 0
wait:
	for finished < len(blocks) {
		select {
		case <-done:
			finished++
		case <-deadline:
			break wait
		}
	}
	out := map[string]any{
		"parcel":  p,
		"session": map[string]any{"id": sess.ID, "municipality": sess.MunicipalityName, "invite_code": sess.InviteCode, "view_url": mapURL(sess.InviteCode, lon, lat, 18)},
	}
	pendingBlocks := []string{}
	mu.Lock()
	var cb map[string]any
	for i, b := range blocks {
		v := results[i]
		if v == nil {
			v = map[string]any{"pending": true}
			pendingBlocks = append(pendingBlocks, b.name)
		}
		if b.name == "context" {
			cb, _ = v.(map[string]any)
			continue // folded into cadastre below
		}
		out[b.name] = v
	}
	mu.Unlock()
	out["cadastre"] = inspectCadastreBlock(bp, fps, cb)
	out["missing"] = inspectMissing(bp, cb)
	out["game"] = s.inspectGame(ctx, sess, p, me, myClaim, out)
	if len(pendingBlocks) > 0 {
		out["pending"] = pendingBlocks
		out["retry_after_s"] = 3
	}
	out["text"] = inspectNarrate(p, out)
	out["notice"] = bevNotice
	out["attribution"] = agentAttribution
	jsonResp(w, out)
}

func jsonRespStatus(w http.ResponseWriter, data any, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}

// sessionClaimFor finds the claim for a parcel in the session's claim list
// (claims store parcelHash(pid), never the id — see parcelhash.go).
func sessionClaimFor(claims []dbgen.ParcelClaim, pid string) *dbgen.ParcelClaim {
	h := parcelHash(pid)
	for i := range claims {
		if claims[i].ParcelHash == h {
			return &claims[i]
		}
	}
	return nil
}

// inspectBaseParcel resolves the parcel from our cadastre cells. With lon/lat
// (query or last look) the cell under the point is built if needed; without,
// only cells we already hold for the KG are scanned.
func (s *Server) inspectBaseParcel(pid string, lon, lat float64) (*bevParcel, int, cellStatus) {
	if lon == 0 || lat == 0 {
		if v, ok := agentSeen.Load(pid); ok {
			e := v.(agentSeenEntry)
			lon, lat = e.p.Lon, e.p.Lat
		}
	}
	if p, ok := s.lookupParcel(pid, lon, lat); ok {
		return p, 200, cellStatus{Status: 200}
	}
	if lon == 0 || lat == 0 {
		return nil, 404, cellStatus{Status: 404}
	}
	if _, st, cs := s.ensureCellStatus(cellOf(lon, lat)); st != 200 {
		return nil, st, cs
	}
	if p, ok := s.lookupParcel(pid, lon, lat); ok {
		return p, 200, cellStatus{Status: 200}
	}
	return nil, 404, cellStatus{Status: 404}
}

// bevParcelRow renders a bevParcel in the row shape parcelFromRow expects
// (same as fetchAgentParcels in agent.go).
func bevParcelRow(p *bevParcel, lon, lat float64) map[string]any {
	row := map[string]any{
		"parcel_id": p.ParcelID, "kg_code": p.KG, "gnr": p.GNR, "ez": p.EZ, "area_sqm": p.AreaSqm,
		"building_count": p.BuildingCnt, "total_building_area_sqm": p.BuildingArea, "lon": p.Lon, "lat": p.Lat,
		"distance_m": distM(lon, lat, p.Lon, p.Lat), "dominant_ns": p.DominantNS, "landuse_areas": p.LanduseAreas,
	}
	if a := admin().KGs[p.KG]; a != nil {
		row["kg_name"] = a.Name
	}
	return row
}

// inspectContext: umfeld /context for the parcel point with our own area
// figures (same cache key as GET /api/parcel-context, 24 h). Returns the
// `data` object or nil / {pending:true}.
func (s *Server) inspectContext(pid string, p *bevParcel, fps []bevFootprint) map[string]any {
	vals := url.Values{}
	vals.Set("lon", strconv.FormatFloat(p.Lon, 'f', 6, 64))
	vals.Set("lat", strconv.FormatFloat(p.Lat, 'f', 6, 64))
	vals.Set("area_sqm", strconv.FormatFloat(math.Round(p.AreaSqm), 'f', 0, 64))
	vals.Set("building_count", strconv.Itoa(max(p.BuildingCnt, len(fps))))
	fpArea := p.BuildingArea
	if fpArea == 0 {
		for _, f := range fps {
			fpArea += f.AreaSqm
		}
	}
	if fpArea > 0 {
		vals.Set("footprint_area_sqm", strconv.FormatFloat(math.Round(fpArea), 'f', 0, 64))
	}
	if len(p.LanduseAreas) > 0 {
		parts := make([]string, 0, len(p.LanduseAreas))
		for code, a := range p.LanduseAreas {
			parts = append(parts, fmt.Sprintf("%s:%.0f", code, a))
		}
		sort.Strings(parts)
		vals.Set("landuse_areas", strings.Join(parts, ","))
	}
	if p.KG != "" {
		vals.Set("kg", p.KG)
	}
	b, st := s.llmGet("pctx:v1:"+parcelHash(pid), umfeldAPI+"/context?"+vals.Encode(), 24*time.Hour)
	if st == 202 {
		return map[string]any{"pending": true}
	}
	if st != 200 {
		return nil
	}
	var d struct {
		Data map[string]any `json:"data"`
	}
	if json.Unmarshal(b, &d) != nil || d.Data == nil {
		return nil
	}
	return d.Data
}

// inspectCadastreBlock: the parcel's cadastre facts from the BEV cell plus the
// point-keyed context (OSM proximity, Natura 2000, legal refs of the KG).
func inspectCadastreBlock(p *bevParcel, fps []bevFootprint, cx map[string]any) map[string]any {
	out := map[string]any{
		"kg_code": p.KG, "gnr": p.GNR, "ez": p.EZ, "rstatus": p.RStatus,
		"dominant_ns": p.DominantNS, "landuse_areas": p.LanduseAreas, "landuse_summary": landuseSummaryOf(p),
		"footprint_count": len(fps), "complete": p.Complete, "parts": max(p.Parts, 1),
		"source": "BEV Katastralmappe vector tiles, assembled live by bevdirect-serve (cell cached ≤ 24 h)",
		"notice": bevNotice,
	}
	if a := admin().KGs[p.KG]; a != nil {
		out["kg_name"], out["gemeinde"], out["gemeinde_code"], out["district"], out["state"] = a.Name, a.GemName, a.Gemeinde, a.District, a.State
	}
	if cx == nil {
		out["context"] = map[string]any{"available": false}
		return out
	}
	if cx["pending"] == true {
		out["context"] = map[string]any{"pending": true}
		return out
	}
	if m := sub(cx, "municipality"); m != nil {
		if out["gemeinde"] == nil {
			out["gemeinde"], out["district"], out["state"] = str(m, "name"), str(m, "district_name"), str(m, "state")
		}
	}
	if osm := sub(cx, "osm"); osm != nil {
		out["osm"] = inspectOSM(osm)
	}
	if n2k := sub(cx, "natura2000"); n2k != nil {
		out["in_natura2000"] = n2k["in_natura2000"]
		if sites := arr(n2k, "sites"); len(sites) > 0 {
			out["natura2000_sites"] = sites
		}
	}
	if pa := sub(cx, "protected_area"); pa != nil {
		out["in_protected_area"] = pa["in_protected_area"]
		if sites := arr(pa, "sites"); len(sites) > 0 {
			out["protected_area_sites"] = sites
		}
	}
	if lg := sub(cx, "legal"); lg != nil {
		l := map[string]any{"scope": "kg", "total_refs": int(toFloat(lg["total_refs"])), "detail": "GET /api/cadastre/legal/kg/" + p.KG}
		if refs := arr(lg, "refs"); len(refs) > 0 {
			if len(refs) > 8 {
				refs = refs[:8]
			}
			l["refs"] = refs
		}
		out["legal_refs"] = l
	}
	return out
}

// landuseSummaryOf: the old `landuse_summary` shape ({code: m²}) kept for
// clients that read it — identical to landuse_areas now.
func landuseSummaryOf(p *bevParcel) map[string]float64 {
	if len(p.LanduseAreas) > 0 {
		return p.LanduseAreas
	}
	if p.DominantNS != "" {
		return map[string]float64{p.DominantNS: math.Round(p.AreaSqm)}
	}
	return map[string]float64{}
}

// inspectMissing documents what the public tiers no longer provide per parcel.
func inspectMissing(p *bevParcel, cx map[string]any) map[string]string {
	m := map[string]string{
		"terrain.forest_loss": "Hansen forest loss is reported at KG resolution (srtm /landscape), not per parcel",
		"cadastre.legal_refs": "RIS legal references are per KG (umfeld /legal/kg), not per parcel",
		"ez.addresses":        "no address register on the public tier; folio = parcels within the fetched BEV tiles only",
		"terrain.auto_class":  "no LiDAR per-parcel landscape label; dom_terrain / cover fractions come from the 25 m heightfield",
	}
	if p.Elev == nil {
		m["terrain.per_parcel"] = "no 25 m heightfield for this KG yet — elevation/slope/cover are bbox or KG aggregates"
	}
	if cx != nil {
		if mm := sub(cx, "missing"); mm != nil {
			for k, v := range mm {
				m["context."+k] = fmt.Sprint(v)
			}
		}
	}
	return m
}

func inspectOSM(osm map[string]any) map[string]any {
	keep := []string{"dist_road_m", "road_name", "road_fclass", "road_on_parcel", "dist_major_road_m", "major_road_ref", "major_road_name",
		"dist_rail_m", "dist_transit_m", "transit_name", "dist_train_station_m", "train_station_name",
		"dist_water_m", "water_name", "water_fclass", "dist_settlement_m", "settlement_name", "remoteness"}
	out := map[string]any{}
	for _, k := range keep {
		if v, ok := osm[k]; ok && v != nil {
			out[k] = v
		}
	}
	return out
}

// inspectTerrain: per-parcel heightfield enrichment from the cell (elevation,
// slope, aspect, cover fractions) + srtm /landscape over the parcel bbox
// (terrain class, tallest trees, KG-level Hansen loss).
func (s *Server) inspectTerrain(p *bevParcel) any {
	out := map[string]any{"available": false}
	if p.NE != nil {
		// Observed layer (srtm NE cells, v2.4): the declared cadastre next to
		// what LiDAR + satellite actually see on this parcel.
		out["observed"] = p.NE
		out["observed_note"] = "srtm-lidar-at NE cells (H3 res 12, ~307 m² each): cover shares, canopy, LiDAR heights, every tree apex ≥ 3 m inside the polygon (species/vitality), segmented structures, NDVI/phenology, surface change, and `verdict` = observation vs. declared land use (consistent | forest_loss | forest_gain | sealed_new | structure_new | green_new | unknown). " + neAttribution
	}
	if p.Elev != nil {
		out["available"] = true
		out["resolution"] = "parcel (25 m heightfield)"
		if p.NE != nil {
			out["resolution"] = fmt.Sprintf("parcel (%d NE cells, H3 res 12)", p.NE.Cells)
		}
		out["elevation_m"] = r1(*p.Elev)
		if p.ElevMin != nil {
			out["elevation_min_m"] = r1(*p.ElevMin)
		}
		if p.ElevMax != nil {
			out["elevation_max_m"] = r1(*p.ElevMax)
		}
		if p.Slope != nil {
			out["slope_deg"] = r1(*p.Slope)
		}
		out["aspect"] = p.Aspect
		out["dominant_cover"] = p.DomTerr
		if len(p.Fracs) > 0 {
			cover := map[string]float64{}
			for k, v := range p.Fracs {
				if v >= 0.01 {
					cover[k] = r2(v)
				}
			}
			out["cover_fractions"] = cover
		}
		if p.TreeFrac != nil {
			out["forested_fraction"] = r2(*p.TreeFrac)
		} else if len(p.Fracs) > 0 {
			out["forested_fraction"] = r2(p.Fracs["tree"])
		}
	}
	bb := parcelBBox(p)
	key := fmt.Sprintf("ls:v1:%.5f,%.5f,%.5f,%.5f:trees,landmarks", bb.W, bb.S, bb.E, bb.N)
	b, st := s.landscapeFor(bb, "trees,landmarks", key)
	if st == 202 {
		if out["available"] == false {
			return map[string]any{"pending": true}
		}
		out["landscape_pending"] = true
		return out
	}
	if st != 200 {
		if out["available"] == false {
			out["note"] = "no landscape product for this KG yet"
		}
		return out
	}
	var d map[string]any
	if json.Unmarshal(b, &d) != nil {
		return out
	}
	res := str(d, "resolution")
	if t := sub(d, "terrain"); t != nil {
		out["available"] = true
		if _, ok := out["resolution"]; !ok {
			out["resolution"] = res + " (bbox aggregate — " + str(t, "source") + ")"
		}
		out["terrain_class"] = str(t, "terrain_class")
		if _, ok := out["elevation_m"]; !ok {
			out["elevation_m"] = r1(numOr(t, "elevation_mean_m", 0))
			out["elevation_min_m"] = r1(numOr(t, "elevation_min_m", 0))
			out["elevation_max_m"] = r1(numOr(t, "elevation_max_m", 0))
			out["slope_deg"] = r1(numOr(t, "slope_mean_deg", 0))
			out["aspect"] = str(t, "aspect")
		}
	}
	if lc := sub(d, "landcover"); lc != nil {
		if _, ok := out["dominant_cover"]; !ok || out["dominant_cover"] == "" {
			out["dominant_cover"] = str(lc, "dom_terrain")
		}
		if _, ok := out["cover_fractions"]; !ok {
			if fr := sub(lc, "fractions"); fr != nil {
				cover := map[string]float64{}
				for k, v := range fr {
					if f := toFloat(v); f >= 0.01 {
						cover[k] = r2(f)
					}
				}
				out["cover_fractions"] = cover
				out["cover_resolution"] = str(lc, "resolution")
				if _, ok := out["forested_fraction"]; !ok {
					out["forested_fraction"] = cover["tree"]
				}
			}
		}
	}
	trees := []map[string]any{}
	if tr := sub(d, "trees"); tr != nil {
		for _, x := range arr(tr, "tallest") {
			m, ok := x.(map[string]any)
			if !ok {
				continue
			}
			lon, lat := toFloat(m["lon"]), toFloat(m["lat"])
			if len(p.Geometry) > 0 && !pointInGeom(lon, lat, p.Geometry) {
				continue // bbox corner, not on the parcel
			}
			t := map[string]any{"height_m": r1(toFloat(m["height_m"])), "lon": lon, "lat": lat}
			if c := toFloat(m["crown_d_m"]); c > 0 {
				t["crown_d_m"] = r1(c)
			}
			trees = append(trees, t)
		}
		sort.Slice(trees, func(i, j int) bool { return toFloat(trees[i]["height_m"]) > toFloat(trees[j]["height_m"]) })
		if len(trees) > 5 {
			trees = trees[:5]
		}
		if hm, ok := num(tr, "height_max_m"); ok {
			out["canopy_max_m"] = r1(hm)
		}
		if hmean, ok := num(tr, "height_mean_m"); ok {
			out["tree_height_m"] = map[string]any{"mean": r1(hmean), "max": r1(numOr(tr, "height_max_m", 0))}
		}
	}
	out["tallest_trees"] = trees
	if len(trees) > 0 && toFloat(trees[0]["height_m"]) >= 25 {
		out["giant_tree_bonus"] = "claiming this parcel awards bonus XP for trees ≥ 25 m (capped +300)"
	}
	if lm := arr(d, "landmarks"); len(lm) > 0 {
		if len(lm) > 5 {
			lm = lm[:5]
		}
		out["landmarks"] = lm
	}
	if fl := sub(d, "forest_loss"); fl != nil {
		h := map[string]any{"resolution": str(fl, "resolution"), "unit": str(fl, "unit"), "source": "Hansen GFC 2001–2024",
			"note": "KG-level sum, not this parcel"}
		if by := sub(fl, "by_year"); len(by) > 0 {
			years := map[string]float64{}
			tot, last, lastPx := 0.0, "", 0.0
			for y, v := range by {
				px := toFloat(v)
				years[y] = px
				tot += px
				if px > 0 && y > last {
					last, lastPx = y, px
				}
			}
			h["loss_by_year_px"] = years
			h["loss_total_px"] = tot
			h["last_loss_year"] = last
			h["last_loss_px"] = lastPx
		}
		if v, ok := num(fl, "since_2020_px"); ok {
			h["loss_since_2020_px"] = v
		}
		out["forest_loss"] = h
	}
	if out["available"] == false {
		out["note"] = "no landscape product for this KG yet"
	}
	return out
}

// inspectEZ: the land-register folio — sibling parcels within the fetched BEV
// tiles around the parcel (bevdirect /ez, 24 h), landuse breakdown, how much
// of it is already claimed in this session and the 20 %-off bulk price.
func (s *Server) inspectEZ(claims []dbgen.ParcelClaim, kg, ez string, lon, lat float64) any {
	if ez == "" {
		return map[string]any{"available": false}
	}
	key := fmt.Sprintf("ez:v1:%s:%s,%s", ezHash(kg, ez), q4(lon), q4(lat))
	u := fmt.Sprintf("%s/ez?kg=%s&ez=%s&lon=%.6f&lat=%.6f", bevAPI, kg, url.QueryEscape(ez), lon, lat)
	b, st := s.llmGetBudget(key, u, time.Duration(cadastreTTL), inspectBudget-500*time.Millisecond)
	if st == 202 {
		return map[string]any{"pending": true}
	}
	if st != 200 {
		return map[string]any{"available": false}
	}
	var d struct {
		Parcels []bevParcel `json:"parcels"`
		Partial bool        `json:"partial"`
		Note    string      `json:"note"`
	}
	if json.Unmarshal(b, &d) != nil {
		return map[string]any{"available": false}
	}
	claimed := map[string]bool{}
	for _, c := range claims {
		claimed[c.ParcelHash] = true
	}
	var total, freeArea float64
	ids := []string{}
	freeN, price := 0, 0
	byLU := map[string]float64{}
	for i := range d.Parcels {
		x := &d.Parcels[i]
		total += x.AreaSqm
		if len(ids) < 40 {
			ids = append(ids, x.ParcelID)
		}
		for code, a := range x.LanduseAreas {
			byLU[code] += a
		}
		if len(x.LanduseAreas) == 0 && x.DominantNS != "" {
			byLU[x.DominantNS] += x.AreaSqm
		}
		if !claimed[parcelHash(x.ParcelID)] {
			freeN++
			freeArea += x.AreaSqm
			lu := x.DominantNS
			if lu == "" {
				lu = "48"
			}
			price += calculatePrice(x.AreaSqm, lu, x.BuildingCnt, x.BuildingArea)
		}
	}
	out := map[string]any{
		"available": true, "ez": ez, "kg_code": kg,
		"parcel_count":    len(d.Parcels),
		"total_area_sqm":  math.Round(total),
		"unclaimed_count": freeN, "unclaimed_area_sqm": math.Round(freeArea),
		"bulk_price_coins": int(math.Round(float64(price) * 0.8)),
		"parcel_ids":       ids,
		"partial":          d.Partial,
		"scope":            "parcels of the folio within the BEV tiles fetched around this parcel; the folio may own land elsewhere",
	}
	lb := []map[string]any{}
	for code, a := range byLU {
		if total > 0 {
			lb = append(lb, map[string]any{"code": code, "name": landuseName(code), "share": r2(a / total), "area_sqm": math.Round(a)})
		}
	}
	sort.Slice(lb, func(i, j int) bool { return toFloat(lb[i]["area_sqm"]) > toFloat(lb[j]["area_sqm"]) })
	if len(lb) > 4 {
		lb = lb[:4]
	}
	out["landuse_breakdown"] = lb
	return out
}

// inspectBuildings: BEV footprints on the parcel (from the cell) joined with
// LiDAR-measured heights from srtm /buildings/bbox (nearest point ≤ 20 m).
func (s *Server) inspectBuildings(p *bevParcel, fps []bevFootprint) any {
	if len(fps) == 0 {
		return map[string]any{"count": 0, "footprint_area_sqm": 0}
	}
	bb := parcelBBox(p).pad(0.0003)
	key := fmt.Sprintf("bldg:v1:%.4f,%.4f,%.4f,%.4f", bb.W, bb.S, bb.E, bb.N)
	var lidar []map[string]any
	pending := false
	u := fmt.Sprintf("%s/buildings/bbox?bbox=%.5f,%.5f,%.5f,%.5f", lidarAPI, bb.W, bb.S, bb.E, bb.N)
	if lb, st := s.llmGet(key, u, 24*time.Hour); st == 200 {
		var r struct {
			Buildings []map[string]any `json:"buildings"`
		}
		json.Unmarshal(lb, &r)
		lidar = r.Buildings
	} else if st == 202 {
		pending = true
	}
	out := []map[string]any{}
	var area float64
	for _, f := range fps {
		fid := f.FootprintID
		if fid == "" {
			fid = f.ID
		}
		row := map[string]any{"footprint_id": fid, "area_sqm": math.Round(f.AreaSqm), "ns_code": f.NSCode, "size_class": sizeClass(f.AreaSqm)}
		if f.OBBLen > 0 {
			row["length_m"], row["width_m"], row["orientation_deg"] = r1(f.OBBLen), r1(f.OBBWid), r1(f.Orient)
			if f.OBBWid > 0 {
				row["shape_class"] = shapeClass(f.OBBLen / f.OBBWid)
			}
		}
		area += f.AreaSqm
		var best map[string]any
		bestD := 20.0
		for _, l := range lidar {
			if dd := distM(f.Lon, f.Lat, toFloat(l["lon"]), toFloat(l["lat"])); dd < bestD {
				best, bestD = l, dd
			}
		}
		if best != nil {
			mean := toFloat(best["height_mean_m"])
			row["lidar"] = map[string]any{
				"max_height_m": toFloat(best["height_max_m"]), "mean_height_m": mean,
				"stories_est": int(math.Max(1, math.Round(mean/2.9))),
				"roof_type":   str(best, "roof"), "ground_m": toFloat(best["ground_elev_m"]), "match_m": math.Round(bestD),
			}
		}
		out = append(out, row)
	}
	res := map[string]any{"count": len(out), "footprint_area_sqm": math.Round(area), "footprints": out}
	if pending {
		res["lidar_pending"] = true
	}
	return res
}

func sizeClass(a float64) string {
	switch {
	case a < 30:
		return "shed"
	case a < 120:
		return "small"
	case a < 300:
		return "medium"
	case a < 800:
		return "large"
	}
	return "very_large"
}

func shapeClass(ratio float64) string {
	switch {
	case ratio < 1.3:
		return "compact"
	case ratio < 2.2:
		return "rectangular"
	}
	return "elongated"
}

// inspectField: AMA INVEKOS Schlag under the centroid (real crop, organic
// flag) and the nearest farmstead.
func (s *Server) inspectField(lon, lat float64) any {
	const d = 0.002
	qs := fmt.Sprintf("west=%.4f&south=%.4f&east=%.4f&north=%.4f", lon-d, lat-d, lon+d, lat+d)
	key := q4(lon) + "," + q4(lat)
	out := map[string]any{}
	b, st := s.llmGet("agentschlag:"+key, farmAPI+"/api/schlaege?limit=300&"+qs, 24*time.Hour)
	if st == 202 {
		return map[string]any{"pending": true}
	}
	if st == 200 {
		var r struct {
			Fields []struct {
				SnarName  string          `json:"snar_name"`
				CropGroup string          `json:"crop_group"`
				AreaHa    float64         `json:"area_ha"`
				Organic   *bool           `json:"organic"`
				Year      any             `json:"year"`
				Geometry  json.RawMessage `json:"geometry"`
			} `json:"fields"`
			Year any `json:"year"`
		}
		json.Unmarshal(b, &r)
		for _, f := range r.Fields {
			if pointInGeom(lon, lat, f.Geometry) {
				out["schlag"] = map[string]any{"crop": f.SnarName, "crop_group": f.CropGroup, "area_ha": r2(f.AreaHa), "organic": f.Organic, "year": r.Year}
				if f.CropGroup != "" {
					out["field_kind"] = map[bool]string{true: "meadow (Förderung only)", false: "crop (harvest every 60 min)"}[cropMeadow[f.CropGroup]]
				}
				break
			}
		}
		if out["schlag"] == nil {
			out["schlag"] = nil
			out["note"] = "no INVEKOS field declared here (not farmed, or forest/built)"
		}
	} else {
		out["available"] = false
	}
	if hb, st := s.llmGet("agenthof:"+key, farmAPI+"/api/hofstellen?"+qs, 24*time.Hour); st == 200 {
		var r struct {
			Points []map[string]any `json:"points"`
		}
		json.Unmarshal(hb, &r)
		best, bestD := (map[string]any)(nil), 1e9
		for _, p := range r.Points {
			if dd := distM(lon, lat, toFloat(p["lon"]), toFloat(p["lat"])); dd < bestD {
				best, bestD = p, dd
			}
		}
		if best != nil && bestD < 400 {
			h := map[string]any{"distance_m": math.Round(bestD)}
			for _, k := range []string{"organic", "size_class"} {
				if v, ok := best[k]; ok && v != nil {
					h[k] = v
				}
			}
			out["nearest_farmstead"] = h
		}
	}
	return out
}

// inspectWater: groundwater index at the parcel point, gauges on the parcel
// (gw points in the parcel bbox, point-in-polygon), nearest station with
// level + trend, drinking-water protection zone, well quote.
func (s *Server) inspectWater(p *bevParcel) any {
	lon, lat := p.Lon, p.Lat
	b, st := s.waterPoint(lon, lat)
	if st == 202 {
		return map[string]any{"pending": true}
	}
	out := map[string]any{}
	if st == 200 {
		var d map[string]any
		if json.Unmarshal(b, &d) == nil {
			m := sub(d, "metrics")
			out["gwi"] = numOr(m, "gwi", 0)
			out["gwi_category"] = str(m, "gwi_category")
			out["aquifer"] = str(m, "aquifer_type")
			out["depth_to_groundwater_m_est"] = numOr(m, "depth_to_gw_m_est", 0)
			out["depth_confidence"] = str(m, "depth_confidence")
			out["nitrate_mg_l"] = numOr(m, "gwi_no3", 0)
			out["gw_trend_m_per_decade"] = numOr(m, "gwi_gw_trend", 0)
			out["as_of"] = str(d, "as_of")
			if st := sub(d, "nearest_gw_station"); st != nil {
				out["nearest_gw_station"] = map[string]any{"name": str(st, "name"), "distance_m": numOr(st, "distance_m", 0), "gw_level_m": numOr(st, "gw_level_m", 0), "trend_m_per_decade": numOr(st, "gw_trend_m_per_decade", 0), "detail": "GET /api/water/station/" + str(st, "id")}
			}
			if now := sub(d, "now"); now != nil {
				out["status_now"] = str(now, "status")
			}
		}
	} else {
		out["available"] = false
	}
	// gauges on the parcel: gw points within the parcel bbox, then PIP
	// (shared with the claim-time Pegelwart check, water.go).
	pts := s.gwPointsOnParcel(p)
	if pts == nil {
		pts = []map[string]any{}
	}
	out["stations_on_parcel"] = pts
	if len(pts) > 0 {
		out["pegelwart_bonus_xp"] = 80
	}
	if zt, zone := s.inWaterProtection(lon, lat); zt != "" {
		out["water_protection"] = map[string]any{"type": zt, "zone": zone, "bonus": "Naturschutz/Naturwald here pays ×1.5 XP"}
	} else {
		out["water_protection"] = nil
	}
	q := s.wellQuote(lon, lat)
	out["well"] = map[string]any{"depth_m": q["depth_m"], "price_coins": q["price"], "protection": q["protection"], "how": "POST /api/dig-well {session_id, player_id, parcel_id} on an owned field"}
	return out
}

// inspectMarket: umfeld land_price for the parcel point + our area figures —
// what this land would cost for real (Statistik Austria baselines).
func inspectMarket(cx map[string]any) any {
	if cx == nil {
		return map[string]any{"available": false}
	}
	if cx["pending"] == true {
		return map[string]any{"pending": true}
	}
	d := sub(cx, "land_price")
	if d == nil {
		return map[string]any{"available": false}
	}
	out := map[string]any{
		"available": true, "year": d["year"], "class": str(d, "class"),
		"buy_eur_per_sqm": r2(numOr(d, "buy_eur_per_sqm", 0)),
		"buy_total_eur":   math.Round(numOr(d, "buy_total_blended_eur", numOr(d, "buy_total_eur", 0))),
		"confidence":      str(d, "confidence"), "baseline_source": str(d, "baseline_source"),
	}
	if v, ok := num(d, "rent_eur_per_year"); ok {
		out["rent_eur_per_year"] = math.Round(v)
	}
	if bl := sub(d, "building"); bl != nil {
		out["building_value_eur"] = math.Round(numOr(bl, "value_eur", 0))
		if v, ok := num(d, "combined_value_eur"); ok {
			out["combined_value_eur"] = math.Round(v)
		}
	}
	if tr := sub(d, "trends"); tr != nil {
		out["trend_cagr_gemeinde"] = r2(numOr(tr, "buy_cagr_gemeinde", 0))
	}
	out["note"] = "model estimate from published Statistik Austria series for this point and our area figures — not an appraisal, no legal effect"
	return out
}

// inspectToponyms: official BEV place / field names around the parcel from
// the context bundle — the landscape's own vocabulary (Riednamen, farms,
// brooks, peaks).
func inspectToponyms(cx map[string]any) any {
	if cx == nil || cx["pending"] == true {
		return []any{}
	}
	tp := sub(cx, "toponyms")
	if tp == nil {
		return []any{}
	}
	out := []map[string]any{}
	for _, x := range arr(tp, "rows") {
		t, ok := x.(map[string]any)
		if !ok {
			continue
		}
		if len(out) == 8 {
			break
		}
		kind := str(t, "art")
		if kind == "" {
			kind = str(t, "f_name")
		}
		out = append(out, map[string]any{"name": str(t, "name"), "kind": kind, "layer": str(t, "layer"), "distance_m": math.Round(toFloat(t["distance_m"]))})
	}
	sort.Slice(out, func(i, j int) bool { return toFloat(out[i]["distance_m"]) < toFloat(out[j]["distance_m"]) })
	return out
}

// inspectChronik: the KG's Gemeinde-Chronik in two lines — drought now and
// the game rules derived from it.
func (s *Server) inspectChronik(kg string) any {
	ds, gw, farm := s.droughtForKG(kg)
	out := map[string]any{"kg_code": kg, "detail": "GET /api/dossier/" + kg}
	if ds.Known {
		out["drought"] = map[string]any{"level": ds.Level, "label": ds.Label, "status": ds.Status, "sigma": ds.Sigma}
		out["yield_factor"] = r2(droughtYieldFactor(ds.Level))
		out["well_protection"] = wellProtection(str(sub(gw, "metrics"), "gwi_category"), ds.Status)
	}
	if farm != nil {
		out["subsidy_coins_per_ha"] = r1(subsidyPerHa(farm))
	}
	return out
}

// inspectForest: the timber estimate (stand facts from the cell enrichment +
// srtm /landscape trees; same cache as GET /api/forest-value).
func (s *Server) inspectForest(kg, pid string, area float64, lu string, lon, lat float64) any {
	var e timberEstimate
	ck := "timber:" + parcelHash(pid)
	if cached, err := s.Q.GetCachedData(context.Background(), ck); err == nil && json.Unmarshal([]byte(cached), &e) == nil {
		// hit
	} else {
		v, _, _ := s.sf.Do(ck, func() (any, error) {
			est := s.estimateTimber(context.Background(), kg, pid, area, lu, true, lon, lat)
			if est.IsForest {
				b, _ := json.Marshal(est)
				ttl := 10 * time.Minute
				if est.Source == "v3" {
					ttl = time.Hour
				}
				s.Q.SetCachedData(context.Background(), dbgen.SetCachedDataParams{CacheKey: ck, Data: string(b), ExpiresAt: time.Now().Add(ttl)})
			}
			return est, nil
		})
		e = v.(timberEstimate)
	}
	if !e.IsForest {
		return map[string]any{"is_forest": false}
	}
	out := map[string]any{
		"is_forest": true, "source": e.Source, "canopy_frac": r2(e.CanopyFrac), "h_mean_m": r1(e.HMean), "h_max_m": r1(e.HMax),
		"standing_stock_vfm": math.Round(e.Vfm), "vfm_per_ha": math.Round(e.VfmPerHa), "harvestable_efm": math.Round(e.Efm),
		"co2_stored_t": math.Round(e.CO2t), "species_shares": e.Species,
		"timber_gross_eur": math.Round(e.GrossEur), "harvest_cost_eur": math.Round(e.CostEur), "timber_net_eur": math.Round(e.NetEur),
		"harvest_coins": e.Coins, "naturwald_xp": e.WildXP,
		"prices": e.Prices,
	}
	if e.NTrees > 0 {
		out["n_trees"] = e.NTrees
	}
	if e.History != nil {
		out["history"] = e.History
	}
	return out
}

// inspectGame: what the player can do here and what it pays.
func (s *Server) inspectGame(ctx context.Context, sess dbgen.GameSession, p agentParcel, me *dbgen.Player, claim *dbgen.ParcelClaim, blocks map[string]any) map[string]any {
	g := map[string]any{"price_coins": p.Price, "owner": p.Owner, "converted_to": p.ConvertedTo}
	acts := []map[string]any{}
	mine := me != nil && p.OwnerID == me.ID
	water, _ := blocks["water"].(map[string]any)
	inWP := water != nil && water["water_protection"] != nil
	bioXP := 100
	verdict := ""
	if t, ok := blocks["terrain"].(map[string]any); ok {
		if ne, ok := t["observed"].(*neParcel); ok && ne != nil {
			verdict = ne.Verdict
		}
	}
	if verdict == "forest_loss" {
		bioXP = 200 // Wiederbewaldung (NE verdict at claim time; here: the live verdict)
	}
	if inWP {
		bioXP = bioXP * 3 / 2
	}
	switch {
	case p.Owner == nil:
		a := map[string]any{"action": "claim", "call": "POST /api/agent/claim {session_id, player_id, parcel_id}", "cost_coins": p.Price}
		if me != nil {
			a["affordable"] = me.Coins >= int64(p.Price)
		}
		if t, ok := blocks["terrain"].(map[string]any); ok {
			if trees, ok := t["tallest_trees"].([]map[string]any); ok && len(trees) > 0 && toFloat(trees[0]["height_m"]) >= 25 {
				a["bonus"] = fmt.Sprintf("giant tree %.1f m → bonus XP on claim", toFloat(trees[0]["height_m"]))
			}
		}
		if water != nil {
			if pts, ok := water["stations_on_parcel"].([]map[string]any); ok && len(pts) > 0 {
				a["bonus_pegelwart_xp"] = 80
			}
		}
		if neDiscrepant(verdict) {
			a["bonus_spurenleser_xp"] = 60
			a["ne_verdict"] = verdict
		}
		acts = append(acts, a)
		if ez, ok := blocks["ez"].(map[string]any); ok && toFloat(ez["unclaimed_count"]) > 1 {
			acts = append(acts, map[string]any{"action": "claim_ez", "call": "POST /api/claim-ez {session_id, player_id, kg_code, ez, parcels:[…from a look]}", "parcels": ez["unclaimed_count"], "cost_coins": ez["bulk_price_coins"], "note": "whole folio (as far as fetched) at 20 % off"})
		}
	case mine && (p.ConvertedTo == nil || *p.ConvertedTo == ""):
		acts = append(acts, map[string]any{"action": "convert", "call": "POST /api/convert-parcel {…, convert_to:'biodiversity', lon, lat}", "xp": bioXP})
		if f, ok := blocks["forest"].(map[string]any); ok && f["is_forest"] == true {
			acts = append(acts, map[string]any{"action": "harvest_forest", "call": "POST /api/harvest-forest {session_id, player_id, parcel_id}", "coins": f["harvest_coins"], "note": "stand regrows over ~8.5 h real time"})
			acts = append(acts, map[string]any{"action": "wildforest", "call": "POST /api/convert-parcel {…, convert_to:'wildforest'}", "xp": f["naturwald_xp"], "note": "permanent; counts toward the 30 % target"})
		}
		if p.Landuse == "48" && claim != nil {
			crop, organic := "", false
			if fb, ok := blocks["field"].(map[string]any); ok {
				if sc, ok := fb["schlag"].(map[string]any); ok {
					crop = str(sc, "crop_group")
					if o, ok := sc["organic"].(*bool); ok && o != nil {
						organic = *o
					}
				}
			}
			base := int64(0)
			if fp := fieldPhaseAtCrop(p.ParcelID, time.Now(), crop); fp.Stage != "meadow" {
				base = harvestYield(claim.AreaSqm)
			}
			eco := s.harvestPayout(p.KgCode, *claim, base, organic)
			acts = append(acts, map[string]any{"action": "harvest", "call": "POST /api/harvest-parcel {session_id, player_id, parcel_id, crop_group, organic}", "economy": eco})
		}
		acts = append(acts, map[string]any{"action": "sell", "call": "POST /api/sell-parcel {session_id, player_id, claim_id}", "claim_id": p.ClaimID, "coins": p.Price * 6 / 10})
	}
	g["actions"] = acts
	if bio, err := s.Q.GetSessionBiodiversityPercent(ctx, sess.ID); err == nil {
		g["session_biodiversity_pct"] = bio
	}
	return g
}

func inspectNarrate(p agentParcel, o map[string]any) string {
	var b strings.Builder
	cad, _ := o["cadastre"].(map[string]any)
	fmt.Fprintf(&b, "Parcel %s (Gst. %s, EZ %s) in KG %s, %s: %.0f m² of %s. ", p.ParcelID, p.Gnr, p.Ez, p.KgName, str(cad, "gemeinde"), p.AreaSqm, p.LanduseName)
	if t, ok := o["terrain"].(map[string]any); ok && t["available"] == true {
		if strings.HasPrefix(str(t, "resolution"), "kg") {
			fmt.Fprintf(&b, "No 25 m terrain grid here yet — the KG averages %.0f m (%s, slope %.0f°, mostly %s)", toFloat(t["elevation_m"]), str(t, "terrain_class"), toFloat(t["slope_deg"]), str(t, "dominant_cover"))
		} else {
			fmt.Fprintf(&b, "It sits at %.0f m (%s, slope %.0f°, facing %s)", toFloat(t["elevation_m"]), str(t, "terrain_class"), toFloat(t["slope_deg"]), str(t, "aspect"))
			if dc := str(t, "dominant_cover"); dc != "" {
				fmt.Fprintf(&b, "; LiDAR sees mostly %s", dc)
			}
		}
		if ne, ok := t["observed"].(*neParcel); ok && ne != nil {
			if ne.TreeN > 0 {
				fmt.Fprintf(&b, "; %d tree crowns on the parcel (tallest %.0f m", ne.TreeN, ne.TreeHMaxM)
				if sp := topN(ne.Species, 1); len(sp) == 1 {
					for k := range sp {
						fmt.Fprintf(&b, ", mostly %s", k)
					}
				}
				b.WriteString(")")
			}
			if ne.StructN > 0 {
				fmt.Fprintf(&b, "; %d structures up to %.0f m", ne.StructN, ne.StructHMaxM)
			}
			switch ne.Verdict {
			case "forest_loss":
				fmt.Fprintf(&b, ". Observation vs cadastre: FOREST LOSS — the declared woodland is largely gone (Hansen %d)", ne.ForestLossYr)
			case "forest_gain":
				b.WriteString(". Observation vs cadastre: forest has grown over the declared use")
			case "sealed_new":
				b.WriteString(". Observation vs cadastre: newly sealed ground not in the declared use")
			case "structure_new":
				b.WriteString(". Observation vs cadastre: a structure stands here that the cadastre does not declare")
			case "green_new":
				b.WriteString(". Observation vs cadastre: declared built/sealed, observed green")
			}
		} else if trees, ok := t["tallest_trees"].([]map[string]any); ok && len(trees) > 0 {
			fmt.Fprintf(&b, ", tallest tree %.1f m", toFloat(trees[0]["height_m"]))
		}
		if fl, ok := t["forest_loss"].(map[string]any); ok && toFloat(fl["loss_since_2020_px"]) > 0 {
			fmt.Fprintf(&b, "; the KG lost forest most recently in %s (Hansen, KG level)", str(fl, "last_loss_year"))
		}
		b.WriteString(". ")
	}
	if f, ok := o["field"].(map[string]any); ok {
		if sc, ok := f["schlag"].(map[string]any); ok {
			fmt.Fprintf(&b, "The farmer declared it as %s (%.1f ha", str(sc, "crop"), toFloat(sc["area_ha"]))
			if org, ok := sc["organic"].(*bool); ok && org != nil && *org {
				b.WriteString(", organic")
			}
			b.WriteString("). ")
		}
	}
	if fo, ok := o["forest"].(map[string]any); ok && fo["is_forest"] == true {
		fmt.Fprintf(&b, "Standing timber ≈ %.0f Vfm (%.0f t CO₂), net value %.0f € → %v 🪙 harvest or %v XP as Naturwald. ", toFloat(fo["standing_stock_vfm"]), toFloat(fo["co2_stored_t"]), toFloat(fo["timber_net_eur"]), fo["harvest_coins"], fo["naturwald_xp"])
	}
	if bl, ok := o["buildings"].(map[string]any); ok && toFloat(bl["count"]) > 0 {
		fmt.Fprintf(&b, "%v building footprint(s), %.0f m²", bl["count"], toFloat(bl["footprint_area_sqm"]))
		if fps, ok := bl["footprints"].([]map[string]any); ok && len(fps) > 0 {
			if l, ok := fps[0]["lidar"].(map[string]any); ok {
				fmt.Fprintf(&b, " — the first one %.1f m tall, %s roof", toFloat(l["max_height_m"]), str(l, "roof_type"))
			}
		}
		b.WriteString(". ")
	}
	if osm := sub(cad, "osm"); osm != nil {
		road, water := str(osm, "road_name"), str(osm, "water_name")
		if road == "" {
			road = str(osm, "road_fclass") + " road"
		}
		if water == "" {
			water = "water"
		}
		fmt.Fprintf(&b, "Nearest road %s %.0f m, %s %.0f m, settlement %s %.0f m (remoteness %v/100). ", road, toFloat(osm["dist_road_m"]), water, toFloat(osm["dist_water_m"]), str(osm, "settlement_name"), toFloat(osm["dist_settlement_m"]), osm["remoteness"])
	}
	if cad != nil && cad["in_natura2000"] == true {
		b.WriteString("It lies inside a Natura 2000 site. ")
	}
	if w, ok := o["water"].(map[string]any); ok && w["gwi_category"] != nil {
		fmt.Fprintf(&b, "Groundwater index %v (%s), est. %.1f m deep, nitrate %.0f mg/l", w["gwi"], str(w, "gwi_category"), toFloat(w["depth_to_groundwater_m_est"]), toFloat(w["nitrate_mg_l"]))
		if w["water_protection"] != nil {
			b.WriteString(" — inside a drinking-water protection zone (Naturschutz ×1.5 XP)")
		}
		b.WriteString(". ")
	}
	if m, ok := o["market"].(map[string]any); ok && m["available"] == true {
		fmt.Fprintf(&b, "Real-world reference value ≈ %.0f € (%s); in the game it costs %d 🪙. ", toFloat(m["buy_total_eur"]), str(m, "class"), p.Price)
	}
	if tp, ok := o["toponyms"].([]map[string]any); ok && len(tp) > 0 {
		names := []string{}
		for i, t := range tp {
			if i == 3 {
				break
			}
			names = append(names, fmt.Sprintf("%s (%s, %.0f m)", str(t, "name"), str(t, "kind"), toFloat(t["distance_m"])))
		}
		fmt.Fprintf(&b, "Nearby names: %s. ", strings.Join(names, ", "))
	}
	if ez, ok := o["ez"].(map[string]any); ok && ez["available"] == true && toFloat(ez["parcel_count"]) > 1 {
		fmt.Fprintf(&b, "The folio EZ %s holds %v parcels nearby (%.1f ha), %v still unclaimed — %v 🪙 for the lot. ", p.Ez, ez["parcel_count"], toFloat(ez["total_area_sqm"])/1e4, ez["unclaimed_count"], ez["bulk_price_coins"])
	}
	if sim, ok := o["similar"].(map[string]any); ok && sim["available"] == true {
		if rows, ok := sim["results"].([]map[string]any); ok && len(rows) > 0 {
			fmt.Fprintf(&b, "Most similar parcel nearby: %s (%.0f m away, score %.2f). ", str(rows[0], "parcel_id"), toFloat(rows[0]["distance_m"]), toFloat(rows[0]["score"]))
		}
	}
	if pend, ok := o["pending"].([]string); ok && len(pend) > 0 {
		fmt.Fprintf(&b, "(Still loading: %s — ask again in a few seconds.)", strings.Join(pend, ", "))
	}
	return strings.TrimSpace(b.String())
}

// inspectSimilar: the vicinity similar-parcels search (same engine as
// GET /api/similar — cells we hold around the point, top 8) — "what else
// around here looks like this".
func (s *Server) inspectSimilar(ctx context.Context, pid string, lon, lat, area float64, bcount int, barea float64, lu string) any {
	if area <= 0 {
		return map[string]any{"available": false}
	}
	c, cancel := context.WithTimeout(ctx, inspectBudget-300*time.Millisecond)
	defer cancel()
	b, _, err := s.similarJSON(c, similarReq{pid: pid, lon: lon, lat: lat, area: area, lu: lu, bcount: bcount, barea: barea, radius: 3000, limit: 8})
	if err != nil {
		return map[string]any{"pending": true}
	}
	var d struct {
		Results    []map[string]any `json:"results"`
		Candidates int              `json:"candidates"`
		Cells      int              `json:"cells"`
		Scope      string           `json:"scope"`
		Lidar      bool             `json:"lidar_terms"`
		Source     string           `json:"source"`
		Radius     float64          `json:"radius_m"`
		Ref        map[string]any   `json:"ref"`
	}
	if json.Unmarshal(b, &d) != nil {
		return map[string]any{"available": false}
	}
	rows := make([]map[string]any, 0, len(d.Results))
	for _, r := range d.Results {
		row := map[string]any{"parcel_id": r["parcel_id"], "score": r["score"], "distance_m": r["distance_m"],
			"area_sqm": r["area_sqm"], "lon": r["lon"], "lat": r["lat"]}
		for _, k := range []string{"parts", "elev", "slope", "aspect", "forest_frac", "dom", "ez", "kg_code", "gnr", "building_count", "landuse_summary"} {
			if v, ok := r[k]; ok && v != nil && v != "" {
				row[k] = v
			}
		}
		rows = append(rows, row)
	}
	out := map[string]any{"available": true, "scope": d.Scope, "radius_m": d.Radius, "cells": d.Cells, "candidates": d.Candidates, "lidar_terms": d.Lidar,
		"source": d.Source, "results": rows,
		"note": "candidates = parcels in the cadastre cells we already hold around the point (explored / warm area, not all of Austria); score 0..1 = size ratio, landuse match, built density, terrain where the 25 m heightfield exists; GET /api/similar?parcel_id=&lon=&lat=&area=&lu=&radius=&limit= for more"}
	if d.Ref != nil {
		out["ref"] = d.Ref
	}
	return out
}
