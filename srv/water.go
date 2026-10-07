package srv

// Water (groundwater-at, GW-1…GW-8) — proxies + the mechanics they unlock.
//
// Proxies (all cached, CORS-free for the browser, upstream never called twice
// for the same quantized input):
//   GET /api/water/point?lon&lat            GW-1  parcel-scale water context (+GW-4 now block)
//   GET /api/water/points?west&south&east&north  GW-2  stations / plants / sites as landmarks
//   GET /api/water/station/{id}             GW-2  one point with its 30-year history (/llm/point/{id})
//   GET /api/water/protection?west…         GW-5  Wasserschutz-/Schongebiete polygons
//   GET /api/water/flowpath?lon&lat         GW-6  downstream reach chain to the border
//   GET /api/water/gwi                      GW-7  all-KG GWI table for the picker tint
//   GET /api/water/parcel/{parcel_id}       GW-8  points snapped to a parcel (Pegelwart)
//
// Mechanics:
//   POST /api/dig-well          — Brunnen on an owned field: price by depth_to_gw_m_est,
//                                 protects the harvest from the real drought level.
//   harvest (server.go)         — yield × droughtYieldFactor unless well; + FARM-1 Förderung;
//                                 meadows collect Förderung once per cycle.
//   convert (server.go)         — Naturschutz inside a Wasserschutzgebiet: XP ×1.5 (GW-5).
//   claim (server.go)           — parcel carries a Messstelle: +80 XP "Pegelwart" (GW-8).

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"srv.exe.dev/db/dbgen"
)

func q4(v float64) string { return strconv.FormatFloat(math.Round(v*1e4)/1e4, 'f', 4, 64) }

func lonLatParams(r *http.Request) (lon, lat float64, ok bool) {
	var e1, e2 error
	lon, e1 = strconv.ParseFloat(r.URL.Query().Get("lon"), 64)
	lat, e2 = strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	return lon, lat, e1 == nil && e2 == nil && lon > 9 && lon < 18 && lat > 46 && lat < 49.5
}

// relay writes an llmGet result (200 / 404 / 202 / 5xx) to the client.
func relay(w http.ResponseWriter, b []byte, st int) {
	w.Header().Set("Content-Type", "application/json")
	if st == http.StatusAccepted {
		w.Header().Set("Retry-After", strconv.Itoa(retryAfterOf(b)))
		w.Header().Set("Cache-Control", "no-store")
	}
	w.WriteHeader(st)
	w.Write(b)
}

// GET /api/water/point?lon&lat  (GW-1, GW-4)
func (s *Server) handleWaterPoint(w http.ResponseWriter, r *http.Request) {
	lon, lat, ok := lonLatParams(r)
	if !ok {
		jsonErr(w, "lon,lat required", 400)
		return
	}
	b, st := s.waterPoint(lon, lat)
	relay(w, b, st)
}

func (s *Server) waterPoint(lon, lat float64) ([]byte, int) {
	k := q4(lon) + "," + q4(lat)
	return s.llmGet("gwpoint:"+k, gwAPI+"/llm/point?lon="+q4(lon)+"&lat="+q4(lat), 12*time.Hour)
}

// GET /api/water/points?west&south&east&north  (GW-2) — no geometry, so not bboxProxy.
func (s *Server) handleWaterPoints(w http.ResponseWriter, r *http.Request) {
	qs, key, ok := bboxParams(r)
	if !ok {
		jsonErr(w, "west,south,east,north required", 400)
		return
	}
	b, st := s.llmGet("gwpoints:"+key, gwAPI+"/llm/points?limit=500&"+qs, 24*time.Hour)
	relay(w, b, st)
}

// GET /api/water/station/{id}  (GW-2 history_url → /llm/point/{id})
func (s *Server) handleWaterStation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || len(id) > 64 || strings.ContainsAny(id, "/?#") {
		jsonErr(w, "bad id", 400)
		return
	}
	b, st := s.llmGet("gwstation:"+id, gwAPI+"/llm/point/"+url.PathEscape(id), 24*time.Hour)
	relay(w, b, st)
}

// GET /api/water/protection?west…  (GW-5)
func (s *Server) handleWaterProtection(w http.ResponseWriter, r *http.Request) {
	s.bboxProxy(w, r, "gwprot:", gwAPI+"/llm/protection?limit=500&", "zones",
		map[string]bool{"source": true}, []string{"as_of", "license", "coverage"}, 24*time.Hour)
}

// GET /api/water/flowpath?lon&lat  (GW-6)
func (s *Server) handleWaterFlowpath(w http.ResponseWriter, r *http.Request) {
	lon, lat, ok := lonLatParams(r)
	if !ok {
		jsonErr(w, "lon,lat required", 400)
		return
	}
	q3 := func(v float64) string { return strconv.FormatFloat(math.Round(v*1e3)/1e3, 'f', 3, 64) }
	b, st := s.llmGet("gwflow:"+q3(lon)+","+q3(lat), gwAPI+"/llm/flowpath?lon="+q3(lon)+"&lat="+q3(lat), 7*24*time.Hour)
	if st == 200 {
		// Glitch #11: the droplet outran the tile loader, so the last kilometres
		// were bare green. Warm the KGs under the reach chain (~every 3 km) on
		// both upstreams while the client is still zooming in. Once per path.
		key := "gwflow-warm:" + q3(lon) + "," + q3(lat)
		if _, err := s.Q.GetCachedData(r.Context(), key); err != nil {
			s.Q.SetCachedData(context.Background(), dbgen.SetCachedDataParams{CacheKey: key, Data: "1", ExpiresAt: time.Now().Add(6 * time.Hour)})
			go func(body []byte) {
				var fp struct {
					Geometry struct {
						Coordinates [][]float64 `json:"coordinates"`
					} `json:"geometry"`
				}
				if json.Unmarshal(body, &fp) == nil {
					s.prewarmKGs(kgsAlongPath(fp.Geometry.Coordinates, 3000))
				}
			}(b)
		}
	}
	relay(w, b, st)
}

// GET /api/water/gwi  (GW-7) — ~150 KB gz table for all 7,850 KGs.
func (s *Server) handleWaterGWI(w http.ResponseWriter, r *http.Request) {
	b, st := s.llmGet("gwi:all", gwAPI+"/llm/gwi.json", 24*time.Hour)
	if st == 200 {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		// GW-7 picker tint: add the mean GWI per Gemeinde, joined over the
		// embedded VGD register (KG → Gemeinde).
		var d map[string]json.RawMessage
		if json.Unmarshal(b, &d) == nil {
			var kgs map[string][]float64
			json.Unmarshal(d["kgs"], &kgs)
			acc := map[string][2]float64{}
			for kg, v := range kgs {
				if len(v) == 0 {
					continue
				}
				if k := admin().KGs[kg]; k != nil {
					a := acc[k.Gemeinde]
					acc[k.Gemeinde] = [2]float64{a[0] + v[0], a[1] + 1}
				}
			}
			gem := make(map[string]float64, len(acc))
			for g, a := range acc {
				gem[g] = math.Round(a[0]/a[1]*1000) / 1000
			}
			enc, _ := json.Marshal(gem)
			d["gemeinden"] = enc
			if out, err := json.Marshal(d); err == nil {
				b = out
			}
		}
	}
	relay(w, b, st)
}

// GET /api/water/parcel/{parcel_id}  (GW-8)
func (s *Server) handleWaterParcel(w http.ResponseWriter, r *http.Request) {
	pid := r.PathValue("pid")
	if pid == "" || len(pid) > 32 {
		jsonErr(w, "bad parcel_id", 400)
		return
	}
	b, st := s.waterParcel(pid)
	relay(w, b, st)
}

func (s *Server) waterParcel(pid string) ([]byte, int) {
	return s.llmGet("gwparcel:"+parcelHash(pid), gwAPI+"/llm/parcel/"+url.PathEscape(pid), 24*time.Hour)
}

// stationOnParcel: is a groundwater measuring point (gauge, nitrate site,
// water-quality site, plant) *on* this parcel? gw /llm/parcel/{pid} answers
// 404 no_data for practically every parcel since the sibling stopped keying
// by parcel id, so — like agent inspect — we take gw /llm/points in the
// parcel bbox (24 h cached) and test them against the parcel polygon from
// our cadastre cell. lon/lat may be 0 (then only the KG's cached cells are
// scanned). Hard budget stationBudget: the claim is never held up — on a
// miss the fetch keeps running in the background (so the next look/inspect
// sees it) and we simply pay no bonus.
const stationBudget = 1500 * time.Millisecond

func (s *Server) stationOnParcel(pid string, lon, lat float64) (bool, string) {
	type res struct {
		ok  bool
		cat string
	}
	ch := make(chan res, 1)
	go func() {
		p, found := s.lookupParcelCached(pid, lon, lat)
		if !found {
			ch <- res{}
			return
		}
		pts := s.gwPointsOnParcel(p)
		if len(pts) == 0 {
			ch <- res{}
			return
		}
		ch <- res{true, str(pts[0], "category")}
	}()
	select {
	case r := <-ch:
		return r.ok, r.cat
	case <-time.After(stationBudget):
		return false, ""
	}
}

// lookupParcelCached is lookupParcel restricted to cells we already hold —
// never asks bevdirect (the claim path must stay cheap).
func (s *Server) lookupParcelCached(pid string, lon, lat float64) (*bevParcel, bool) {
	if lon != 0 && lat != 0 {
		if d := s.cachedCell(cellOf(lon, lat)); d != nil {
			for i := range d.Parcels {
				if d.Parcels[i].ParcelID == pid {
					return &d.Parcels[i], true
				}
			}
		}
	}
	if a := admin().KGs[kgCodeOf(pid)]; a != nil {
		for _, c := range a.cells() {
			if d := s.cachedCell(c); d != nil {
				for i := range d.Parcels {
					if d.Parcels[i].ParcelID == pid {
						return &d.Parcels[i], true
					}
				}
			}
		}
	}
	return nil, false
}

// gwPointsOnParcel: gw /llm/points within the parcel bbox (24 h cache, same
// key as agent inspect), filtered by point-in-polygon. Rows: id, name, category.
func (s *Server) gwPointsOnParcel(p *bevParcel) []map[string]any {
	bb := parcelBBox(p).pad(0.0002)
	qs := fmt.Sprintf("west=%.5f&south=%.5f&east=%.5f&north=%.5f", bb.W, bb.S, bb.E, bb.N)
	key := fmt.Sprintf("%.4f,%.4f,%.4f,%.4f", bb.W, bb.S, bb.E, bb.N)
	pb, st := s.llmGet("gwpoints:"+key, gwAPI+"/llm/points?limit=50&"+qs, 24*time.Hour)
	if st != 200 {
		return nil
	}
	var r struct {
		Points []map[string]any `json:"points"`
	}
	json.Unmarshal(pb, &r)
	var pts []map[string]any
	for _, m := range r.Points {
		plon, plat := toFloat(m["lon"]), toFloat(m["lat"])
		if len(p.Geometry) > 0 && !pointInGeom(plon, plat, p.Geometry) {
			continue
		}
		pts = append(pts, map[string]any{"id": str(m, "id"), "name": str(m, "name"), "category": str(m, "category")})
	}
	return pts
}

// inWaterProtection: point-in-polygon against the cached Wasserschutzgebiete
// around (lon,lat). Returns the zone type ("schutzgebiet"|"schongebiet") or "".
func (s *Server) inWaterProtection(lon, lat float64) (string, string) {
	const d = 0.003
	qs := fmt.Sprintf("west=%.5f&south=%.5f&east=%.5f&north=%.5f", lon-d, lat-d, lon+d, lat+d)
	key := q4(lon) + "," + q4(lat)
	b, st := s.llmGet("gwprotpt:"+key, gwAPI+"/llm/protection?limit=200&"+qs, 24*time.Hour)
	if st != 200 {
		return "", ""
	}
	var z struct {
		Zones []struct {
			Name     string          `json:"name"`
			Type     string          `json:"type"`
			Zone     *string         `json:"zone"`
			Geometry json.RawMessage `json:"geometry"`
		} `json:"zones"`
	}
	if json.Unmarshal(b, &z) != nil {
		return "", ""
	}
	for _, zn := range z.Zones {
		if pointInGeom(lon, lat, zn.Geometry) {
			zone := ""
			if zn.Zone != nil {
				zone = *zn.Zone
			}
			return zn.Type, zone
		}
	}
	return "", ""
}

// pointInGeom: even-odd over all rings of a Polygon / MultiPolygon.
func pointInGeom(lon, lat float64, raw json.RawMessage) bool {
	var g struct {
		Type   string          `json:"type"`
		Coords json.RawMessage `json:"coordinates"`
	}
	if json.Unmarshal(raw, &g) != nil {
		return false
	}
	var polys [][][][2]float64
	switch g.Type {
	case "Polygon":
		var p [][][2]float64
		if json.Unmarshal(g.Coords, &p) != nil {
			return false
		}
		polys = [][][][2]float64{p}
	case "MultiPolygon":
		if json.Unmarshal(g.Coords, &polys) != nil {
			return false
		}
	default:
		return false
	}
	in := false
	for _, poly := range polys {
		for _, ring := range poly {
			n := len(ring)
			for i, j := 0, n-1; i < n; j, i = i, i+1 {
				xi, yi, xj, yj := ring[i][0], ring[i][1], ring[j][0], ring[j][1]
				if (yi > lat) != (yj > lat) && lon < (xj-xi)*(lat-yi)/(yj-yi)+xi {
					in = !in
				}
			}
		}
	}
	return in
}

// ---- Brunnen ----

const (
	wellBaseCoins  = 150.0
	wellCoinsPerM  = 35.0
	wellDefaultDep = 12.0
	wellXP         = 40
)

func wellPrice(depthM float64) int64 {
	return int64(math.Round(math.Max(120, math.Min(2500, wellBaseCoins+wellCoinsPerM*depthM))))
}

// wellQuote: depth + price + protection for a parcel centroid. depth falls
// back to 12 m when the service has no confident estimate.
func (s *Server) wellQuote(lon, lat float64) map[string]any {
	out := map[string]any{"depth_m": wellDefaultDep, "depth_confidence": "none", "price": wellPrice(wellDefaultDep), "gwi_category": "", "protection": 0.75}
	b, st := s.waterPoint(lon, lat)
	if st != 200 {
		return out
	}
	var p map[string]any
	if json.Unmarshal(b, &p) != nil {
		return out
	}
	m, now := sub(p, "metrics"), sub(p, "now")
	if dep, ok := num(m, "depth_to_gw_m_est"); ok && dep > 0 {
		out["depth_m"] = r1(math.Min(80, dep))
		out["depth_confidence"] = str(m, "depth_confidence")
		out["price"] = wellPrice(math.Min(80, dep))
	}
	out["gwi_category"] = str(m, "gwi_category")
	out["gwi"] = numOr(m, "gwi", 0)
	out["aquifer"] = str(m, "aquifer_type")
	out["now_status"] = str(now, "status")
	out["protection"] = wellProtection(str(m, "gwi_category"), str(now, "status"))
	if st := sub(p, "nearest_gw_station"); st != nil {
		out["station"] = map[string]any{"name": str(st, "name"), "distance_m": numOr(st, "distance_m", 0), "gw_level_m": numOr(st, "gw_level_m", 0), "trend_m_decade": numOr(st, "gw_trend_m_per_decade", 0)}
	}
	return out
}

// GET /api/well-quote?lon&lat → what a well here would cost / do (for the popup button).
func (s *Server) handleWellQuote(w http.ResponseWriter, r *http.Request) {
	lon, lat, ok := lonLatParams(r)
	if !ok {
		jsonErr(w, "lon,lat required", 400)
		return
	}
	jsonResp(w, s.wellQuote(lon, lat))
}

// POST /api/dig-well {session_id, player_id, parcel_id, lon, lat}
func (s *Server) handleDigWell(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID string  `json:"session_id"`
		PlayerID  string  `json:"player_id"`
		ParcelID  string  `json:"parcel_id"`
		Lon       float64 `json:"lon"`
		Lat       float64 `json:"lat"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonErr(w, "invalid request", 400)
		return
	}
	player, ok := s.authPlayer(r, req.PlayerID)
	if !ok {
		jsonErr(w, "unauthorized", 401)
		return
	}
	claim, err := s.Q.GetParcelClaim(r.Context(), dbgen.GetParcelClaimParams{SessionID: req.SessionID, ParcelHash: parcelHash(req.ParcelID)})
	if err != nil || claim.PlayerID != req.PlayerID {
		jsonErr(w, "Nicht deine Parzelle", 403)
		return
	}
	if claim.ConvertedTo != nil && *claim.ConvertedTo != "" {
		jsonErr(w, "Diese Parzelle wird nicht mehr bewirtschaftet", 400)
		return
	}
	if claim.Landuse == nil || *claim.Landuse != "48" {
		jsonErr(w, "Ein Brunnen lohnt sich nur auf Äckern und Wiesen", 400)
		return
	}
	if claim.WellAt != nil {
		jsonErr(w, "Hier steht schon ein Brunnen", 409)
		return
	}
	quote := s.wellQuote(req.Lon, req.Lat)
	price := quote["price"].(int64)
	if player.Coins < price {
		jsonErr(w, fmt.Sprintf("Nicht genug Münzen! Brauche %d, habe %d", price, player.Coins), 400)
		return
	}
	depth := quote["depth_m"].(float64)
	ctx := r.Context()
	s.Q.SetParcelWell(ctx, dbgen.SetParcelWellParams{WellDepthM: depth, ID: claim.ID})
	s.Q.UpdatePlayerCoins(ctx, dbgen.UpdatePlayerCoinsParams{Coins: -price, ID: req.PlayerID})
	s.Q.UpdatePlayerXP(ctx, dbgen.UpdatePlayerXPParams{Xp: wellXP, ID: req.PlayerID})
	quests := s.autoCompleteChallenges(ctx, req.SessionID, req.PlayerID)
	p, _ := s.Q.GetPlayerByID(ctx, req.PlayerID)
	s.broadcast(req.SessionID, map[string]any{"type": "well_dug", "parcel_id": req.ParcelID, "player": p.Name, "depth_m": depth})
	jsonResp(w, map[string]any{"success": true, "price": price, "depth_m": depth, "xp": wellXP, "protection": quote["protection"], "quote": quote, "player": p, "quests": quests})
}

// ---- Förderung / Dürre: the harvest payout ----

type harvestEconomy struct {
	Base        int64   `json:"base"`         // crop coins before drought
	Drought     int     `json:"drought"`      // level 0..3
	YieldFactor float64 `json:"yield_factor"` // after well protection
	Well        bool    `json:"well"`
	Crop        int64   `json:"crop"`    // paid for the crop
	Subsidy     int64   `json:"subsidy"` // FARM-1 Förderung
	Organic     bool    `json:"organic"`
	Total       int64   `json:"total"`
	Label       string  `json:"label"`
}

// harvestPayout applies the real local drought (GW-3/4) and the municipal
// subsidy median (FARM-1) to a field. Meadows (base 0) collect subsidy only.
func (s *Server) harvestPayout(kg string, claim dbgen.ParcelClaim, base int64, organic bool) harvestEconomy {
	ds, gw, farm := s.droughtForKG(kg)
	e := harvestEconomy{Base: base, Drought: ds.Level, Well: claim.WellAt != nil, Organic: organic, Label: ds.Label}
	f := droughtYieldFactor(ds.Level)
	if e.Well {
		f += (1 - f) * wellProtection(str(sub(gw, "metrics"), "gwi_category"), ds.Status)
	}
	e.YieldFactor = r2(f)
	if base > 0 {
		e.Crop = int64(math.Max(3, math.Round(float64(base)*f)))
	}
	e.Subsidy = subsidyCoins(farm, claim.AreaSqm, organic)
	e.Total = e.Crop + e.Subsidy
	return e
}

// GET /api/field-economy?session_id&parcel_id&organic=1  → preview of harvestPayout for the popup.
func (s *Server) handleFieldEconomy(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	claim, err := s.Q.GetParcelClaim(r.Context(), dbgen.GetParcelClaimParams{SessionID: q.Get("session_id"), ParcelHash: parcelHash(q.Get("parcel_id"))})
	if err != nil {
		jsonErr(w, "Parcel not found", 404)
		return
	}
	base := int64(0)
	if fp := fieldPhaseAtCrop(q.Get("parcel_id"), time.Now(), q.Get("crop_group")); fp.Stage != "meadow" {
		base = harvestYield(claim.AreaSqm)
	}
	jsonResp(w, s.harvestPayout(claim.KgCode, claim, base, q.Get("organic") == "1"))
}

// GET /api/session/{id}/drought → current drought state of the session's KGs
// (one per KG the players own or the session centre; client shows the
// "Grundwasser heute" bar from the dossier — this is the cheap game-state view).
func (s *Server) handleSessionDrought(w http.ResponseWriter, r *http.Request) {
	kg := fmt.Sprintf("%05s", r.URL.Query().Get("kg"))
	if len(kg) != 5 || kg == "00000" {
		jsonErr(w, "kg required", 400)
		return
	}
	ds, gw, farm := s.droughtForKG(kg)
	jsonResp(w, map[string]any{
		"kg_code": kg, "drought": ds, "yield_factor": r2(droughtYieldFactor(ds.Level)),
		"well_protection": wellProtection(str(sub(gw, "metrics"), "gwi_category"), ds.Status),
		"subsidy_per_ha":  r1(subsidyPerHa(farm)), "gwi_category": str(sub(gw, "metrics"), "gwi_category"),
	})
}

var _ = context.Background

func fmtMinutes(d time.Duration) string {
	m := int(math.Ceil(d.Minutes()))
	if m < 1 {
		m = 1
	}
	return fmt.Sprintf("%d min", m)
}

// nextPayoutAt: when this claim can pay out again — next ripe window for a
// crop, one cycle from now for a meadow's Förderung.
func nextPayoutAt(fp fieldPhase, now time.Time) time.Time {
	if fp.Stage == "meadow" {
		return now.Add(fieldCycle)
	}
	return fp.RipeAt.Add(fp.Cycle)
}
