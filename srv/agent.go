package srv

// /llm/game — the text-only edition of Siedler Österreich for LLM agents.
//
// Humans play on a canvas map; agents get the same world as compact JSON plus
// a short narration, act through the same game endpoints, and every response
// links to the live map so a human can watch (or join) the very same session.
//
// Promises we keep here (Impressum / Datenschutz):
//   - pseudonyms only, no personal data; agents wear a visible 🤖 prefix
//   - agents chat with quick phrases only (Datenschutz §7, minors may be present)
//   - no IP logging: rate limits are in-memory, see ratelimit.go
//   - every payload carries the data licences/attribution + the "no official
//     information, no owner data, no legal effect" disclaimer
//   - MERIT flow paths (CC BY-NC-SA) are not exposed to agents

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"srv.exe.dev/db/dbgen"
)

const agentPrefix = "🤖 "

var agentAttribution = map[string]any{
	// The BEV notice rides along in every agent payload (look / claim /
	// municipality / inspect) — cadastre-derived data must be shown with it.
	"notice":     bevNotice,
	"disclaimer": "Game data. Simplified and playfully altered; not an official register extract, no legal effect. Ownership in the game is fictional — real owner data is never processed.",
	"sources": []string{
		"Cadastre (parcels, footprints, land use): © BEV – Bundesamt für Eich- und Vermessungswesen, Katastralmappe vector tiles (kataster.bev.gv.at), CC BY 4.0, assembled live, cached ≤ 24 h (modified; BEV does not endorse this project)",
		"Administrative boundaries, terrain, tree/building heights: BEV VGD & ALS DTM/DSM, CC BY 4.0 (modified); Copernicus Sentinel (ESA); ESA WorldCover; Hansen/UMD GFC",
		"Municipalities & land-price model: Statistik Austria, CC BY 4.0 (model values)",
		"Roads, water, rail, transit stops, addresses: © OpenStreetMap contributors, ODbL 1.0",
		"Protected areas: Natura 2000 (EEA); WDPA (UNEP-WCMC & IUCN, non-commercial); species treasures: IUCN European Red List (European Commission / EEA)",
		"Field & place names: BEV DLM, CC BY 4.0; legal references: RIS, CC BY 4.0",
		"Fields (INVEKOS) and farmsteads: AMA / BML via data.gv.at, CC BY 4.0 (aggregated, hashed points); subsidy profiles: AMA transparency database, municipal aggregates",
		"Timber prices & forest history: Statistik Austria, LK-Holzmarktberichte, Hansen/GFW, Harris et al. — model values, not offers",
		"Groundwater, gauges, nitrate, water protection: BML eHYD, Wasserschatz 2021, WISE / EEA, CC BY 4.0; drought: Copernicus EDO; flow paths: MERIT Hydro, CC BY-NC-SA 4.0 (display only)",
		"National border: geoBoundaries gbOpen, CC BY-SA 4.0",
	},
	"licenses": siteURL + "/licenses",
	"more":     siteURL + "/impressum",
}

// agentParcel is the text-edition view of one cadastre parcel.
type agentParcel struct {
	ParcelID      string  `json:"parcel_id"`
	KgCode        string  `json:"kg_code"`
	KgName        string  `json:"kg_name,omitempty"`
	Gnr           string  `json:"gnr"`
	Ez            string  `json:"ez,omitempty"`
	AreaSqm       float64 `json:"area_sqm"`
	Landuse       string  `json:"landuse"`
	LanduseName   string  `json:"landuse_name"`
	BuildingCount int     `json:"building_count"`
	BuildingArea  float64 `json:"building_area_sqm"`
	Lon           float64 `json:"lon"`
	Lat           float64 `json:"lat"`
	DistanceM     float64 `json:"distance_m"`
	Price         int     `json:"price"`
	Owner         *string `json:"owner"`
	OwnerID       string  `json:"owner_id,omitempty"`
	ConvertedTo   *string `json:"converted_to,omitempty"`
	ClaimID       int64   `json:"claim_id,omitempty"`
	MapURL        string  `json:"map_url"`
}

// agentSeen remembers parcels an agent has looked at (2 h) so /api/agent/claim
// can price them from our data instead of trusting the caller.
var agentSeen sync.Map // parcel_id → agentSeenEntry

type agentSeenEntry struct {
	p   agentParcel
	exp time.Time
}

func mapURL(code string, lon, lat float64, zoom float64) string {
	return fmt.Sprintf("%s/join/%s#v=%.5f,%.5f,%.1f", siteURL, code, lon, lat, zoom)
}

// nsNames: BEV Nutzungssymbole (V2.9 Tab. 8). Mirrors NS_TABLE in game.js.
var nsNames = map[string]string{
	"40": "Dauerkulturen", "41": "Gebäude", "42": "Parkplätze", "48": "Äcker, Wiesen oder Weiden",
	"52": "Gärten", "53": "Weingärten", "54": "Alpen", "55": "Krummholzflächen", "56": "Wälder",
	"57": "Verbuschte Flächen", "58": "Forststraßen", "59": "Fließende Gewässer", "60": "Stehende Gewässer",
	"61": "Feuchtgebiete", "62": "Vegetationsarme Flächen", "63": "Betriebsflächen", "64": "Gewässerrandflächen",
	"65": "Verkehrsrandflächen", "72": "Friedhöfe", "83": "Gebäudenebenflächen", "84": "Abbauflächen, Halden, Deponien",
	"87": "Fels- und Geröllflächen", "88": "Gletscher", "92": "Schienenverkehrsanlagen", "95": "Straßenverkehrsanlagen",
	"96": "Freizeitflächen",
}

// dominantNS: weighted mode of the NS symbol list ("41,56,41"). NS entries are
// symbol counts, not areas — a forest with two building glyphs is still a
// forest — so traffic (0.25) and building (0.5) symbols are down-weighted,
// mirroring nsWeight() in game.js.
func dominantNS(codes string) string {
	w := map[string]float64{}
	best, bestW := "", -1.0
	for _, c := range strings.Split(codes, ",") {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		wt := 1.0
		switch c {
		case "58", "65", "92", "95", "42":
			wt = 0.25
		case "41", "83":
			wt = 0.5
		}
		w[c] += wt
		if w[c] > bestW {
			best, bestW = c, w[c]
		}
	}
	return best
}

func landuseName(code string) string {
	if n, ok := nsNames[code]; ok {
		return n
	}
	if code == "" {
		return "unbekannte Nutzung"
	}
	return "NS " + code
}

// fetchAgentParcels: parcels around a point from the cadastre cells. The
// centre cell is assembled on demand (bevdirect, ≤ bevWait s), neighbouring
// cells are used when cached. Rows mirror the old /spatial/point shape
// (parcel_id, kg_code, kg_name, gnr, ez, area_sqm, building_count,
// total_building_area_sqm, lon, lat, distance_m, dominant_ns, landuse_areas).
func (s *Server) fetchAgentParcels(lon, lat float64, radius, limit int, landuse string) ([]map[string]any, cellStatus, error) {
	if _, st, cs := s.ensureCellStatus(cellOf(lon, lat)); st != 200 {
		if st == 202 {
			return nil, cs, fmt.Errorf("cadastre cell still assembling")
		}
		return nil, cs, fmt.Errorf("cadastre cell build failed: %d", st)
	}
	ps, _ := s.parcelsNear(lon, lat, float64(radius), 9)
	sort.Slice(ps, func(i, j int) bool {
		return distM(lon, lat, ps[i].Lon, ps[i].Lat) < distM(lon, lat, ps[j].Lon, ps[j].Lat)
	})
	adm := admin()
	rows := make([]map[string]any, 0, limit)
	for i := range ps {
		p := &ps[i]
		if !p.Complete {
			continue
		}
		if landuse != "" && p.DominantNS != landuse {
			continue
		}
		row := map[string]any{
			"parcel_id": p.ParcelID, "kg_code": p.KG, "gnr": p.GNR, "ez": p.EZ, "area_sqm": p.AreaSqm,
			"building_count": p.BuildingCnt, "total_building_area_sqm": p.BuildingArea, "lon": p.Lon, "lat": p.Lat,
			"distance_m": distM(lon, lat, p.Lon, p.Lat), "dominant_ns": p.DominantNS, "landuse_areas": p.LanduseAreas,
		}
		if a := adm.KGs[p.KG]; a != nil {
			row["kg_name"] = a.Name
		}
		if p.Elev != nil {
			row["elev_m"], row["slope_deg"], row["dom_terrain"] = *p.Elev, p.Slope, p.DomTerr
		}
		rows = append(rows, row)
		if len(rows) >= limit {
			break
		}
	}
	return rows, cellStatus{Status: 200}, nil
}

// relayCellStatus answers an agent endpoint when the cadastre cell under the
// point is not usable: 202 {status:"pending", retry_after_s, …} + Retry-After
// while bevdirect assembles it (never "no parcels"), the breaker's
// {status:"down", retry_after_s} body on 503, 502 otherwise. Mirrors what
// cachedFetch does for the browser proxies (upstream_pending.go, breaker.go).
func relayCellStatus(w http.ResponseWriter, cs cellStatus, extra map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	switch cs.Status {
	case 202:
		ra := cs.RetryAfter
		if ra <= 0 {
			ra = 3
		}
		w.Header().Set("Retry-After", strconv.Itoa(int(ra+0.999)))
		out := map[string]any{"status": "pending", "retry_after_s": ra, "note": "cadastre cell is being assembled from the BEV tiles — repeat the same request after retry_after_s; this is not \"no parcels here\""}
		for k, v := range extra {
			out[k] = v
		}
		w.WriteHeader(202)
		json.NewEncoder(w).Encode(out)
	case 503:
		ra := cs.RetryAfter
		if ra <= 0 {
			ra = 20
		}
		w.Header().Set("X-Upstream", "down")
		w.Header().Set("Retry-After", strconv.Itoa(int(ra+0.999)))
		w.WriteHeader(503)
		if len(cs.Body) > 0 {
			w.Write(cs.Body)
		} else {
			json.NewEncoder(w).Encode(map[string]any{"error": "cadastre service unavailable", "status": "down", "service": "cadastre", "retry_after_s": ra})
		}
	default:
		w.WriteHeader(502)
		json.NewEncoder(w).Encode(map[string]any{"error": "cadastre service error, retry shortly", "retry_after_s": 5})
	}
}

// strn: like str but also renders numeric JSON values (upstream sends some
// codes as numbers).
func strn(m map[string]any, k string) string {
	if v, ok := m[k].(float64); ok {
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return str(m, k)
}

func parcelFromRow(row map[string]any, sess dbgen.GameSession) agentParcel {
	p := agentParcel{
		ParcelID:      strn(row, "parcel_id"),
		KgCode:        strn(row, "kg_code"),
		KgName:        strn(row, "kg_name"),
		Gnr:           strn(row, "gnr"),
		Ez:            strn(row, "ez"),
		AreaSqm:       math.Round(toFloat(row["area_sqm"])),
		BuildingCount: int(toFloat(row["building_count"])),
		BuildingArea:  math.Round(toFloat(row["total_building_area_sqm"])),
		Lon:           toFloat(row["lon"]),
		Lat:           toFloat(row["lat"]),
		DistanceM:     math.Round(toFloat(row["distance_m"])),
	}
	if d := strn(row, "dominant_ns"); d != "" {
		p.Landuse = d
	} else {
		p.Landuse = dominantNS(strn(row, "landuse_codes"))
	}
	p.LanduseName = landuseName(p.Landuse)
	p.Price = calculatePrice(p.AreaSqm, p.Landuse, p.BuildingCount, p.BuildingArea)
	p.MapURL = mapURL(sess.InviteCode, p.Lon, p.Lat, 17.5)
	return p
}

func agentParam(r *http.Request, k string, def, lo, hi int) int {
	v, err := strconv.Atoi(r.URL.Query().Get(k))
	if err != nil {
		return def
	}
	return max(lo, min(hi, v))
}

// GET /api/agent/look?session_id=&player_id=&lon=&lat=&radius=300&limit=25&landuse=&unclaimed=1
func (s *Server) handleAgentLook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	qs := r.URL.Query()
	sess, err := s.Q.GetSession(ctx, qs.Get("session_id"))
	if err != nil {
		jsonErr(w, "session not found — create one with POST /api/session/create (see /llm/game)", 404)
		return
	}
	lon, lat := sess.CenterLon, sess.CenterLat
	if v, err := strconv.ParseFloat(qs.Get("lon"), 64); err == nil {
		lon = v
	}
	if v, err := strconv.ParseFloat(qs.Get("lat"), 64); err == nil {
		lat = v
	}
	if lon < 9 || lon > 17.5 || lat < 46 || lat > 49.2 {
		jsonErr(w, "lon/lat must be inside Austria", 400)
		return
	}
	radius := agentParam(r, "radius", 300, 50, 1500)
	limit := agentParam(r, "limit", 25, 1, 60)
	landuse := qs.Get("landuse")
	if landuse != "" {
		if _, ok := nsBasePrice[landuse]; !ok {
			jsonErr(w, "landuse must be a BEV NS code (41 buildings, 48 fields, 56 forest, …)", 400)
			return
		}
	}
	onlyFree := qs.Get("unclaimed") == "1"

	// Fetch a bit more than asked when filtering out claimed parcels.
	fetchN := limit
	if onlyFree {
		fetchN = min(200, limit*3)
	}
	rows, cs, err := s.fetchAgentParcels(lon, lat, radius, fetchN, landuse)
	if err != nil {
		relayCellStatus(w, cs, map[string]any{"center": map[string]any{"lon": lon, "lat": lat, "radius_m": radius}})
		return
	}

	claims, _ := s.Q.GetSessionParcels(ctx, sess.ID)
	players, _ := s.Q.GetSessionPlayers(ctx, sess.ID)
	pname := map[string]string{}
	for _, p := range players {
		pname[p.ID] = p.Name
	}
	claimBy := map[string]dbgen.ParcelClaim{}
	for _, c := range claims {
		claimBy[c.ParcelHash] = c
	}

	parcels := make([]agentParcel, 0, limit)
	now := time.Now()
	for _, row := range rows {
		p := parcelFromRow(row, sess)
		if p.ParcelID == "" {
			continue
		}
		if c, ok := claimBy[parcelHash(p.ParcelID)]; ok {
			if onlyFree {
				continue
			}
			n := pname[c.PlayerID]
			p.Owner, p.OwnerID, p.ConvertedTo, p.ClaimID = &n, c.PlayerID, c.ConvertedTo, c.ID
			p.Price = int(c.PurchasePrice)
		}
		agentSeen.Store(p.ParcelID, agentSeenEntry{p, now.Add(2 * time.Hour)})
		parcels = append(parcels, p)
		if len(parcels) >= limit {
			break
		}
	}
	sort.Slice(parcels, func(i, j int) bool { return parcels[i].DistanceM < parcels[j].DistanceM })
	sweepAgentSeen(now)

	// Treasures in range (unfound only).
	treasures := []agentTreasure{}
	if all, err := s.Q.GetSessionTreasures(ctx, sess.ID); err == nil {
		for _, t := range all {
			if t.FoundBy != nil || roamingGone(t, now) {
				continue
			}
			d := distM(lon, lat, t.Lon, t.Lat)
			if d <= float64(radius) {
				at := agentTreasure{t.ID, t.TreasureType, t.Value, t.SpeciesGerman, t.Lon, t.Lat, math.Round(d), 0}
				if t.TreasureType == "roaming" {
					at.MovesOnInS = int(math.Max(0, roamingLifetime.Seconds()-now.Sub(t.CreatedAt).Seconds()))
				}
				treasures = append(treasures, at)
			}
		}
		sort.Slice(treasures, func(i, j int) bool { return treasures[i].DistanceM < treasures[j].DistanceM })
	}

	// Player-specific context (quests, purse) when the caller identifies itself.
	var me map[string]any
	var quests []map[string]any
	if pid := qs.Get("player_id"); pid != "" {
		if pl, ok := s.authPlayer(r, pid); ok {
			owned, conv := 0, 0
			for _, c := range claims {
				if c.PlayerID == pid {
					owned++
					if c.ConvertedTo != nil && *c.ConvertedTo != "" {
						conv++
					}
				}
			}
			me = map[string]any{"id": pl.ID, "name": pl.Name, "coins": pl.Coins, "xp": pl.Xp, "level": pl.Level, "parcels": owned, "converted": conv}
			s.autoCompleteChallenges(ctx, sess.ID, pid)
			prog := s.questProgress(ctx, sess.ID, pid)
			if cs, err := s.Q.GetPlayerChallenges(ctx, dbgen.GetPlayerChallengesParams{SessionID: sess.ID, PlayerID: pid}); err == nil {
				for _, c := range cs {
					if c.Completed != 0 {
						continue
					}
					have, goal := questProgressFor(c.Title, prog)
					q := map[string]any{"id": c.ID, "title": c.Title, "progress": have, "goal": goal, "reward_coins": c.RewardCoins, "reward_xp": c.RewardXp}
					if c.Description != nil {
						q["description"] = *c.Description
					}
					quests = append(quests, q)
				}
			}
		}
	}

	// Local drought state of the nearest KG (6 h cached sibling data).
	var drought any
	kg := ""
	if len(parcels) > 0 {
		kg = parcels[0].KgCode
		if ds, _, _ := s.droughtForKG(kg); ds.Known {
			drought = map[string]any{"kg_code": kg, "level": ds.Level, "label": ds.Label, "status": ds.Status, "sigma": ds.Sigma, "yield_factor": r2(droughtYieldFactor(ds.Level))}
		}
	}

	others := []map[string]any{}
	for _, p := range players {
		others = append(others, map[string]any{"id": p.ID, "name": p.Name, "coins": p.Coins, "xp": p.Xp, "agent": p.Agent != ""})
	}

	bio, _ := s.Q.GetSessionBiodiversityPercent(ctx, sess.ID)

	out := map[string]any{
		"session":   map[string]any{"id": sess.ID, "municipality": sess.MunicipalityName, "invite_code": sess.InviteCode, "view_url": mapURL(sess.InviteCode, lon, lat, 16.5), "biodiversity_pct": bio},
		"center":    map[string]any{"lon": lon, "lat": lat, "radius_m": radius},
		"parcels":   parcels,
		"treasures": treasures,
		"players":   others,
		"drought":   drought,
		"quests":    quests,
		"me":        me,
		"text":      narrate(sess, lon, lat, radius, parcels, treasures, quests, me, drought),
		"actions": map[string]string{
			"inspect":  "GET /api/agent/inspect?session_id=&player_id=&parcel_id=  (full data dossier of one parcel: terrain, trees, crop, water, market, folio, payouts)",
			"claim":    "POST /api/agent/claim {session_id, player_id, parcel_id}",
			"convert":  "POST /api/convert-parcel {session_id, player_id, parcel_id, convert_to:'biodiversity'|'wildforest', lon, lat}",
			"sell":     "POST /api/sell-parcel {session_id, player_id, claim_id}",
			"treasure": "POST /api/claim-treasure {player_id, treasure_id}",
			"roam":     "POST /api/session/{id}/treasures/roam {player_id, lon, lat}  (no unfound treasure within 1.5 km? ask for passing wildlife; 1 per 2 min)",
			"giants":   "GET /api/giants-near?lon=&lat=  (nearest LiDAR giant trees ≥ 25 m — claiming their parcel pays bonus XP)",
			"chat":     "POST /api/session/{id}/chat {player_id, quick:1..N}  (quick phrases only for agents)",
			"look":     "GET /api/agent/look?session_id=&player_id=&lon=&lat=&radius=",
		},
		"attribution": agentAttribution,
	}
	jsonResp(w, out)
}

func sweepAgentSeen(now time.Time) {
	if now.Unix()%37 != 0 { // cheap amortised sweep
		return
	}
	agentSeen.Range(func(k, v any) bool {
		if v.(agentSeenEntry).exp.Before(now) {
			agentSeen.Delete(k)
		}
		return true
	})
}

// narrate renders the scene as a few lines of prose — the Herald speaking to
// a model. English on purpose: the agent reads it, the humans read the map.
type agentTreasure struct {
	ID        int64   `json:"id"`
	Type      string  `json:"type"`
	Value     int64   `json:"value"`
	Species   string  `json:"species,omitempty"`
	Lon       float64 `json:"lon"`
	Lat       float64 `json:"lat"`
	DistanceM float64 `json:"distance_m"`
	// roaming wildlife only: seconds until the animal moves on (then the id is gone)
	MovesOnInS int `json:"moves_on_in_s,omitempty"`
}

func narrate(sess dbgen.GameSession, lon, lat float64, radius int, ps []agentParcel, tl []agentTreasure, qs []map[string]any, me map[string]any, drought any) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You stand in %s at %.5f, %.5f looking %d m around you. ", sess.MunicipalityName, lon, lat, radius)
	if len(ps) == 0 {
		b.WriteString("No parcels here — move (change lon/lat) or widen the radius.")
		return b.String()
	}
	free, mine, theirs := 0, 0, 0
	byUse := map[string]int{}
	var cheapest *agentParcel
	for i := range ps {
		p := &ps[i]
		byUse[p.LanduseName]++
		switch {
		case p.Owner == nil:
			free++
			if cheapest == nil || p.Price < cheapest.Price {
				cheapest = p
			}
		case me != nil && p.OwnerID == me["id"]:
			mine++
		default:
			theirs++
		}
	}
	type kv struct {
		k string
		v int
	}
	uses := []kv{}
	for k, v := range byUse {
		uses = append(uses, kv{k, v})
	}
	sort.Slice(uses, func(i, j int) bool { return uses[i].v > uses[j].v })
	parts := []string{}
	for i, u := range uses {
		if i == 3 {
			break
		}
		parts = append(parts, fmt.Sprintf("%d× %s", u.v, u.k))
	}
	fmt.Fprintf(&b, "%d parcels in %s: %s. ", len(ps), ps[0].KgName, strings.Join(parts, ", "))
	fmt.Fprintf(&b, "%d are unclaimed, %d yours, %d owned by others. ", free, mine, theirs)
	if cheapest != nil {
		fmt.Fprintf(&b, "Cheapest free plot: %s (%s, %.0f m², %d 🪙, %.0f m away). ", cheapest.ParcelID, cheapest.LanduseName, cheapest.AreaSqm, cheapest.Price, cheapest.DistanceM)
	}
	if len(tl) > 0 {
		t := tl[0]
		what := t.Type
		if t.Species != "" {
			what = t.Species
		}
		if t.Type == "roaming" {
			fmt.Fprintf(&b, "A %s is passing through %.0f m away (%d 🪙 + XP, moves on in %d min) — id %d. ", what, t.DistanceM, t.Value, t.MovesOnInS/60, t.ID)
		} else {
			fmt.Fprintf(&b, "A treasure glints %.0f m away (%s, %d) — id %d. ", t.DistanceM, what, t.Value, t.ID)
		}
	}
	if d, ok := drought.(map[string]any); ok {
		fmt.Fprintf(&b, "Groundwater here: %v (yield ×%v). ", d["label"], d["yield_factor"])
	}
	if me != nil {
		fmt.Fprintf(&b, "You have %v 🪙 and %v XP. ", me["coins"], me["xp"])
	}
	if len(qs) > 0 {
		q := qs[0]
		fmt.Fprintf(&b, "Open quest: %s (%v/%v).", q["title"], q["progress"], q["goal"])
	}
	return b.String()
}

// POST /api/agent/claim {session_id, player_id, parcel_id}
// The parcel must have appeared in a recent look (we price it from our copy).
func (s *Server) handleAgentClaim(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID string `json:"session_id"`
		PlayerID  string `json:"player_id"`
		ParcelID  string `json:"parcel_id"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonErr(w, "invalid request", 400)
		return
	}
	v, ok := agentSeen.Load(req.ParcelID)
	if !ok || v.(agentSeenEntry).exp.Before(time.Now()) {
		jsonErr(w, "unknown parcel — it must appear in a GET /api/agent/look response first", 409)
		return
	}
	p := v.(agentSeenEntry).p
	// GwStation: ask claimParcel to run the gauge check (gw /llm/points in
	// the parcel bbox + PIP, ≤ 1.5 s) — awards Pegelwart like the browser does.
	s.claimParcel(w, r, claimReq{
		SessionID: req.SessionID, PlayerID: req.PlayerID, ParcelID: p.ParcelID,
		KgCode: p.KgCode, Gnr: p.Gnr, Ez: p.Ez, AreaSqm: p.AreaSqm, Landuse: p.Landuse,
		BuildingCount: p.BuildingCount, TotalBuildingArea: p.BuildingArea, GwStation: true,
		Lon: p.Lon, Lat: p.Lat,
	})
}

// GET /api/agent/municipality?q=Name | ?random=1 — exactly what
// POST /api/session/create needs, plus whether LiDAR "enhanced" data exists.
func (s *Server) handleAgentMunicipality(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	random := r.URL.Query().Get("random") == "1"
	if q == "" && !random {
		jsonErr(w, "q= (name) or random=1 required", 400)
		return
	}
	url := cadastreAPI + "/search/municipalities?limit=8&format=json&q=" + urlQueryEscape(q)
	key := "agentmuni:" + strings.ToLower(q)
	if random {
		url = cadastreAPI + "/search/municipalities?list=all&limit=5000&format=json"
		key = "agentmuni:*all*"
	}
	b, st := s.llmGet(key, url, 24*time.Hour)
	if st != 200 {
		jsonErr(w, "municipality search unavailable", 502)
		return
	}
	var res struct {
		Data []map[string]any `json:"data"`
	}
	if json.Unmarshal(b, &res) != nil || len(res.Data) == 0 {
		jsonErr(w, "no municipality matches", 404)
		return
	}
	rows := res.Data
	if random {
		m := res.Data[rand.Intn(len(res.Data))]
		rows = []map[string]any{m}
		go s.settlementCenter(strn(m, "name"), toFloat(m["lon"]), toFloat(m["lat"]))
	} else {
		s.prememoSettlements(b) // session create for one of these stays < 1 s
	}
	out := []map[string]any{}
	for _, m := range rows {
		out = append(out, map[string]any{
			"municipality_code": strn(m, "gemeinde_code"),
			"municipality_name": strn(m, "name"),
			"center_lon":        toFloat(m["lon"]),
			"center_lat":        toFloat(m["lat"]),
			"state":             strn(m, "state"),
			"district":          strn(m, "district_name"),
		})
	}
	jsonResp(w, map[string]any{"municipalities": out, "next": "POST /api/session/create with these four fields + player_id and a session name", "attribution": agentAttribution})
}

func urlQueryEscape(s string) string {
	return strings.NewReplacer(" ", "+", "&", "%26", "#", "%23", "?", "%3F").Replace(s)
}

// ---- /llm/game and /llms.txt ----

func (s *Server) handleLLMsTxt(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	fmt.Fprintf(w, `# Siedler Österreich
> Browser game over real Austrian open data: millions of cadastral parcels with LiDAR terrain and tree heights, INVEKOS crops, groundwater gauges, timber prices, forest-loss history and official place names. Claim parcels, turn land into nature reserves.

## For agents
- [How to play as an agent](%s/llm/game): text-only edition, register → session → look → inspect → act. Includes the data catalogue (what one parcel dossier contains), rate limits + data licences.
- [Agent leaderboard](%s/agents): per-model ranking by hectares protected.
- [OpenAPI 3.1](%s/openapi.json): the agent-facing endpoints for tool generators.
- [MCP server](https://github.com/raffopenssh/siedler--sterreich/tree/main/tools/mcp-server): npx siedler-oesterreich-mcp — look/claim/convert as MCP tools.

## Legal
- [Data sources & licences](%s/licenses): every provider, licence, attribution text and our cache age (machine-readable: %s/api/licenses)
- [Impressum](%s/impressum)
- [Datenschutz](%s/datenschutz)
`, siteURL, siteURL, siteURL, siteURL, siteURL, siteURL, siteURL)
}

const llmGameMD = `# Siedler Österreich — the text edition for agents

Base URL: %s  ·  players so far: %d (🤖 %d)  ·  sessions: %d

You are about to play a land game on **real Austrian open data**. Nothing on
the map is generated: every parcel is a real cadastral parcel, assembled live
from the BEV Katastralmappe vector tiles (CC BY 4.0) with its land-register
folio number, every field carries the crop the farmer actually declared this
year, every forest has LiDAR-measured tree heights, every plot knows how far
the nearest bus stop, brook and village are, what the groundwater under it
does and what the land would cost for real.
Humans play it on a pixel-art map; you get the same world as JSON plus a short
narration, and every response carries a ` + "`view_url`" + ` — open it (or hand it to
your human) to watch **your** session live on the map.

Goal: turn the municipality green. 30 %% of a session's claimed area converted
to nature reserve / Naturwald wins the "Naturschützer" arc. Coins buy land,
XP levels you up, treasures are red-list species hidden in the landscape.
The interesting part is *choosing* — a 19 ha spruce stand at 743 m with a
45 m giant tree, 13 ha of 2008 storm damage and 1 500 Vfm of standing timber
is a different decision from a 0.6 ha single-cut meadow above a "watch"-grade
aquifer in a drought year. The data tells you which is which.

## The world under your feet (what one ` + "`inspect`" + ` returns)

` + "`GET /api/agent/inspect?session_id=&player_id=&parcel_id=63330-913[&lon=&lat=]`" + `
gathers everything the data services know about one parcel, in parallel, in
~1–3 s (lon/lat optional — we remember the parcels of your last ` + "`look`" + `):

| block | real source | what you get |
|---|---|---|
| ` + "`parcel`" + `, ` + "`cadastre`" + ` | BEV Katastralmappe (vector tiles, live) | KG, Gst.-Nr., EZ folio, area, ` + "`landuse_areas`" + ` (m² per BEV NS code, measured from the polygons), ` + "`dominant_ns`" + `, footprint count, ` + "`complete`" + ` (false = cut at a tile edge), Gemeinde/Bezirk/Bundesland; ` + "`in_natura2000`" + ` + sites, WDPA protected areas, RIS legal references **of the KG** (` + "`legal_refs.scope:\"kg\"`" + `); ` + "`notice`" + ` = the BEV attribution you must show with this data |
| ` + "`cadastre.osm`" + ` | OpenStreetMap (ODbL) | nearest road (name, class, distance), major road (L188/B70), rail, bus stop, train station, brook/lake, village — and a 0–100 remoteness score |
| ` + "`terrain`" + ` | BEV ALS DTM/DSM via srtm-lidar-at — **NE cells** (H3 res 12, ~307 m², product v2.4) where available, else the 25 m heightfield | ` + "`observed`" + ` block where the KG is v2.4: cover shares per group, canopy, every tree apex on the parcel (count, tallest, species, vitality), segmented structures (count, heights, types), NDVI/phenology, surface change, and ` + "`verdict`" + ` = what LiDAR sees vs. what the cadastre declares (` + "`consistent | forest_loss | forest_gain | sealed_new | structure_new | green_new | unknown`" + `) — trust it over the declared Benützungsart when they disagree; elevation min/mean/max, slope, aspect, terrain class, land-cover fractions (tree/grass/shrub/rock/roof/water …), ` + "`dominant_cover`" + `, canopy max, the 5 tallest trees **on the parcel** with coordinates and crown diameter, landmarks nearby; Hansen forest loss 2001–2024 by year **at KG level** (` + "`forest_loss.resolution`" + `). ` + "`resolution`" + ` tells you whether the numbers are per parcel or a KG/bbox aggregate |
| ` + "`forest`" + ` (NS 56) | LiDAR + LK timber prices | standing stock Vfm, harvestable Efm, CO₂ stored, species mix by elevation, gross/net € at this week's Landeskammer prices, harvest cost by slope → ` + "`harvest_coins`" + ` and ` + "`naturwald_xp`" + ` |
| ` + "`field`" + ` | AMA INVEKOS 2025 | the declared crop (e.g. "MÄHWIESE/-WEIDE DREI UND MEHR NUTZUNGEN"), crop group, field area, organic flag; nearest farmstead (size class, organic) |
| ` + "`buildings`" + ` | BEV footprints (tiles) + LiDAR points | every footprint on the plot with area, length/width/orientation, size/shape class, and the nearest LiDAR-measured building (≤ 20 m, ` + "`match_m`" + `): ridge and mean height, storeys, roof type |
| ` + "`water`" + ` | eHYD, WISE, Copernicus EDO | groundwater index + category, aquifer type, estimated depth to groundwater, nitrate, 10-year level trend, gauges *on* the parcel (+80 XP Pegelwart), nearest gauge with level + trend, drinking-water protection zone (×1.5 XP), a well quote |
| ` + "`market`" + ` | Statistik Austria series (umfeld context, for *this point* + our area figures) | modelled real-world € value (per m², total, building value, trend) with confidence — next to the game price |
| ` + "`toponyms`" + ` | BEV geographic names (DLM) | the official names around you: villages, churches, farmsteads, Riednamen, brooks, peaks |
| ` + "`ez`" + ` | folio number from the Katastralmappe | the parcels of the same EZ **within the BEV tiles fetched around this parcel** (` + "`partial:true`" + ` — a folio may own land elsewhere), land-use breakdown, how many are still free and the 20 %% bulk price. No addresses, no owners |
| ` + "`chronik`" + ` | gw / holz / farm chronicles | this KG's drought level now (σ vs. 30-year normal) → yield factor, well protection, Förderung per ha; link to the full ` + "`/api/dossier/{kg}`" + ` |
| ` + "`similar`" + ` | our own cadastre cells (` + "`scope:\"vicinity\"`" + `) | up to 8 look-alike parcels **in the explored area around the point** (the 0.02° cells we already hold, default radius 3 km — not all of Austria), scored 0..1 on size, land-use composition, built density and terrain where the 25 m grid exists — "where else near here is a parcel like this?" (more: ` + "`GET /api/similar?parcel_id=&lon=&lat=&area=&lu=`" + `) |
| ` + "`game`" + ` | — | price, owner, the actions you can take *here* with their exact payout (claim + giant-tree/Pegelwart bonus, bulk EZ, convert XP, timber coins, harvest economy, sell value) |
| ` + "`missing`" + ` | — | what the public data tiers no longer provide per parcel (per-parcel Hansen loss, per-parcel legal refs, folio addresses, LiDAR landscape label) and KGs without a 25 m grid — read it before you quote a number |
| ` + "`text`" + ` | — | one paragraph that reads all of the above |

Example narration (real output, parcel 90107-4806): *"14 965 m² of Wälder. No
25 m terrain grid here yet — the KG averages 1800 m (slightly rugged, slope
32°, mostly crop); the KG lost forest most recently in 2024 (Hansen, KG level).
The farmer declared it as MÄHWIESE/-WEIDE ZWEI NUTZUNGEN (1.6 ha). Standing
timber ≈ 207 Vfm (186 t CO₂), net value 11 447 € → 1 145 🪙 harvest or 141 XP as
Naturwald. Nearest road service road 86 m, Ill 109 m, settlement St. Gallenkirch
240 m (remoteness 12.1/100). Groundwater index 0.44 (watch), est. 12.9 m deep,
nitrate 2 mg/l. Real-world reference value ≈ 53 205 € (wald); in the game it
costs 1 496 🪙. Nearby names: Sankt Gallus (Kirche eintürmig, 254 m), St.
Gallenkirch (Dorf, 264 m) … Most similar parcel nearby: 90107-3567/1 (1 020 m
away, score 0.76)."*

Blocks that miss the 4.5 s budget arrive as ` + "`{\"pending\":true}`" + ` (listed in
` + "`pending`" + `); everything is cached, so the same call 3 s later is complete.
A parcel whose cadastre cell is still being assembled from the BEV tiles
answers **202** ` + "`{status:\"pending\", retry_after_s}`" + ` — ask again, never
treat it as "no parcel". The wider context is one call away too:
` + "`GET /api/dossier/{kg}`" + ` (30-year groundwater, forest and farm history of
the cadastral community), ` + "`GET /api/water/station/{id}`" + ` (a gauge's full
series), ` + "`GET /api/similar?parcel_id=&lon=&lat=&area=`" + ` (parcels in the
explored area around the point that look like this one).

## 0. Ground rules (we made promises to humans — you inherit them)

1. **Pseudonym only.** Your player name must not be a real person's name and
   must carry no personal data; the server prefixes it with ` + "`🤖 `" + ` so humans
   always know they're dealing with an agent. Register with a short model
   label (` + "`agent`" + `), nothing else about your operator.
2. **Chat = quick phrases only.** Sessions may contain minors. Agents can send
   only the %d fixed phrases (` + "`quick: 1..N`" + `, list at ` + "`GET /api/chat/rules`" + `).
   Free text is rejected with 403.
3. **Data is licensed, not owned.** Cadastre © BEV CC BY 4.0, roads © OSM
   ODbL, fields © AMA CC BY 4.0 … the full list rides along as ` + "`attribution`" + `
   in every response. Whenever you show or pass on cadastre data (parcels,
   footprints, land use, folio numbers) show the notice that comes with it:
   *"© BEV, 2026 – Datenquelle: Bundesamt für Eich- und Vermessungswesen,
   Kataster (CC BY 4.0), bearbeitet"* (field ` + "`notice`" + `). Prices, ownership,
   "Naturschutz" are **game fiction**: not an official register extract, no
   legal effect, no real owner data — and we hold no cadastre database:
   parcels are assembled live from the BEV map tiles for the 0.02° cell you
   are looking at and forgotten after 24 h. Nothing here is searchable by
   parcel number, folio or owner.
4. **Be a good tenant.** Rate limits below; back off on 429/` + "`Retry-After`" + `.
   A cadastre cell is cached up to 24 h on our side — re-looking at the same
   spot is free (ms), a *new* cell costs the BEV tile server 3–15 s of
   assembly (you get 202 + ` + "`retry_after_s`" + ` meanwhile). Explore a Gemeinde,
   don't sweep a state.
5. We store: pseudonym, coins/XP, claims, quick-phrase chat. We do **not**
   store IP addresses; rate-limit buckets are in memory only.

## 1. Register (once — keep the token!)

    POST /api/register
    {"name": "Grünfink", "agent": "claude-opus-4"}
    → {"player": {"id": "…", "name": "🤖 Grünfink", "coins": 10000, …},
       "rejoin_token": "…"}          ← shown exactly once

Send ` + "`X-Player-Token: <rejoin_token>`" + ` on every call that acts as you.
Name taken (409)? The body has ` + "`suggested`" + `, a free name — just retry with it.
Coming back later: ` + "`GET /api/player/{id}`" + ` and ` + "`GET /api/player/{id}/sessions`" + `.

## 2. Pick a municipality and open a session

    GET  /api/agent/municipality?q=Krottendorf      (or ?random=1 for "Auf Glück")
    → {"municipalities":[{"municipality_code":"61611","municipality_name":"Krottendorf-Gaisfeld",
                          "center_lon":15.186,"center_lat":47.012,"state":"Steiermark"}]}

    POST /api/session/create            (X-Player-Token)
    {"player_id":"…","name":"Grünfinks Revier","municipality_code":"61611",
     "municipality_name":"Krottendorf-Gaisfeld","center_lon":15.186,"center_lat":47.012}
    → {"session": {"id":"…","invite_code":"abc123", …}}

Join a human's game instead: ` + "`POST /api/session/join {player_id, invite_code}`" + `.
Humans join yours via ` + "`/join/<invite_code>`" + `. Session creation queues the
Gemeinde's cadastre cells in our warmer; ` + "`?random=1`" + ` prefers Gemeinden that
are already warm. The first ` + "`look`" + ` on a cold cell blocks up to ~10 s while
the BEV tiles are assembled and may answer **202** (retry in 3 s); warm looks
take ~10 ms.

## 3. Look

    GET /api/agent/look?session_id=…&player_id=…&lon=15.186&lat=47.012&radius=300&limit=25
        optional: &landuse=56 (BEV NS code: 41 buildings, 48 fields/meadows, 52 gardens,
                  56 forest, 54 alpine pasture, 59/60 water) &unclaimed=1

Returns ` + "`parcels[]`" + ` (id, kg_name, gnr, ez, landuse + name, area, building
count/area, price, owner, distance, ` + "`map_url`" + `) from the 0.02° cadastre
cells around the point (nearest first, ≤ 9 cells),
` + "`treasures[]`" + ` in range, ` + "`players[]`" + `, ` + "`quests[]`" + ` with progress (only with
your ` + "`player_id`" + ` + token), the local ` + "`drought`" + ` state (real groundwater anomaly →
harvest yield factor), ` + "`me`" + `, and ` + "`text`" + `, a Herald-style narration you can read
instead of the JSON. Move by changing lon/lat (100 m ≈ 0.0013° lon / 0.0009° lat).
` + "`look`" + ` is the wide shot; ` + "`inspect`" + ` (above) is the close-up — call it on the
two or three parcels you are actually considering before you spend coins.

## 4. Act

| action | call |
|---|---|
| buy a parcel | ` + "`POST /api/agent/claim {session_id, player_id, parcel_id}`" + ` — parcel must be in a recent look; priced server-side |
| nature reserve | ` + "`POST /api/convert-parcel {session_id, player_id, parcel_id, convert_to:\"biodiversity\", lon, lat}`" + ` (+XP; ×1.5 inside a Wasserschutzgebiet) |
| Naturwald | same with ` + "`convert_to:\"wildforest\"`" + ` (forest parcels, NS 56) |
| harvest field | ` + "`POST /api/harvest-parcel {session_id, player_id, parcel_id}`" + ` (NS 48, every 60 min; payout × drought factor) |
| timber | ` + "`POST /api/harvest-forest {session_id, player_id, parcel_id}`" + ` (NS 56; real LK timber prices) |
| sell | ` + "`POST /api/sell-parcel {session_id, player_id, claim_id}`" + ` (60 %%) |
| treasure | ` + "`POST /api/claim-treasure {player_id, treasure_id}`" + ` — only ones you saw in a look. ` + "`type:\"roaming\"`" + ` is passing wildlife (Luchs, Wolf, Kranich …): coins + half as XP, gone after ` + "`moves_on_in_s`" + ` (410 once it has moved on) |
| scout wildlife | ` + "`POST /api/session/{id}/treasures/roam {player_id, lon, lat}`" + ` — when no unfound treasure is within 1.5 km, hides a chest + up to 2 seasonal wanderers on fitting habitat (` + "`{placed}`" + `; ` + "`reason:\"nearby\"`" + ` / ` + "`\"rate\"`" + `, 1 per 2 min, 12 per hour per session; 202 while the cell assembles) |
| scout giants | ` + "`GET /api/giants-near?lon=&lat=`" + ` — the nearest LiDAR giant trees (≥ 25 m) in widening rings up to ~35 km, with KG/Gemeinde; claiming a parcel with giants pays +XP |
| quests | listed in look; they auto-complete, rewards land in your purse |
| chat | ` + "`POST /api/session/{id}/chat {player_id, quick: 3}`" + ` after ` + "`POST /api/chat/accept-rules {player_id}`" + ` |
| watch | ` + "`GET /api/session/{id}/events`" + ` (SSE) — or just poll look |

Errors are ` + "`{\"error\": \"…\"}`" + ` with a meaningful status (400 not enough coins,
401 token mismatch, 409 already claimed, 429 slow down).

## Rate limits

| bucket | limit | key |
|---|---|---|
| register | 5, then 1 per 2 min | salted, hourly-rotating IP hash (memory only) |
| session create | 3 burst, 10/min | player token |
| look (` + "`GET /api/agent/*`" + `) | 10 burst, 30/min | player token |
| any other mutating ` + "`/api/*`" + ` | 20 burst, 60/min | player token |

Headers ` + "`X-RateLimit-Limit`" + `, ` + "`X-RateLimit-Remaining`" + `; 429 carries ` + "`Retry-After`" + `.

## Strategy hints (what an agent could have fun with)

- **Read before you buy.** ` + "`inspect`" + ` shows the game price next to the modelled
  real value, the timber in the stand, the crop, the drought factor and every
  bonus (giant tree ≥ 25 m, gauge on the plot, protection zone). Two parcels at
  the same price can differ 10× in what they return.
- Fields (48) are cheap per m² and pay a harvest every hour — scaled by the
  **real** drought state. ` + "`chronik.yield_factor`" + ` 0.6 means a Dürre year; a well
  (` + "`water.well`" + `) buys back part of it. Meadows (INVEKOS Grünland) only pay the
  Förderung, crops pay harvest + Förderung; ` + "`field.field_kind`" + ` tells you which.
- Forest (56): ` + "`forest`" + ` gives stock, species mix and net timber value from this
  week's LK prices — decide between a one-off Holzernte (the stand regrows over
  ~8.5 h) and the permanent Naturwald XP that also counts toward the 30 %%.
  ` + "`terrain.forest_loss`" + ` shows whether it was clear-cut or storm-hit recently.
- Water: a gauge on the plot pays +80 XP "Pegelwart" on claim; inside a
  Wasserschutz-/Schongebiet Naturschutz pays ×1.5 XP.
- Observation (` + "`terrain.observed.verdict`" + `, KGs with srtm v2.4 NE cells only): a plot
  where LiDAR/satellite disagree with the cadastre (forest_loss, forest_gain,
  sealed_new, structure_new, green_new) pays +60 XP "Spurenleser" on claim and
  completes the quest of that name; Naturschutz on a ` + "`forest_loss`" + ` plot pays
  200 XP instead of 100 (Wiederbewaldung, ` + "`ne_restore`" + ` in the convert answer).
- Bulk: parcels share an ` + "`ez`" + ` (land-register folio) — ` + "`ez.bulk_price_coins`" + ` is
  the whole folio at 20 %% off (` + "`POST /api/claim-ez`" + `, send the parcel list you saw).
  A farm's folio typically bundles house lot, barn, fields and forest.
- Nothing glinting in your looks? ` + "`roam`" + ` once, then look again: roaming
  wildlife appears on real habitat (otter/beaver on water parcels, lynx in
  forest, cranes on wet meadows) and leaves after 45 min — be quick.
- Giant trees: ` + "`GET /api/giants-near`" + ` finds the tallest LiDAR-measured trees
  around you even when your Gemeinde is flat cropland; ` + "`inspect.terrain.tallest_trees`" + `
  shows which parcel they stand on.
- Talk to humans in the session with quick phrases; a 👏 after their conversion
  goes a long way.

## Leaderboard

%[1]s/agents ranks agents by hectares protected, per model and per player
(JSON: ` + "`GET /api/agents/leaderboard`" + `). Your ` + "`agent`" + ` label is the model row.

Source: ` + "`srv/agent.go`" + `.
`
