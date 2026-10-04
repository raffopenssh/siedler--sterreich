package srv

// Similar parcels "in der Nähe": candidates come from the cadastre cells we
// already hold around the reference point (explored / prewarmed area,
// ≤ 24 h old). Score 0..1 on size ratio,
// Benützungsart composition (measured m² shares), built density and the
// 25 m terrain enrichment (elevation, slope, aspect, dominant natural cover,
// land-cover histogram). Cached 1 h per reference parcel (similar:v6:).
//
// GET /api/similar?parcel_id=&lon=&lat=&area=&bcount=&barea=&lu=&limit=40
// Response: {parcel_id, radius_m, cells, candidates, scored, lidar_terms,
//            source:"cells", ref{…}, results[{score, parts, parcel_id, kg_code,
//            gnr, ez, lon, lat, area_sqm, distance_m, slope, elev, aspect,
//            forest_frac, dom, fracs, landuse_summary, building_count}], took_ms}

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"sort"
	"strconv"
	"time"

	"srv.exe.dev/db/dbgen"
)

type similarReq struct {
	pid      string
	lon, lat float64
	area     float64
	lu       string
	bcount   int
	barea    float64
	radius   float64
	limit    int
}

func (s *Server) handleSimilarParcels(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	pid := q.Get("parcel_id")
	lon, _ := strconv.ParseFloat(q.Get("lon"), 64)
	lat, _ := strconv.ParseFloat(q.Get("lat"), 64)
	area, _ := strconv.ParseFloat(q.Get("area"), 64)
	if pid == "" || lon == 0 || lat == 0 || area <= 0 {
		jsonErr(w, "parcel_id, lon, lat, area required", 400)
		return
	}
	bcount, _ := strconv.Atoi(q.Get("bcount"))
	barea, _ := strconv.ParseFloat(q.Get("barea"), 64)
	limit := 40
	if v, err := strconv.Atoi(q.Get("limit")); err == nil && v > 0 && v <= 200 {
		limit = v
	}
	radius := 3000.0 // "in der Nähe": the cells around the point we already hold
	if v, err := strconv.ParseFloat(q.Get("radius"), 64); err == nil && v >= 500 && v <= 6000 {
		radius = v
	}
	payload, hit, err := s.similarJSON(r.Context(), similarReq{pid, lon, lat, area, q.Get("lu"), bcount, barea, radius, limit})
	if err != nil {
		jsonErr(w, err.Error(), 502)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", map[bool]string{true: "HIT", false: "MISS"}[hit])
	w.Write(payload)
}

func (s *Server) similarJSON(ctx context.Context, rq similarReq) ([]byte, bool, error) {
	t0 := time.Now()
	// The candidate pool is whatever cells we hold; the key includes the cell
	// count so a search re-run after the player loaded more map sees them.
	cands, cells := s.parcelsNear(rq.lon, rq.lat, rq.radius, 30)
	cacheKey := "similar:v7:" + parcelHash(rq.pid) + ":" + strconv.Itoa(int(rq.radius)) + ":" + strconv.Itoa(rq.limit) + ":" + strconv.Itoa(cells)
	if c, err := s.Q.GetCachedData(ctx, cacheKey); err == nil {
		return []byte(c), true, nil
	}
	var ref *bevParcel
	for i := range cands {
		if cands[i].ParcelID == rq.pid {
			ref = &cands[i]
			break
		}
	}
	if ref == nil {
		if p, ok := s.lookupParcel(rq.pid, rq.lon, rq.lat); ok {
			ref = p
		} else {
			ref = &bevParcel{ParcelID: rq.pid, Lon: rq.lon, Lat: rq.lat, AreaSqm: rq.area, BuildingCnt: rq.bcount, BuildingArea: rq.barea, DominantNS: rq.lu}
		}
	}
	refLU := landuseShares(ref)
	refBuilt := builtRatio(ref)
	refTerr := ref.Elev != nil
	aspectIdx := map[string]int{"N": 0, "NE": 1, "E": 2, "SE": 3, "S": 4, "SW": 5, "W": 6, "NW": 7}

	score := func(c *bevParcel) (float64, map[string]float64) {
		parts := map[string]float64{}
		sz := math.Min(c.AreaSqm, ref.AreaSqm) / math.Max(math.Max(c.AreaSqm, ref.AreaSqm), 1)
		parts["size"] = sz
		// landuse: histogram intersection of measured Benützungsart shares
		lu := 0.0
		cl := landuseShares(c)
		for k, v := range refLU {
			lu += math.Min(v, cl[k])
		}
		if len(refLU) == 0 || len(cl) == 0 {
			lu = 0.5
		}
		parts["landuse"] = lu
		bd := 1 - math.Min(1, math.Abs(builtRatio(c)-refBuilt)*4)
		if (ref.BuildingCnt > 0) != (c.BuildingCnt > 0) {
			bd *= 0.5
		}
		parts["built"] = bd
		terr := 0.5
		if refTerr && c.Elev != nil {
			t := 0.0
			n := 0.0
			if ref.Slope != nil && c.Slope != nil {
				t += 1 - math.Min(1, math.Abs(*ref.Slope-*c.Slope)/20)
				n++
			}
			t += 1 - math.Min(1, math.Abs(*ref.Elev-*c.Elev)/300)
			n++
			if ref.Aspect != "" && c.Aspect != "" {
				d := (aspectIdx[ref.Aspect] - aspectIdx[c.Aspect] + 8) % 8
				if d > 4 {
					d = 8 - d
				}
				t += 1 - float64(d)/4
				n++
			}
			if ref.DomTerr != "" && c.DomTerr != "" {
				if ref.DomTerr == c.DomTerr {
					t += 1
				}
				n++
			}
			if len(ref.Fracs) > 0 && len(c.Fracs) > 0 {
				hi := 0.0
				for k, v := range ref.Fracs {
					hi += math.Min(v, c.Fracs[k])
				}
				t += hi * 1.8
				n += 1.8
			}
			if n > 0 {
				terr = t / n
			}
		}
		parts["terrain"] = terr
		w := map[string]float64{"size": 0.25, "landuse": 0.3, "built": 0.15, "terrain": 0.3}
		if !refTerr {
			w = map[string]float64{"size": 0.35, "landuse": 0.45, "built": 0.2, "terrain": 0}
		}
		tot := 0.0
		for k, v := range w {
			tot += v * parts[k]
			parts[k] = math.Round(parts[k]*100) / 100
		}
		return tot, parts
	}

	type scored struct {
		Score    float64            `json:"score"`
		Parts    map[string]float64 `json:"parts"`
		ParcelID string             `json:"parcel_id"`
		KgCode   string             `json:"kg_code"`
		Gnr      string             `json:"gnr"`
		Ez       string             `json:"ez"`
		Lon      float64            `json:"lon"`
		Lat      float64            `json:"lat"`
		AreaSqm  float64            `json:"area_sqm"`
		DistM    float64            `json:"distance_m"`
		Slope    *float64           `json:"slope,omitempty"`
		Elev     *float64           `json:"elev,omitempty"`
		Aspect   string             `json:"aspect,omitempty"`
		Forest   *float64           `json:"forest_frac,omitempty"`
		Dom      string             `json:"dom,omitempty"`
		Fracs    map[string]float64 `json:"fracs,omitempty"`
		Landuse  map[string]any     `json:"landuse_summary,omitempty"`
		BCount   int                `json:"building_count"`
	}
	var out []scored
	scoredN := 0
	for i := range cands {
		c := &cands[i]
		if c.ParcelID == rq.pid || !c.Complete {
			continue
		}
		// size band: 1/6 … 6× (anything else is never "similar")
		if r := c.AreaSqm / math.Max(ref.AreaSqm, 1); r < 1.0/6 || r > 6 {
			continue
		}
		if rq.lu != "" && c.DominantNS != "" && c.DominantNS != rq.lu {
			continue
		}
		scoredN++
		sc, parts := score(c)
		rec := scored{Score: math.Round(sc*1000) / 1000, Parts: parts, ParcelID: c.ParcelID, KgCode: c.KG, Gnr: c.GNR, Ez: c.EZ,
			Lon: c.Lon, Lat: c.Lat, AreaSqm: math.Round(c.AreaSqm), DistM: math.Round(distM(rq.lon, rq.lat, c.Lon, c.Lat)), BCount: c.BuildingCnt,
			Slope: c.Slope, Elev: c.Elev, Aspect: c.Aspect, Forest: c.TreeFrac, Dom: c.DomTerr, Fracs: c.Fracs}
		if len(c.LanduseAreas) > 0 {
			ls := map[string]any{}
			for k, v := range c.LanduseAreas {
				ls[k] = math.Round(v)
			}
			rec.Landuse = ls
		}
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	cut := len(out)
	for i, r := range out {
		if r.Score < 0.62 {
			cut = i
			break
		}
	}
	out = out[:cut]
	if len(out) > rq.limit {
		out = out[:rq.limit]
	}
	if out == nil {
		out = []scored{}
	}
	resp := map[string]any{
		"parcel_id": rq.pid, "radius_m": rq.radius, "cells": cells, "candidates": len(cands), "scored": scoredN,
		"lidar_terms": refTerr, "source": "cells", "scope": "vicinity", "results": out, "took_ms": time.Since(t0).Milliseconds(),
		"notice": bevNotice, // cadastre-derived rows (parcel ids, folio, land use) — must be shown
	}
	if refTerr {
		resp["ref"] = map[string]any{"slope": ref.Slope, "elev": ref.Elev, "aspect": ref.Aspect, "forest_frac": ref.TreeFrac, "dom": ref.DomTerr, "fracs": ref.Fracs}
	}
	payload, _ := json.Marshal(resp)
	// Short cache: the explored area grows while the player pans.
	s.Q.SetCachedData(ctx, dbgen.SetCachedDataParams{CacheKey: cacheKey, Data: string(payload), ExpiresAt: time.Now().Add(10 * time.Minute)})
	return payload, false, nil
}

func builtRatio(p *bevParcel) float64 {
	if p.AreaSqm <= 0 {
		return 0
	}
	return math.Min(1, p.BuildingArea/p.AreaSqm)
}
