package srv

// GET /api/metrics — cheap ops snapshot: per-route request counts and
// latency percentiles (bounded reservoir, memory stays flat), cache hit
// ratio from the X-Cache headers handlers set, bevdirect-serve /health
// passthrough (1 s timeout, cached 5 s), warm planner stats, api_cache size
// and process vitals. `?format=text` prints a compact table for humans.
//
// Collected by metricsMiddleware, which wraps the mux directly (inside the
// gzip/ratelimit/maintenance layers) so r.Pattern is the registered mux
// pattern ("GET /api/session/{id}/parcels") rather than the raw path.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const metricsReservoir = 512

var processStart = time.Now()

// routeStats is one route's bounded sample ring + counters.
type routeStats struct {
	mu      sync.Mutex
	count   uint64
	err5xx  uint64
	err4xx  uint64
	down503 uint64 // breaker / busy 503s (X-Upstream: down|busy) — not our fault, reported separately
	cache   map[string]uint64
	samples [metricsReservoir]float64 // latency ms ring
	n       int                       // samples filled (≤ metricsReservoir)
	pos     int                       // next write position
	maxMs   float64
}

func (rs *routeStats) record(ms float64, status int, xcache string, down bool) {
	rs.mu.Lock()
	rs.count++
	if status >= 500 {
		if down {
			rs.down503++
		} else {
			rs.err5xx++
		}
	} else if status >= 400 {
		rs.err4xx++
	}
	if xcache != "" {
		if rs.cache == nil {
			rs.cache = map[string]uint64{}
		}
		rs.cache[xcache]++
	}
	rs.samples[rs.pos] = ms
	rs.pos = (rs.pos + 1) % metricsReservoir
	if rs.n < metricsReservoir {
		rs.n++
	}
	if ms > rs.maxMs {
		rs.maxMs = ms
	}
	rs.mu.Unlock()
}

type routeSnapshot struct {
	Route   string            `json:"route"`
	Count   uint64            `json:"count"`
	P50Ms   float64           `json:"p50_ms"`
	P95Ms   float64           `json:"p95_ms"`
	MaxMs   float64           `json:"max_ms"`
	Err5xx  uint64            `json:"err_5xx"`
	Err4xx  uint64            `json:"err_4xx"`
	Down503 uint64            `json:"upstream_down_503,omitempty"`
	Cache   map[string]uint64 `json:"cache,omitempty"`
	Samples int               `json:"samples"`
}

func (rs *routeStats) snapshot(route string) routeSnapshot {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	out := routeSnapshot{Route: route, Count: rs.count, Err5xx: rs.err5xx, Err4xx: rs.err4xx, Down503: rs.down503, MaxMs: round1(rs.maxMs), Samples: rs.n}
	if len(rs.cache) > 0 {
		out.Cache = make(map[string]uint64, len(rs.cache))
		for k, v := range rs.cache {
			out.Cache[k] = v
		}
	}
	if rs.n > 0 {
		buf := make([]float64, rs.n)
		copy(buf, rs.samples[:rs.n])
		sort.Float64s(buf)
		out.P50Ms = round1(buf[pctIdx(rs.n, 0.50)])
		out.P95Ms = round1(buf[pctIdx(rs.n, 0.95)])
	}
	return out
}

func pctIdx(n int, p float64) int {
	i := int(float64(n-1)*p + 0.5)
	if i < 0 {
		i = 0
	}
	if i >= n {
		i = n - 1
	}
	return i
}

func round1(f float64) float64 { return float64(int64(f*10+0.5)) / 10 }

// metricsRegistry holds all routes. Routes are bounded by the mux pattern
// set, so the map cannot grow with traffic.
type metricsRegistry struct {
	mu     sync.RWMutex
	routes map[string]*routeStats
	total  atomic.Uint64

	healthMu sync.Mutex
	healthAt time.Time
	health   map[string]any

	// api_cache SUM(LENGTH(data)) scans every blob (~200 ms on a 200 MB
	// cache), so the result is cached 30 s (`?fresh=1` bypasses).
	cacheMu sync.Mutex
	cacheAt time.Time
	cache   map[string]any
}

var metrics = &metricsRegistry{routes: map[string]*routeStats{}}

func (m *metricsRegistry) route(name string) *routeStats {
	m.mu.RLock()
	rs := m.routes[name]
	m.mu.RUnlock()
	if rs != nil {
		return rs
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if rs = m.routes[name]; rs == nil {
		if len(m.routes) >= 256 { // safety valve against pattern explosion
			name = "other"
			if rs = m.routes[name]; rs != nil {
				return rs
			}
		}
		rs = &routeStats{}
		m.routes[name] = rs
	}
	return rs
}

// metricsRecorder captures the status code; it keeps Flush (SSE) working.
type metricsRecorder struct {
	http.ResponseWriter
	status int
}

func (w *metricsRecorder) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *metricsRecorder) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func (w *metricsRecorder) Flush() {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// normaliseRoute collapses a path to a stable route name when the mux did
// not set r.Pattern (e.g. redirects / 404 before matching).
func normaliseRoute(method, path string) string {
	switch {
	case strings.HasPrefix(path, "/static/"):
		path = "/static/"
	case strings.HasPrefix(path, "/api/tiles/hillshade/"):
		path = "/api/tiles/hillshade/{z}/{x}/{y}"
	case strings.HasPrefix(path, "/api/session/"):
		parts := strings.SplitN(strings.TrimPrefix(path, "/api/session/"), "/", 2)
		path = "/api/session/{id}"
		if len(parts) == 2 {
			path += "/" + parts[1]
		}
	case strings.HasPrefix(path, "/api/player/"):
		parts := strings.SplitN(strings.TrimPrefix(path, "/api/player/"), "/", 2)
		path = "/api/player/{id}"
		if len(parts) == 2 {
			path += "/" + parts[1]
		}
	case strings.HasPrefix(path, "/api/umfeld/"):
		path = "/api/umfeld/"
	case strings.HasPrefix(path, "/api/lidar/"):
		path = "/api/lidar/"
	case strings.HasPrefix(path, "/join/"), strings.HasPrefix(path, "/rejoin/"):
		path = path[:strings.Index(path[1:], "/")+2] + "{code}"
	default:
		if i := strings.LastIndexByte(path, '/'); i > 0 && len(path) > i+1 && strings.ContainsAny(path[i+1:], "0123456789") {
			path = path[:i+1] + "{id}"
		}
	}
	return method + " " + path
}

// metricsMiddleware wraps the mux. Must be the innermost layer so that
// r.Pattern is populated and handler-set headers (X-Cache, X-Upstream) are
// readable after ServeHTTP.
func metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &metricsRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		ms := float64(time.Since(start).Microseconds()) / 1000
		route := r.Pattern
		if route == "" {
			route = normaliseRoute(r.Method, r.URL.Path)
		}
		st := rec.status
		if st == 0 {
			st = http.StatusOK
		}
		h := w.Header()
		metrics.total.Add(1)
		metrics.route(route).record(ms, st, h.Get("X-Cache"), h.Get("X-Upstream") != "") // down | busy: sibling's fault, not ours
	})
}

// bevHealth fetches bevdirect-serve /health with a 1 s timeout, cached 5 s.
func (m *metricsRegistry) bevHealth() map[string]any {
	m.healthMu.Lock()
	defer m.healthMu.Unlock()
	if m.health != nil && time.Since(m.healthAt) < 5*time.Second {
		return m.health
	}
	t0 := time.Now()
	out := map[string]any{"ok": false}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", bevAPI+"/health", nil)
	resp, err := http.DefaultTransport.RoundTrip(req) // local, no breaker/pool needed
	if err != nil {
		out["error"] = err.Error()
	} else {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		resp.Body.Close()
		var h map[string]any
		if json.Unmarshal(body, &h) == nil {
			out = h
			delete(out, "notice")
		} else {
			out["error"] = fmt.Sprintf("status %d", resp.StatusCode)
		}
		out["http_status"] = resp.StatusCode
	}
	out["probe_ms"] = round1(float64(time.Since(t0).Microseconds()) / 1000)
	out["checked_at"] = time.Now().UTC().Format(time.RFC3339)
	m.health, m.healthAt = out, time.Now()
	return out
}

// apiCacheStats: row count, total bytes, oldest live vp: cell age (cached 30 s).
func (s *Server) apiCacheStats(fresh bool) map[string]any {
	metrics.cacheMu.Lock()
	defer metrics.cacheMu.Unlock()
	if !fresh && metrics.cache != nil && time.Since(metrics.cacheAt) < 30*time.Second {
		return metrics.cache
	}
	out := s.apiCacheStatsQuery()
	metrics.cache, metrics.cacheAt = out, time.Now()
	return out
}

func (s *Server) apiCacheStatsQuery() map[string]any {
	t0 := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	out := map[string]any{}
	var rows int64
	var bytes *int64
	if err := s.DB.QueryRowContext(ctx, "SELECT COUNT(*), SUM(LENGTH(CAST(data AS BLOB))) FROM api_cache").Scan(&rows, &bytes); err != nil {
		out["error"] = err.Error()
		return out
	}
	out["rows"] = rows
	out["query_ms"] = round1(float64(time.Since(t0).Microseconds()) / 1000)
	if bytes != nil {
		out["bytes"] = *bytes
		out["mb"] = round1(float64(*bytes) / 1e6)
	} else {
		out["bytes"] = 0
	}
	var vpRows int64
	var oldest *string
	if err := s.DB.QueryRowContext(ctx, "SELECT COUNT(*), MIN(fetched_at) FROM api_cache WHERE cache_key LIKE 'vp:%'").Scan(&vpRows, &oldest); err == nil {
		out["vp_rows"] = vpRows
		if oldest != nil {
			if t, err := parseSQLiteTime(*oldest); err == nil {
				out["vp_oldest_age_s"] = int64(time.Since(t).Seconds())
				out["vp_oldest_fetched_at"] = t.UTC().Format(time.RFC3339)
			}
		}
	}
	return out
}

func (s *Server) metricsSnapshot(fresh bool) map[string]any {
	metrics.mu.RLock()
	names := make([]string, 0, len(metrics.routes))
	for n := range metrics.routes {
		names = append(names, n)
	}
	metrics.mu.RUnlock()
	sort.Strings(names)
	routes := make([]routeSnapshot, 0, len(names))
	cache := map[string]uint64{}
	var total5xx, total4xx, down uint64
	for _, n := range names {
		metrics.mu.RLock()
		rs := metrics.routes[n]
		metrics.mu.RUnlock()
		snap := rs.snapshot(n)
		routes = append(routes, snap)
		for k, v := range snap.Cache {
			cache[k] += v
		}
		total5xx += snap.Err5xx
		total4xx += snap.Err4xx
		down += snap.Down503
	}
	var hits, misses uint64
	for k, v := range cache {
		if k == "HIT" || k == "WARM" {
			hits += v
		} else {
			misses += v
		}
	}
	cacheOut := map[string]any{"by_header": cache, "hits": hits, "misses": misses}
	if hits+misses > 0 {
		cacheOut["hit_ratio"] = round1(100*float64(hits)/float64(hits+misses)) / 100
	}
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return map[string]any{
		"now":            time.Now().UTC().Format(time.RFC3339),
		"uptime_s":       int64(time.Since(processStart).Seconds()),
		"requests_total": metrics.total.Load(),
		"errors":         map[string]uint64{"5xx": total5xx, "4xx": total4xx, "upstream_down_503": down},
		"routes":         routes,
		"cache":          cacheOut,
		"bevdirect":      metrics.bevHealth(),
		"warm":           s.warmStatusMap(),
		"api_cache":      s.apiCacheStats(fresh),
		"process": map[string]any{
			"goroutines":    runtime.NumGoroutine(),
			"heap_alloc_mb": round1(float64(ms.HeapAlloc) / 1e6),
			"heap_sys_mb":   round1(float64(ms.HeapSys) / 1e6),
			"heap_objects":  ms.HeapObjects,
			"num_gc":        ms.NumGC,
			"go":            runtime.Version(),
		},
	}
}

// GET /api/metrics[?format=text][&fresh=1]
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	snap := s.metricsSnapshot(r.URL.Query().Get("fresh") == "1")
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.Query().Get("format") != "text" {
		jsonResp(w, snap)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	var b strings.Builder
	fmt.Fprintf(&b, "siedler metrics  %s  uptime %ds  requests %d\n", snap["now"], snap["uptime_s"], snap["requests_total"])
	errs := snap["errors"].(map[string]uint64)
	fmt.Fprintf(&b, "errors: 5xx=%d 4xx=%d upstream_down_503=%d\n", errs["5xx"], errs["4xx"], errs["upstream_down_503"])
	c := snap["cache"].(map[string]any)
	fmt.Fprintf(&b, "cache: hits=%d misses=%d", c["hits"], c["misses"])
	if hr, ok := c["hit_ratio"]; ok {
		fmt.Fprintf(&b, " ratio=%.2f", hr)
	}
	fmt.Fprintf(&b, " by_header=%v\n", c["by_header"])
	bev := snap["bevdirect"].(map[string]any)
	fmt.Fprintf(&b, "bevdirect: ok=%v cells_cached=%v prefetch_queue=%v probe_ms=%v", bev["ok"], bev["cells_cached"], bev["prefetch_queue"], bev["probe_ms"])
	if e, ok := bev["error"]; ok {
		fmt.Fprintf(&b, " error=%v", e)
	}
	b.WriteString("\n")
	wm := snap["warm"].(map[string]any)
	fmt.Fprintf(&b, "warm: kgs=%v gemeinden=%v queue=%v current=%q done=%v cells_built=%v\n", wm["warm_kgs"], wm["warm_gemeinden"], wm["queue_len"], wm["current"], wm["done"], wm["cells_built"])
	ac := snap["api_cache"].(map[string]any)
	fmt.Fprintf(&b, "api_cache: rows=%v mb=%v vp_rows=%v vp_oldest_age_s=%v\n", ac["rows"], ac["mb"], ac["vp_rows"], ac["vp_oldest_age_s"])
	p := snap["process"].(map[string]any)
	fmt.Fprintf(&b, "process: goroutines=%v heap_alloc_mb=%v heap_sys_mb=%v num_gc=%v\n\n", p["goroutines"], p["heap_alloc_mb"], p["heap_sys_mb"], p["num_gc"])
	fmt.Fprintf(&b, "%-48s %8s %9s %9s %9s %5s %5s %5s  %s\n", "route", "count", "p50_ms", "p95_ms", "max_ms", "5xx", "4xx", "down", "cache")
	for _, rt := range snap["routes"].([]routeSnapshot) {
		cache := ""
		if len(rt.Cache) > 0 {
			keys := make([]string, 0, len(rt.Cache))
			for k := range rt.Cache {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			parts := make([]string, 0, len(keys))
			for _, k := range keys {
				parts = append(parts, fmt.Sprintf("%s=%d", k, rt.Cache[k]))
			}
			cache = strings.Join(parts, " ")
		}
		fmt.Fprintf(&b, "%-48s %8d %9.1f %9.1f %9.1f %5d %5d %5d  %s\n", rt.Route, rt.Count, rt.P50Ms, rt.P95Ms, rt.MaxMs, rt.Err5xx, rt.Err4xx, rt.Down503, cache)
	}
	io.WriteString(w, b.String())
}
