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
	"context"
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
	"disclaimer": "Game data. Simplified and playfully altered; not an official register extract, no legal effect. Ownership in the game is fictional — real owner data is never processed.",
	"sources": []string{
		"Kataster (DKM), Grundstücksverzeichnis, Nutzungsflächen, ALS-Höhenmodell: © BEV – Bundesamt für Eich- und Vermessungswesen, data.bev.gv.at, CC BY 4.0 (bearbeitet; BEV does not endorse this project)",
		"Roads, water, rail, addresses: © OpenStreetMap contributors, ODbL 1.0",
		"Protected areas: Natura 2000 (EEA); Red List: IUCN European Red List / EEA",
		"Fields (INVEKOS) and farmsteads: AMA / BML via data.gv.at, CC BY 4.0 (aggregated, hashed points)",
		"Groundwater, gauges, water protection: BML eHYD, WISE / EEA, CC BY 4.0; drought: Copernicus EDO",
		"Timber prices: Statistik Austria (CC BY 4.0), LK-Holzmarktberichte — model values, not offers",
	},
	"more": siteURL + "/impressum",
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

func landuseName(code string) string {
	if n, ok := nsNames[code]; ok {
		return n
	}
	if code == "" {
		return "unbekannte Nutzung"
	}
	return "NS " + code
}

// fetchAgentParcels: cadastre /spatial/point (attrs only), 1 h cache keyed by
// a ~10 m quantised point so an agent re-looking pays nothing upstream.
func (s *Server) fetchAgentParcels(lon, lat float64, radius, limit int, landuse string) ([]map[string]any, int, error) {
	q := fmt.Sprintf("lon=%.6f&lat=%.6f&radius=%d&layer=parcels&attrs_only=true&limit=%d", lon, lat, radius, limit)
	if landuse != "" {
		q += "&landuse=" + landuse
	}
	key := fmt.Sprintf("agentlook:%.4f:%.4f:%d:%d:%s", lon, lat, radius, limit, landuse)
	b, st := s.llmGet(key, cadastreAPI+"/spatial/point?"+q, time.Hour)
	if st != 200 {
		return nil, st, fmt.Errorf("cadastre %d", st)
	}
	var res struct {
		Data struct {
			Parcels []map[string]any `json:"parcels"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return nil, 502, err
	}
	return res.Data.Parcels, 200, nil
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
		codes := strings.Split(strn(row, "landuse_codes"), ",")
		p.Landuse = strings.TrimSpace(codes[0])
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
	rows, st, err := s.fetchAgentParcels(lon, lat, radius, fetchN, landuse)
	if err != nil {
		jsonErr(w, "cadastre service unavailable, retry shortly", st)
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
		claimBy[c.ParcelID] = c
	}

	parcels := make([]agentParcel, 0, limit)
	now := time.Now()
	for _, row := range rows {
		p := parcelFromRow(row, sess)
		if p.ParcelID == "" {
			continue
		}
		if c, ok := claimBy[p.ParcelID]; ok {
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
			if t.FoundBy != nil {
				continue
			}
			d := distM(lon, lat, t.Lon, t.Lat)
			if d <= float64(radius) {
				treasures = append(treasures, agentTreasure{t.ID, t.TreasureType, t.Value, t.SpeciesGerman, t.Lon, t.Lat, math.Round(d)})
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
			"claim":    "POST /api/agent/claim {session_id, player_id, parcel_id}",
			"convert":  "POST /api/convert-parcel {session_id, player_id, parcel_id, convert_to:'biodiversity'|'wildforest', lon, lat}",
			"sell":     "POST /api/sell-parcel {session_id, player_id, claim_id}",
			"treasure": "POST /api/claim-treasure {player_id, treasure_id}",
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
		fmt.Fprintf(&b, "A treasure glints %.0f m away (%s, %d) — id %d. ", t.DistanceM, what, t.Value, t.ID)
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
	station, _ := s.stationOnParcel(p.ParcelID) // cached GW-2 lookup; awards Pegelwart like the browser does
	s.claimParcel(w, r, claimReq{
		SessionID: req.SessionID, PlayerID: req.PlayerID, ParcelID: p.ParcelID,
		KgCode: p.KgCode, Gnr: p.Gnr, Ez: p.Ez, AreaSqm: p.AreaSqm, Landuse: p.Landuse,
		BuildingCount: p.BuildingCount, TotalBuildingArea: p.BuildingArea, GwStation: station,
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
		rows = []map[string]any{res.Data[rand.Intn(len(res.Data))]}
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
> Browser game over real Austrian cadastre and environmental open data: claim parcels, turn land into nature reserves.

## For agents
- [How to play as an agent](%s/llm/game): text-only edition, register → session → look → act. Rate limits + data licences inside.
- [Sibling data-service roadmap](%s/llm/ahead): what we ask our upstream data APIs for (with a live conformance harness).

## Legal
- [Impressum & data sources](%s/impressum)
- [Datenschutz](%s/datenschutz)
`, siteURL, siteURL, siteURL, siteURL)
}

func (s *Server) handleLLMGame(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=600")
	var stats struct{ Agents, Players, Sessions int64 }
	s.DB.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM players WHERE agent != ''").Scan(&stats.Agents)
	s.DB.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM players").Scan(&stats.Players)
	s.DB.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM game_sessions").Scan(&stats.Sessions)
	fmt.Fprintf(w, llmGameMD, siteURL, stats.Players, stats.Agents, stats.Sessions, len(quickPhrases))
}

const llmGameMD = `# Siedler Österreich — the text edition for agents

Base URL: %s  ·  players so far: %d (🤖 %d)  ·  sessions: %d

You are about to play a land game on **real Austrian open data**: every parcel
is a real cadastral parcel (BEV), fields are real INVEKOS fields, forests carry
LiDAR-measured tree heights, the groundwater under you is the real gauge
network. Humans play it on a pixel-art map; you get the same world as JSON
plus a short narration, and every response carries a ` + "`view_url`" + ` — open
it (or hand it to your human) to watch **your** session live on the map.

Goal: turn the municipality green. 30 %% of a session's claimed area converted
to nature reserve / Naturwald wins the "Naturschützer" arc. Coins buy land,
XP levels you up, treasures are red-list species hidden in the landscape.

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
   in every response. Prices, ownership, "Naturschutz" are **game fiction**:
   not an official register extract, no legal effect, no real owner data.
4. **Be a good tenant.** Rate limits below; back off on 429/` + "`Retry-After`" + `.
   Cadastre responses are cached one hour on our side — re-looking at the same
   spot is free, sweeping a whole state is not welcome.
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
Humans join yours via ` + "`/join/<invite_code>`" + `. Session creation triggers a
prewarm of the municipality's data upstream; the first ` + "`look`" + ` may take a few
seconds, later ones ~50 ms.

## 3. Look

    GET /api/agent/look?session_id=…&player_id=…&lon=15.186&lat=47.012&radius=300&limit=25
        optional: &landuse=56 (BEV NS code: 41 buildings, 48 fields/meadows, 52 gardens,
                  56 forest, 54 alpine pasture, 59/60 water) &unclaimed=1

Returns ` + "`parcels[]`" + ` (id, landuse, area, price, owner, distance, ` + "`map_url`" + `),
` + "`treasures[]`" + ` in range, ` + "`players[]`" + `, ` + "`quests[]`" + ` with progress (only with
your ` + "`player_id`" + ` + token), the local ` + "`drought`" + ` state (real groundwater anomaly →
harvest yield factor), ` + "`me`" + `, and ` + "`text`" + `, a Herald-style narration you can read
instead of the JSON. Move by changing lon/lat (100 m ≈ 0.0013° lon / 0.0009° lat).

## 4. Act

| action | call |
|---|---|
| buy a parcel | ` + "`POST /api/agent/claim {session_id, player_id, parcel_id}`" + ` — parcel must be in a recent look; priced server-side |
| nature reserve | ` + "`POST /api/convert-parcel {session_id, player_id, parcel_id, convert_to:\"biodiversity\", lon, lat}`" + ` (+XP; ×1.5 inside a Wasserschutzgebiet) |
| Naturwald | same with ` + "`convert_to:\"wildforest\"`" + ` (forest parcels, NS 56) |
| harvest field | ` + "`POST /api/harvest-parcel {session_id, player_id, parcel_id}`" + ` (NS 48, every 60 min; payout × drought factor) |
| timber | ` + "`POST /api/harvest-forest {session_id, player_id, parcel_id}`" + ` (NS 56; real LK timber prices) |
| sell | ` + "`POST /api/sell-parcel {session_id, player_id, claim_id}`" + ` (60 %%) |
| treasure | ` + "`POST /api/claim-treasure {player_id, treasure_id}`" + ` — only ones you saw in a look |
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

- Fields (48) are cheap per m² and pay a harvest every hour — scaled by the
  **real** drought state. Look at ` + "`drought.yield_factor`" + ` before buying farmland.
- Forest (56) with tall LiDAR trees: ` + "`GET /api/forest-value?parcel_id=&kg=&area=&lu=56&session_id=`" + `
  gives stock, species mix and a net timber value from current LK prices —
  decide between one-off Holzernte and the permanent Naturwald XP.
- Water: ` + "`GET /api/water/parcel/{parcel_id}`" + ` tells you if a real gauge sits on the
  plot (+80 XP "Pegelwart" on claim), ` + "`GET /api/dossier/{kg}`" + ` is the
  Gemeinde-Chronik (water / forest / farm history of the KG).
- Bulk: parcels share an ` + "`ez`" + ` (land-register folio) — ` + "`POST /api/claim-ez`" + ` buys
  a whole folio at 20 %% off (browser contract: send the parcel list you saw).
- Talk to humans in the session with quick phrases; a 👏 after their conversion
  goes a long way.

Source: ` + "`srv/agent.go`" + `. Roadmap for our data siblings: /llm/ahead.
`
