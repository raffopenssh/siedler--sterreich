package srv

// Consumers of the sibling-service roadmap (/llm/ahead) that landed upstream:
//   ALL-4  POST /api/v1/prewarm       — warm a session's KGs on create (cadastre + srtm)
//   CAD-1  /spatial/landuse            — viewport landuse backdrop instead of whole-KG export
//   FARM-2 /api/schlaege               — INVEKOS field polygons → real crops per field
//   HOLZ-2 /data/prices/state/{n}.json — slim regional timber prices (see timber.go)
// CAD-2 (dominant_ns / landuse_areas) needs no server code: the viewport proxy
// passes parcel props through and game.js reads them (extractLuCode).

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
	"strconv"
	"strings"
	"time"

	"srv.exe.dev/db/dbgen"
)

const farmAPI = "https://farm-subsidies-austria.exe.xyz"

// prewarmMunicipality (ALL-4) asks cadastre + srtm to pull every KG of the
// municipality from Zenodo *now*, so the first viewport tiles don't hit the
// 202 / ready:false path. Fire-and-forget; idempotent upstream.
func (s *Server) prewarmMunicipality(muniCode, muniName string) {
	q := muniName
	if q == "" {
		q = muniCode
	}
	if q == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", cadastreAPI+"/lookup?type=kg&limit=50&q="+url.QueryEscape(q), nil)
	resp, err := upstreamClient.Do(req)
	if err != nil {
		return
	}
	var d struct {
		Data []struct {
			KG  string `json:"kg_code"`
			Gem string `json:"gemeinde_code"`
		} `json:"data"`
	}
	json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&d)
	resp.Body.Close()
	var kgs []string
	for _, r := range d.Data {
		if r.KG == "" || (muniCode != "" && unpadKG(r.Gem) != unpadKG(muniCode)) {
			continue
		}
		kgs = append(kgs, fmt.Sprintf("%05s", r.KG))
		if len(kgs) >= 50 {
			break
		}
	}
	if len(kgs) == 0 {
		return
	}
	list := strings.Join(kgs, ",")
	for _, base := range []string{cadastreAPI, lidarAPI} {
		go func(base string) {
			c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			req, _ := http.NewRequestWithContext(c, "POST", base+"/prewarm?kgs="+list, nil)
			if r, err := upstreamClient.Do(req); err == nil {
				io.Copy(io.Discard, io.LimitReader(r.Body, 64<<10))
				r.Body.Close()
			}
		}(base)
	}
	log.Printf("prewarm %s: %d KGs", q, len(kgs))
}

// bboxParams validates west/south/east/north and returns (query string, cache
// suffix quantized to ~150 m so small pans reuse the same upstream slice).
func bboxParams(r *http.Request) (qs, key string, ok bool) {
	q := r.URL.Query()
	parts := make([]string, 0, 4)
	keys := make([]string, 0, 4)
	for _, k := range []string{"west", "south", "east", "north"} {
		f, err := strconv.ParseFloat(q.Get(k), 64)
		if err != nil {
			return "", "", false
		}
		parts = append(parts, k+"="+strconv.FormatFloat(f, 'f', 5, 64))
		keys = append(keys, strconv.FormatFloat(math.Round(f/0.002)*0.002, 'f', 3, 64))
	}
	return strings.Join(parts, "&"), strings.Join(keys, ","), true
}

// bboxProxy is the shared shape for viewport-sliced layers: fetch one upstream
// bbox endpoint, keep only `arrayKey` (+ passthrough scalars), round coords,
// drop props the client never reads, cache when upstream reports ready.
func (s *Server) bboxProxy(w http.ResponseWriter, r *http.Request, cachePrefix, upstream, arrayKey string, drop map[string]bool, scalars []string, ttl time.Duration) {
	s.bboxProxyOpt(w, r, cachePrefix, upstream, arrayKey, drop, scalars, ttl, false)
}

// bboxProxyOpt: points=true keeps rows without geometry (lon/lat point layers).
func (s *Server) bboxProxyOpt(w http.ResponseWriter, r *http.Request, cachePrefix, upstream, arrayKey string, drop map[string]bool, scalars []string, ttl time.Duration, points bool) {
	qs, key, ok := bboxParams(r)
	if !ok {
		jsonErr(w, "west,south,east,north required", 400)
		return
	}
	s.cachedFetch(w, cachePrefix+key, func() ([]byte, int) {
		resp, err := upstreamGet(upstream + qs)
		if err != nil {
			return jsonErrBody("data service error"), 502
		}
		defer resp.Body.Close()
		raw, err := io.ReadAll(io.LimitReader(resp.Body, 50<<20))
		if err != nil {
			return jsonErrBody("data service error"), 502
		}
		if resp.StatusCode == http.StatusAccepted {
			p := parsePending(resp.Header, raw)
			return p.body(), http.StatusAccepted
		}
		if resp.StatusCode != 200 {
			return jsonErrBody("upstream error"), resp.StatusCode
		}
		var parsed map[string]json.RawMessage
		if json.Unmarshal(raw, &parsed) != nil {
			return jsonErrBody("bad upstream body"), 502
		}
		ready, truncated := true, false
		if v, ok := parsed["ready"]; ok {
			json.Unmarshal(v, &ready)
		}
		json.Unmarshal(parsed["truncated"], &truncated)
		var arr []map[string]any
		json.Unmarshal(parsed[arrayKey], &arr)
		var b bytes.Buffer
		fmt.Fprintf(&b, `{"%s":[`, arrayKey)
		n := 0
		for _, it := range arr {
			if points {
				if it["lon"] == nil || it["lat"] == nil {
					continue
				}
			} else if it["geometry"] == nil {
				continue
			}
			for k := range drop {
				delete(it, k)
			}
			roundCoords(it)
			enc, err := json.Marshal(it)
			if err != nil {
				continue
			}
			if n > 0 {
				b.WriteByte(',')
			}
			b.Write(enc)
			n++
		}
		fmt.Fprintf(&b, `],"count":%d,"ready":%v,"truncated":%v`, n, ready, truncated)
		for _, k := range scalars {
			if v, ok := parsed[k]; ok {
				fmt.Fprintf(&b, `,"%s":%s`, k, v)
			}
		}
		if !ready {
			p := parsePending(resp.Header, raw)
			fmt.Fprintf(&b, `,"retry_after_s":%g,"warming":%s`, p.RetryAfter, p.body())
		}
		b.WriteByte('}')
		out := b.Bytes()
		if ready {
			s.Q.SetCachedData(context.Background(), dbgen.SetCachedDataParams{CacheKey: cachePrefix + key, Data: string(out), ExpiresAt: time.Now().Add(ttl)})
		}
		return out, 200
	})
}

// GET /api/viewport-landuse?west&south&east&north  (CAD-1)
// Landuse polygons for just the bbox — replaces streaming the ~7 MB whole-KG
// landuse export for non-enhanced KGs. tolerance_m=1 is invisible at any zoom
// we render and cuts ~35 % of the payload.
func (s *Server) handleViewportLanduse(w http.ResponseWriter, r *http.Request) {
	s.bboxProxy(w, r, "vplanduse:", cadastreAPI+"/spatial/landuse?limit=5000&tolerance_m=1&", "landuse",
		map[string]bool{"name": true, "abbr": true}, nil, 6*time.Hour)
}

// GET /api/schlaege?west&south&east&north  (FARM-2)
// INVEKOS field polygons: the real crop on every field. Open AMA data, CC BY 4.0.
func (s *Server) handleSchlaege(w http.ResponseWriter, r *http.Request) {
	s.bboxProxy(w, r, "schlaege:", farmAPI+"/api/schlaege?limit=3000&", "fields",
		map[string]bool{"snar_code": true}, []string{"year", "source", "license"}, 24*time.Hour)
}

// GET /api/trees?west&south&east&north  (LID-2)
// Tree apices from the srtm-lidar index: the ≤5 tallest measured trees per
// cadastre parcel with height + crown diameter. Anchors the forest sprites
// where the real dominant trees stand and feeds giant-tree gameplay for
// every indexed KG (not only the ones whose 1 MB lidar-slim is loaded).
func (s *Server) handleTrees(w http.ResponseWriter, r *http.Request) {
	s.bboxProxyOpt(w, r, "trees:", lidarAPI+"/trees/bbox?min_height=8&limit=2000&", "trees",
		map[string]bool{"kg_code": true}, []string{"source"}, 6*time.Hour, true)
}

// GET /api/hofstellen?west&south&east&north  (FARM-4)
// AMA INVEKOS farmsteads (hashed ids, no names): the real "home" of a farm's EZ.
func (s *Server) handleHofstellen(w http.ResponseWriter, r *http.Request) {
	s.bboxProxyOpt(w, r, "hof:", farmAPI+"/api/hofstellen?", "points",
		nil, []string{"year", "source", "license"}, 24*time.Hour, true)
}
