package srv

// srtm-lidar-at public-tier adapters: landscape, trees, buildings,
// landmarks, heightfield, KG registry — all point/bbox/KG-code keyed. No
// parcel or footprint ids cross the wire; the client joins by geometry.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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

const enhancedKGsKey = "enhanced-kgs:v4"

// ---------------------------------------------------------------------------
// GET /api/enhanced-kgs — srtm-processed KGs (v2 = grid25 terrain available)

func (s *Server) handleEnhancedKGs(w http.ResponseWriter, r *http.Request) {
	s.cachedFetchX(w, enhancedKGsKey, func() ([]byte, int) { return s.buildEnhancedKGs(enhancedKGsKey) }, func(b []byte) []byte {
		// Weak ETag = registry_hash (universe + per-KG product/updated_at).
		var h struct {
			RegistryHash string `json:"registry_hash"`
			Source       string `json:"source"`
		}
		if json.Unmarshal(b, &h) == nil && h.RegistryHash != "" {
			w.Header().Set("ETag", `W/"`+h.RegistryHash+`"`)
			w.Header().Set("X-Registry-Source", h.Source)
		}
		return b
	})
}

type srtmKG struct {
	KgCode       string    `json:"kg_code"`
	KgName       string    `json:"kg_name"`
	GemeindeCode string    `json:"gemeinde_code"`
	GemeindeName string    `json:"gemeinde_name"`
	Processed    bool      `json:"processed"`
	ProductVer   string    `json:"product_version"`
	UpdatedAt    string    `json:"updated_at"`
	Grid25       *bool     `json:"grid25"`
	BBox         []float64 `json:"bbox"`
	Centroid     []float64 `json:"centroid"`
	Quality      string    `json:"quality_grade"`
}

func (s *Server) buildEnhancedKGs(cacheKey string) ([]byte, int) {
	type kgEntry struct {
		KgCode       string  `json:"kg_code"`
		KgName       string  `json:"kg_name"`
		GemeindeCode string  `json:"gemeinde_code"`
		GemeindeName string  `json:"gemeinde_name"`
		Lon          float64 `json:"lon"`
		Lat          float64 `json:"lat"`
		V2           bool    `json:"v2,omitempty"`
		V24          bool    `json:"v24,omitempty"` // product v2.4 = NE cells published
		Product      string  `json:"product_version,omitempty"`
	}
	// One list of the whole KG universe (srtm /kgs, mirrored to
	// data/srtm-kgs.json.gz) — kgs[] below keeps only the processed rows,
	// which is what the picker glow and the warm planner key on.
	uni, source, err := loadSrtmKGUniverse()
	if err != nil {
		return jsonErrBody("data service error"), 502
	}
	s.noteSrtmUniverseHash(uni.UniverseHash)
	var all []kgEntry
	gen := map[string]string{}
	adm := admin()
	for _, k := range uni.KGs {
		if !k.Processed || k.ProductVer == "" {
			continue
		}
		e := kgEntry{KgCode: k.KgCode, KgName: k.KgName, GemeindeCode: k.GemeindeCode, GemeindeName: k.GemeindeName}
		if len(k.Centroid) == 2 {
			e.Lon, e.Lat = k.Centroid[0], k.Centroid[1]
		} else if a := adm.KGs[k.KgCode]; a != nil {
			e.Lon, e.Lat = a.center()
		}
		if a := adm.KGs[k.KgCode]; a != nil && e.GemeindeCode == "" {
			e.GemeindeCode, e.GemeindeName = a.Gemeinde, a.GemName
		}
		e.V2 = (k.Grid25 != nil && *k.Grid25) || isV2Product(k.ProductVer)
		e.V24 = isV24Product(k.ProductVer)
		e.Product = k.ProductVer
		all = append(all, e)
		gen[k.KgCode] = k.UpdatedAt
	}
	nv2, nv24 := 0, 0
	for _, e := range all {
		if e.V2 {
			nv2++
		}
		if e.V24 {
			nv24++
		}
	}
	ttl := 30 * time.Minute
	if source == "local-copy" {
		ttl = 5 * time.Minute // retry srtm soon
	} else {
		// Only a live answer may purge derived caches / adopt v2.4 KGs — the
		// mirror carries no news.
		s.invalidateRegeneratedKGs(gen)
	}
	out, _ := json.Marshal(map[string]any{
		"count": len(all), "v2_count": nv2, "v24_count": nv24,
		"kg_count": uni.KGsTotal, "kg_universe": kgUniverseCount,
		"universe_hash": uni.UniverseHash, "registry_hash": uni.RegistryHash,
		"source": source, "fetched_at": uni.FetchedAt, "upstream_etag": uni.ETag,
		"kgs": all,
	})
	s.Q.SetCachedData(context.Background(), dbgen.SetCachedDataParams{CacheKey: cacheKey, Data: string(out), ExpiresAt: time.Now().Add(ttl)})
	if source == "srtm" {
		v24 := map[string]bool{}
		for _, e := range all {
			if e.V24 {
				v24[e.KgCode] = true
			}
		}
		go s.neAdoptKGs(gen, v24)
	}
	return out, 200
}

// ---------------------------------------------------------------------------
// GET /api/lidar/kg/{code} — "lidar slim": KG terrain + buildings + giant
// trees + landmarks from the public bbox endpoints. parcels[] is empty now —
// per-parcel terrain rides in /api/viewport.

func (s *Server) buildLidarSlimUncached(kg string) ([]byte, int) {
	cacheKey := "lidar-slim2:/kg/" + kg
	a := admin().KGs[kg]
	if a == nil {
		return jsonErrBody("unknown kg"), 404
	}
	bb := fmt.Sprintf("%.5f,%.5f,%.5f,%.5f", a.MinLon, a.MinLat, a.MaxLon, a.MaxLat)
	// srtm bbox limit 0.03 deg²: split big KGs into chunks for the point layers.
	chunks := splitBBox(bbox{a.MinLon, a.MinLat, a.MaxLon, a.MaxLat}, 0.028)

	var (
		terrain map[string]any
		product string
		trees   []map[string]any
		objects []map[string]any
		bldgs   []map[string]any
		mu      sync.Mutex
		wg      sync.WaitGroup
		pending bool
		retryS  float64
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		body, st := s.llmGet("srtmkg:"+kg, lidarAPI+"/kg/"+kg, 6*time.Hour)
		if st == 202 {
			pending = true
			retryS = float64(retryAfterOf(body))
			return
		}
		if st != 200 {
			return
		}
		var d struct {
			Terrain    map[string]any `json:"terrain"`
			ProductVer string         `json:"product_version"`
		}
		if json.Unmarshal(body, &d) == nil {
			terrain, product = d.Terrain, d.ProductVer
		}
	}()
	fetchPts := func(path, key string, fn func(m map[string]any)) {
		for _, c := range chunks {
			wg.Add(1)
			go func(c bbox) {
				defer wg.Done()
				u := fmt.Sprintf("%s%s?bbox=%.5f,%.5f,%.5f,%.5f", lidarAPI, path, c.W, c.S, c.E, c.N)
				body, st := s.llmGet("srtm:"+path+":"+c.qs(), u, 6*time.Hour)
				if st != 200 {
					return
				}
				var d map[string]json.RawMessage
				if json.Unmarshal(body, &d) != nil {
					return
				}
				var rows []map[string]any
				json.Unmarshal(d[key], &rows)
				mu.Lock()
				for _, m := range rows {
					fn(m)
				}
				mu.Unlock()
			}(c)
		}
	}
	inKG := func(m map[string]any) bool {
		k, _ := m["kg_code"].(string)
		return k == "" || k == kg
	}
	fetchPts("/trees/bbox", "trees", func(m map[string]any) {
		if !inKG(m) {
			return
		}
		h, _ := m["height_m"].(float64)
		if h < 25 || h > 60 {
			return
		}
		t := map[string]any{"height_m": h, "lon": m["lon"], "lat": m["lat"]}
		if cd, ok := m["crown_d_m"].(float64); ok && cd > 0 && cd < 40 && cd/h > 0.62 {
			t["broad"] = 1
		}
		trees = append(trees, t)
	})
	fetchPts("/landmarks/bbox", "landmarks", func(m map[string]any) {
		if !inKG(m) {
			return
		}
		typ, _ := m["type"].(string)
		h, _ := m["height_m"].(float64)
		_, minH, maxH := landmarkClass(landmarkLetter(typ))
		if minH == 0 || h < minH || h > maxH {
			return
		}
		objects = append(objects, map[string]any{"type": typ, "height_m": math.Round(h*10) / 10, "lon": m["lon"], "lat": m["lat"]})
	})
	fetchPts("/buildings/bbox", "buildings", func(m map[string]any) {
		if !inKG(m) {
			return
		}
		roof, _ := m["roof"].(string)
		bldgs = append(bldgs, map[string]any{
			"lon": m["lon"], "lat": m["lat"], "max_height_m": m["height_max_m"], "mean_height_m": m["height_mean_m"],
			"stories_est": m["stories_est"], "roof_type_hint": roof,
		})
	})
	wg.Wait()
	if pending && terrain == nil {
		p := pendingInfo{RetryAfter: math.Max(retryS, 10)}
		return p.body(), http.StatusAccepted
	}
	// Dedupe giants on a ~15 m grid, tallest first, cap 120.
	sort.Slice(trees, func(i, j int) bool { return trees[i]["height_m"].(float64) > trees[j]["height_m"].(float64) })
	seen := map[string]bool{}
	kept := trees[:0]
	for _, t := range trees {
		lon, _ := t["lon"].(float64)
		lat, _ := t["lat"].(float64)
		k := fmt.Sprintf("%d:%d", int(lon*5000), int(lat*7000))
		if seen[k] {
			continue
		}
		seen[k] = true
		kept = append(kept, t)
		if len(kept) >= 120 {
			break
		}
	}
	sort.Slice(objects, func(i, j int) bool { return objects[i]["height_m"].(float64) > objects[j]["height_m"].(float64) })
	if len(objects) > 60 {
		objects = objects[:60]
	}
	slim := map[string]any{
		"kg_code": kg, "kg_name": a.Name, "product_version": product, "bbox": bb,
		"terrain": terrain, "parcels": []any{}, "buildings": nonNil(bldgs), "top_trees": nonNil(kept), "top_objects": nonNil(objects),
		"attribution": "Datenquelle: BEV – ALS DGM/DOM 1 m & Orthophoto DOP RGBI (CC BY 4.0, bearbeitet) · Contains modified Copernicus Sentinel data 2022–2025 · © ESA WorldCover 2021 · Hansen/UMD/Google/USGS/NASA GFC · srtm-lidar-at landscape segmentation (CC BY 4.0)",
	}
	out, err := json.Marshal(slim)
	if err != nil {
		return jsonErrBody("encode error"), 500
	}
	s.Q.SetCachedData(context.Background(), dbgen.SetCachedDataParams{CacheKey: cacheKey, Data: string(out), ExpiresAt: time.Now().Add(6 * time.Hour)})
	return out, 200
}

// splitBBox splits b into chunks of at most maxSpan° per edge.
func splitBBox(b bbox, maxSpan float64) []bbox {
	nx := int(math.Ceil((b.E - b.W) / maxSpan))
	ny := int(math.Ceil((b.N - b.S) / maxSpan))
	if nx < 1 {
		nx = 1
	}
	if ny < 1 {
		ny = 1
	}
	dx, dy := (b.E-b.W)/float64(nx), (b.N-b.S)/float64(ny)
	var out []bbox
	for i := 0; i < nx; i++ {
		for j := 0; j < ny; j++ {
			out = append(out, bbox{b.W + float64(i)*dx, b.S + float64(j)*dy, b.W + float64(i+1)*dx, b.S + float64(j+1)*dy})
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Per-tile point layers (LID-2 / LID-3) on the public endpoints.

// GET /api/trees?west&south&east&north → {trees:[{lon,lat,h_m,crown_d_m}]}
func (s *Server) handleTrees(w http.ResponseWriter, r *http.Request) {
	slim := func(m map[string]any) map[string]any {
		h, _ := m["height_m"].(float64)
		if h <= 0 {
			return nil
		}
		return map[string]any{"lon": m["lon"], "lat": m["lat"], "h_m": h, "crown_d_m": m["crown_d_m"]}
	}
	// NE cells (every apex ≥ 8 m with species/vitality) for aligned cells of
	// processed KGs; legacy /trees/bbox sample otherwise.
	if b, ok := parseBBox(r.URL.Query()); ok {
		if c, aligned := isAlignedCell(b.W, b.S, b.E, b.N); aligned {
			if s.neLayer(w, c, "trees", func() ([]map[string]any, bool) {
				return s.legacyBboxRows(b, "/trees/bbox", "min_height=8&limit=2000", "trees", slim)
			}) {
				return
			}
		}
	}
	s.bboxLayer(w, r, "trees2:", "/trees/bbox", "min_height=8&limit=2000", "trees", slim)
}

// GET /api/buildings?west&south&east&north → {buildings:[{lon,lat,max_height_m,mean_height_m,stories_est,roof_type}]}
func (s *Server) handleBuildings(w http.ResponseWriter, r *http.Request) {
	slim := func(m map[string]any) map[string]any {
		h, _ := m["height_max_m"].(float64)
		if h <= 0 {
			return nil
		}
		return map[string]any{"lon": m["lon"], "lat": m["lat"], "max_height_m": h, "mean_height_m": m["height_mean_m"],
			"stories_est": m["stories_est"], "roof_type": m["roof"], "ground_elev_m": m["ground_elev_m"]}
	}
	if b, ok := parseBBox(r.URL.Query()); ok {
		if c, aligned := isAlignedCell(b.W, b.S, b.E, b.N); aligned {
			if s.neLayer(w, c, "buildings", func() ([]map[string]any, bool) {
				return s.legacyBboxRows(b, "/buildings/bbox", "limit=3000", "buildings", slim)
			}) {
				return
			}
		}
	}
	s.bboxLayer(w, r, "bldg2:", "/buildings/bbox", "limit=3000", "buildings", slim)
}

// GET /api/landmarks?west&south&east&north → {landmarks:[{type,lon,lat,height_m}]}
func (s *Server) handleLandmarks(w http.ResponseWriter, r *http.Request) {
	s.bboxLayer(w, r, "lm:", "/landmarks/bbox", "", "landmarks", func(m map[string]any) map[string]any {
		return map[string]any{"type": m["type"], "lon": m["lon"], "lat": m["lat"], "height_m": m["height_m"]}
	})
}

// bboxLayer: srtm bbox point layer → slim rows, 6 h cache keyed by the
// quantised bbox. ready:false relayed with retry_after_s.
func (s *Server) bboxLayer(w http.ResponseWriter, r *http.Request, prefix, path, extra, key string, slim func(map[string]any) map[string]any) {
	b, ok := parseBBox(r.URL.Query())
	if !ok {
		jsonErr(w, "west,south,east,north required", 400)
		return
	}
	ckey := bboxKeyOf(b)
	s.cachedFetch(w, prefix+ckey, func() ([]byte, int) {
		u := fmt.Sprintf("%s%s?bbox=%.5f,%.5f,%.5f,%.5f", lidarAPI, path, b.W, b.S, b.E, b.N)
		if extra != "" {
			u += "&" + extra
		}
		code, hdr, raw, err := upstreamGetWait(u, 12*time.Second, 20<<20)
		if err != nil {
			return jsonErrBody("data service error"), 502
		}
		if code == http.StatusAccepted {
			return parsePending(hdr, raw).body(), http.StatusAccepted
		}
		if hdr.Get("X-Upstream") == "down" {
			return raw, 503
		}
		if code != 200 {
			return jsonErrBody("upstream error"), code
		}
		var d map[string]json.RawMessage
		if json.Unmarshal(raw, &d) != nil {
			return jsonErrBody("parse error"), 502
		}
		var rows []map[string]any
		json.Unmarshal(d[key], &rows)
		out := make([]map[string]any, 0, len(rows))
		for _, m := range rows {
			if r := slim(m); r != nil {
				out = append(out, r)
			}
		}
		ready := true
		if v, ok := d["ready"]; ok {
			json.Unmarshal(v, &ready)
		}
		res := map[string]any{key: out, "ready": ready, "source": "srtm"}
		if !ready {
			res["retry_after_s"] = 15
		}
		enc, _ := json.Marshal(res)
		if ready {
			s.Q.SetCachedData(context.Background(), dbgen.SetCachedDataParams{CacheKey: prefix + ckey, Data: string(enc), ExpiresAt: time.Now().Add(6 * time.Hour)})
		}
		return enc, 200
	})
}

// bboxParams helper variant returning only the quantised key.
func bboxKeyOf(b bbox) string {
	q := func(f float64) string { return strconv.FormatFloat(math.Round(f/0.002)*0.002, 'f', 3, 64) }
	return q(b.W) + "," + q(b.S) + "," + q(b.E) + "," + q(b.N)
}

// ---------------------------------------------------------------------------
// GET /api/landscape?west&south&east&north[&include=] — srtm landscape for a
// parcel bbox (terrain, land cover, trees, buildings). 24 h cache.

func (s *Server) handleLandscape(w http.ResponseWriter, r *http.Request) {
	b, ok := parseBBox(r.URL.Query())
	if !ok {
		jsonErr(w, "west,south,east,north required", 400)
		return
	}
	inc := r.URL.Query().Get("include")
	if inc == "" {
		inc = "trees,buildings"
	}
	key := fmt.Sprintf("ls:v1:%.5f,%.5f,%.5f,%.5f:%s", b.W, b.S, b.E, b.N, inc)
	body, st := s.landscapeFor(b, inc, key)
	w.Header().Set("Content-Type", "application/json")
	if st != 200 {
		w.WriteHeader(st)
	}
	w.Write(body)
}

func (s *Server) landscapeFor(b bbox, inc, key string) ([]byte, int) {
	u := fmt.Sprintf("%s/landscape?bbox=%.5f,%.5f,%.5f,%.5f&include=%s", lidarAPI, b.W, b.S, b.E, b.N, url.QueryEscape(inc))
	return s.llmGet(key, u, 24*time.Hour)
}

// ---------------------------------------------------------------------------
// GET /api/parcel-context?pid&lon&lat&area_sqm&building_count&footprint_area_sqm&landuse_areas=52:917,41:344&kg=
// umfeld /context for a parcel the client already holds (24 h cache per
// parcel id — the id is only our cache key, it is never sent upstream).

func (s *Server) handleParcelContext(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	lon, e1 := strconv.ParseFloat(q.Get("lon"), 64)
	lat, e2 := strconv.ParseFloat(q.Get("lat"), 64)
	if e1 != nil || e2 != nil {
		jsonErr(w, "lon,lat required", 400)
		return
	}
	pid := q.Get("pid")
	key := "pctx:v2:" + parcelHash(pid)
	if pid == "" {
		key = fmt.Sprintf("pctx:v2:%.5f,%.5f,%s", lon, lat, q.Get("area_sqm"))
	}
	vals := url.Values{}
	vals.Set("lon", strconv.FormatFloat(lon, 'f', 6, 64))
	vals.Set("lat", strconv.FormatFloat(lat, 'f', 6, 64))
	for _, k := range []string{"area_sqm", "building_count", "footprint_area_sqm", "landuse_areas", "kg", "include", "radius_m"} {
		if v := q.Get(k); v != "" {
			vals.Set(k, v)
		}
	}
	if vals.Get("include") == "" {
		vals.Set("include", neContextInclude) // + declared NE fabric (nefabric.go)
	}
	body, st := s.llmGet(key, umfeldAPI+"/context?"+vals.Encode(), 24*time.Hour)
	w.Header().Set("Content-Type", "application/json")
	if st != 200 {
		w.WriteHeader(st)
	}
	w.Write(body)
}

// ---------------------------------------------------------------------------
// GET /api/osm-lines?west&south&east&north[&cat=road,rail,water][&major=1]
// umfeld /osm/geometry for a tile (ODbL, 24 h). Same GeoJSON passthrough the
// client already parses (features[].geometry LineString + properties).

func (s *Server) handleOsmLines(w http.ResponseWriter, r *http.Request) {
	b, ok := parseBBox(r.URL.Query())
	if !ok {
		jsonErr(w, "west,south,east,north required", 400)
		return
	}
	cat := r.URL.Query().Get("cat")
	if cat == "" {
		cat = "road,rail,water"
	}
	cat = strings.Map(func(c rune) rune {
		if c == ',' || c == '_' || (c >= 'a' && c <= 'z') {
			return c
		}
		return -1
	}, cat)
	major := r.URL.Query().Get("major") == "1"
	key := "osml:v1:" + bboxKeyOf(b) + ":" + cat
	u := fmt.Sprintf("%s/osm/geometry?bbox=%.5f,%.5f,%.5f,%.5f&cat=%s&limit=4000", umfeldAPI, b.W, b.S, b.E, b.N, cat)
	if major {
		key += ":m"
		u += "&major=1"
	}
	s.serveLLM(w, key, u, 24*time.Hour)
}

// ---------------------------------------------------------------------------
// GET /api/n2k?west&south&east&north — Natura 2000 sites intersecting the
// bbox, with geometry (EEA via umfeld). Site list cached 24 h, geometry 7 d.

type n2kSite struct {
	Code     string   `json:"sitecode"`
	Name     string   `json:"sitename"`
	Type     string   `json:"sitetype"`
	Label    string   `json:"site_type_label"`
	Habitats []string `json:"habitats"`
	AreaHa   float64  `json:"area_ha"`
	BBox     struct {
		MinLon, MinLat, MaxLon, MaxLat float64
	} `json:"-"`
	RawBBox map[string]float64 `json:"bbox"`
}

func (s *Server) n2kSites() []n2kSite {
	body, st := s.llmGet("n2k:list:v1", umfeldAPI+"/natura2000/search?list=all&limit=1000", 24*time.Hour)
	if st != 200 {
		return nil
	}
	var d struct {
		Data struct {
			Results []n2kSite `json:"results"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &d) != nil {
		return nil
	}
	for i := range d.Data.Results {
		r := &d.Data.Results[i]
		r.BBox.MinLon, r.BBox.MinLat, r.BBox.MaxLon, r.BBox.MaxLat = r.RawBBox["min_lon"], r.RawBBox["min_lat"], r.RawBBox["max_lon"], r.RawBBox["max_lat"]
	}
	return d.Data.Results
}

func (s *Server) handleN2K(w http.ResponseWriter, r *http.Request) {
	b, ok := parseBBox(r.URL.Query())
	if !ok {
		jsonErr(w, "west,south,east,north required", 400)
		return
	}
	// Not cached per bbox: the site list is in memory and each site's geometry
	// is cached 7 d under n2k:site:v1:<code>. Caching the assembled tile used to
	// duplicate the multi-MB Wachau polygon into every 0.02° tile (250 MB of
	// api_cache for 240 rows).
	var hits []n2kSite
	for _, st := range s.n2kSites() {
		if st.BBox.MaxLon < b.W || st.BBox.MinLon > b.E || st.BBox.MaxLat < b.S || st.BBox.MinLat > b.N {
			continue
		}
		hits = append(hits, st)
		if len(hits) >= 12 {
			break
		}
	}
	out := make([]map[string]any, 0, len(hits))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, st := range hits {
		wg.Add(1)
		go func(st n2kSite) {
			defer wg.Done()
			body, code := s.llmGet("n2k:site:v1:"+st.Code, umfeldAPI+"/natura2000/site/"+url.PathEscape(st.Code)+"?geometry=1", 7*24*time.Hour)
			var geom json.RawMessage
			if code == 200 {
				var d struct {
					Data struct {
						Geometry json.RawMessage `json:"geometry"`
					} `json:"data"`
					Geometry json.RawMessage `json:"geometry"`
				}
				if json.Unmarshal(body, &d) == nil {
					geom = d.Data.Geometry
					if len(geom) == 0 {
						geom = d.Geometry
					}
				}
			}
			mu.Lock()
			out = append(out, map[string]any{"sitecode": st.Code, "sitename": st.Name, "sitetype": st.Type, "site_type_label": st.Label,
				"habitats": st.Habitats, "area_ha": st.AreaHa, "geometry": geom})
			mu.Unlock()
		}(st)
	}
	wg.Wait()
	sort.Slice(out, func(i, j int) bool { return out[i]["sitecode"].(string) < out[j]["sitecode"].(string) })
	enc, _ := json.Marshal(map[string]any{"sites": out, "attribution": "Source: European Environment Agency (EEA), Natura 2000 data"})
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(enc)
}

// ---------------------------------------------------------------------------
// Municipality lookups

// GET /api/municipality?lon&lat → bevdirect /municipality (Gemeinde + KG at point), 24 h cache
func (s *Server) handleMunicipalityAt(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	lon, e1 := strconv.ParseFloat(q.Get("lon"), 64)
	lat, e2 := strconv.ParseFloat(q.Get("lat"), 64)
	if e1 != nil || e2 != nil {
		jsonErr(w, "lon,lat required", 400)
		return
	}
	key := fmt.Sprintf("muni:at:v1:%.4f,%.4f", math.Round(lon*2500)/2500, math.Round(lat*2500)/2500)
	s.cachedFetch(w, key, func() ([]byte, int) {
		u := fmt.Sprintf("%s/municipality?lon=%.6f&lat=%.6f", bevAPI, lon, lat)
		code, _, raw, err := upstreamGetWait(u, 15*time.Second, 1<<20)
		if err == nil && code == 200 {
			s.Q.SetCachedData(context.Background(), dbgen.SetCachedDataParams{CacheKey: key, Data: string(raw), ExpiresAt: time.Now().Add(time.Duration(cadastreTTL))})
			return raw, 200
		}
		// Fallback: embedded register (bbox containment).
		if k := admin().kgAt(lon, lat); k != nil {
			g := admin().Gemeinde[k.Gemeinde]
			out, _ := json.Marshal(map[string]any{"kg": k, "gemeinde": map[string]any{"gemeinde_code": g.Code, "gemeinde_name": g.Name, "kg_codes": g.KGs, "state_name": g.State}, "source": "vgd-bbox"})
			return out, 200
		}
		return jsonErrBody("no municipality here"), 404
	})
}

// GET /api/municipalities?q= | ?list=all | ?state=&format=geojson | ?contains_lon&contains_lat
// → umfeld /search/municipalities (Statistik Austria). Shapes unchanged for the picker.
func (s *Server) handleMunicipalities(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	vals := url.Values{}
	for _, k := range []string{"q", "list", "state", "format", "limit", "contains_lon", "contains_lat", "id", "district"} {
		if v := q.Get(k); v != "" {
			vals.Set(k, v)
		}
	}
	if vals.Get("limit") == "" {
		vals.Set("limit", "5000")
	}
	key := "munis:v1:" + vals.Encode()
	ttl := 24 * time.Hour
	s.serveLLM(w, key, umfeldAPI+"/search/municipalities?"+vals.Encode(), ttl)
	if vals.Get("q") != "" {
		// Picker name search: the player is about to create a session for one
		// of the top hits — memoise their settlement centres now so
		// POST /api/session/create stays < 1 s (umfeld address_osm is 0.4–1.5 s).
		if body, err := s.Q.GetCachedData(context.Background(), key); err == nil {
			s.prememoSettlements([]byte(body))
		}
	}
}

var settlePrememo sync.Map // name → time of last background memo

// prememoSettlements kicks settlementCenter() in the background for the top
// hits of a municipality search body ({data:[{name,lon,lat}]}), at most 2 per
// call and once per name per 10 min (memo itself lives 24 h in api_cache).
func (s *Server) prememoSettlements(body []byte) {
	var d struct {
		Data []struct {
			Name string  `json:"name"`
			Lon  float64 `json:"lon"`
			Lat  float64 `json:"lat"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &d) != nil {
		return
	}
	n := 0
	for _, m := range d.Data {
		if m.Name == "" || m.Lon == 0 || n >= 2 {
			continue
		}
		if t, ok := settlePrememo.Load(m.Name); ok && time.Since(t.(time.Time)) < 10*time.Minute {
			continue
		}
		settlePrememo.Store(m.Name, time.Now())
		n++
		go s.settlementCenter(m.Name, m.Lon, m.Lat)
	}
}

// serveLLM writes an llmGet result with an X-Cache header. Never wrap llmGet
// in cachedFetch with the same key: both take the singleflight on that key
// and the inner call deadlocks (the request hangs until the client gives up).
func (s *Server) serveLLM(w http.ResponseWriter, key, u string, ttl time.Duration) {
	_, cached := s.Q.GetCachedData(context.Background(), key)
	body, st := s.llmGet(key, u, ttl)
	w.Header().Set("Content-Type", "application/json")
	if cached == nil {
		w.Header().Set("X-Cache", "HIT")
	} else {
		w.Header().Set("X-Cache", "MISS")
	}
	if st != 200 {
		w.WriteHeader(st)
	}
	w.Write(body)
}

var _ = slog.Info

// GET /api/giants-near?lon&lat → nearest giant trees (h ≥ 25 m) around a
// point, searched in widening rings on srtm /trees/bbox. Used by the client
// as a scout when the loaded KGs hold no giants at all (flat crop Gemeinden
// like Höflein): the golden mist then points to the nearest ones in a
// neighbouring KG. Cached 6 h per ~0.02° quantised point.
func (s *Server) handleGiantsNear(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	lon, err1 := strconv.ParseFloat(q.Get("lon"), 64)
	lat, err2 := strconv.ParseFloat(q.Get("lat"), 64)
	if err1 != nil || err2 != nil || lon < 9 || lon > 18 || lat < 46 || lat > 49.5 {
		jsonErr(w, "lon/lat required", 400)
		return
	}
	qlon, qlat := math.Floor(lon/0.02)*0.02+0.01, math.Floor(lat/0.02)*0.02+0.01
	key := fmt.Sprintf("giants-near:v1:%.2f,%.2f", qlon, qlat)
	s.cachedFetch(w, key, func() ([]byte, int) {
		type giant struct {
			Lon, Lat, H, Dist float64
			KG                string
		}
		mLon := 111320 * math.Cos(qlat*math.Pi/180)
		var found []giant
		for _, half := range []float64{0.04, 0.08, 0.16, 0.32} {
			b := bbox{qlon - half, qlat - half, qlon + half, qlat + half}
			seen := map[string]bool{}
			found = found[:0]
			var mu sync.Mutex
			var wg sync.WaitGroup
			for _, c := range splitBBox(b, 0.08) {
				wg.Add(1)
				go func(c bbox) {
					defer wg.Done()
					u := fmt.Sprintf("%s/trees/bbox?bbox=%.5f,%.5f,%.5f,%.5f&min_height=25&limit=400", lidarAPI, c.W, c.S, c.E, c.N)
					body, st := s.llmGet("srtm:/trees/bbox:g25:"+c.qs(), u, 6*time.Hour)
					if st != 200 {
						return
					}
					var d struct {
						Trees []map[string]any `json:"trees"`
					}
					if json.Unmarshal(body, &d) != nil {
						return
					}
					mu.Lock()
					defer mu.Unlock()
					for _, m := range d.Trees {
						h, _ := m["height_m"].(float64)
						tlon, _ := m["lon"].(float64)
						tlat, _ := m["lat"].(float64)
						if h < 25 || h > 60 || tlon == 0 {
							continue
						}
						k := fmt.Sprintf("%d:%d", int(tlon*5000), int(tlat*7000))
						if seen[k] {
							continue
						}
						seen[k] = true
						kg, _ := m["kg_code"].(string)
						found = append(found, giant{tlon, tlat, h, math.Hypot((tlon-lon)*mLon, (tlat-lat)*110540), kg})
					}
				}(c)
			}
			wg.Wait()
			if len(found) >= 6 {
				break
			}
		}
		sort.Slice(found, func(i, j int) bool { return found[i].Dist < found[j].Dist })
		if len(found) > 24 {
			found = found[:24]
		}
		a := admin()
		out := make([]map[string]any, 0, len(found))
		for _, g := range found {
			row := map[string]any{"lon": g.Lon, "lat": g.Lat, "height_m": g.H, "dist_m": math.Round(g.Dist), "kg_code": g.KG}
			if k := a.KGs[g.KG]; k != nil {
				row["kg_name"] = k.Name
				row["gemeinde_name"] = k.GemName
			}
			out = append(out, row)
		}
		b, _ := json.Marshal(map[string]any{"lon": lon, "lat": lat, "trees": out,
			"attribution": "Datenquelle: BEV – ALS DGM/DOM 1 m (CC BY 4.0, bearbeitet) · srtm-lidar-at landscape segmentation (CC BY 4.0)"})
		s.Q.SetCachedData(context.Background(), dbgen.SetCachedDataParams{CacheKey: key, Data: string(b), ExpiresAt: time.Now().Add(6 * time.Hour)})
		return b, 200
	})
}
