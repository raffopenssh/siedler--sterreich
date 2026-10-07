package srv

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Search support for the picker / in-game search bar.
//
// GET /api/search-index — the whole admin table (2 092 Gemeinden, 7 850 KGs)
// as one compact JSON document the browser indexes itself: typeahead over
// Gemeinde/KG names and codes is then a local, zero-latency operation; only
// addresses (OSM) and toponyms (BEV DLM) still go to umfeld. ~90 KB gzipped,
// strong ETag (= KG universe hash), one day of browser cache.
//
// GET /api/parcel-find?kg=12105&gnr=68/3[&lon&lat] — resolves one parcel id
// to a point: our cached cells first (free), then bevdirect /parcel near the
// hint (or the KG centre). 202 pending while bevdirect still assembles tiles.

var (
	searchIdxOnce sync.Once
	searchIdxBody []byte
	searchIdxETag string
)

func searchIndex() ([]byte, string) {
	searchIdxOnce.Do(func() {
		a := admin()
		gems := make([]*gemeindeAdmin, 0, len(a.Gemeinde))
		for _, g := range a.Gemeinde {
			gems = append(gems, g)
		}
		sort.Slice(gems, func(i, j int) bool { return gems[i].Code < gems[j].Code })
		gi := map[string]int{}
		g := make([][]any, 0, len(gems))
		for i, x := range gems {
			gi[x.Code] = i
			lon, lat := x.center()
			g = append(g, []any{x.Code, x.Name, x.District, x.State, mdeg(lon), mdeg(lat), span(x.MinLon, x.MinLat, x.MaxLon, x.MaxLat)})
		}
		kgs := append([]*kgAdmin(nil), a.all...)
		sort.Slice(kgs, func(i, j int) bool { return kgs[i].KG < kgs[j].KG })
		k := make([][]any, 0, len(kgs))
		codes := make([]string, 0, len(kgs))
		for _, x := range kgs {
			lon, lat := x.center()
			k = append(k, []any{x.KG, x.Name, gi[x.Gemeinde], mdeg(lon), mdeg(lat), span(x.MinLon, x.MinLat, x.MaxLon, x.MaxLat)})
			codes = append(codes, x.KG)
		}
		doc := map[string]any{
			"v":      2,
			"fields": map[string]any{"g": "code,name,district,state,lon_mdeg,lat_mdeg,span_mdeg", "k": "code,name,gi,lon_mdeg,lat_mdeg,span_mdeg", "note": "coordinates in 1/1000°, span = larger bbox side in 1/1000° (lat side ×1.5)"},
			"g":      g, "k": k,
			"source": a.Source,
			"notice": "© BEV VGD 1:50 000 (CC BY 4.0), bearbeitet",
		}
		searchIdxBody, _ = json.Marshal(doc)
		sum := sha256.Sum256(searchIdxBody)
		searchIdxETag = `"si2-` + hex.EncodeToString(sum[:8]) + `"`
	})
	return searchIdxBody, searchIdxETag
}

// mdeg: 1/1000° (~110 m north–south, ~75 m east–west) — plenty for a fly-to target.
func mdeg(v float64) int { return int(v*1e3 + 0.5) }

// span: the larger bbox side in 1/1000° (lat side weighted ×1.5 like the client's zoomForResult).
func span(w, so, e, n float64) int {
	d := e - w
	if (n-so)*1.5 > d {
		d = (n - so) * 1.5
	}
	return int(d*1e3 + 0.5)
}

func (s *Server) handleSearchIndex(w http.ResponseWriter, r *http.Request) {
	body, etag := searchIndex()
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=86400, stale-while-revalidate=604800")
	if strings.Contains(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

var gnrRe = regexp.MustCompile(`^\.?(\d{1,6})(?:/(\d{1,5}))?$`)

func (s *Server) handleParcelFind(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	kg := padKGCode(strings.TrimSpace(q.Get("kg")))
	gnr := strings.TrimSpace(q.Get("gnr"))
	k := admin().KGs[kg]
	if k == nil || !gnrRe.MatchString(gnr) {
		jsonErr(w, "kg (5-digit) and gnr (e.g. 68/3) required", 400)
		return
	}
	gnr = strings.TrimPrefix(gnr, ".")
	pid := kg + "-" + gnr
	lon, _ := strconv.ParseFloat(q.Get("lon"), 64)
	lat, _ := strconv.ParseFloat(q.Get("lat"), 64)
	playerSeen()

	// Cells of the KG, nearest to the hint (camera) or the KG centre first.
	// Cached cells are free; cold ones are built (and thereby cached for the
	// fly-to that follows) within a wall-clock budget, after which we answer
	// 202 + progress and the client keeps polling — every poll resumes where
	// the last one stopped because the built cells are now cached.
	cx, cy := k.center()
	if lon != 0 && lat != 0 && lon >= k.MinLon-0.01 && lon <= k.MaxLon+0.01 && lat >= k.MinLat-0.01 && lat <= k.MaxLat+0.01 {
		cx, cy = lon, lat
	}
	cells := k.cells()
	sort.Slice(cells, func(i, j int) bool {
		ai, bi := cellDist2(cells[i], cx, cy), cellDist2(cells[j], cx, cy)
		return ai < bi
	})
	if len(cells) > parcelFindMaxCells {
		cells = cells[:parcelFindMaxCells]
	}
	deadline := time.Now().Add(parcelFindBudget)
	live := 0
	for i, c := range cells {
		d := s.cachedCell(c)
		if d == nil {
			if time.Now().After(deadline) || live >= 3 {
				w.Header().Set("Cache-Control", "no-store")
				w.Header().Set("Retry-After", "2")
				jsonRespStatus(w, map[string]any{"status": "pending", "pending": true, "retry_after_s": 2, "parcel_id": pid,
					"kg_name": k.Name, "progress": map[string]any{"searched": i, "total": len(cells)}}, http.StatusAccepted)
				return
			}
			var st int
			d, st, _ = s.ensureCellStatus(c)
			live++
			if st != 200 || d == nil {
				continue
			}
		}
		for j := range d.Parcels {
			if d.Parcels[j].ParcelID == pid {
				writeParcelFind(w, &d.Parcels[j], "cells", k)
				return
			}
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	jsonResp(w, map[string]any{"found": false, "parcel_id": pid, "kg_code": kg, "kg_name": k.Name,
		"gemeinde_name": k.GemName, "searched": len(cells), "hint": "Grundstück in der KG nicht gefunden (Nummer prüfen)"})
}

const (
	parcelFindBudget   = 8 * time.Second // wall clock per request before answering 202
	parcelFindMaxCells = 40              // never sweep more than this many cells for one id
)

func cellDist2(c cellID, lon, lat float64) float64 {
	w, so, e, n := c.bbox()
	dx, dy := (w+e)/2-lon, (so+n)/2-lat
	return dx*dx + dy*dy*2.2
}

func writeParcelFind(w http.ResponseWriter, p *bevParcel, src string, k *kgAdmin) {
	w.Header().Set("Cache-Control", "private, max-age=3600")
	jsonResp(w, map[string]any{
		"found": true, "source": src,
		"parcel": map[string]any{"parcel_id": p.ParcelID, "kg_code": p.KG, "gnr": p.GNR, "ez": p.EZ,
			"area_sqm": p.AreaSqm, "lon": p.Lon, "lat": p.Lat, "building_count": p.BuildingCnt, "dominant_ns": p.DominantNS},
		"kg_name": k.Name, "gemeinde_name": k.GemName, "gemeinde_code": k.Gemeinde,
		"notice": fmt.Sprintf("© BEV, %d – Kataster (CC BY 4.0), bearbeitet", time.Now().Year()),
	})
}
