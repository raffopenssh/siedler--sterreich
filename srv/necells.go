package srv

// NE cells — the observed landscape layer of srtm-lidar-at (product ≥ v2.4):
// one H3 res-12 cell (~307 m²) per record with land-cover groups, LiDAR
// heights, canopy, NDVI, change, terrain, a declared-vs-observed
// `consistency` verdict, plus every tree apex (h ≥ 3 m, species, vitality)
// and every segmented structure (type, heights, area). Same cell ids as
// umfeld-at's declared layer (/api/v1/ne). We fetch one 0.02° grid cell at a
// time as `format=columns` (quantised integers, ~20 B/cell gzipped), cache
// the raw document 24 h and derive:
//   - per-parcel enrichment in buildCell (replaces the 25 m heightfield
//     wherever a KG is NE-ready; the heightfield stays as fallback),
//   - /api/trees and /api/buildings rows (species, vitality, structure type),
//   - footprint heights (structure centroid inside the footprint ring),
//   - /api/ne heat-layer columns for the client.
// Attribution: srtm-lidar-at CC BY 4.0 (`neAttribution`).

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"sort"
	"sync"
	"time"

	"srv.exe.dev/db/dbgen"
)

const (
	neTTL         = 24 * time.Hour
	neMissTTL     = time.Hour
	neBucketDeg   = 0.0005 // ~37 × 55 m buckets for point-in-polygon scans
	neAttribution = "srtm-lidar-at landscape segmentation (CC BY 4.0) — NE cells, observed layer"
)

var neGroups = [9]string{"geb", "bau", "acker", "gruen", "wald", "wasser", "verkehr", "alpen", "sonst"}

// NE group → the srtm land-cover vocabulary the client already renders
// (DOM_TERRAIN / FRAC_COLOR / correctedFracs).
var neGroupLC = [9]string{"roof", "parking", "crop", "grass", "tree", "water", "road", "rock", "bare_soil"}

var neConsistency = []string{"consistent", "forest_loss", "forest_gain", "sealed_new", "structure_new", "green_new", "unknown"}
var nePhenology = []string{"unknown", "forest", "crop", "pasture", "seasonal_vegetation", "road_or_bare"}
var neAspect = []string{"N", "NE", "E", "SE", "S", "SW", "W", "NW", "flat"}
var neSpecies = []string{"unknown", "spruce", "fir", "pine", "larch", "beech", "oak", "maple", "ash", "birch", "alder", "poplar", "willow", "fruit", "conifer", "broadleaf"}
var neVitality = []string{"unknown", "vital", "normal", "stressed", "declining", "dead"}
var neStructTypes = []string{"roof", "greenhouse", "solar_panel", "mast", "wind_turbine", "substation", "bridge", "wall", "fence"}

// neCols is the parsed `format=columns` document of one grid cell.
type neCols struct {
	Cells          []string    `json:"cells"`
	KG             []string    `json:"kg"`
	Cover          [][9]uint8  `json:"cover"`
	StructCover    []uint8     `json:"structures_cover"`
	StructN        []uint8     `json:"structures_n"`
	HMean          []uint8     `json:"h_mean"`
	HP90           []uint8     `json:"h_p90"`
	HMax           []uint8     `json:"h_max"`
	TreeN          []uint8     `json:"tree_n"`
	TreeHMax       []uint8     `json:"tree_h_max"`
	Canopy         []uint8     `json:"canopy"`
	NDVI           []uint8     `json:"ndvi_mean"`
	NDVIAmp        []uint8     `json:"ndvi_amp"`
	DH             []int16     `json:"dh_m"`
	ALSYears       []uint8     `json:"als_years"`
	Consistency    []uint8     `json:"consistency"`
	Elev           []uint16    `json:"elev_m"`
	Slope          []uint8     `json:"slope"`
	SlopeP90       []uint8     `json:"slope_p90"`
	Aspect         []uint8     `json:"aspect"`
	Roughness      []uint8     `json:"roughness"`
	TRI            []uint8     `json:"tri"`
	TPI            []int16     `json:"tpi"`
	DTM            []int16     `json:"dtm_m"`
	ForestLossYear []uint8     `json:"forest_loss_year"`
	Phenology      []uint8     `json:"phenology"`
	Lon            []float64   `json:"lon"`
	Lat            []float64   `json:"lat"`
	Trees          neTreeCols  `json:"trees"`
	Structures     neStructCol `json:"structures"`
	Meta           struct {
		N          int      `json:"n"`
		Epoch      string   `json:"epoch"`
		KGs        []string `json:"kgs"`
		KGsMissing []string `json:"kgs_missing"`
		Partial    bool     `json:"partial"`
		CellsMiss  int      `json:"cells_missing"`
	} `json:"meta"`

	// derived
	cell    cellID
	fetched time.Time
	bCells  map[int64][]int32 // bucket → cell indices
	bTrees  map[int64][]int32
	bStruct map[int64][]int32
}

type neTreeCols struct {
	CellIndex []int32   `json:"cell_index"`
	HM        []float64 `json:"h_m"`
	CrownM2   []float64 `json:"crown_m2"`
	Species   []uint8   `json:"species"`
	Vitality  []uint8   `json:"vitality"`
	NDSMOnly  []uint8   `json:"ndsm_only"`
	Lon       []float64 `json:"lon"`
	Lat       []float64 `json:"lat"`
}

type neStructCol struct {
	CellIndex []int32   `json:"cell_index"`
	HMax      []float64 `json:"h_max_m"`
	HRobust   []float64 `json:"h_robust_m"`
	AreaM2    []float64 `json:"area_m2"`
	Type      []uint8   `json:"type"`
	Conf      []float64 `json:"conf"`
	DH        []float64 `json:"dh_m"`
	Lon       []float64 `json:"lon"`
	Lat       []float64 `json:"lat"`
}

func neBucket(lon, lat float64) int64 {
	return int64(math.Floor(lon/neBucketDeg))*1_000_000 + int64(math.Floor(lat/neBucketDeg))
}

func neIndexPts(lons, lats []float64) map[int64][]int32 {
	m := map[int64][]int32{}
	for i := range lons {
		if i >= len(lats) {
			break
		}
		k := neBucket(lons[i], lats[i])
		m[k] = append(m[k], int32(i))
	}
	return m
}

func (n *neCols) index() {
	n.bCells = neIndexPts(n.Lon, n.Lat)
	n.bTrees = neIndexPts(n.Trees.Lon, n.Trees.Lat)
	n.bStruct = neIndexPts(n.Structures.Lon, n.Structures.Lat)
}

// neScan calls fn(idx) for every indexed point inside the bbox.
func neScan(b map[int64][]int32, lons, lats []float64, minLon, minLat, maxLon, maxLat float64, fn func(i int32)) {
	bi0, bi1 := int64(math.Floor(minLon/neBucketDeg)), int64(math.Floor(maxLon/neBucketDeg))
	bj0, bj1 := int64(math.Floor(minLat/neBucketDeg)), int64(math.Floor(maxLat/neBucketDeg))
	if (bi1-bi0+1)*(bj1-bj0+1) > 40000 { // absurd bbox: bail
		return
	}
	for bi := bi0; bi <= bi1; bi++ {
		for bj := bj0; bj <= bj1; bj++ {
			for _, i := range b[bi*1_000_000+bj] {
				lo, la := lons[i], lats[i]
				if lo < minLon || lo > maxLon || la < minLat || la > maxLat {
					continue
				}
				fn(i)
			}
		}
	}
}

func metresSq(lon1, lat1, lon2, lat2 float64) float64 {
	dx := (lon2 - lon1) * math.Cos(lat1*math.Pi/180) * 111320
	dy := (lat2 - lat1) * 111320
	return dx*dx + dy*dy
}

// covered reports whether an NE cell centre lies within ~30 m of the point —
// i.e. the point is inside a processed KG. Used to merge legacy rows for the
// unprocessed part of a partially covered grid cell.
func (n *neCols) covered(lon, lat float64) bool {
	const r = 0.0004
	found := false
	neScan(n.bCells, n.Lon, n.Lat, lon-r, lat-r, lon+r, lat+r, func(i int32) {
		if !found && metresSq(lon, lat, n.Lon[i], n.Lat[i]) < 30*30 {
			found = true
		}
	})
	return found
}

// ---------------------------------------------------------------------------
// fetch + cache

type neStatus struct {
	Ready      bool     `json:"ready"`
	Status     string   `json:"status,omitempty"`
	RetryAfter float64  `json:"retry_after_s,omitempty"`
	KGsMissing []string `json:"kgs_missing,omitempty"`
}

var neHot = struct {
	sync.Mutex
	m map[cellID]*neCols
	o []cellID
}{m: map[cellID]*neCols{}}

func neHotGet(c cellID) *neCols {
	neHot.Lock()
	defer neHot.Unlock()
	v := neHot.m[c]
	if v != nil && time.Since(v.fetched) > neTTL {
		delete(neHot.m, c)
		return nil
	}
	return v
}

func neHotPut(c cellID, n *neCols) {
	neHot.Lock()
	defer neHot.Unlock()
	if _, ok := neHot.m[c]; !ok {
		neHot.o = append(neHot.o, c)
		for len(neHot.o) > 24 {
			delete(neHot.m, neHot.o[0])
			neHot.o = neHot.o[1:]
		}
	}
	neHot.m[c] = n
}

func neKey(c cellID) string { return fmt.Sprintf("ne:v1:%d:%d", c.I, c.J) }

func neParse(c cellID, raw []byte) (*neCols, *neStatus, int) {
	var st neStatus
	if json.Unmarshal(raw, &st) == nil && !st.Ready && st.Status != "" {
		return nil, &st, 404
	}
	n := &neCols{}
	if err := json.Unmarshal(raw, n); err != nil || len(n.Cells) == 0 {
		return nil, &neStatus{Status: "parse_error", RetryAfter: 600}, 502
	}
	n.cell, n.fetched = c, time.Now()
	n.index()
	neHotPut(c, n)
	return n, nil, 200
}

// neCells returns the parsed NE document for a grid cell. status 200 = ready
// (possibly partial — see Meta.Partial), 404 = nothing processed here (status
// carries retry_after_s), 502/503 = srtm error / breaker open. Cached 24 h
// (raw, gzipped by the Store), negative 1 h; a hot LRU keeps 24 parsed cells.
func (s *Server) neCells(c cellID) (*neCols, *neStatus, int) {
	if n := neHotGet(c); n != nil {
		return n, nil, 200
	}
	key := neKey(c)
	ctx := context.Background()
	if cached, err := s.Q.GetCachedData(ctx, key); err == nil {
		return neParse(c, []byte(cached))
	}
	v, _, _ := s.sf.Do(key, func() (any, error) {
		if cached, err := s.Q.GetCachedData(ctx, key); err == nil {
			return sfRes{[]byte(cached), 200}, nil
		}
		w, so, e, no := c.bbox()
		u := fmt.Sprintf("%s/cells?bbox=%.5f,%.5f,%.5f,%.5f&format=columns&centres=1&layers=obs,trees,structures", lidarAPI, w, so, e, no)
		rctx, cancel := context.WithTimeout(ctx, 40*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(rctx, "GET", u, nil)
		t0 := time.Now()
		resp, err := upstreamClient.Do(req)
		if err != nil {
			return sfRes{[]byte(`{"ready":false,"status":"error","retry_after_s":120}`), 503}, nil
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		switch resp.StatusCode {
		case 200:
			s.Q.SetCachedData(ctx, dbgen.SetCachedDataParams{CacheKey: key, Data: string(raw), ExpiresAt: time.Now().Add(neTTL)})
			slog.Info("ne cells fetched", "cell", key, "bytes", len(raw), "ms", time.Since(t0).Milliseconds())
			return sfRes{raw, 200}, nil
		case 404:
			var st neStatus
			json.Unmarshal(raw, &st)
			st.Ready = false
			if st.Status == "" {
				st.Status = "not_processed"
			}
			if st.RetryAfter <= 0 {
				st.RetryAfter = 3600
			}
			b, _ := json.Marshal(st)
			s.Q.SetCachedData(ctx, dbgen.SetCachedDataParams{CacheKey: key, Data: string(b), ExpiresAt: time.Now().Add(neMissTTL)})
			return sfRes{b, 404}, nil
		case 503:
			if resp.Header.Get("X-Upstream") == "down" {
				return sfRes{[]byte(`{"ready":false,"status":"down","retry_after_s":60}`), 503}, nil
			}
		}
		slog.Warn("ne cells: upstream", "cell", key, "status", resp.StatusCode, "body", truncStr(string(raw), 160))
		return sfRes{[]byte(fmt.Sprintf(`{"ready":false,"status":"error","retry_after_s":300,"upstream_status":%d}`, resp.StatusCode)), 502}, nil
	})
	r := v.(sfRes)
	if r.status != 200 {
		var st neStatus
		json.Unmarshal(r.body, &st)
		return nil, &st, r.status
	}
	return neParse(c, r.body)
}

// neCached returns the parsed cell without any upstream call.
func (s *Server) neCached(c cellID) *neCols {
	if n := neHotGet(c); n != nil {
		return n
	}
	cached, err := s.Q.GetCachedData(context.Background(), neKey(c))
	if err != nil {
		return nil
	}
	n, _, st := neParse(c, []byte(cached))
	if st != 200 {
		return nil
	}
	return n
}

// ---------------------------------------------------------------------------
// parcel enrichment

// neParcel is the per-parcel observed block (`ne` on the parcel row).
type neParcel struct {
	Cells        int                `json:"cells"`
	Cover        map[string]float64 `json:"cover"` // 9 groups, area shares (≥ 0.01)
	Canopy       float64            `json:"canopy"`
	HMaxM        float64            `json:"h_max_m"`
	HMeanM       float64            `json:"h_mean_m"`
	TreeN        int                `json:"tree_n"`
	TreeHMaxM    float64            `json:"tree_h_max_m,omitempty"`
	TreesTall    int                `json:"trees_tall,omitempty"` // ≥ 20 m
	Species      map[string]int     `json:"species,omitempty"`
	Vitality     map[string]int     `json:"vitality,omitempty"`
	StructN      int                `json:"structures_n"`
	StructCover  float64            `json:"structures_cover"`
	StructHMaxM  float64            `json:"structure_h_max_m,omitempty"`
	StructTypes  map[string]int     `json:"structure_types,omitempty"`
	Consistency  map[string]float64 `json:"consistency"` // code → cell share
	Verdict      string             `json:"verdict"`     // dominant non-consistent code (≥ 20 %) | consistent | unknown
	Phenology    string             `json:"phenology,omitempty"`
	NDVI         *float64           `json:"ndvi,omitempty"`
	NDVIAmp      *float64           `json:"ndvi_amp,omitempty"`
	DHM          *float64           `json:"dh_m,omitempty"`
	DTMM         *float64           `json:"dtm_m,omitempty"`
	ForestLossYr int                `json:"forest_loss_year,omitempty"`
	ALSYears     []int              `json:"als_years,omitempty"`
	Roughness    *float64           `json:"roughness,omitempty"`
	TPI          *float64           `json:"tpi,omitempty"`
	Epoch        string             `json:"epoch"`
	KGs          []string           `json:"kgs,omitempty"`
}

func r1p(v float64) *float64 {
	x := math.Round(v*10) / 10
	return &x
}
func r2p(v float64) *float64 { x := r2(v); return &x }

// neEnrichParcel aggregates the res-12 cells whose centre lies inside the
// parcel (even-odd over all rings); parcels smaller than a cell fall back to
// the nearest centre within 20 m of the representative point. Fills the
// legacy terrain fields (elev/slope/aspect/dom_terrain/fracs/tree_frac) and
// the `ne` block. Returns false when the parcel is outside NE coverage.
func neEnrichParcel(p *bevParcel, ne *neCols) bool {
	rings := geomRings(p.Geometry)
	if len(rings) == 0 {
		return false
	}
	minLon, minLat, maxLon, maxLat := ringsBounds(rings)
	var idx []int32
	neScan(ne.bCells, ne.Lon, ne.Lat, minLon, minLat, maxLon, maxLat, func(i int32) {
		if pipRingsGo(ne.Lon[i], ne.Lat[i], rings) {
			idx = append(idx, i)
		}
	})
	if len(idx) == 0 {
		const r = 0.0003
		best, bestD := int32(-1), 20.0*20.0
		neScan(ne.bCells, ne.Lon, ne.Lat, p.Lon-r, p.Lat-r, p.Lon+r, p.Lat+r, func(i int32) {
			if d := metresSq(p.Lon, p.Lat, ne.Lon[i], ne.Lat[i]); d < bestD {
				best, bestD = i, d
			}
		})
		if best < 0 {
			return false
		}
		idx = []int32{best}
	}
	// Partial coverage (parcel mostly in an unprocessed neighbour KG): a
	// handful of border cells would misrepresent the whole polygon — leave
	// it to the heightfield path.
	if exp := p.AreaSqm / 307; exp >= 4 && float64(len(idx)) < 0.3*exp {
		return false
	}
	n := float64(len(idx))
	out := &neParcel{Cells: len(idx), Cover: map[string]float64{}, Consistency: map[string]float64{}, Epoch: ne.Meta.Epoch}
	var cover [9]float64
	var canopy, canopyN, hmean, hmeanN, elevSum float64
	var elevN int
	elevMin, elevMax := math.Inf(1), math.Inf(-1)
	var slopeSum float64
	var slopeN int
	aspectCnt := map[int]int{}
	consCnt := map[int]int{}
	phenCnt := map[int]int{}
	var ndvi, ndviAmp, ndviN, dh, dhN, dtm, dtmN, rough, roughN, tpi, tpiN, scover float64
	var structN int
	alsMask := 0
	lossYr := 0
	kgSeen := map[string]bool{}
	get8 := func(a []uint8, i int32) (uint8, bool) {
		if int(i) < len(a) {
			return a[i], true
		}
		return 0, false
	}
	for _, i := range idx {
		if int(i) < len(ne.KG) {
			kgSeen[ne.KG[i]] = true
		}
		if int(i) < len(ne.Cover) {
			for g := 0; g < 9; g++ {
				cover[g] += float64(ne.Cover[i][g]) / 255
			}
		}
		if v, ok := get8(ne.Canopy, i); ok && v != 255 {
			canopy += float64(v) / 254
			canopyN++
		}
		if v, ok := get8(ne.HMax, i); ok && v != 255 {
			if h := float64(v) * 0.5; h > out.HMaxM {
				out.HMaxM = h
			}
		}
		if v, ok := get8(ne.HMean, i); ok && v != 255 {
			hmean += float64(v) * 0.5
			hmeanN++
		}
		if v, ok := get8(ne.TreeHMax, i); ok && v != 255 {
			if h := float64(v) * 0.5; h > out.TreeHMaxM {
				out.TreeHMaxM = h
			}
		}
		if int(i) < len(ne.Elev) && ne.Elev[i] != 65535 {
			z := float64(ne.Elev[i])
			elevSum += z
			elevN++
			if z < elevMin {
				elevMin = z
			}
			if z > elevMax {
				elevMax = z
			}
		}
		if v, ok := get8(ne.Slope, i); ok && v != 255 {
			slopeSum += float64(v)
			slopeN++
		}
		if v, ok := get8(ne.Aspect, i); ok && v != 255 {
			aspectCnt[int(v)]++
		}
		if v, ok := get8(ne.Consistency, i); ok {
			consCnt[int(v)]++
		}
		if v, ok := get8(ne.Phenology, i); ok && v != 255 {
			phenCnt[int(v)]++
		}
		if v, ok := get8(ne.NDVI, i); ok && v != 255 {
			ndvi += float64(v)/254*2 - 1
			if a, ok := get8(ne.NDVIAmp, i); ok && a != 255 {
				ndviAmp += float64(a) / 254
			}
			ndviN++
		}
		if int(i) < len(ne.DH) && ne.DH[i] != -128 {
			dh += float64(ne.DH[i]) * 0.25
			dhN++
		}
		if int(i) < len(ne.DTM) && ne.DTM[i] != -128 {
			dtm += float64(ne.DTM[i]) * 0.25
			dtmN++
		}
		if v, ok := get8(ne.Roughness, i); ok && v != 255 {
			rough += float64(v) / 10
			roughN++
		}
		if int(i) < len(ne.TPI) && ne.TPI[i] != -128 {
			tpi += float64(ne.TPI[i]) * 0.25
			tpiN++
		}
		if v, ok := get8(ne.ALSYears, i); ok {
			alsMask |= int(v)
		}
		if v, ok := get8(ne.ForestLossYear, i); ok && v != 0 && v != 255 {
			if y := 2000 + int(v); y > lossYr {
				lossYr = y
			}
		}
		if v, ok := get8(ne.StructCover, i); ok {
			scover += float64(v) / 255
		}
		if v, ok := get8(ne.StructN, i); ok {
			structN += int(v)
		}
	}
	// --- legacy terrain fields ---
	if elevN > 0 {
		p.Elev = r1p(elevSum / float64(elevN))
		p.ElevMin, p.ElevMax = r1p(elevMin), r1p(elevMax)
	}
	if slopeN > 0 {
		p.Slope = r1p(slopeSum / float64(slopeN))
	}
	if len(aspectCnt) > 0 {
		best, bestN := -1, 0
		for a, c := range aspectCnt {
			if c > bestN || (c == bestN && a < best) {
				best, bestN = a, c
			}
		}
		if best >= 0 && best < len(neAspect) && neAspect[best] != "flat" {
			p.Aspect = neAspect[best]
		}
	}
	fr := map[string]float64{}
	domBest, domBestV := "", 0.0
	for g := 0; g < 9; g++ {
		v := cover[g] / n
		if v >= 0.01 {
			out.Cover[neGroups[g]] = r2(v)
		}
		if v >= 0.02 {
			fr[neGroupLC[g]] = r2(v)
		}
		if lcNatural[neGroupLC[g]] && v > domBestV {
			domBest, domBestV = neGroupLC[g], v
		}
	}
	// A parcel smaller than one cell inherits a single hexagon's mix — only
	// trust the composition when it covers ≥ 2 cells or is reasonably large
	// (same rule as the heightfield path).
	if len(idx) > 1 || p.AreaSqm >= 400 {
		if len(fr) > 0 {
			p.Fracs = fr
		}
		tf := cover[4] / n
		if canopyN > 0 {
			tf = math.Max(tf, canopy/canopyN)
		}
		p.TreeFrac = r2p(tf)
		p.DomTerr = domBest
	}
	// --- observed block ---
	if canopyN > 0 {
		out.Canopy = r2(canopy / canopyN)
	}
	if hmeanN > 0 {
		out.HMeanM = math.Round(hmean/hmeanN*10) / 10
	}
	out.StructCover = r2(scover / n)
	if ndviN > 0 {
		out.NDVI = r2p(ndvi / ndviN)
		out.NDVIAmp = r2p(ndviAmp / ndviN)
	}
	if dhN > 0 {
		out.DHM = r1p(dh / dhN)
	}
	if dtmN > 0 {
		out.DTMM = r1p(dtm / dtmN)
	}
	if roughN > 0 {
		out.Roughness = r1p(rough / roughN)
	}
	if tpiN > 0 {
		out.TPI = r1p(tpi / tpiN)
	}
	for y := 0; y < 8; y++ {
		if alsMask&(1<<y) != 0 {
			out.ALSYears = append(out.ALSYears, 2020+y)
		}
	}
	out.ForestLossYr = lossYr
	bestCode, bestShare := "", 0.0
	for c, cnt := range consCnt {
		if c < len(neConsistency) {
			sh := float64(cnt) / n
			out.Consistency[neConsistency[c]] = r2(sh)
			if c != 0 && c != 6 && sh > bestShare {
				bestCode, bestShare = neConsistency[c], sh
			}
		}
	}
	switch {
	case bestShare >= 0.2:
		out.Verdict = bestCode
	case out.Consistency["consistent"] >= 0.5:
		out.Verdict = "consistent"
	default:
		out.Verdict = "unknown"
	}
	if len(phenCnt) > 0 {
		best, bestN := 0, 0
		for c, cnt := range phenCnt {
			if cnt > bestN || (cnt == bestN && c < best) {
				best, bestN = c, cnt
			}
		}
		if best < len(nePhenology) {
			out.Phenology = nePhenology[best]
		}
	}
	for kg := range kgSeen {
		out.KGs = append(out.KGs, kg)
	}
	sort.Strings(out.KGs)
	// --- actual trees / structures inside the polygon ---
	species, vital := map[string]int{}, map[string]int{}
	neScan(ne.bTrees, ne.Trees.Lon, ne.Trees.Lat, minLon, minLat, maxLon, maxLat, func(i int32) {
		if !pipRingsGo(ne.Trees.Lon[i], ne.Trees.Lat[i], rings) {
			return
		}
		out.TreeN++
		if int(i) < len(ne.Trees.HM) {
			h := ne.Trees.HM[i]
			if h >= 20 {
				out.TreesTall++
			}
			if h > out.TreeHMaxM {
				out.TreeHMaxM = h
			}
		}
		if int(i) < len(ne.Trees.Species) && int(ne.Trees.Species[i]) < len(neSpecies) && ne.Trees.Species[i] != 0 {
			species[neSpecies[ne.Trees.Species[i]]]++
		}
		if int(i) < len(ne.Trees.Vitality) && int(ne.Trees.Vitality[i]) < len(neVitality) && ne.Trees.Vitality[i] != 0 {
			vital[neVitality[ne.Trees.Vitality[i]]]++
		}
	})
	if out.TreeN == 0 && len(idx) == 1 && int(idx[0]) < len(ne.TreeN) {
		out.TreeN = int(ne.TreeN[idx[0]]) // sub-cell parcel: the cell's count
	}
	if len(species) > 0 {
		out.Species = topN(species, 4)
	}
	if len(vital) > 0 {
		out.Vitality = vital
	}
	types := map[string]int{}
	sn := 0
	neScan(ne.bStruct, ne.Structures.Lon, ne.Structures.Lat, minLon, minLat, maxLon, maxLat, func(i int32) {
		if !pipRingsGo(ne.Structures.Lon[i], ne.Structures.Lat[i], rings) {
			return
		}
		sn++
		if int(i) < len(ne.Structures.HMax) && ne.Structures.HMax[i] > out.StructHMaxM {
			out.StructHMaxM = ne.Structures.HMax[i]
		}
		if int(i) < len(ne.Structures.Type) && int(ne.Structures.Type[i]) < len(neStructTypes) {
			types[neStructTypes[ne.Structures.Type[i]]]++
		}
	})
	if sn > 0 || len(idx) > 1 {
		out.StructN = sn
	} else {
		out.StructN = structN
	}
	if len(types) > 0 {
		out.StructTypes = types
	}
	p.NE = out
	return true
}

func topN(m map[string]int, n int) map[string]int {
	type kv struct {
		k string
		v int
	}
	var all []kv
	for k, v := range m {
		all = append(all, kv{k, v})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].v > all[j].v || (all[i].v == all[j].v && all[i].k < all[j].k) })
	out := map[string]int{}
	for i, e := range all {
		if i >= n {
			break
		}
		out[e.k] = e.v
	}
	return out
}

type neFootprint struct {
	HMaxM    float64 `json:"h_max_m"`
	HRobustM float64 `json:"h_robust_m"`
	Stories  int     `json:"stories_est"`
	Type     string  `json:"type,omitempty"`
	AreaM2   float64 `json:"area_m2,omitempty"`
	DHM      float64 `json:"dh_m"`
	Conf     float64 `json:"conf,omitempty"`
}

// neEnrichFootprint attaches the tallest segmented structure whose centroid
// lies inside the footprint ring.
func neEnrichFootprint(f *bevFootprint, ne *neCols) bool {
	rings := geomRings(f.Geometry)
	if len(rings) == 0 {
		return false
	}
	minLon, minLat, maxLon, maxLat := ringsBounds(rings)
	best := int32(-1)
	st := &ne.Structures
	neScan(ne.bStruct, st.Lon, st.Lat, minLon, minLat, maxLon, maxLat, func(i int32) {
		if !pipRingsGo(st.Lon[i], st.Lat[i], rings) {
			return
		}
		if best < 0 || (int(i) < len(st.HMax) && int(best) < len(st.HMax) && st.HMax[i] > st.HMax[best]) {
			best = i
		}
	})
	if best < 0 {
		return false
	}
	i := int(best)
	fh := &neFootprint{}
	if i < len(st.HMax) {
		fh.HMaxM = st.HMax[i]
	}
	if i < len(st.HRobust) {
		fh.HRobustM = st.HRobust[i]
		fh.Stories = int(math.Max(1, math.Round(st.HRobust[i]/2.9)))
	}
	if i < len(st.Type) && int(st.Type[i]) < len(neStructTypes) {
		fh.Type = neStructTypes[st.Type[i]]
	}
	if i < len(st.AreaM2) {
		fh.AreaM2 = st.AreaM2[i]
	}
	if i < len(st.DH) {
		fh.DHM = st.DH[i]
	}
	if i < len(st.Conf) {
		fh.Conf = st.Conf[i]
	}
	f.NE = fh
	return true
}

// ---------------------------------------------------------------------------
// /api/trees and /api/buildings from NE cells

// neTreeRows: every apex ≥ minH (tallest first, ≤ limit) with species /
// vitality.
func neTreeRows(ne *neCols, minH float64, limit int) []map[string]any {
	t := &ne.Trees
	var idx []int
	for i := range t.Lon {
		if i < len(t.HM) && i < len(t.Lat) && t.HM[i] >= minH {
			idx = append(idx, i)
		}
	}
	sort.Slice(idx, func(a, b int) bool { return t.HM[idx[a]] > t.HM[idx[b]] })
	if len(idx) > limit {
		idx = idx[:limit]
	}
	out := make([]map[string]any, 0, len(idx))
	for _, i := range idx {
		row := map[string]any{"lon": t.Lon[i], "lat": t.Lat[i], "h_m": t.HM[i]}
		if i < len(t.CrownM2) && t.CrownM2[i] > 0 {
			row["crown_d_m"] = math.Round(2*math.Sqrt(t.CrownM2[i]/math.Pi)*10) / 10
		}
		if i < len(t.Species) && int(t.Species[i]) < len(neSpecies) && t.Species[i] != 0 {
			row["species"] = neSpecies[t.Species[i]]
		}
		if i < len(t.Vitality) && int(t.Vitality[i]) < len(neVitality) && t.Vitality[i] != 0 {
			row["vitality"] = neVitality[t.Vitality[i]]
		}
		out = append(out, row)
	}
	return out
}

func neBuildingRows(ne *neCols) []map[string]any {
	st := &ne.Structures
	out := make([]map[string]any, 0, len(st.Lon))
	for i := range st.Lon {
		if i >= len(st.HMax) || i >= len(st.Lat) || st.HMax[i] <= 0 {
			continue
		}
		typ := "roof"
		if i < len(st.Type) && int(st.Type[i]) < len(neStructTypes) {
			typ = neStructTypes[st.Type[i]]
		}
		if typ == "wall" || typ == "fence" {
			continue
		}
		hr := st.HMax[i]
		if i < len(st.HRobust) && st.HRobust[i] > 0 {
			hr = st.HRobust[i]
		}
		row := map[string]any{"lon": st.Lon[i], "lat": st.Lat[i], "max_height_m": st.HMax[i], "mean_height_m": hr,
			"stories_est": int(math.Max(1, math.Round(hr/2.9))), "type": typ}
		if i < len(st.AreaM2) {
			row["area_m2"] = st.AreaM2[i]
		}
		if i < len(st.DH) && st.DH[i] != 0 {
			row["dh_m"] = st.DH[i]
		}
		out = append(out, row)
	}
	return out
}

// neLayer serves /api/trees or /api/buildings for an aligned grid cell from
// NE cells when the KG is processed; partial cells merge the legacy srtm rows
// for the uncovered part; unprocessed cells fall back entirely (returns
// false, nothing written).
func (s *Server) neLayer(w http.ResponseWriter, c cellID, kind string, legacy func() ([]map[string]any, bool)) bool {
	ne, _, st := s.neCells(c)
	if st != 200 || ne == nil {
		return false
	}
	key := fmt.Sprintf("%s:ne:v1:%d:%d", kind, c.I, c.J)
	s.cachedFetch(w, key, func() ([]byte, int) {
		var rows []map[string]any
		if kind == "trees" {
			rows = neTreeRows(ne, 8, 2500)
		} else {
			rows = neBuildingRows(ne)
		}
		merged := 0
		if ne.Meta.Partial && legacy != nil {
			if lr, ok := legacy(); ok {
				for _, r := range lr {
					lon, _ := r["lon"].(float64)
					lat, _ := r["lat"].(float64)
					if !ne.covered(lon, lat) {
						rows = append(rows, r)
						merged++
					}
				}
			}
		}
		res := map[string]any{kind: rows, "ready": true, "source": "ne-cells", "epoch": ne.Meta.Epoch, "partial": ne.Meta.Partial,
			"attribution": neAttribution}
		if merged > 0 {
			res["legacy_rows"] = merged
		}
		enc, _ := json.Marshal(res)
		s.Q.SetCachedData(context.Background(), dbgen.SetCachedDataParams{CacheKey: key, Data: string(enc), ExpiresAt: time.Now().Add(neTTL)})
		return enc, 200
	})
	return true
}

// legacyBboxRows fetches the srtm bbox layer rows (same slimming as
// bboxLayer) without writing a response — fills the unprocessed part of a
// partial NE cell.
func (s *Server) legacyBboxRows(b bbox, path, extra, key string, slim func(map[string]any) map[string]any) ([]map[string]any, bool) {
	u := fmt.Sprintf("%s%s?bbox=%.5f,%.5f,%.5f,%.5f", lidarAPI, path, b.W, b.S, b.E, b.N)
	if extra != "" {
		u += "&" + extra
	}
	code, _, raw, err := upstreamGetWait(u, 6*time.Second, 20<<20)
	if err != nil || code != 200 {
		return nil, false
	}
	var d map[string]json.RawMessage
	if json.Unmarshal(raw, &d) != nil {
		return nil, false
	}
	var rows []map[string]any
	json.Unmarshal(d[key], &rows)
	out := make([]map[string]any, 0, len(rows))
	for _, m := range rows {
		if r := slim(m); r != nil {
			out = append(out, r)
		}
	}
	return out, true
}

// GET /api/ne?west&south&east&north — heat-layer columns for one aligned
// cell (lon/lat + cover/canopy/h_max/consistency/phenology/…), derived from
// the cached cell document. Not ready → 200 {ready:false, status,
// retry_after_s} (never "no data").
func (s *Server) handleNE(w http.ResponseWriter, r *http.Request) {
	b, ok := parseBBox(r.URL.Query())
	if !ok {
		jsonErr(w, "west,south,east,north required", 400)
		return
	}
	c, aligned := isAlignedCell(b.W, b.S, b.E, b.N)
	if !aligned {
		jsonErr(w, "bbox must be one aligned 0.02° grid cell", 400)
		return
	}
	ne, st, code := s.neCells(c)
	if code != 200 || ne == nil {
		if st == nil {
			st = &neStatus{Status: "error", RetryAfter: 300}
		}
		w.Header().Set("Cache-Control", "no-store")
		if st.RetryAfter > 0 {
			w.Header().Set("Retry-After", fmt.Sprintf("%.0f", st.RetryAfter))
		}
		body := map[string]any{"ready": false, "status": st.Status, "retry_after_s": st.RetryAfter, "kgs_missing": st.KGsMissing, "cell": map[string]int{"i": c.I, "j": c.J}}
		if code == 404 {
			jsonRespStatus(w, body, 200)
		} else {
			jsonRespStatus(w, body, code)
		}
		return
	}
	key := fmt.Sprintf("neheat:v2:%d:%d", c.I, c.J)
	s.cachedFetch(w, key, func() ([]byte, int) {
		n := len(ne.Cells)
		// []uint8 would be base64 in JSON — emit plain integer arrays.
		pick8 := func(a []uint8) []int {
			if len(a) > n {
				a = a[:n]
			}
			out := make([]int, len(a))
			for i, v := range a {
				out[i] = int(v)
			}
			return out
		}
		out := map[string]any{
			"ready": true, "epoch": ne.Meta.Epoch, "partial": ne.Meta.Partial, "kgs": ne.Meta.KGs, "kgs_missing": ne.Meta.KGsMissing,
			"n": n, "lon": ne.Lon, "lat": ne.Lat, "cover": ne.Cover, "canopy": pick8(ne.Canopy), "h_max": pick8(ne.HMax),
			"consistency": pick8(ne.Consistency), "phenology": pick8(ne.Phenology), "structures_cover": pick8(ne.StructCover),
			"forest_loss_year": pick8(ne.ForestLossYear), "slope": pick8(ne.Slope),
			"codes":       map[string]any{"groups": neGroups, "consistency": neConsistency, "phenology": nePhenology},
			"units":       map[string]string{"cover": "u8/255 per group", "canopy": "u8/254, 255 null", "h_max": "u8 × 0.5 m, 255 null", "slope": "deg, 255 null", "forest_loss_year": "0 none, else year−2000"},
			"attribution": neAttribution, "cell": map[string]int{"i": c.I, "j": c.J},
		}
		enc, _ := json.Marshal(out)
		s.Q.SetCachedData(context.Background(), dbgen.SetCachedDataParams{CacheKey: key, Data: string(enc), ExpiresAt: time.Now().Add(neTTL)})
		return enc, 200
	})
}

// neReadyKGSet: KG codes whose srtm product is v2.4 (= NE cells published),
// read from the cached enhanced-KG registry; padded + unpadded forms.
func (s *Server) neReadyKGSet() map[string]bool {
	out := map[string]bool{}
	raw, err := s.Q.GetCachedData(context.Background(), enhancedKGsKey)
	if err != nil {
		return out
	}
	var d struct {
		KGs []struct {
			KG  string `json:"kg_code"`
			V24 bool   `json:"v24"`
		} `json:"kgs"`
	}
	if json.Unmarshal([]byte(raw), &d) != nil {
		return out
	}
	for _, k := range d.KGs {
		if k.V24 {
			out[k.KG] = true
			out[unpadKG(k.KG)] = true
			if len(k.KG) == 4 {
				out["0"+k.KG] = true
			}
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// adoption of NE-ready KGs

func hotCellDrop(key string) {
	hotMu.Lock()
	delete(hotCells, key)
	hotMu.Unlock()
}

// neAdoptKGs runs after every registry refresh: a KG that is (newly) v2.4 or
// was regenerated gets its derived caches purged — our viewport cells
// (`vp:v1:i:j`, enriched from the heightfield before), the NE documents and
// the NE-derived tree/building layers — and is enqueued for warming so the
// cells are rebuilt with the observed layer before a player arrives. Once
// per KG per product generation (`ne-adopt:v1:<kg>` = updated_at).
func (s *Server) neAdoptKGs(gen map[string]string, v24 map[string]bool) {
	ctx := context.Background()
	adm := admin()
	adopted := 0
	for kg := range v24 {
		if len(kg) != 5 {
			continue
		}
		mark := "ne-adopt:v1:" + kg
		cur := gen[kg]
		if prev, err := s.Q.GetCachedData(ctx, mark); err == nil && prev == cur {
			continue
		}
		a := adm.KGs[kg]
		if a == nil {
			continue
		}
		for _, c := range cellsForBBox(a.MinLon, a.MinLat, a.MaxLon, a.MaxLat) {
			for _, k := range []string{fmt.Sprintf("vp:v1:%d:%d", c.I, c.J), neKey(c), fmt.Sprintf("trees:ne:v1:%d:%d", c.I, c.J),
				fmt.Sprintf("buildings:ne:v1:%d:%d", c.I, c.J), fmt.Sprintf("neheat:v2:%d:%d", c.I, c.J)} {
				s.Q.DeleteCacheLike(ctx, k)
				hotCellDrop(k)
			}
			neHot.Lock()
			delete(neHot.m, c)
			neHot.Unlock()
		}
		s.Q.DeleteCacheLike(ctx, "similar:v6:"+kg+"-%")
		s.DB.ExecContext(ctx, "DELETE FROM kg_warm WHERE kg_code = ?", kg)
		s.enqueueWarm(kg, "v24", 1)
		s.Q.SetCachedData(ctx, dbgen.SetCachedDataParams{CacheKey: mark, Data: cur, ExpiresAt: time.Now().Add(10 * 365 * 24 * time.Hour)})
		adopted++
	}
	if adopted > 0 {
		slog.Info("ne: adopted v2.4 KGs (cells purged, warm enqueued)", "kgs", adopted)
	}
}

// neDiscrepant reports whether an NE verdict means "observation disagrees with
// the cadastre" (forest_loss, forest_gain, sealed_new, structure_new, green_new).
func neDiscrepant(verdict string) bool {
	return verdict != "" && verdict != "consistent" && verdict != "unknown"
}

// neVerdictOf returns the NE consistency verdict of a parcel from our cached
// cell ("" when unknown / KG not observed). Never fetches — the claim path
// must stay instant; the client has just loaded this cell anyway.
func (s *Server) neVerdictOf(pid string, lon, lat float64) string {
	if p, ok := s.lookupParcelCached(pid, lon, lat); ok && p != nil && p.NE != nil {
		return p.NE.Verdict
	}
	return ""
}
