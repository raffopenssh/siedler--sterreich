package srv

// GET /api/tiles/hillshade/{z}/{x}/{y}.png — proxy for the srtm sibling's
// hillshade tiles (LID-4), so no upstream host is exposed to the browser.
//
// srtm-lidar-at's public tier budgets tile *renders* (cache misses upstream):
// 25/s per client, burst 150 → 429 + Retry-After; the renderer answers 503 +
// Retry-After when saturated. Our side of the contract (llm.txt §"Terrain
// grids and tiles"):
//   - ≤ 8 upstream tile requests in flight (tileSlots), the browser queues the
//     rest and only asks for what the viewport needs (game.js reliefTile).
//   - z ≥ 12 only (z10–11 are the most expensive renders; the client never
//     asks for them — reliefZoom() clamps at 12).
//   - 429/503 → honour Retry-After: no upstream call for that long, then
//     exponential back-off (cap 30 s). Clients get a 503 + Retry-After
//     straight from here while we pause (never a hot loop).
//   - tiles are immutable upstream (1 y, strong ETag): we keep them on disk
//     in api_cache for a year and revalidate with If-None-Match after 30 d
//     (304 = free, does not count against the render budget). 204 (no data)
//     is cached the same way.
//   - X-Tile-Source: hit|render is counted in /api/metrics (tiles{}).

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"srv.exe.dev/db/dbgen"
)

const (
	hillshadeUpstream = "https://srtm-lidar-at.exe.xyz/tiles/hillshade/"
	tileMinZoom       = 12
	tileMaxZoom       = 17
	tileMaxInflight   = 8
	tileKeepFor       = 365 * 24 * time.Hour // upstream says immutable for a year
	tileFreshFor      = 30 * 24 * time.Hour  // revalidate (If-None-Match) after this
	tileBackoffMax    = 30 * time.Second
)

// tileRec is what we keep in api_cache per tile (key hs:v2:z/x/y).
type tileRec struct {
	ETag       string `json:"etag,omitempty"`
	B64        string `json:"b64,omitempty"` // empty = 204 no data
	FreshUntil int64  `json:"fresh_until"`   // unix seconds; past it we revalidate
	Src        string `json:"-"`             // upstream X-Tile-Source of this fetch (hit|render)
}

var (
	tileSlots = make(chan struct{}, tileMaxInflight)
	tileGate  struct {
		mu         sync.Mutex
		pauseUntil time.Time
		backoff    time.Duration // last applied pause; doubles on consecutive 429/503
		lastCode   int
	}
	tileStats struct {
		served, local, hit, render, revalidated, nodata, throttled, errors atomic.Int64
	}
	tileAttribution atomic.Value // last X-Data-Attribution seen upstream
)

const tileAttributionDefault = "Datenquelle: BEV – Bundesamt für Eich- und Vermessungswesen (CC BY 4.0, bearbeitet) · Contains modified Copernicus Sentinel data · Relief via srtm-lidar-at, CC BY 4.0"

func tileAttrib() string {
	if v, ok := tileAttribution.Load().(string); ok && v != "" {
		return v
	}
	return tileAttributionDefault
}

// tilePaused reports how long we must still wait before the next upstream call.
func tilePaused() time.Duration {
	tileGate.mu.Lock()
	defer tileGate.mu.Unlock()
	return time.Until(tileGate.pauseUntil)
}

// tileThrottle records a 429/503: pause for Retry-After (or the doubled
// previous pause when absent), capped at tileBackoffMax.
func tileThrottle(code int, retryAfter string) time.Duration {
	tileGate.mu.Lock()
	defer tileGate.mu.Unlock()
	d := 2 * tileGate.backoff
	if d < time.Second {
		d = time.Second
	}
	if ra, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && ra > 0 {
		if rd := time.Duration(ra) * time.Second; rd > d {
			d = rd
		}
	}
	if d > tileBackoffMax {
		d = tileBackoffMax
	}
	tileGate.backoff = d
	tileGate.lastCode = code
	if until := time.Now().Add(d); until.After(tileGate.pauseUntil) {
		tileGate.pauseUntil = until
	}
	tileStats.throttled.Add(1)
	slog.Warn("tiles: upstream throttled", "code", code, "retry_after", retryAfter, "pause", d)
	return d
}

func tileRecovered() {
	tileGate.mu.Lock()
	tileGate.backoff = 0
	tileGate.mu.Unlock()
}

// tileMetrics for /api/metrics.
func tileMetrics() map[string]any {
	tileGate.mu.Lock()
	pause := time.Until(tileGate.pauseUntil)
	code := tileGate.lastCode
	tileGate.mu.Unlock()
	m := map[string]any{
		"served": tileStats.served.Load(), "local_cache": tileStats.local.Load(),
		"upstream_hit": tileStats.hit.Load(), "upstream_render": tileStats.render.Load(),
		"revalidated_304": tileStats.revalidated.Load(), "nodata_204": tileStats.nodata.Load(),
		"throttled": tileStats.throttled.Load(), "errors": tileStats.errors.Load(),
		"inflight": len(tileSlots), "max_inflight": tileMaxInflight,
	}
	if pause > 0 {
		m["paused_s"] = round1(pause.Seconds())
		m["paused_by"] = code
	}
	return m
}

func (s *Server) readTileRec(ctx context.Context, key, legacyKey string) (*tileRec, bool) {
	if raw, err := s.Q.GetCachedData(ctx, key); err == nil {
		var rec tileRec
		if json.Unmarshal([]byte(raw), &rec) == nil {
			return &rec, true
		}
	}
	// pre-v2 rows (plain base64 / "" for no data, 30 d): adopt as fresh-until-expiry
	if raw, err := s.Q.GetCachedData(ctx, legacyKey); err == nil {
		return &tileRec{B64: raw, FreshUntil: time.Now().Add(tileFreshFor).Unix()}, true
	}
	return nil, false
}

func (s *Server) writeTileRec(ctx context.Context, key string, rec *tileRec) {
	b, _ := json.Marshal(rec)
	s.Q.SetCachedData(ctx, dbgen.SetCachedDataParams{CacheKey: key, Data: string(b), ExpiresAt: time.Now().Add(tileKeepFor)})
}

// fetchTile does one upstream round trip under the in-flight gate.
// status: 200 (rec filled), 204 (rec empty), 304 (unchanged), 429/503
// (throttled, retryAfter set), other = error.
func (s *Server) fetchTile(path, etag string) (status int, rec *tileRec, retryAfter time.Duration) {
	if wait := tilePaused(); wait > 0 {
		return 503, nil, wait
	}
	select {
	case tileSlots <- struct{}{}:
	case <-time.After(8 * time.Second):
		return 503, nil, 2 * time.Second // gate saturated: tell the client to come back
	}
	defer func() { <-tileSlots }()
	if wait := tilePaused(); wait > 0 { // the pause may have started while we queued
		return 503, nil, wait
	}
	req, _ := http.NewRequest("GET", hillshadeUpstream+path+".png", nil)
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	resp, err := upstreamClient.Do(req)
	if err != nil {
		tileStats.errors.Add(1)
		return 502, nil, 0
	}
	defer resp.Body.Close()
	if a := resp.Header.Get("X-Data-Attribution"); a != "" && utf8.ValidString(a) && !strings.ContainsRune(a, '\uFFFD') {
		tileAttribution.Store(a)
	}
	switch resp.StatusCode {
	case 429, 503:
		ra := resp.Header.Get("Retry-After")
		if resp.Header.Get("X-Upstream") == "down" { // our own breaker's synthetic answer
			return 503, nil, 20 * time.Second
		}
		return resp.StatusCode, nil, tileThrottle(resp.StatusCode, ra)
	case 304:
		tileRecovered()
		tileStats.revalidated.Add(1)
		return 304, nil, 0
	case 204, 404:
		tileRecovered()
		tileStats.nodata.Add(1)
		return 204, &tileRec{ETag: resp.Header.Get("ETag"), FreshUntil: time.Now().Add(tileFreshFor).Unix()}, 0
	case 200:
		b, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		if err != nil || len(b) == 0 {
			tileStats.errors.Add(1)
			return 502, nil, 0
		}
		tileRecovered()
		src := resp.Header.Get("X-Tile-Source")
		if src == "render" {
			tileStats.render.Add(1)
		} else {
			src = "hit"
			tileStats.hit.Add(1)
		}
		return 200, &tileRec{ETag: resp.Header.Get("ETag"), B64: base64.StdEncoding.EncodeToString(b), FreshUntil: time.Now().Add(tileFreshFor).Unix(), Src: src}, 0
	}
	if resp.StatusCode >= 500 {
		tileStats.errors.Add(1)
	}
	return resp.StatusCode, nil, 0
}

type tileAnswer struct {
	status     int
	body       []byte
	retryAfter time.Duration
	source     string // local | hit | render | stale
}

func (s *Server) handleHillshadeTile(w http.ResponseWriter, r *http.Request) {
	z, x, y := r.PathValue("z"), r.PathValue("x"), strings.TrimSuffix(r.PathValue("y"), ".png")
	zi, err1 := strconv.Atoi(z)
	xi, err2 := strconv.Atoi(x)
	yi, err3 := strconv.Atoi(y)
	if err1 != nil || err2 != nil || err3 != nil || zi < 8 || zi > tileMaxZoom || xi < 0 || yi < 0 || xi >= 1<<zi || yi >= 1<<zi {
		http.Error(w, "bad tile", 400)
		return
	}
	w.Header().Set("X-Data-Attribution", tileAttrib())
	if zi < tileMinZoom {
		// z10–11 are the most expensive renders upstream and the game never
		// needs them (relief starts at z12.5). Cacheable "no data".
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.WriteHeader(204)
		return
	}
	path := z + "/" + x + "/" + y
	key, legacyKey := "hs:v2:"+path, "hs:"+path
	ctx := context.Background()
	tileStats.served.Add(1)

	v, _, _ := s.sf.Do(key, func() (any, error) {
		rec, have := s.readTileRec(ctx, key, legacyKey)
		if have && time.Now().Unix() < rec.FreshUntil {
			tileStats.local.Add(1)
			return s.tileFromRec(rec, "local"), nil
		}
		etag := ""
		if have {
			etag = rec.ETag
		}
		st, fresh, ra := s.fetchTile(path, etag)
		switch st {
		case 200, 204:
			s.writeTileRec(ctx, key, fresh)
			src := fresh.Src
			if src == "" {
				src = "hit"
			}
			return s.tileFromRec(fresh, src), nil
		case 304:
			rec.FreshUntil = time.Now().Add(tileFreshFor).Unix()
			s.writeTileRec(ctx, key, rec)
			return s.tileFromRec(rec, "local"), nil
		}
		if have { // stale but usable beats an error
			return s.tileFromRec(rec, "stale"), nil
		}
		if st == 429 || st == 503 {
			return tileAnswer{status: 503, retryAfter: ra}, nil
		}
		return tileAnswer{status: st}, nil
	})
	a := v.(tileAnswer)
	switch a.source {
	case "local", "stale":
		w.Header().Set("X-Cache", "HIT")
	case "":
	default:
		w.Header().Set("X-Cache", "MISS")
	}
	if a.source != "" {
		w.Header().Set("X-Tile-Source", a.source)
	}
	switch a.status {
	case 200:
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Write(a.body)
	case 204:
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.WriteHeader(204)
	case 503:
		ra := int(a.retryAfter.Seconds() + 0.999)
		if ra < 1 {
			ra = 1
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Retry-After", strconv.Itoa(ra))
		w.Header().Set("X-Upstream", "busy")
		w.WriteHeader(503)
	default:
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(a.status)
	}
}

func (s *Server) tileFromRec(rec *tileRec, source string) tileAnswer {
	if rec.B64 == "" {
		return tileAnswer{status: 204, source: source}
	}
	b, err := base64.StdEncoding.DecodeString(rec.B64)
	if err != nil || len(b) == 0 {
		return tileAnswer{status: 204, source: source}
	}
	return tileAnswer{status: 200, body: b, source: source}
}
