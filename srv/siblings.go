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
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"srv.exe.dev/db/dbgen"
)

const farmAPI = "https://farm-subsidies-austria.exe.xyz"

// hostSlots: per-host concurrency limiter for the bbox layer proxies.
// farm-subsidies answers HTTP 503 under ~10 req/s of cold cells (soak test
// 2026-10-03: 7 relayed 5xx, then the breaker opened for 25 s and every farm
// layer went dark). A viewport pan fans out up to 12 tiles × layers, so
// bursts queue here (FIFO-ish, ≤ slotWait) instead of failing upstream.
// Hosts not listed are unlimited (the pooled client caps them at 32 conns).
var hostSlots = map[string]chan struct{}{
	hostOf(farmAPI): make(chan struct{}, 3),
}

const slotWait = 8 * time.Second

// acquireSlot blocks until a slot for the upstream's host is free (or
// slotWait passed). ok=false means "busy — tell the client to retry".
func acquireSlot(upstream string) (release func(), ok bool) {
	ch := hostSlots[hostOf(upstream)]
	if ch == nil {
		return func() {}, true
	}
	select {
	case ch <- struct{}{}:
		return func() { <-ch }, true
	case <-time.After(slotWait):
		return func() {}, false
	}
}

// busyBody is the 503 relayed when a sibling is overloaded (its own 503, or
// no slot within slotWait). Same vocabulary as the breaker body so clients
// pace on retry_after_s; status "busy" (not "down") keeps the client from
// declaring the service offline.
func busyBody(upstream string, retryAfter int) []byte {
	b, _ := json.Marshal(map[string]any{
		"error": "data service busy, retry shortly", "status": "busy",
		"service": serviceLabel(hostOf(upstream)), "retry_after_s": retryAfter,
	})
	return b
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
		release, ok := acquireSlot(upstream)
		if !ok {
			return busyBody(upstream, 4), 503
		}
		resp, err := upstreamGet(upstream + qs)
		release()
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
		if resp.Header.Get("X-Upstream") == "down" {
			return raw, 503 // breaker body: {status:"down", retry_after_s} for client pacing
		}
		if resp.StatusCode == 503 || resp.StatusCode == 429 {
			// The sibling itself is overloaded (farm under a cold-cell burst):
			// relay as a paced "busy" 503, not as an error of ours.
			ra := 5
			if v, err := strconv.Atoi(strings.TrimSpace(resp.Header.Get("Retry-After"))); err == nil && v > 0 && v < 120 {
				ra = v
			}
			return busyBody(upstream, ra), 503
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

// GET /api/schlaege?west&south&east&north  (FARM-2)
// INVEKOS field polygons: the real crop on every field. Open AMA data, CC BY 4.0.
func (s *Server) handleSchlaege(w http.ResponseWriter, r *http.Request) {
	s.bboxProxy(w, r, "schlaege:", farmAPI+"/api/schlaege?limit=3000&", "fields",
		map[string]bool{"snar_code": true}, []string{"year", "source", "license"}, 24*time.Hour)
}

// GET /api/hofstellen?west&south&east&north  (FARM-4)
// AMA INVEKOS farmsteads (hashed ids, no names): the real "home" of a farm's EZ.
func (s *Server) handleHofstellen(w http.ResponseWriter, r *http.Request) {
	s.bboxProxyOpt(w, r, "hof:", farmAPI+"/api/hofstellen?", "points",
		nil, []string{"year", "source", "license"}, 24*time.Hour, true)
}

// prewarmKGs enqueues KGs in our own cell warmer (low priority).
func (s *Server) prewarmKGs(kgs []string) {
	for i, kg := range kgs {
		if i >= 30 {
			break
		}
		s.enqueueWarm(kg, "flowpath", 2)
	}
}

// kgsAlongPath samples a LineString every ~stepM metres and resolves the KG
// under each sample from the embedded VGD register (bbox containment).
func kgsAlongPath(coords [][]float64, stepM float64) []string {
	if len(coords) < 2 {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	acc := stepM
	for i := 1; i < len(coords) && len(out) < 60; i++ {
		a, b := coords[i-1], coords[i]
		if len(a) < 2 || len(b) < 2 {
			continue
		}
		dx := (b[0] - a[0]) * 111320 * math.Cos(a[1]*math.Pi/180)
		dy := (b[1] - a[1]) * 110574
		acc += math.Hypot(dx, dy)
		if acc >= stepM {
			acc = 0
			if k := admin().kgAt(b[0], b[1]); k != nil && !seen[k.KG] {
				seen[k.KG] = true
				out = append(out, k.KG)
			}
		}
	}
	return out
}

var kgCodeRe = regexp.MustCompile(`"kg(?:_code)?"\s*:\s*"(\d{5})"`)
