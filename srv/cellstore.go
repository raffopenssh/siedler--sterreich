package srv

// Read-side helpers over the cached cadastre cells (api_cache `vp:v1:i:j`,
// ≤ 24 h). Similar parcels, treasure placement, agent look/inspect and timber
// stand facts all read the cells we already hold — "in der Nähe" means
// within the explored/warm area.

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"sort"
	"sync"
	"time"

	"srv.exe.dev/db/dbgen"
)

type cellData struct {
	Parcels    []bevParcel
	Footprints []bevFootprint
	At         time.Time
}

var (
	cellMemMu sync.Mutex
	cellMem   = map[cellID]*cellData{} // parsed-cell LRU-ish (bounded below)
)

// cachedCell returns the parsed cell if it is in api_cache (no fetch).
func (s *Server) cachedCell(c cellID) *cellData {
	cellMemMu.Lock()
	if d, ok := cellMem[c]; ok && time.Since(d.At) < 10*time.Minute {
		cellMemMu.Unlock()
		return d
	}
	cellMemMu.Unlock()
	key := fmt.Sprintf("%s%d:%d", vpKeyPrefix, c.I, c.J)
	raw, err := s.Q.GetCachedData(context.Background(), key)
	if err != nil {
		return nil
	}
	var d struct {
		Parcels    []bevParcel    `json:"parcels"`
		Footprints []bevFootprint `json:"footprints"`
	}
	if json.Unmarshal([]byte(raw), &d) != nil {
		return nil
	}
	cd := &cellData{Parcels: d.Parcels, Footprints: d.Footprints, At: time.Now()}
	cellMemMu.Lock()
	if len(cellMem) > 48 {
		for k := range cellMem { // drop a few arbitrary entries
			delete(cellMem, k)
			if len(cellMem) <= 32 {
				break
			}
		}
	}
	cellMem[c] = cd
	cellMemMu.Unlock()
	return cd
}

// ensureCell builds the cell if needed (blocking, singleflight) and returns
// it parsed. ok=false when upstream is not ready / failed.
func (s *Server) ensureCell(c cellID) (*cellData, bool) {
	d, st, _ := s.ensureCellStatus(c)
	return d, st == 200
}

// cellStatus is what ensureCellStatus knows about a cell build: the HTTP-ish
// status (200 ready, 202 still assembling, 502/503 upstream error) plus the
// retry hint for 202/503 — the same vocabulary parsePending() uses, so agent
// endpoints can relay a proper 202 + Retry-After instead of a bare error.
type cellStatus struct {
	Status     int
	RetryAfter float64 // seconds; > 0 on 202 / breaker 503
	Body       []byte  // upstream/breaker body on 5xx (relayable)
}

// ensureCellStatus is ensureCell with the reason: data is non-nil on 200 and
// 202 (partial rows while bevdirect is still assembling the cell).
func (s *Server) ensureCellStatus(c cellID) (*cellData, int, cellStatus) {
	if d := s.cachedCell(c); d != nil {
		return d, 200, cellStatus{Status: 200}
	}
	w, so, e, n := c.bbox()
	b := bbox{w, so, e, n}
	key, _ := vpCacheKey(b)
	s.warm.fg.Add(1)
	body, st := s.cellJSON(b, key)
	s.warm.fg.Add(-1)
	if st != 200 {
		cs := cellStatus{Status: st, Body: body}
		if st == 503 {
			cs.RetryAfter = float64(retryAfterOf(body))
		}
		return nil, st, cs
	}
	var d struct {
		Parcels    []bevParcel    `json:"parcels"`
		Footprints []bevFootprint `json:"footprints"`
		Ready      bool           `json:"ready"`
		RetryAfter float64        `json:"retry_after_s"`
	}
	if json.Unmarshal(body, &d) != nil {
		return nil, 502, cellStatus{Status: 502, Body: jsonErrBody("cadastre cell decode error")}
	}
	cd := &cellData{Parcels: d.Parcels, Footprints: d.Footprints, At: time.Now()}
	if !d.Ready {
		ra := d.RetryAfter
		if ra <= 0 {
			ra = 3
		}
		return cd, 202, cellStatus{Status: 202, RetryAfter: ra}
	}
	return cd, 200, cellStatus{Status: 200}
}

// parcelsNear collects parcels from cached cells within radiusM of the
// point, nearest cell first, at most maxCells cells. Only cells we already
// hold are read — nothing is fetched. Returns parcels + number of cells read.
func (s *Server) parcelsNear(lon, lat, radiusM float64, maxCells int) ([]bevParcel, int) {
	dLat := radiusM / 111320
	dLon := radiusM / (111320 * math.Cos(lat*math.Pi/180))
	cells := cellsForBBox(lon-dLon, lat-dLat, lon+dLon, lat+dLat)
	sort.Slice(cells, func(i, j int) bool {
		di := math.Hypot(float64(cells[i].I)+0.5-lon/gridDeg, (float64(cells[i].J)+0.5-lat/gridDeg)/0.68)
		dj := math.Hypot(float64(cells[j].I)+0.5-lon/gridDeg, (float64(cells[j].J)+0.5-lat/gridDeg)/0.68)
		return di < dj
	})
	var out []bevParcel
	seen := map[string]bool{}
	read := 0
	for _, c := range cells {
		if read >= maxCells {
			break
		}
		d := s.cachedCell(c)
		if d == nil {
			continue
		}
		read++
		for _, p := range d.Parcels {
			if seen[p.ParcelID] || (!p.Complete && seen[p.ParcelID+"!"]) {
				continue
			}
			if distM(lon, lat, p.Lon, p.Lat) > radiusM {
				continue
			}
			seen[p.ParcelID] = true
			out = append(out, p)
		}
	}
	return out, read
}

// lookupParcel finds one parcel: cached cell under (lon,lat) first, then
// bevdirect /parcel/{id}?lon&lat (24 h cache). lon/lat may be 0 when the
// parcel is expected in cache (we try the KG bbox cells then).
func (s *Server) lookupParcel(pid string, lon, lat float64) (*bevParcel, bool) {
	if p, ok := s.lookupParcelCached(pid, lon, lat); ok {
		return p, true
	}
	if lon == 0 || lat == 0 {
		return nil, false
	}
	key := "parcel:v1:" + pid
	u := fmt.Sprintf("%s/parcel/%s?lon=%.6f&lat=%.6f", bevAPI, url.PathEscape(pid), lon, lat)
	body, st := s.llmGetBudget(key, u, time.Duration(cadastreTTL), 15*time.Second)
	if st != 200 {
		return nil, false
	}
	var d struct {
		Parcel *bevParcel `json:"parcel"`
	}
	if json.Unmarshal(body, &d) != nil || d.Parcel == nil {
		return nil, false
	}
	return d.Parcel, true
}

// footprintsOf returns cached footprints of a parcel (cell under its point).
func (s *Server) footprintsOf(p *bevParcel) []bevFootprint {
	d := s.cachedCell(cellOf(p.Lon, p.Lat))
	if d == nil {
		return nil
	}
	var out []bevFootprint
	for _, f := range d.Footprints {
		if f.ParcelID == p.ParcelID {
			out = append(out, f)
		}
	}
	return out
}

// llmGetBudget is llmGet with a custom timeout (bevdirect may assemble live).
func (s *Server) llmGetBudget(key, u string, ttl, budget time.Duration) ([]byte, int) {
	ctx := context.Background()
	if c, err := s.Q.GetCachedData(ctx, key); err == nil {
		return []byte(c), 200
	}
	v, _, _ := s.sf.Do(key, func() (any, error) {
		if c, err := s.Q.GetCachedData(ctx, key); err == nil {
			return sfRes{[]byte(c), 200}, nil
		}
		code, _, body, err := upstreamGetWait(u, budget, 40<<20)
		if err != nil {
			return sfRes{jsonErrBody("data service error"), 502}, nil
		}
		if code == 200 {
			s.Q.SetCachedData(ctx, dbgen.SetCachedDataParams{CacheKey: key, Data: string(body), ExpiresAt: time.Now().Add(ttl)})
		}
		return sfRes{body, code}, nil
	})
	r := v.(sfRes)
	return r.body, r.status
}

// parcelBBox returns the parcel geometry bbox (falls back to a square around
// the point sized by area).
func parcelBBox(p *bevParcel) bbox {
	if b, ok := bboxOfRaw(p.Geometry); ok {
		return b
	}
	r := math.Sqrt(math.Max(p.AreaSqm, 100)) / 2
	dLat := r / 111320
	dLon := r / (111320 * math.Cos(p.Lat*math.Pi/180))
	return bbox{p.Lon - dLon, p.Lat - dLat, p.Lon + dLon, p.Lat + dLat}
}

// landuseShares normalises landuse_areas into shares.
func landuseShares(p *bevParcel) map[string]float64 {
	tot := 0.0
	for _, v := range p.LanduseAreas {
		tot += v
	}
	out := map[string]float64{}
	if tot <= 0 {
		if p.DominantNS != "" {
			out[p.DominantNS] = 1
		}
		return out
	}
	for k, v := range p.LanduseAreas {
		out[k] = v / tot
	}
	return out
}
