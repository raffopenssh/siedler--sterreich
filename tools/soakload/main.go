// soakload — load test for Siedler Österreich (run via tools/soak.sh).
//
// Simulates N virtual users for D seconds across three Gemeinden:
//
//	cold  — a Gemeinde whose KGs are not warm (≤ 9 distinct cells: 3×3 around
//	        the centre, so the live BEV assembly stays polite),
//	warm  — /api/lucky pick (all KGs cached),
//	lucky — a second /api/lucky pick (may coincide with warm when the pool is
//	        small; reported).
//
// Each user registers, creates a session, then loops: grid-aligned
// /api/viewport cell (random walk ±2 cells) + trees/buildings/osm-lines/n2k/
// schlaege for the same cell, session treasures + parcels, and occasionally a
// POST /api/claim-parcel + /api/similar for a parcel from the viewport.
//
// Prints per-role/per-endpoint count, p50, p95, max, 5xx, X-Cache HIT ratio
// and asserts (exit 1 on failure):
//   - p95 < 1.5 s for /api/viewport answers with X-Cache HIT|WARM,
//   - zero non-breaker 5xx (503 + X-Upstream: down counted separately; only
//     fails when > 1 % of all requests),
//   - bevdirect-serve MemoryCurrent < MemoryMax (systemctl show),
//   - api_cache bounded: total bytes ≤ 1.5 GB and growth ≤ 400 MB.
//
// Flags: -base, -users, -seconds, -admin (path to admin.json.gz), -cold-cells,
// -claim-every, -v.
package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const cellDeg = 0.02

var (
	base       = flag.String("base", "http://localhost:8000", "server base URL")
	users      = flag.Int("users", 50, "concurrent virtual users")
	seconds    = flag.Int("seconds", 60, "run duration")
	adminPath  = flag.String("admin", "srv/data/admin.json.gz", "embedded admin table (for the cold pick)")
	coldCells  = flag.Int("cold-cells", 9, "max distinct cells for the cold Gemeinde (≤ 9)")
	claimEvery = flag.Int("claim-every", 6, "claim a parcel every Nth loop iteration")
	verbose    = flag.Bool("v", false, "log every request")
	cookie     = flag.String("cookie", "siedler_dev=1", "cookie sent with every request (maintenance bypass)")
)

var client = &http.Client{Timeout: 90 * time.Second, Transport: &http.Transport{MaxIdleConnsPerHost: 128, MaxConnsPerHost: 0, IdleConnTimeout: 90 * time.Second}}

// ---- stats --------------------------------------------------------------

type sample struct {
	role, ep string
	ms       float64
	status   int
	xcache   string
	down     bool
}

type agg struct {
	ms                []float64
	n, e5, e4, down   int
	connErr           int // transport errors (connection refused / reset / timeout)
	hit, miss, warmXC int
}

type stats struct {
	mu       sync.Mutex
	byRE     map[string]*agg // role|ep
	all      agg
	vpWarmMs []float64 // viewport latencies with X-Cache HIT/WARM (any role)
	notReady atomic.Int64
}

func newStats() *stats { return &stats{byRE: map[string]*agg{}} }

func (s *stats) add(sm sample) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := sm.role + "|" + sm.ep
	a := s.byRE[k]
	if a == nil {
		a = &agg{}
		s.byRE[k] = a
	}
	for _, x := range []*agg{a, &s.all} {
		x.ms = append(x.ms, sm.ms)
		x.n++
		if sm.status == 0 {
			x.connErr++
		} else if sm.status >= 500 {
			if sm.down {
				x.down++
			} else {
				x.e5++
			}
		} else if sm.status >= 400 {
			x.e4++
		}
		switch sm.xcache {
		case "HIT":
			x.hit++
		case "WARM":
			x.warmXC++
		case "":
		default:
			x.miss++
		}
	}
	if sm.ep == "/api/viewport" && (sm.xcache == "HIT" || sm.xcache == "WARM") && sm.status == 200 {
		s.vpWarmMs = append(s.vpWarmMs, sm.ms)
	}
}

func pct(v []float64, p float64) float64 {
	if len(v) == 0 {
		return 0
	}
	c := append([]float64(nil), v...)
	sort.Float64s(c)
	i := int(float64(len(c)-1)*p + 0.5)
	return c[i]
}

func maxf(v []float64) float64 {
	m := 0.0
	for _, x := range v {
		if x > m {
			m = x
		}
	}
	return m
}

// ---- http helpers --------------------------------------------------------

type vuser struct {
	id      int
	role    string
	token   string
	player  string
	session string
	xff     string
}

func (u *vuser) do(ctx context.Context, st *stats, method, ep, rawURL string, body any) (int, []byte, http.Header, error) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, *base+rawURL, rd)
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("Cookie", *cookie)
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("User-Agent", "siedler-soakload/1")
	if u != nil {
		// Each virtual user looks like its own client to the per-IP buckets
		// (register is 5 burst / 2 min per IP; the server trusts XFF).
		req.Header.Set("X-Forwarded-For", u.xff)
		if u.token != "" {
			req.Header.Set("X-Player-Token", u.token)
		}
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	t0 := time.Now()
	resp, err := client.Do(req)
	ms := float64(time.Since(t0).Microseconds()) / 1000
	if err != nil {
		if st != nil {
			role := "setup"
			if u != nil {
				role = u.role
			}
			st.add(sample{role: role, ep: ep, ms: ms, status: 0})
		}
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	var r io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(resp.Body)
		if err == nil {
			defer gz.Close()
			r = gz
		}
	}
	b, _ := io.ReadAll(r)
	ms = float64(time.Since(t0).Microseconds()) / 1000
	if st != nil {
		role := "setup"
		if u != nil {
			role = u.role
		}
		st.add(sample{role: role, ep: ep, ms: ms, status: resp.StatusCode, xcache: resp.Header.Get("X-Cache"), down: resp.Header.Get("X-Upstream") == "down"})
	}
	if *verbose {
		fmt.Fprintf(os.Stderr, "%-5s u%02d %s %s %d %.0fms %s\n", roleOf(u), idOf(u), method, rawURL, resp.StatusCode, ms, resp.Header.Get("X-Cache"))
	}
	return resp.StatusCode, b, resp.Header, nil
}

func roleOf(u *vuser) string {
	if u == nil {
		return "setup"
	}
	return u.role
}
func idOf(u *vuser) int {
	if u == nil {
		return 0
	}
	return u.id
}

func getJSON(ctx context.Context, path string, out any) (int, error) {
	st, b, _, err := (*vuser)(nil).do(ctx, nil, "GET", path, path, nil)
	if err != nil {
		return st, err
	}
	if out != nil {
		if err := json.Unmarshal(b, out); err != nil {
			return st, fmt.Errorf("%s: %w (%.120s)", path, err, b)
		}
	}
	return st, nil
}

// ---- Gemeinde picking ----------------------------------------------------

type gemeinde struct {
	Role, Code, Name string
	Lon, Lat         float64
	KGs              []string
	Warm             bool
}

type luckyResp struct {
	Code string   `json:"gemeinde_code"`
	Name string   `json:"name"`
	Lon  float64  `json:"lon"`
	Lat  float64  `json:"lat"`
	Warm bool     `json:"warm"`
	KGs  []string `json:"kgs"`
	Pool int      `json:"pool"`
}

type adminKG struct {
	KG, Name, Gem, GemName string
	MinLon, MinLat, MaxLon float64
	MaxLat, Area           float64
}

func loadAdmin(path string) ([]adminKG, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	var doc struct {
		KGs []struct {
			KG      string  `json:"kg_code"`
			Name    string  `json:"kg_name"`
			Gem     string  `json:"gemeinde_code"`
			GemName string  `json:"gemeinde_name"`
			MinLon  float64 `json:"min_lon"`
			MinLat  float64 `json:"min_lat"`
			MaxLon  float64 `json:"max_lon"`
			MaxLat  float64 `json:"max_lat"`
			Area    float64 `json:"area_sqkm"`
		} `json:"kgs"`
	}
	if err := json.NewDecoder(gz).Decode(&doc); err != nil {
		return nil, err
	}
	out := make([]adminKG, 0, len(doc.KGs))
	for _, k := range doc.KGs {
		out = append(out, adminKG{k.KG, k.Name, k.Gem, k.GemName, k.MinLon, k.MinLat, k.MaxLon, k.MaxLat, k.Area})
	}
	return out, nil
}

func pickLucky(ctx context.Context, role string) (*gemeinde, error) {
	var l luckyResp
	if st, err := getJSON(ctx, "/api/lucky", &l); err != nil || st != 200 {
		return nil, fmt.Errorf("/api/lucky: status %d err %v", st, err)
	}
	return &gemeinde{Role: role, Code: l.Code, Name: l.Name, Lon: l.Lon, Lat: l.Lat, KGs: l.KGs, Warm: l.Warm}, nil
}

// pickCold chooses a random small-ish, not-warm KG from the admin table and
// resolves its Gemeinde via /api/municipality at the KG's bbox centre.
func pickCold(ctx context.Context, rng *rand.Rand) (*gemeinde, error) {
	kgs, err := loadAdmin(*adminPath)
	if err != nil {
		return nil, fmt.Errorf("admin table: %w (pass -admin)", err)
	}
	for try := 0; try < 40; try++ {
		k := kgs[rng.Intn(len(kgs))]
		if k.Area < 2 || k.Area > 40 || k.Gem == "90001" { // skip slivers, huge alpine KGs and Wien
			continue
		}
		var geo struct {
			Warm bool `json:"warm"`
		}
		if st, err := getJSON(ctx, "/api/kg-geo/"+k.KG, &geo); err != nil || st != 200 || geo.Warm {
			continue
		}
		lon, lat := (k.MinLon+k.MaxLon)/2, (k.MinLat+k.MaxLat)/2
		var m struct {
			Gem struct {
				Code string   `json:"gemeinde_code"`
				Name string   `json:"gemeinde_name"`
				KGs  []string `json:"kg_codes"`
			} `json:"gemeinde"`
		}
		q := fmt.Sprintf("/api/municipality?lon=%.5f&lat=%.5f", lon, lat)
		if st, err := getJSON(ctx, q, &m); err != nil || st != 200 || m.Gem.Code == "" {
			continue
		}
		return &gemeinde{Role: "cold", Code: m.Gem.Code, Name: m.Gem.Name, Lon: lon, Lat: lat, KGs: m.Gem.KGs}, nil
	}
	return nil, fmt.Errorf("could not find a cold KG after 40 tries")
}

// ---- metrics / system reads ---------------------------------------------

type metricsSnap struct {
	APICache struct {
		Bytes int64 `json:"bytes"`
		Rows  int64 `json:"rows"`
	} `json:"api_cache"`
	Uptime int64          `json:"uptime_s"`
	Bev    map[string]any `json:"bevdirect"`
}

func readMetrics(ctx context.Context) (*metricsSnap, error) {
	var lastErr error
	for try := 0; try < 10; try++ { // tolerate a deploy restart (connection refused for a few seconds)
		var m metricsSnap
		st, err := getJSON(ctx, "/api/metrics?fresh=1", &m)
		if err == nil && st == 200 {
			return &m, nil
		}
		if err == nil {
			err = fmt.Errorf("/api/metrics status %d", st)
		}
		lastErr = err
		time.Sleep(time.Second)
	}
	return nil, lastErr
}

func bevMemory() (cur, max int64, ok bool) {
	out, err := exec.Command("systemctl", "show", "bevdirect-serve", "-p", "MemoryCurrent", "-p", "MemoryMax").Output()
	if err != nil {
		return 0, 0, false
	}
	for _, line := range strings.Split(string(out), "\n") {
		k, v, found := strings.Cut(strings.TrimSpace(line), "=")
		if !found {
			continue
		}
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil { // "infinity" / "[not set]"
			n = -1
		}
		switch k {
		case "MemoryCurrent":
			cur = n
		case "MemoryMax":
			max = n
		}
	}
	return cur, max, true
}

// ---- virtual user --------------------------------------------------------

type parcelRow struct {
	ParcelID   string             `json:"parcel_id"`
	KG         string             `json:"kg_code"`
	GNR        string             `json:"gnr"`
	EZ         string             `json:"ez"`
	Area       float64            `json:"area_sqm"`
	Lon        float64            `json:"lon"`
	Lat        float64            `json:"lat"`
	DominantNS string             `json:"dominant_ns"`
	Landuse    map[string]float64 `json:"landuse_areas"`
	BCount     int                `json:"building_count"`
	BArea      float64            `json:"total_building_area_sqm"`
}

func cellBBox(i, j int) string {
	return fmt.Sprintf("west=%.2f&south=%.2f&east=%.2f&north=%.2f", float64(i)*cellDeg, float64(j)*cellDeg, float64(i+1)*cellDeg, float64(j+1)*cellDeg)
}

func (u *vuser) setup(ctx context.Context, st *stats, g *gemeinde) error {
	name := fmt.Sprintf("soak-%s-%d-%04d", g.Role, u.id, rand.Intn(10000))
	code, b, _, err := u.do(ctx, st, "POST", "/api/register", "/api/register", map[string]any{"name": name})
	if err != nil || code != 200 {
		return fmt.Errorf("register: %d %v %.120s", code, err, b)
	}
	var reg struct {
		Player struct {
			ID string `json:"id"`
		} `json:"player"`
		Token string `json:"rejoin_token"`
	}
	if err := json.Unmarshal(b, &reg); err != nil || reg.Token == "" {
		return fmt.Errorf("register body: %v %.120s", err, b)
	}
	u.token, u.player = reg.Token, reg.Player.ID
	code, b, _, err = u.do(ctx, st, "POST", "/api/session/create", "/api/session/create", map[string]any{
		"player_id": u.player, "name": name + "s Reich", "municipality_code": g.Code, "municipality_name": g.Name,
		"center_lon": g.Lon, "center_lat": g.Lat,
	})
	if err != nil || code != 200 {
		return fmt.Errorf("session/create: %d %v %.160s", code, err, b)
	}
	var sess struct {
		ID      string `json:"id"`
		Session struct {
			ID string `json:"id"`
		} `json:"session"`
	}
	json.Unmarshal(b, &sess)
	u.session = sess.ID
	if u.session == "" {
		u.session = sess.Session.ID
	}
	if u.session == "" {
		return fmt.Errorf("session/create: no id in %.160s", b)
	}
	return nil
}

func (u *vuser) run(ctx context.Context, st *stats, g *gemeinde, deadline time.Time, rng *rand.Rand) {
	ci, cj := int(math.Floor(g.Lon/cellDeg)), int(math.Floor(g.Lat/cellDeg))
	i, j := ci, cj
	maxOff := 2
	if g.Role == "cold" {
		maxOff = 1 // 3×3 = 9 cells
		if *coldCells < 9 {
			maxOff = 0
		}
	}
	var lastParcels []parcelRow
	for iter := 0; time.Now().Before(deadline); iter++ {
		// random walk ±1 cell per step, clamped to ±maxOff around the centre
		i += rng.Intn(3) - 1
		j += rng.Intn(3) - 1
		i = clamp(i, ci-maxOff, ci+maxOff)
		j = clamp(j, cj-maxOff, cj+maxOff)
		bb := cellBBox(i, j)
		code, b, _, _ := u.do(ctx, st, "GET", "/api/viewport", "/api/viewport?"+bb, nil)
		if code == 200 {
			var vp struct {
				Ready   bool        `json:"ready"`
				Parcels []parcelRow `json:"parcels"`
			}
			if json.Unmarshal(b, &vp) == nil {
				if !vp.Ready {
					st.notReady.Add(1)
				}
				if len(vp.Parcels) > 0 {
					lastParcels = vp.Parcels
				}
			}
		}
		if ctx.Err() != nil {
			return
		}
		layers := []struct{ ep, q string }{
			{"/api/trees", "/api/trees?" + bb},
			{"/api/buildings", "/api/buildings?" + bb},
			{"/api/osm-lines", "/api/osm-lines?" + bb + "&cat=road,rail,water"},
			{"/api/n2k", "/api/n2k?" + bb},
			{"/api/schlaege", "/api/schlaege?" + bb},
		}
		var wg sync.WaitGroup
		for _, l := range layers {
			wg.Add(1)
			go func(ep, q string) {
				defer wg.Done()
				u.do(ctx, st, "GET", ep, q, nil)
			}(l.ep, l.q)
		}
		wg.Wait()
		u.do(ctx, st, "GET", "/api/session/{id}/treasures", "/api/session/"+u.session+"/treasures", nil)
		u.do(ctx, st, "GET", "/api/session/{id}/parcels", "/api/session/"+u.session+"/parcels", nil)
		if *claimEvery > 0 && iter%*claimEvery == *claimEvery-1 && len(lastParcels) > 0 {
			p := lastParcels[rng.Intn(len(lastParcels))]
			lu := p.DominantNS
			if lu == "" {
				lu = "48"
			}
			u.do(ctx, st, "POST", "/api/claim-parcel", "/api/claim-parcel", map[string]any{
				"session_id": u.session, "player_id": u.player, "parcel_id": p.ParcelID, "kg_code": p.KG,
				"gnr": p.GNR, "ez": p.EZ, "area_sqm": p.Area, "landuse": lu,
				"building_count": p.BCount, "total_building_area": p.BArea,
			})
			q := url.Values{}
			q.Set("parcel_id", p.ParcelID)
			q.Set("lon", strconv.FormatFloat(p.Lon, 'f', 6, 64))
			q.Set("lat", strconv.FormatFloat(p.Lat, 'f', 6, 64))
			q.Set("area", strconv.FormatFloat(math.Max(p.Area, 1), 'f', 1, 64))
			q.Set("bcount", strconv.Itoa(p.BCount))
			q.Set("barea", strconv.FormatFloat(p.BArea, 'f', 1, 64))
			q.Set("lu", lu)
			u.do(ctx, st, "GET", "/api/similar", "/api/similar?"+q.Encode(), nil)
		}
		// think time
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(200+rng.Intn(600)) * time.Millisecond):
		}
	}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ---- main ----------------------------------------------------------------

func main() {
	flag.Parse()
	if *coldCells > 9 {
		*coldCells = 9
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	ctx := context.Background()
	fail := func(f string, a ...any) { fmt.Fprintf(os.Stderr, "soakload: "+f+"\n", a...); os.Exit(2) }

	before, err := readMetrics(ctx)
	if err != nil {
		fail("metrics before: %v", err)
	}
	warmG, err := pickLucky(ctx, "warm")
	if err != nil {
		fail("%v", err)
	}
	luckyG, err := pickLucky(ctx, "lucky")
	if err != nil {
		fail("%v", err)
	}
	coldG, err := pickCold(ctx, rng)
	if err != nil {
		fail("%v", err)
	}
	gems := []*gemeinde{coldG, warmG, luckyG}
	fmt.Printf("soakload: base=%s users=%d seconds=%d\n", *base, *users, *seconds)
	for _, g := range gems {
		fmt.Printf("  %-5s %s %s (%.4f, %.4f) kgs=%v warm=%v\n", g.Role, g.Code, g.Name, g.Lon, g.Lat, g.KGs, g.Warm)
	}
	if warmG.Code == luckyG.Code {
		fmt.Println("  note: warm and lucky picked the same Gemeinde (lucky pool is small)")
	}
	if cur, max, ok := bevMemory(); ok {
		fmt.Printf("  bevdirect-serve memory before: %d MB / max %d MB\n", cur>>20, max>>20)
	}
	fmt.Printf("  api_cache before: %d rows, %.1f MB\n", before.APICache.Rows, float64(before.APICache.Bytes)/1e6)

	st := newStats()
	deadline := time.Now().Add(time.Duration(*seconds) * time.Second)
	runCtx, cancel := context.WithDeadline(ctx, deadline.Add(20*time.Second))
	defer cancel()
	var wg sync.WaitGroup
	var setupFail atomic.Int64
	for n := 0; n < *users; n++ {
		g := gems[n%3]
		u := &vuser{id: n, role: g.Role, xff: fmt.Sprintf("10.%d.%d.%d", 100+n/65536, (n/256)%256, n%256)}
		wg.Add(1)
		go func() {
			defer wg.Done()
			// stagger start so 50 registers don't land in the same ms
			time.Sleep(time.Duration(rng.Intn(1500)) * time.Millisecond)
			r := rand.New(rand.NewSource(int64(u.id)*7919 + time.Now().UnixNano()))
			if err := u.setup(runCtx, st, g); err != nil {
				setupFail.Add(1)
				fmt.Fprintf(os.Stderr, "  user %d (%s): setup failed: %v\n", u.id, u.role, err)
				return
			}
			u.run(runCtx, st, g, deadline, r)
		}()
	}
	wg.Wait()

	after, err := readMetrics(ctx)
	if err != nil {
		fail("metrics after: %v", err)
	}

	// ---- report ----
	st.mu.Lock()
	keys := make([]string, 0, len(st.byRE))
	for k := range st.byRE {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Printf("\n%-6s %-32s %7s %8s %8s %8s %4s %4s %5s %4s %s\n", "role", "endpoint", "count", "p50_ms", "p95_ms", "max_ms", "5xx", "4xx", "down", "conn", "x-cache")
	for _, k := range keys {
		role, ep, _ := strings.Cut(k, "|")
		a := st.byRE[k]
		xc := ""
		if a.hit+a.miss+a.warmXC > 0 {
			xc = fmt.Sprintf("HIT %d / WARM %d / MISS %d (hit %.0f%%)", a.hit, a.warmXC, a.miss, 100*float64(a.hit+a.warmXC)/float64(a.hit+a.miss+a.warmXC))
		}
		fmt.Printf("%-6s %-32s %7d %8.0f %8.0f %8.0f %4d %4d %5d %4d %s\n", role, ep, a.n, pct(a.ms, .5), pct(a.ms, .95), maxf(a.ms), a.e5, a.e4, a.down, a.connErr, xc)
	}
	all := st.all
	vpWarm := append([]float64(nil), st.vpWarmMs...)
	notReady := st.notReady.Load()
	st.mu.Unlock()

	fmt.Printf("\ntotal requests %d  5xx %d  4xx %d  breaker-503 %d  conn-errors %d  viewport ready:false %d  setup failures %d\n", all.n, all.e5, all.e4, all.down, all.connErr, notReady, setupFail.Load())
	if after.Uptime < int64(*seconds) {
		fmt.Printf("WARNING: server uptime is %d s < run length — the server restarted during the run (conn-errors / lost in-memory metrics expected)\n", after.Uptime)
	}
	fmt.Printf("viewport warm (X-Cache HIT|WARM): n=%d p50=%.0f ms p95=%.0f ms max=%.0f ms\n", len(vpWarm), pct(vpWarm, .5), pct(vpWarm, .95), maxf(vpWarm))
	fmt.Printf("api_cache after: %d rows, %.1f MB (Δ %+.1f MB)\n", after.APICache.Rows, float64(after.APICache.Bytes)/1e6, float64(after.APICache.Bytes-before.APICache.Bytes)/1e6)
	if after.Bev != nil {
		fmt.Printf("bevdirect health: ok=%v cells_cached=%v prefetch_queue=%v\n", after.Bev["ok"], after.Bev["cells_cached"], after.Bev["prefetch_queue"])
	}

	// ---- assertions ----
	failed := 0
	check := func(ok bool, f string, a ...any) {
		mark := "PASS"
		if !ok {
			mark = "FAIL"
			failed++
		}
		fmt.Printf("[%s] %s\n", mark, fmt.Sprintf(f, a...))
	}
	fmt.Println()
	p95 := pct(vpWarm, .95)
	check(len(vpWarm) > 0 && p95 < 1500, "viewport warm p95 %.0f ms < 1500 ms (n=%d)", p95, len(vpWarm))
	check(all.e5 == 0, "zero 5xx (got %d)", all.e5)
	connPct := 0.0
	if all.n > 0 {
		connPct = 100 * float64(all.connErr) / float64(all.n)
	}
	check(connPct <= 1, "transport errors %.2f%% ≤ 1%% (%d)", connPct, all.connErr)
	downPct := 0.0
	if all.n > 0 {
		downPct = 100 * float64(all.down) / float64(all.n)
	}
	check(downPct <= 1, "breaker 503 (X-Upstream: down) %.2f%% ≤ 1%% (%d)", downPct, all.down)
	if cur, max, ok := bevMemory(); ok {
		if max > 0 {
			check(cur < max, "bevdirect-serve memory %d MB < MemoryMax %d MB", cur>>20, max>>20)
		} else {
			fmt.Printf("[SKIP] bevdirect-serve MemoryMax not set (current %d MB)\n", cur>>20)
		}
	} else {
		fmt.Println("[SKIP] systemctl show bevdirect-serve not available")
	}
	grow := after.APICache.Bytes - before.APICache.Bytes
	check(after.APICache.Bytes <= 1500e6, "api_cache total %.1f MB ≤ 1500 MB", float64(after.APICache.Bytes)/1e6)
	check(grow <= 400e6, "api_cache growth %+.1f MB ≤ 400 MB", float64(grow)/1e6)
	check(setupFail.Load() == 0, "all %d users registered + created a session (%d failed)", *users, setupFail.Load())
	if failed > 0 {
		fmt.Printf("\n%d assertion(s) FAILED\n", failed)
		os.Exit(1)
	}
	fmt.Println("\nall assertions passed")
}
