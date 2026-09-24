package srv

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"srv.exe.dev/db/dbgen"
)

// Forest plots: timber estimate + harvest cycle.
//
// Shared contract with forestStage() in game.js. A forest parcel (NS 56 or a
// lidar tree fraction ≥ forestTreeMin) can be
//   - harvested (Holzernte / Kahlschlag): coins now ≈ the real net timber
//     value (EUR / eurPerCoin), then the stand regrows through
//     Schlag → Jungwuchs → Stangenholz → Baumholz on real time from
//     parcel_claims.harvested_at; a second harvest is possible once the stand
//     is Baumholz again, at a value that climbs back from 50 % to 100 %.
//   - set aside as Naturwald (converted_to = 'wildforest'): permanent, awards
//     XP scaled by the standing stock, counts toward the 30 % nature target.
const (
	forestTreeMin      = 0.5
	forestSchlagMin    = 40.0  // minutes: stumps, slash, a few Überhälter
	forestJungwuchsMin = 90.0  // saplings between the stumps
	forestStangenMin   = 150.0 // pole stage → harvestable again (50 % value)
	forestFullValueMin = 510.0 // value back to 100 %
	eurPerCoin         = 10.0
)

const timberAPI = "https://holzeinschlag-at.exe.xyz"

// ---- prices --------------------------------------------------------------

type timberPrices struct {
	State      string  `json:"state"`
	Date       string  `json:"date"`
	Spruce     float64 `json:"spruce_eur_efm"`     // Blochholz Fichte/Tanne Media 2b
	Larch      float64 `json:"larch_eur_efm"`      // Blochholz Lärche 3a+
	Pine       float64 `json:"pine_eur_efm"`       // Blochholz Kiefer 2a+
	Beech      float64 `json:"beech_eur_efm"`      // Blochholz Buche B3
	Industrial float64 `json:"industrial_eur_efm"` // Schleif-/Faserholz
	FuelHard   float64 `json:"fuel_hard_eur_efm"`  // Brennholz hart (per Efm ≈ 1.4 RM)
	FuelSoft   float64 `json:"fuel_soft_eur_efm"`
	Source     string  `json:"source"`
	Live       bool    `json:"live"` // false = built-in fallback table
}

var fallbackPrices = timberPrices{Spruce: 120, Larch: 160, Pine: 80, Beech: 90, Industrial: 50, FuelHard: 140, FuelSoft: 95, Source: "Richtwerte (offline)", Date: ""}

// kgState maps a 5-digit KG code to the Bundesland used by the LK price pages
// (Vienna has no LK forest page → NÖ). See AGENTS.md "KG codes".
func kgState(kg string) string {
	kg = fmt.Sprintf("%05s", kg)
	switch kg[0] {
	case '0', '1', '2':
		return "Niederösterreich"
	case '3':
		return "Burgenland"
	case '4':
		return "Oberösterreich"
	case '5':
		if kg[1] >= '5' {
			return "Salzburg"
		}
		return "Oberösterreich"
	case '6':
		return "Steiermark"
	case '7':
		return "Kärnten"
	case '8':
		return "Tirol"
	case '9':
		return "Vorarlberg"
	}
	return "Steiermark"
}

var timberCatalogMu sync.Mutex

// timberCatalog returns the (≤700 KB) price catalogue, cached 24 h in api_cache.
func (s *Server) timberCatalog(ctx context.Context) (map[string]any, error) {
	const key = "timber:catalog"
	if cached, err := s.Q.GetCachedData(ctx, key); err == nil {
		var m map[string]any
		if json.Unmarshal([]byte(cached), &m) == nil {
			return m, nil
		}
	}
	timberCatalogMu.Lock()
	defer timberCatalogMu.Unlock()
	if cached, err := s.Q.GetCachedData(ctx, key); err == nil {
		var m map[string]any
		if json.Unmarshal([]byte(cached), &m) == nil {
			return m, nil
		}
	}
	cctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(cctx, "GET", timberAPI+"/data/timber_price_catalog.json", nil)
	resp, err := upstreamClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("catalog %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	s.Q.SetCachedData(context.Background(), dbgen.SetCachedDataParams{CacheKey: key, Data: string(raw), ExpiresAt: time.Now().Add(24 * time.Hour)})
	return m, nil
}

// timberPricesFor extracts the regional assortment prices for a state from
// the catalogue (LK chamber ranges), falling back to national STAT series and
// finally to the built-in table. Units are normalised to EUR per Efm.
func (s *Server) timberPricesFor(ctx context.Context, state string) timberPrices {
	p := fallbackPrices
	p.State = state
	cat, err := s.timberCatalog(ctx)
	if err != nil {
		log.Printf("timber catalog: %v", err)
		return p
	}
	pages, _ := cat["pages"].(map[string]any)
	sp, _ := cat["state_pages"].(map[string]any)
	stPages, _ := sp[state].(map[string]any)
	series := func(page string) map[string]any {
		pg, _ := pages[page].(map[string]any)
		sr, _ := pg["series"].(map[string]any)
		return sr
	}
	// pick(series, code) → (value, date, ok); values older than 3 years are ignored
	pick := func(sr map[string]any, code string) (float64, string, bool) {
		s0, _ := sr[code].(map[string]any)
		v, ok := s0["latest_value"].(float64)
		last, _ := s0["last"].(string)
		if !ok || v <= 0 || len(last) < 4 || last < fmt.Sprint(time.Now().Year()-3) {
			return 0, "", false
		}
		return v, last, true
	}
	set := func(dst *float64, v float64, date string) {
		*dst = math.Round(v*100) / 100
		if date > p.Date {
			p.Date = date
		}
		p.Live = true
	}
	// national STAT sawlog series (EURO/M3 ≈ EUR/Efm)
	nat := series("saegeholz_monatl")
	if v, d, ok := pick(nat, "at_blochholz_fita_klbmed2b_monat"); ok {
		set(&p.Spruce, v, d)
	}
	if v, d, ok := pick(nat, "at_blochholz_ki_klb2aplus_monat"); ok {
		set(&p.Pine, v, d)
	}
	if v, d, ok := pick(nat, "at_blochholz_bu_klb3_monat"); ok {
		set(&p.Beech, v, d)
	}
	if v, d, ok := pick(series("industrierundholz_monatl"), "at_faserholz_fita_monat"); ok {
		// Faserholz is quoted per AMM (atro tonne); spruce ≈ 2.3 Fm per atro t.
		set(&p.Industrial, v/2.3, d)
	}
	if stPages != nil {
		saw := series(fmt.Sprint(stPages["sawlog"]))
		if v, d, ok := pick(saw, "LK_BLFIM2b"); ok {
			set(&p.Spruce, v, d)
		}
		if v, d, ok := pick(saw, "LK_BLLA3aplus"); ok {
			set(&p.Larch, v, d)
		}
		if v, d, ok := pick(saw, "LK_BLKI2aplus"); ok {
			set(&p.Pine, v, d)
		}
		if v, d, ok := pick(saw, "LK_BLBU3plus"); ok {
			set(&p.Beech, v, d)
		}
		ind := series(fmt.Sprint(stPages["industrial_roundwood"]))
		if v, d, ok := pick(ind, "LK_ISFI_FMO"); ok {
			set(&p.Industrial, v, d)
		} else if v, d, ok := pick(ind, "LK_IFFI_AMM"); ok {
			set(&p.Industrial, v/2.3, d)
		}
		fuel := series(fmt.Sprint(stPages["fuelwood"]))
		if v, d, ok := pick(fuel, "LK_BHH"); ok {
			set(&p.FuelHard, v*1.4, d) // EURO/RM → per Fm (1 Fm ≈ 1.4 RM Scheitholz)
		}
		if v, d, ok := pick(fuel, "LK_BHW"); ok {
			set(&p.FuelSoft, v*1.4, d)
		}
		p.Source = "LK " + state + " (preise.agrarforschung.at)"
	} else {
		p.Source = "Statistik Austria (preise.agrarforschung.at)"
	}
	return p
}

// ---- estimate ------------------------------------------------------------

type timberAssortment struct {
	Efm       float64 `json:"efm"`
	EurPerEfm float64 `json:"eur_per_efm"`
	Eur       float64 `json:"eur"`
}

type timberEstimate struct {
	ParcelID    string  `json:"parcel_id"`
	Source      string  `json:"source"` // v3 | lidar | landuse | none
	IsForest    bool    `json:"is_forest"`
	AreaHa      float64 `json:"area_ha"`
	CanopyFrac  float64 `json:"canopy_frac"`
	HMean       float64 `json:"h_mean_m"`
	HMax        float64 `json:"h_max_m,omitempty"`
	NTrees      int     `json:"n_trees,omitempty"`
	Vfm         float64 `json:"vfm"`        // standing stock, Vorratsfestmeter
	VfmPerHa    float64 `json:"vfm_per_ha"` // per canopy ha
	Efm         float64 `json:"efm"`        // harvestable (bark + losses removed)
	CO2t        float64 `json:"co2_t"`      // stored in the stems (≈0.9 t/Vfm)
	Species     map[string]float64 `json:"species"` // shares: spruce_fir, larch, pine, broadleaf
	Assortments map[string]timberAssortment `json:"assortments"`
	GrossEur    float64 `json:"gross_eur"`
	CostEur     float64 `json:"cost_eur"`
	CostPerEfm  float64 `json:"cost_per_efm"`
	NetEur      float64 `json:"net_eur"`
	Coins       int64   `json:"coins"`     // full-value harvest payout
	WildXP      int64   `json:"wild_xp"`   // Naturwald XP reward
	Prices      timberPrices `json:"prices"`
	Elev        float64 `json:"elev_m,omitempty"`
	Slope       float64 `json:"slope_deg,omitempty"`
	V3          string  `json:"v3,omitempty"` // "" | ok | cold | error | off
	TookMs      int64   `json:"took_ms"`
}

// v3cold remembers KGs whose product is still downloading upstream so we don't
// burn 2.5 s per popup on them; retried after 4 minutes.
var v3cold sync.Map // kg → time.Time

// slimParcel reads one parcel from the cached lidar-slim (never warms).
func (s *Server) slimParcel(ctx context.Context, kg, pid string) (map[string]any, string) {
	cached, err := s.Q.GetCachedData(ctx, "lidar-slim2:/kg/"+kg)
	if err != nil {
		return nil, ""
	}
	var slim struct {
		ProductVersion any              `json:"product_version"`
		Parcels        []map[string]any `json:"parcels"`
	}
	if json.Unmarshal([]byte(cached), &slim) != nil {
		return nil, ""
	}
	pv := fmt.Sprint(slim.ProductVersion)
	for _, p := range slim.Parcels {
		if p["parcel_id"] == pid {
			return p, pv
		}
	}
	return nil, pv
}

// parcelGeometry returns the parcel polygon (cached per id like handleGeometryBatch).
func (s *Server) parcelGeometry(ctx context.Context, pid string) (json.RawMessage, error) {
	ck := "geom:parcels:" + pid
	if cached, err := s.Q.GetCachedData(ctx, ck); err == nil {
		var it struct {
			Geometry json.RawMessage `json:"geometry"`
		}
		if json.Unmarshal([]byte(cached), &it) == nil && len(it.Geometry) > 2 {
			return it.Geometry, nil
		}
	}
	code, _, body, err := upstreamGetWait(cadastreAPI+"/parcels/geometry/batch?ids="+url.QueryEscape(pid), 4*time.Second, 4<<20)
	if err != nil || code != 200 {
		return nil, fmt.Errorf("geometry %d %v", code, err)
	}
	var res struct {
		Parcels []json.RawMessage `json:"parcels"`
	}
	if json.Unmarshal(body, &res) != nil || len(res.Parcels) == 0 {
		return nil, fmt.Errorf("no geometry")
	}
	var it struct {
		Geometry json.RawMessage `json:"geometry"`
	}
	if json.Unmarshal(res.Parcels[0], &it) != nil || len(it.Geometry) < 3 {
		return nil, fmt.Errorf("no geometry")
	}
	s.Q.SetCachedData(context.Background(), dbgen.SetCachedDataParams{CacheKey: ck, Data: string(res.Parcels[0]), ExpiresAt: time.Now().Add(24 * time.Hour)})
	return it.Geometry, nil
}

type v3Summary struct {
	NTrees        int                `json:"n_trees"`
	AreaHaCanopy  float64            `json:"area_ha_canopy"`
	AreaHaTotal   float64            `json:"area_ha_total"`
	HMean         float64            `json:"h_mean_m"`
	HMax          float64            `json:"h_max_m"`
	Volume        float64            `json:"volume_m3_est_total"`
	ByLeaf        map[string]float64 `json:"by_leaf_type"`
	BySpecies     map[string]float64 `json:"by_species_hint"`
}

// v3Trees: the srtm product-backed single-tree inventory — ~0.2 s when the
// KG's light GPKG is in their cache, minutes when cold. We give it `budget`
// and fall back to the per-parcel heuristic otherwise (marking the KG cold).
func (s *Server) v3Trees(kg string, geom json.RawMessage, budget time.Duration) (*v3Summary, string) {
	if t, ok := v3cold.Load(kg); ok && time.Since(t.(time.Time)) < 4*time.Minute {
		return nil, "cold"
	}
	body, _ := json.Marshal(map[string]any{"geometry": geom, "include_trees": false, "fallback_live": false})
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "POST", strings.TrimSuffix(lidarAPI, "/v1")+"/v3/trees", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := upstreamClient.Do(req)
	if err != nil {
		v3cold.Store(kg, time.Now())
		return nil, "cold"
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode != 200 {
		if resp.StatusCode == http.StatusAccepted {
			v3cold.Store(kg, time.Now())
			return nil, "cold"
		}
		return nil, "error"
	}
	var out struct {
		Summary v3Summary `json:"summary"`
	}
	if json.Unmarshal(raw, &out) != nil {
		return nil, "error"
	}
	return &out.Summary, "ok"
}

func clampF(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }

// estimateTimber builds the harvest-value estimate for one parcel.
// landuse = dominant NS code as stored on the claim / sent by the client.
func (s *Server) estimateTimber(ctx context.Context, kg, pid string, areaSqm float64, landuse string, tryV3 bool) timberEstimate {
	t0 := time.Now()
	e := timberEstimate{ParcelID: pid, Source: "none", Species: map[string]float64{}, Assortments: map[string]timberAssortment{}}
	e.AreaHa = areaSqm / 1e4
	if e.AreaHa <= 0 {
		return e
	}
	// 1. what we know about the stand
	treeFrac, hMean, hMax, elev, slope := -1.0, 0.0, 0.0, 600.0, 8.0
	sp, pv := s.slimParcel(ctx, kg, pid)
	if sp != nil {
		e.Source = "lidar"
		if fr, ok := sp["fracs"].(map[string]any); ok {
			if t, ok := fr["tree"].(float64); ok {
				treeFrac = t
			} else {
				treeFrac = 0
			}
		}
		if treeFrac < 0 {
			if ff, ok := sp["forested_fraction"].(float64); ok {
				treeFrac = ff
			}
		}
		if th, ok := sp["tree_h"].(map[string]any); ok {
			hMean, _ = th["mean"].(float64)
			hMax, _ = th["max"].(float64)
		}
		if hMean == 0 {
			if nm, ok := sp["ndsm_max_m"].(float64); ok && nm > 0 {
				hMax = nm
				hMean = clampF(nm*0.72, 3, 38)
			}
		}
		if v, ok := sp["elevation_m"].(float64); ok {
			elev = v
		}
		if v, ok := sp["slope_mean_deg"].(float64); ok {
			slope = v
		}
	}
	if treeFrac < 0 { // no lidar: trust the cadastre
		if landuse == "56" {
			treeFrac = 0.85
			e.Source = "landuse"
		} else {
			treeFrac = 0
		}
	}
	if hMean == 0 {
		hMean = 18
	}
	e.IsForest = landuse == "56" || treeFrac >= forestTreeMin
	e.CanopyFrac = math.Round(treeFrac*100) / 100
	e.Elev, e.Slope = math.Round(elev), math.Round(slope*10)/10
	if !e.IsForest {
		e.TookMs = time.Since(t0).Milliseconds()
		return e
	}

	// species mix from elevation (refined by v3 below)
	conifer := clampF(0.45+(elev-500)/1500, 0.3, 0.95)
	larch := 0.05
	if elev > 1200 {
		larch = 0.3
	} else if elev > 800 {
		larch = 0.15
	}
	pine := 0.05
	if elev < 450 {
		pine = 0.15
	}
	e.Species["spruce_fir"] = conifer * (1 - larch - pine)
	e.Species["larch"] = conifer * larch
	e.Species["pine"] = conifer * pine
	e.Species["broadleaf"] = 1 - conifer

	// 2. standing stock. Ertragstafel-ish: V/ha ≈ 0.9·h_mean^1.95 for a closed canopy
	//    (h 15 → 175, 22 → 375, 27 → 560 Vfm/ha).
	var vfm float64
	canopyHa := e.AreaHa * treeFrac
	if tryV3 && isV2Product(pv) && e.AreaHa < 60 {
		if geom, err := s.parcelGeometry(ctx, pid); err == nil {
			sum, st := s.v3Trees(kg, geom, 2500*time.Millisecond)
			e.V3 = st
			if sum != nil && sum.NTrees > 0 && sum.Volume > 0 {
				e.Source = "v3"
				e.NTrees = sum.NTrees
				vfm = sum.Volume
				hMean, hMax = sum.HMean, sum.HMax
				if sum.AreaHaCanopy > 0 {
					canopyHa = sum.AreaHaCanopy
					e.CanopyFrac = math.Round(clampF(sum.AreaHaCanopy/e.AreaHa, 0, 1)*100) / 100
				}
				var con, tot float64
				for k, v := range sum.ByLeaf {
					tot += v
					if strings.HasPrefix(k, "conif") {
						con += v
					}
				}
				if tot > 0 {
					conifer = con / tot
					var la, pi float64
					for k, v := range sum.BySpecies {
						switch k {
						case "larch":
							la += v
						case "pine":
							pi += v
						}
					}
					// unspecified conifers keep the elevation prior for larch/pine
					e.Species["larch"] = math.Max(la/tot, conifer*larch*0.5)
					e.Species["pine"] = math.Max(pi/tot, conifer*pine*0.5)
					e.Species["spruce_fir"] = math.Max(0, conifer-e.Species["larch"]-e.Species["pine"])
					e.Species["broadleaf"] = 1 - conifer
				}
			}
		}
	} else if tryV3 {
		e.V3 = "off"
	}
	if vfm == 0 {
		vfm = 0.9 * math.Pow(clampF(hMean, 3, 40), 1.95) * canopyHa
	}
	e.HMean, e.HMax = math.Round(hMean*10)/10, math.Round(hMax*10)/10
	e.Vfm = math.Round(vfm)
	if canopyHa > 0 {
		e.VfmPerHa = math.Round(vfm / canopyHa)
	}
	e.Efm = math.Round(vfm * 0.8)
	e.CO2t = math.Round(vfm * 0.9)

	// 3. assortments by stand height
	saw := clampF((hMean-10)/18, 0, 0.7)
	fuel := clampF(0.35-saw*0.3, 0.1, 0.35)
	ind := 1 - saw - fuel
	pr := s.timberPricesFor(ctx, kgState(kg))
	e.Prices = pr
	sawPrice := e.Species["spruce_fir"]*pr.Spruce + e.Species["larch"]*pr.Larch + e.Species["pine"]*pr.Pine + e.Species["broadleaf"]*pr.Beech
	fuelPrice := conifer*pr.FuelSoft + (1-conifer)*pr.FuelHard
	add := func(name string, share, price float64) {
		efm := math.Round(e.Efm * share)
		a := timberAssortment{Efm: efm, EurPerEfm: math.Round(price), Eur: math.Round(efm * price)}
		e.Assortments[name] = a
		e.GrossEur += a.Eur
	}
	add("sawlog", saw, sawPrice)
	add("industrial", ind, pr.Industrial)
	add("fuelwood", fuel, fuelPrice)
	// 4. harvest cost: Motorsäge+Schlepper flat ground, Seilkran on steep slopes
	cost := 28.0
	if slope > 30 {
		cost = 45
	} else if slope > 20 {
		cost = 36
	}
	e.CostPerEfm = cost
	e.CostEur = math.Round(e.Efm*cost + 150)
	e.NetEur = math.Max(0, e.GrossEur-e.CostEur)
	e.Coins = int64(clampF(math.Round(e.NetEur/eurPerCoin), 5, 6000))
	e.WildXP = 120 + int64(math.Min(180, math.Round(vfm/10)))
	e.TookMs = time.Since(t0).Milliseconds()
	return e
}

// forestPhase mirrors forestStage() in game.js: stage + value factor from the
// minutes since the last harvest (nil → mature stand).
func forestPhase(harvestedAt *time.Time, now time.Time) (stage string, factor float64, minutes float64) {
	if harvestedAt == nil {
		return "baumholz", 1, math.Inf(1)
	}
	minutes = now.Sub(*harvestedAt).Minutes()
	switch {
	case minutes < forestSchlagMin:
		return "schlag", 0, minutes
	case minutes < forestJungwuchsMin:
		return "jungwuchs", 0, minutes
	case minutes < forestStangenMin:
		return "stangenholz", 0, minutes
	}
	return "baumholz", clampF(0.5+0.5*(minutes-forestStangenMin)/(forestFullValueMin-forestStangenMin), 0.5, 1), minutes
}

// GET /api/forest-value?parcel_id=&kg=&area=&lu=&session_id=
// Estimate for the popup; cached 1h (v3) / 10 min (heuristic) per parcel.
func (s *Server) handleForestValue(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	pid, kg := q.Get("parcel_id"), q.Get("kg")
	if pid == "" || !validKG(kg) {
		jsonErr(w, "parcel_id and kg required", 400)
		return
	}
	var area float64
	fmt.Sscanf(q.Get("area"), "%g", &area)
	lu := q.Get("lu")
	ck := "timber:" + pid
	var e timberEstimate
	if cached, err := s.Q.GetCachedData(r.Context(), ck); err == nil && json.Unmarshal([]byte(cached), &e) == nil {
		w.Header().Set("X-Cache", "HIT")
	} else {
		v, _, _ := s.sf.Do(ck, func() (any, error) {
			est := s.estimateTimber(context.Background(), kg, pid, area, lu, true)
			ttl := 10 * time.Minute
			if est.Source == "v3" {
				ttl = time.Hour
			}
			if est.IsForest {
				b, _ := json.Marshal(est)
				s.Q.SetCachedData(context.Background(), dbgen.SetCachedDataParams{CacheKey: ck, Data: string(b), ExpiresAt: time.Now().Add(ttl)})
			}
			return est, nil
		})
		e = v.(timberEstimate)
	}
	out := map[string]any{"estimate": e}
	if sid := q.Get("session_id"); sid != "" {
		if claim, err := s.Q.GetParcelClaim(r.Context(), dbgen.GetParcelClaimParams{SessionID: sid, ParcelID: pid}); err == nil {
			stage, factor, mins := forestPhase(claim.HarvestedAt, time.Now())
			out["stage"] = stage
			out["value_factor"] = factor
			if !math.IsInf(mins, 1) {
				out["minutes_since_harvest"] = math.Round(mins)
			}
			out["coins_now"] = int64(math.Round(float64(e.Coins) * factor))
		}
	}
	jsonResp(w, out)
}

// POST /api/harvest-forest — Kahlschlag on an owned forest parcel.
func (s *Server) handleHarvestForest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID string `json:"session_id"`
		PlayerID  string `json:"player_id"`
		ParcelID  string `json:"parcel_id"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonErr(w, "invalid request", 400)
		return
	}
	if _, ok := s.authPlayer(r, req.PlayerID); !ok {
		jsonErr(w, "unauthorized", 401)
		return
	}
	claim, err := s.Q.GetParcelClaim(r.Context(), dbgen.GetParcelClaimParams{SessionID: req.SessionID, ParcelID: req.ParcelID})
	if err != nil {
		jsonErr(w, "Parcel not found", 404)
		return
	}
	if claim.PlayerID != req.PlayerID {
		jsonErr(w, "Not your parcel", 403)
		return
	}
	if claim.ConvertedTo != nil && *claim.ConvertedTo != "" {
		jsonErr(w, "Dieser Wald ist außer Nutzung gestellt", 400)
		return
	}
	lu := ""
	if claim.Landuse != nil {
		lu = *claim.Landuse
	}
	est := s.estimateTimber(r.Context(), claim.KgCode, req.ParcelID, claim.AreaSqm, lu, false)
	if !est.IsForest {
		jsonErr(w, "Das ist kein Wald", 400)
		return
	}
	now := time.Now()
	stage, factor, _ := forestPhase(claim.HarvestedAt, now)
	if stage != "baumholz" {
		jsonErr(w, "Der Wald muss erst nachwachsen", 409)
		return
	}
	coins := int64(math.Max(5, math.Round(float64(est.Coins)*factor)))
	xp := 5 + coins/25
	ctx := r.Context()
	s.Q.HarvestParcel(ctx, claim.ID)
	s.Q.UpdatePlayerCoins(ctx, dbgen.UpdatePlayerCoinsParams{Coins: coins, ID: req.PlayerID})
	s.Q.UpdatePlayerXP(ctx, dbgen.UpdatePlayerXPParams{Xp: xp, ID: req.PlayerID})
	quests := s.autoCompleteChallenges(ctx, req.SessionID, req.PlayerID)
	player, _ := s.Q.GetPlayerByID(ctx, req.PlayerID)
	s.broadcast(req.SessionID, map[string]any{
		"type": "parcel_harvested", "parcel_id": req.ParcelID, "player": player.Name, "coins": coins, "forest": true,
	})
	jsonResp(w, map[string]any{
		"success": true, "coins": coins, "xp": xp, "player": player, "quests": quests,
		"harvested_at": now, "efm": est.Efm, "net_eur": est.NetEur, "co2_t": est.CO2t,
		"regrow_at": now.Add(time.Duration(forestStangenMin) * time.Minute),
	})
}
