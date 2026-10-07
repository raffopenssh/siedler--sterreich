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
	"strconv"
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

// kgStateDigit: Bundesland digit as used by /data/prices/state/{1-9}.json
// (1 Bgld, 2 Ktn, 3 NÖ, 4 OÖ, 5 Sbg, 6 Stmk, 7 T, 8 Vbg, 9 W).
var stateDigit = map[string]string{"Burgenland": "1", "Kärnten": "2", "Niederösterreich": "3", "Oberösterreich": "4",
	"Salzburg": "5", "Steiermark": "6", "Tirol": "7", "Vorarlberg": "8", "Wien": "9"}

// timberStatePrices (HOLZ-2) reads the ≤10 KB per-state price file instead of
// the 700 KB catalogue: 7 LK series, latest value + date. Cached 24 h.
func (s *Server) timberStatePrices(ctx context.Context, state string) (timberPrices, bool) {
	p := fallbackPrices
	p.State = state
	digit, ok := stateDigit[state]
	if !ok {
		return p, false
	}
	key := "timber:state:" + digit
	raw, err := s.Q.GetCachedData(ctx, key)
	if err != nil {
		cctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(cctx, "GET", timberAPI+"/data/prices/state/"+digit+".json", nil)
		resp, err := upstreamClient.Do(req)
		if err != nil {
			return p, false
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return p, false
		}
		b, err := io.ReadAll(io.LimitReader(resp.Body, 256<<10))
		if err != nil || !json.Valid(b) {
			return p, false
		}
		raw = string(b)
		s.Q.SetCachedData(context.Background(), dbgen.SetCachedDataParams{CacheKey: key, Data: raw, ExpiresAt: time.Now().Add(24 * time.Hour)})
	}
	var d struct {
		StateName string `json:"state_name"`
		Series    map[string]struct {
			Latest     float64 `json:"latest"`
			LatestDate string  `json:"latest_date"`
			Unit       string  `json:"unit"`
		} `json:"series"`
	}
	if json.Unmarshal([]byte(raw), &d) != nil || len(d.Series) == 0 {
		return p, false
	}
	minDate := fmt.Sprint(time.Now().Year() - 3)
	set := func(dst *float64, code string, mul float64) {
		sr, ok := d.Series[code]
		if !ok || sr.Latest <= 0 || len(sr.LatestDate) < 4 || sr.LatestDate < minDate {
			return
		}
		*dst = math.Round(sr.Latest*mul*100) / 100
		if sr.LatestDate > p.Date {
			p.Date = sr.LatestDate
		}
		p.Live = true
	}
	set(&p.Spruce, "LK_BLFIM2b", 1)
	set(&p.Larch, "LK_BLLA3aplus", 1)
	set(&p.Pine, "LK_BLKI2aplus", 1)
	set(&p.Beech, "LK_BLBU3plus", 1)
	set(&p.Industrial, "LK_ISFI_FMO", 1)
	set(&p.FuelHard, "LK_BHH", 1.4) // EURO/RM → per Fm
	set(&p.FuelSoft, "LK_BHW", 1.4)
	if !p.Live {
		return p, false
	}
	p.Source = "LK " + state + " (preise.agrarforschung.at)"
	return p, true
}

// timberPricesFor extracts the regional assortment prices for a state from
// the catalogue (LK chamber ranges), falling back to national STAT series and
// finally to the built-in table. Units are normalised to EUR per Efm.
func (s *Server) timberPricesFor(ctx context.Context, state string) timberPrices {
	p := fallbackPrices
	p.State = state
	if sp, ok := s.timberStatePrices(ctx, state); ok {
		return sp
	}
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
	ParcelID      string                      `json:"parcel_id"`
	Source        string                      `json:"source"` // v3 | lidar | landuse | none
	IsForest      bool                        `json:"is_forest"`
	AreaHa        float64                     `json:"area_ha"`
	CanopyFrac    float64                     `json:"canopy_frac"`
	HMean         float64                     `json:"h_mean_m"`
	HMax          float64                     `json:"h_max_m,omitempty"`
	NTrees        int                         `json:"n_trees,omitempty"`
	Vfm           float64                     `json:"vfm"`        // standing stock, Vorratsfestmeter
	VfmPerHa      float64                     `json:"vfm_per_ha"` // per canopy ha
	Efm           float64                     `json:"efm"`        // harvestable (bark + losses removed)
	CO2t          float64                     `json:"co2_t"`      // stored in the stems (≈0.9 t/Vfm)
	Species       map[string]float64          `json:"species"`    // shares: spruce_fir, larch, pine, broadleaf
	Assortments   map[string]timberAssortment `json:"assortments"`
	GrossEur      float64                     `json:"gross_eur"`
	CostEur       float64                     `json:"cost_eur"`
	CostPerEfm    float64                     `json:"cost_per_efm"`
	NetEur        float64                     `json:"net_eur"`
	Coins         int64                       `json:"coins"`   // full-value harvest payout
	WildXP        int64                       `json:"wild_xp"` // Naturwald XP reward
	Prices        timberPrices                `json:"prices"`
	Elev          float64                     `json:"elev_m,omitempty"`
	Slope         float64                     `json:"slope_deg,omitempty"`
	SpeciesSource string                      `json:"species_source,omitempty"` // "" (elevation guess) | ne
	TreeN         int                         `json:"tree_n,omitempty"`         // NE apices ≥ 3 m on the parcel
	TreesTall     int                         `json:"trees_tall,omitempty"`     // NE apices ≥ 20 m
	DeadFrac      float64                     `json:"dead_frac,omitempty"`      // NE dead+declining share of apices
	V3            string                      `json:"v3,omitempty"`             // "" | ok | cold | error | off
	History       *timberHistory              `json:"history,omitempty"`        // HOLZ-3 Hansen loss history of this plot
	TookMs        int64                       `json:"took_ms"`
}

// standFacts gathers what we know about a parcel's stand without any
// parcel-keyed upstream: the cadastre cell row (terrain enrichment: fracs,
// elevation, slope, geometry) + srtm /landscape for the parcel bbox
// (sampled apex heights). lon/lat may be 0 when the parcel is expected in a
// cached cell of its KG.
type standFacts struct {
	TreeFrac float64 // -1 unknown
	HMean    float64
	HMax     float64
	Elev     float64
	Slope    float64
	Grid25   bool
	HasLidar bool
	Geometry json.RawMessage
	// observed layer (NE cells): the measured stand itself
	NE           bool
	Species      map[string]int // NE species counts (top 4)
	TreeN        int
	TreesTall    int
	Vitality     map[string]int
	ForestLossYr int
}

func (s *Server) standFacts(ctx context.Context, kg, pid string, lon, lat float64) standFacts {
	f := standFacts{TreeFrac: -1, Elev: 600, Slope: 8}
	p, ok := s.lookupParcel(pid, lon, lat)
	if !ok {
		return f
	}
	f.Geometry = p.Geometry
	if p.Elev != nil {
		f.HasLidar = true
		f.Elev = *p.Elev
		if p.Slope != nil {
			f.Slope = *p.Slope
		}
		if p.TreeFrac != nil {
			f.TreeFrac = *p.TreeFrac
		} else if len(p.Fracs) > 0 {
			f.TreeFrac = p.Fracs["tree"]
		}
	}
	// NE cell row: every apex ≥ 3 m of the parcel is counted — canopy share,
	// mean/max height, species and vitality are measured, not sampled. No
	// need for the srtm /landscape sample (apices ≥ 20 m only) at all.
	if ne := p.NE; ne != nil && ne.Cells > 0 {
		f.NE, f.HasLidar = true, true
		f.TreeFrac = ne.Canopy
		f.TreeN, f.TreesTall = ne.TreeN, ne.TreesTall
		f.Species, f.Vitality, f.ForestLossYr = ne.Species, ne.Vitality, ne.ForestLossYr
		f.HMax = math.Max(ne.TreeHMaxM, ne.HMaxM)
		if ne.HMeanM > 0 {
			f.HMean = clampF(ne.HMeanM, 3, 38) // mean canopy height over the parcel's cells
		} else if f.HMax > 0 {
			f.HMean = clampF(f.HMax*0.72, 3, 38)
		}
		return f
	}
	bb := parcelBBox(p)
	key := fmt.Sprintf("ls:v1:%.5f,%.5f,%.5f,%.5f:trees", bb.W, bb.S, bb.E, bb.N)
	body, st := s.landscapeFor(bb, "trees", key)
	if st != 200 {
		return f
	}
	var d struct {
		Resolution string `json:"resolution"`
		Terrain    struct {
			ElevMean  *float64 `json:"elevation_mean_m"`
			SlopeMean *float64 `json:"slope_mean_deg"`
		} `json:"terrain"`
		Landcover struct {
			Fractions map[string]float64 `json:"fractions"`
		} `json:"landcover"`
		Trees struct {
			HMax  *float64 `json:"height_max_m"`
			HMean *float64 `json:"height_mean_m"`
		} `json:"trees"`
	}
	if json.Unmarshal(body, &d) != nil {
		return f
	}
	f.Grid25 = d.Resolution == "25m"
	if !f.HasLidar && d.Terrain.ElevMean != nil {
		f.HasLidar = true
		f.Elev = *d.Terrain.ElevMean
		if d.Terrain.SlopeMean != nil {
			f.Slope = *d.Terrain.SlopeMean
		}
		if t, ok := d.Landcover.Fractions["tree"]; ok {
			f.TreeFrac = t
		} else if len(d.Landcover.Fractions) > 0 {
			f.TreeFrac = 0
		}
	}
	// The apex index samples the tallest trees (h ≥ 20 m), so its mean is the
	// dominant height, not the stand mean: scale to a stand mean.
	if d.Trees.HMax != nil && *d.Trees.HMax > 0 {
		f.HMax = *d.Trees.HMax
		if d.Trees.HMean != nil && *d.Trees.HMean > 0 {
			f.HMean = clampF(*d.Trees.HMean*0.82, 3, 38)
		} else {
			f.HMean = clampF(f.HMax*0.72, 3, 38)
		}
	}
	return f
}

func clampF(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }

// timberHistory: what the Hansen GFC 30 m record says happened on this plot
// since 2001 (holz POST /api/plot-context?fast=1, HOLZ-3). A stand that was
// clear-cut in 2019 cannot carry a mature stock in 2026, whatever the
// cadastre says — young_frac scales the heuristic Vfm down.
type timberHistory struct {
	ForestShare2000 float64 `json:"forest_share_2000_pct"`
	LossTotalHa     float64 `json:"loss_total_ha"`
	LossRecentHa    float64 `json:"loss_recent_ha"` // last 10 years
	LastLossYear    int     `json:"last_loss_year,omitempty"`
	LastLossHa      float64 `json:"last_loss_ha,omitempty"`
	YoungFrac       float64 `json:"young_frac"`        // share of canopy regrowing (<25 y)
	StockFactor     float64 `json:"stock_factor"`      // applied to heuristic Vfm (1 = untouched)
	NetFluxTCO2eHa  float64 `json:"net_flux_tco2e_ha"` // cumulative 2001-2024, negative = sink
	Source          string  `json:"source"`
}

var holzCold sync.Map // "plot" → time.Time (429/timeout backoff)

// plotHistory calls the fast plot-context with a hard budget; nil on any miss.
func plotHistory(geom json.RawMessage, canopyHa float64, budget time.Duration) *timberHistory {
	if t, ok := holzCold.Load("plot"); ok && time.Since(t.(time.Time)) < 2*time.Minute {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "POST", holzAPI+"/api/plot-context?fast=1", bytes.NewReader(geom))
	req.Header.Set("Content-Type", "application/json")
	resp, err := upstreamClient.Do(req)
	if err != nil {
		holzCold.Store("plot", time.Now())
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode == 429 {
		holzCold.Store("plot", time.Now())
		return nil
	}
	if resp.StatusCode != 200 {
		return nil
	}
	var out struct {
		Plot struct {
			ForestShare float64            `json:"forest_share_2000_pct"`
			LossByYear  map[string]float64 `json:"loss_ha_by_year"`
			LossTotal   float64            `json:"loss_total"`
			NetFlux     float64            `json:"net_flux_tco2e_ha"`
		} `json:"plot"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out) != nil {
		return nil
	}
	h := &timberHistory{ForestShare2000: out.Plot.ForestShare, LossTotalHa: out.Plot.LossTotal, NetFluxTCO2eHa: out.Plot.NetFlux, Source: "hansen_gfc_2024", StockFactor: 1}
	year := time.Now().Year()
	var lostStock float64 // ha-equivalents of mature stock missing
	for ys, ha := range out.Plot.LossByYear {
		if ha <= 0 {
			continue
		}
		var y int
		fmt.Sscanf(ys, "%d", &y)
		if y == 0 {
			continue
		}
		age := year - y
		if age <= 10 {
			h.LossRecentHa += ha
		}
		if y > h.LastLossYear {
			h.LastLossYear, h.LastLossHa = y, ha
		}
		if age < 25 {
			h.YoungFrac += ha
		}
		// stock regrows roughly with (age/50)^1.6: 10 y → 8 %, 25 y → 33 %
		lostStock += ha * (1 - math.Pow(clampF(float64(age)/50, 0, 1), 1.6))
	}
	if canopyHa > 0 {
		h.YoungFrac = math.Round(clampF(h.YoungFrac/canopyHa, 0, 1)*100) / 100
		h.StockFactor = math.Round(clampF(1-lostStock/canopyHa, 0.08, 1)*100) / 100
	}
	h.LossRecentHa = math.Round(h.LossRecentHa*100) / 100
	h.LastLossHa = math.Round(h.LastLossHa*100) / 100
	return h
}

// estimateTimber builds the harvest-value estimate for one parcel.
// landuse = dominant NS code as stored on the claim / sent by the client.
func (s *Server) estimateTimber(ctx context.Context, kg, pid string, areaSqm float64, landuse string, tryV3 bool, lonlat ...float64) timberEstimate {
	t0 := time.Now()
	e := timberEstimate{ParcelID: pid, Source: "none", Species: map[string]float64{}, Assortments: map[string]timberAssortment{}}
	e.AreaHa = areaSqm / 1e4
	if e.AreaHa <= 0 {
		return e
	}
	// 1. what we know about the stand
	treeFrac, hMean, hMax, elev, slope := -1.0, 0.0, 0.0, 600.0, 8.0
	var lon, lat float64
	if len(lonlat) == 2 {
		lon, lat = lonlat[0], lonlat[1]
	}
	sf := s.standFacts(ctx, kg, pid, lon, lat)
	if sf.HasLidar {
		e.Source = "lidar"
		if sf.NE {
			e.Source = "ne"
		}
		treeFrac, elev, slope = sf.TreeFrac, sf.Elev, sf.Slope
		hMean, hMax = sf.HMean, sf.HMax
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
	// NE: the observed species of the stand beat the elevation guess
	if mix := neSpeciesMix(sf.Species); mix != nil {
		e.Species = mix
		conifer = mix["spruce_fir"] + mix["larch"] + mix["pine"]
		e.SpeciesSource = "ne"
	}
	if sf.NE {
		e.TreeN, e.TreesTall = sf.TreeN, sf.TreesTall
		if n := sf.Vitality["dead"] + sf.Vitality["declining"]; n > 0 && sf.TreeN > 0 {
			e.DeadFrac = math.Round(float64(n)/float64(sf.TreeN)*100) / 100
		}
	}

	// 2. standing stock. Ertragstafel-ish: V/ha ≈ 0.9·h_mean^1.95 for a closed canopy
	//    (h 15 → 175, 22 → 375, 27 → 560 Vfm/ha).
	var vfm float64
	canopyHa := e.AreaHa * treeFrac
	// HOLZ-3: the Hansen loss record of the plot (fast plot-context, 2.5 s budget).
	histCh := make(chan *timberHistory, 1)
	if tryV3 && e.AreaHa < 200 && len(sf.Geometry) > 2 {
		go func() { histCh <- plotHistory(sf.Geometry, canopyHa, 2500*time.Millisecond) }()
	} else {
		histCh <- nil
	}
	e.V3 = "off" // no single-tree inventory in the data tiers; lidar apex heights + heuristic instead
	e.History = <-histCh
	if e.History == nil && sf.ForestLossYr > 0 {
		// no Hansen answer, but the NE cells saw the loss themselves
		e.History = &timberHistory{LastLossYear: sf.ForestLossYr, StockFactor: 1, Source: "ne-cells"}
	}
	if vfm == 0 {
		vfm = 0.9 * math.Pow(clampF(hMean, 3, 40), 1.95) * canopyHa
		// Lidar heights already see a young stand; the NS-56 default does not.
		if e.History != nil && e.Source == "landuse" {
			vfm *= e.History.StockFactor
		}
		// Dead / declining crowns carry no sawlog value: scale the stock down
		if e.DeadFrac > 0 {
			vfm *= 1 - 0.6*e.DeadFrac
		}
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
	plon, _ := strconv.ParseFloat(q.Get("lon"), 64)
	plat, _ := strconv.ParseFloat(q.Get("lat"), 64)
	ck := "timber:" + parcelHash(pid)
	var e timberEstimate
	if cached, err := s.Q.GetCachedData(r.Context(), ck); err == nil && json.Unmarshal([]byte(cached), &e) == nil {
		w.Header().Set("X-Cache", "HIT")
	} else {
		v, _, _ := s.sf.Do(ck, func() (any, error) {
			est := s.estimateTimber(context.Background(), kg, pid, area, lu, true, plon, plat)
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
		if claim, err := s.Q.GetParcelClaim(r.Context(), dbgen.GetParcelClaimParams{SessionID: sid, ParcelHash: parcelHash(pid)}); err == nil {
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
	claim, err := s.Q.GetParcelClaim(r.Context(), dbgen.GetParcelClaimParams{SessionID: req.SessionID, ParcelHash: parcelHash(req.ParcelID)})
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
	// Pay what the popup promised: reuse the cached (v3/history-backed) estimate.
	var est timberEstimate
	if cached, err := s.Q.GetCachedData(r.Context(), "timber:"+parcelHash(req.ParcelID)); err != nil || json.Unmarshal([]byte(cached), &est) != nil || est.Coins == 0 {
		est = s.estimateTimber(r.Context(), claim.KgCode, req.ParcelID, claim.AreaSqm, lu, false)
	}
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
	s.recordHarvestState(ctx, req.SessionID, req.ParcelID, "forest", "", now, claim.Harvests+1)
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

// neSpeciesMix folds the NE species counts (spruce, fir, pine, larch, beech,
// oak, …, conifer, broadleaf) into the four price classes; nil when unknown.
func neSpeciesMix(sp map[string]int) map[string]float64 {
	tot := 0
	for k, n := range sp {
		if k != "unknown" {
			tot += n
		}
	}
	if tot < 3 {
		return nil
	}
	m := map[string]float64{"spruce_fir": 0, "larch": 0, "pine": 0, "broadleaf": 0}
	for k, n := range sp {
		v := float64(n) / float64(tot)
		switch k {
		case "spruce", "fir", "conifer":
			m["spruce_fir"] += v
		case "larch":
			m["larch"] += v
		case "pine":
			m["pine"] += v
		case "unknown":
		default:
			m["broadleaf"] += v
		}
	}
	for k, v := range m {
		m[k] = math.Round(v*100) / 100
	}
	return m
}
