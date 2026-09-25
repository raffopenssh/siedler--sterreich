package srv

// KG dossiers from the sibling data services (sibling spec: GET /llm/kg/{kg}).
//
//   GW-3/GW-4  groundwater-at   → water block (GWI, nitrate, drought calendar, "today")
//   HOLZ-1     holzeinschlag-at → municipal forest timeline 2001-2024
//   FARM-1     farm-subsidies   → municipal CAP subsidy profile (aggregate only)
//
// GET /api/dossier/{kg} fetches all three in parallel, normalises them into
// one compact JSON the client renders as the "Gemeinde-Chronik" panel, and
// derives the game-facing numbers (drought level, subsidy per ha, CO₂ sink)
// the mechanics in water.go use server-side. Each upstream block is cached
// on its own (6 h; 404 "no_data" negative-cached 1 h) so a service being
// down never blanks the others.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"srv.exe.dev/db/dbgen"
)

const (
	gwAPI   = "https://groundwater-at.exe.xyz"
	holzAPI = "https://holzeinschlag-at.exe.xyz"
)

// llmGet fetches a sibling JSON document with a bounded timeout and caches
// 200 bodies for ttl and 404 "no_data" for 1 h. Returns (body, status).
// Never caches 5xx / transport errors.
func (s *Server) llmGet(key, url string, ttl time.Duration) ([]byte, int) {
	ctx := context.Background()
	if c, err := s.Q.GetCachedData(ctx, key); err == nil {
		if strings.HasPrefix(c, `{"error":"no_data"`) {
			return []byte(c), 404
		}
		return []byte(c), 200
	}
	type res struct {
		b  []byte
		st int
	}
	v, _, _ := s.sf.Do(key, func() (any, error) {
		if c, err := s.Q.GetCachedData(ctx, key); err == nil {
			return res{[]byte(c), 200}, nil
		}
		rctx, cancel := context.WithTimeout(ctx, 6*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(rctx, "GET", url, nil)
		resp, err := upstreamClient.Do(req)
		if err != nil {
			return res{jsonErrBody("data service error"), 502}, nil
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		switch resp.StatusCode {
		case 200:
			s.Q.SetCachedData(ctx, dbgen.SetCachedDataParams{CacheKey: key, Data: string(raw), ExpiresAt: time.Now().Add(ttl)})
			return res{raw, 200}, nil
		case 404:
			b := []byte(`{"error":"no_data"}`)
			s.Q.SetCachedData(ctx, dbgen.SetCachedDataParams{CacheKey: key, Data: string(b), ExpiresAt: time.Now().Add(time.Hour)})
			return res{b, 404}, nil
		case http.StatusAccepted:
			return res{parsePending(resp.Header, raw).body(), 202}, nil
		}
		return res{jsonErrBody("upstream error"), resp.StatusCode}, nil
	})
	r := v.(res)
	return r.b, r.st
}

func (s *Server) llmKG(service, kg string) map[string]any {
	kg = fmt.Sprintf("%05s", kg)
	var url string
	switch service {
	case "gw":
		url = gwAPI + "/llm/kg/" + kg + "?fields=metrics,drought,now,history,gemeinde_code,gemeinde_name,kg_name,as_of"
	case "holz":
		url = holzAPI + "/llm/kg/" + kg
	case "farm":
		url = farmAPI + "/llm/kg/" + kg
	default:
		return nil
	}
	b, st := s.llmGet("llmkg:"+service+":"+kg, url, 6*time.Hour)
	if st != 200 {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return nil
	}
	return m
}

// ---- helpers over the loosely-typed upstream JSON ----

func num(m map[string]any, k string) (float64, bool) {
	if m == nil {
		return 0, false
	}
	switch v := m[k].(type) {
	case float64:
		if math.IsNaN(v) {
			return 0, false
		}
		return v, true
	case int:
		return float64(v), true
	}
	return 0, false
}
func numOr(m map[string]any, k string, def float64) float64 {
	if v, ok := num(m, k); ok {
		return v
	}
	return def
}
func str(m map[string]any, k string) string {
	if m == nil {
		return ""
	}
	v, _ := m[k].(string)
	return v
}
func sub(m map[string]any, k string) map[string]any {
	if m == nil {
		return nil
	}
	v, _ := m[k].(map[string]any)
	return v
}
func arr(m map[string]any, k string) []any {
	if m == nil {
		return nil
	}
	v, _ := m[k].([]any)
	return v
}
func r1(v float64) float64 { return math.Round(v*10) / 10 }
func r2(v float64) float64 { return math.Round(v*100) / 100 }

// yearOf accepts "2024", "2024-12-31" or 2024.
func yearOf(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case string:
		if len(t) >= 4 {
			var y int
			fmt.Sscanf(t[:4], "%d", &y)
			return y
		}
	}
	return 0
}

// ---- drought level (GW-3 + GW-4) ----
//
// The game world follows the real aquifer: the daily eHYD "now" status sets
// the base (normal 0 · low 1 · very_low 2 · high 0), and a calendar month
// that is historically dry here (season_profile ≥ 0.7 mean CDI class) adds
// one. 0 normal · 1 trocken · 2 Dürre · 3 schwere Dürre. Field yield is
// scaled by droughtYieldFactor unless the parcel has a well.

type droughtState struct {
	Level  int     `json:"level"`
	Label  string  `json:"label"`
	Season float64 `json:"season"` // this month's mean CDI class
	Status string  `json:"status"` // now.status
	Sigma  float64 `json:"sigma"`  // now.gw_level_anomaly_sigma
	PYear  float64 `json:"p_drought_year"`
	AsOf   string  `json:"as_of"`
	Known  bool    `json:"known"`
}

var droughtLabels = []string{"normal", "trocken", "Dürre", "schwere Dürre"}

func droughtFrom(gw map[string]any, now time.Time) droughtState {
	d := droughtState{Label: droughtLabels[0]}
	if gw == nil {
		return d
	}
	nowB, dr := sub(gw, "now"), sub(gw, "drought")
	if nowB == nil && dr == nil {
		return d
	}
	d.Known = true
	d.Status = str(nowB, "status")
	d.Sigma = numOr(nowB, "gw_level_anomaly_sigma", 0)
	d.AsOf = str(nowB, "as_of")
	d.PYear = numOr(dr, "p_drought_year", 0)
	lvl := 0
	switch d.Status {
	case "low":
		lvl = 1
	case "very_low":
		lvl = 2
	}
	if sp := arr(dr, "season_profile"); len(sp) == 12 {
		if v, ok := sp[int(now.Month())-1].(float64); ok {
			d.Season = v
			if v >= 0.7 {
				lvl++
			}
		}
	}
	if lvl > 3 {
		lvl = 3
	}
	d.Level, d.Label = lvl, droughtLabels[lvl]
	return d
}

func droughtYieldFactor(level int) float64 { return 1 - 0.2*float64(level) }

// wellProtection: how much of the drought loss a well recovers, by the real
// groundwater stress of the body it taps (GWI category) and today's level.
// good 1.0 · watch 0.75 · stressed 0.5; very_low today → the well runs low.
func wellProtection(gwiCategory, nowStatus string) float64 {
	p := 0.75
	switch gwiCategory {
	case "good":
		p = 1
	case "stressed":
		p = 0.5
	}
	if nowStatus == "very_low" {
		p *= 0.5
	}
	return p
}

// ---- subsidy (FARM-1) ----
// Coins per hectare a harvest (or a meadow's yearly claim) earns on top of the
// crop: the municipality's median CAP payment per eligible ha, in game coins
// (eurPerCoin), stretched over ~4 harvests. Organic fields (INVEKOS Schlag
// organic flag, FARM-2) get the ÖPUL premium: ×1.3.
func subsidyPerHa(farm map[string]any) float64 {
	if farm == nil {
		return 0
	}
	m := sub(farm, "metrics")
	eur, ok := num(m, "eur_per_ha_median")
	if !ok || eur <= 0 {
		return 0
	}
	return eur / eurPerCoin / 4
}

func subsidyCoins(farm map[string]any, areaSqm float64, organic bool) int64 {
	per := subsidyPerHa(farm)
	if per <= 0 {
		return 0
	}
	c := per * areaSqm / 10000
	if organic {
		c *= 1.3
	}
	return int64(math.Min(250, math.Round(c)))
}

// ---- the dossier ----

type kgDossier struct {
	KG           string         `json:"kg_code"`
	Gemeinde     string         `json:"gemeinde_code,omitempty"`
	GemeindeName string         `json:"gemeinde_name,omitempty"`
	KGName       string         `json:"kg_name,omitempty"`
	Water        map[string]any `json:"water"`
	Forest       map[string]any `json:"forest"`
	Farm         map[string]any `json:"farm"`
	Drought      droughtState   `json:"drought"`
	Game         map[string]any `json:"game"`
	FetchedAt    string         `json:"fetched_at"`
}

func (s *Server) buildDossier(kg string) kgDossier {
	var gw, holz, farm map[string]any
	var wg sync.WaitGroup
	wg.Add(3)
	go func() { defer wg.Done(); gw = s.llmKG("gw", kg) }()
	go func() { defer wg.Done(); holz = s.llmKG("holz", kg) }()
	go func() { defer wg.Done(); farm = s.llmKG("farm", kg) }()
	wg.Wait()
	d := kgDossier{KG: fmt.Sprintf("%05s", kg), FetchedAt: time.Now().UTC().Format(time.RFC3339)}
	for _, m := range []map[string]any{holz, farm, gw} {
		if d.Gemeinde == "" {
			d.Gemeinde = str(m, "gemeinde_code")
		}
		if d.GemeindeName == "" {
			d.GemeindeName = str(m, "gemeinde_name")
		}
		if d.KGName == "" {
			d.KGName = str(m, "kg_name")
		}
	}
	d.Drought = droughtFrom(gw, time.Now())
	d.Water = waterBlock(gw)
	d.Forest = forestBlock(holz)
	d.Farm = farmBlock(farm)
	d.Game = map[string]any{
		"yield_factor":    r2(droughtYieldFactor(d.Drought.Level)),
		"subsidy_per_ha":  r1(subsidyPerHa(farm)),
		"well_protection": wellProtection(str(sub(gw, "metrics"), "gwi_category"), d.Drought.Status),
	}
	return d
}

func waterBlock(gw map[string]any) map[string]any {
	if gw == nil {
		return nil
	}
	m, now, dr := sub(gw, "metrics"), sub(gw, "now"), sub(gw, "drought")
	out := map[string]any{
		"as_of":             str(gw, "as_of"),
		"gwi":               numOr(m, "gwi", 0),
		"gwi_category":      str(m, "gwi_category"),
		"no3_mg_l":          numOr(m, "gwi_no3", 0),
		"use_pct":           numOr(m, "gwi_use_pct", 0),
		"gw_trend_m_decade": numOr(m, "gw_trend_m_per_decade_mean", numOr(m, "gwi_gw_trend", 0)),
		"precip_mean_mm":    numOr(m, "precip_mean_mm", 0),
		"gw_body":           str(m, "gwi_gwk"),
		"stations":          numOr(m, "gw_station_count", 0),
		"risk_category":     str(m, "risk_category"),
	}
	if now != nil {
		out["now"] = map[string]any{
			"as_of": str(now, "as_of"), "status": str(now, "status"),
			"sigma": numOr(now, "gw_level_anomaly_sigma", 0), "percentile": numOr(now, "gw_percentile_of_month", 0),
			"trend_30d_cm": numOr(now, "trend_30d_cm", 0), "n_stations": numOr(now, "n_stations", 0),
			"estimated": now["estimated"] == true,
		}
	}
	if dr != nil {
		ev := arr(dr, "events")
		events := make([]map[string]any, 0, len(ev))
		for _, e := range ev {
			em, _ := e.(map[string]any)
			if em == nil {
				continue
			}
			events = append(events, map[string]any{"year": yearOf(em["year"]), "from": numOr(em, "start_month", 0), "to": numOr(em, "end_month", 0), "class": str(em, "class")})
		}
		out["drought"] = map[string]any{
			"p_drought_year": numOr(dr, "p_drought_year", 0), "worst_year": yearOf(dr["worst_year"]),
			"season_profile": arr(dr, "season_profile"), "events": events, "as_of": str(dr, "as_of"),
		}
	}
	var hist []map[string]any
	for _, h := range arr(gw, "history") {
		hm, _ := h.(map[string]any)
		if hm == nil {
			continue
		}
		hist = append(hist, map[string]any{"year": yearOf(hm["as_of"]), "precip_mm": numOr(hm, "precip_mm", 0), "gw_anom_m": numOr(hm, "gw_level_anomaly_m", 0), "cdi": numOr(hm, "edo_cdi_mean", 0)})
	}
	out["history"] = hist
	return out
}

func forestBlock(h map[string]any) map[string]any {
	if h == nil {
		return nil
	}
	m := sub(h, "metrics")
	out := map[string]any{
		"as_of":             str(h, "as_of"),
		"forest_area_ha":    numOr(m, "forest_area_ha", 0),
		"loss_ha_latest":    numOr(m, "loss_ha_2024", numOr(m, "loss_ha", 0)),
		"loss_total_ha":     numOr(m, "loss_total_ha", 0),
		"harvest_efm":       numOr(m, "harvest_efm", 0),
		"harvest_value_eur": numOr(m, "harvest_value_eur", 0),
		"co2_t":             numOr(m, "co2_t", 0),
		"net_flux_tco2e_ha": numOr(m, "net_flux_tco2e_ha", 0),
		"price_spruce":      numOr(m, "price_spruce_eur_efm", 0),
		"state":             str(h, "state"),
	}
	var hist []map[string]any
	for _, x := range arr(h, "history") {
		hm, _ := x.(map[string]any)
		if hm == nil {
			continue
		}
		hist = append(hist, map[string]any{"year": yearOf(hm["as_of"]), "loss_ha": r2(numOr(hm, "loss_ha", 0)), "harvest_efm": numOr(hm, "harvest_efm", 0), "co2_t": numOr(hm, "co2_t", 0), "price": numOr(hm, "price_spruce_eur_efm", 0)})
	}
	out["history"] = hist
	return out
}

func farmBlock(f map[string]any) map[string]any {
	if f == nil {
		return nil
	}
	m := sub(f, "metrics")
	out := map[string]any{
		"as_of":                    yearOf(f["as_of"]),
		"recipients_n":             numOr(m, "recipients_n", 0),
		"total_eur":                numOr(m, "total_eur", 0),
		"eur_per_ha_median":        numOr(m, "eur_per_ha_median", 0),
		"eur_per_recipient_median": numOr(m, "eur_per_recipient_median", 0),
		"eligible_ha_total":        numOr(m, "eligible_ha_total", 0),
		"organic_share":            numOr(m, "organic_share", 0),
		"mountain_share":           numOr(m, "mountain_share", 0),
		"young_farmer_share":       numOr(m, "young_farmer_share", 0),
		"livestock_share":          numOr(m, "livestock_share", 0),
		"archetype_mix":            sub(m, "archetype_mix"),
	}
	type meas struct {
		Name  string  `json:"name"`
		Share float64 `json:"share"`
	}
	var tm []meas
	for _, x := range arr(m, "top_measures") {
		mm, _ := x.(map[string]any)
		if mm == nil {
			continue
		}
		tm = append(tm, meas{str(mm, "name"), r2(numOr(mm, "share", 0))})
	}
	sort.Slice(tm, func(i, j int) bool { return tm[i].Share > tm[j].Share })
	if len(tm) > 3 {
		tm = tm[:3]
	}
	out["top_measures"] = tm
	var hist []map[string]any
	for _, x := range arr(f, "history") {
		hm, _ := x.(map[string]any)
		if hm == nil {
			continue
		}
		hist = append(hist, map[string]any{"year": yearOf(hm["as_of"]), "total_eur": numOr(hm, "total_eur", 0), "recipients_n": numOr(hm, "recipients_n", 0), "eur_per_ha": numOr(hm, "eur_per_ha_median", 0), "organic_share": numOr(hm, "organic_share", 0)})
	}
	out["history"] = hist
	return out
}

// GET /api/dossier/{kg}
func (s *Server) handleDossier(w http.ResponseWriter, r *http.Request) {
	kg := fmt.Sprintf("%05s", r.PathValue("kg"))
	if len(kg) != 5 {
		jsonErr(w, "bad kg", 400)
		return
	}
	s.cachedFetch(w, "dossier:v1:"+kg, func() ([]byte, int) {
		d := s.buildDossier(kg)
		b, _ := json.Marshal(d)
		if d.Water != nil || d.Forest != nil || d.Farm != nil {
			// Short TTL: the water "now" block changes daily and the drought
			// level flips at month boundaries.
			s.Q.SetCachedData(context.Background(), dbgen.SetCachedDataParams{CacheKey: "dossier:v1:" + kg, Data: string(b), ExpiresAt: time.Now().Add(2 * time.Hour)})
		}
		return b, 200
	})
}

// droughtForKG is the server-side view of the same numbers the client shows,
// used by the harvest / well mechanics. Cheap: both upstream blocks are cached.
func (s *Server) droughtForKG(kg string) (droughtState, map[string]any, map[string]any) {
	gw := s.llmKG("gw", kg)
	farm := s.llmKG("farm", kg)
	return droughtFrom(gw, time.Now()), gw, farm
}
