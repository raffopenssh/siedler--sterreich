package srv

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Revisit hints — umfeld steers which KGs a contributor should observe again
// (GET /contrib/api/v1/ne/revisit?kind=…&observer=…&limit=…, Bearer token, ETag /
// If-None-Match → 304 at zero bytes; umfeld §5b: poll only with kind= and
// observer=, never the full list more than once a day).
//   pending[]   KGs holding chunk versions sighted once (a second sighting
//               ≥ same_pass_window later confirms the change)
//   stale[]     KGs with a single baseline-only report
//   no_log[]    KGs without any chunk log (declared picture still the March index)
//   next_pass[] the standing work queue since Austria is 100 % digested
//               (2026-11-06): {kg, pass, chunks, chunks_below, last_at, bbox},
//               ordered by umfeld (lowest pass first, most chunks behind first,
//               oldest visit first). One normal chunk-bearing report whose bbox
//               COVERS the row's bbox lifts the whole KG by one pass.
// `obs_on_index[]`/`obs_missing[]` are for NEO1 observers — not us.
// contribPlanNow puts them at the head of the night (pending → next_pass →
// stale → no_log, see contribRevisit), capped at contribRevisitShare of
// night_min; the rest of the night is the normal order. pending/stale/no_log refresh every
// 10 min (umfeld recomputes every 10 min); next_pass is fetched once a day with
// limit = the revisit budget (304 ⇒ yesterday's list, minus what we reported
// since). ne_report.py sends every revisit KG as a full report (no header-only
// step 0). Small JSON in RAM, no disk. An unreachable endpoint = no hints, never
// a failed plan.
const (
	contribObserver       = "siedler-oesterreich"
	contribRevisitShare   = 1.0 / 3 // ≤ ⅓ of night_min (pending + stale + no_log + next_pass, per-peer filtered by umfeld since 2026-11-01)
	contribRevisitLimit   = 200     // pending/stale per list (umfeld allows ≤ 8000)
	contribRevisitTTL     = 10 * time.Minute
	contribNextPassTTL    = 24 * time.Hour // umfeld: fetch the next_pass slice once a day
	contribSamePassWindow = 20 * time.Hour // umfeld same_pass_window: a re-post inside it is not a sighting
	contribRevisitMinAge  = 7 * 24 * time.Hour // umfeld min_age_days: next_pass hides KGs we visited within it; mirrored on a 304-reused list
)

type contribRevisitKG struct {
	KG    string `json:"kg"`
	Kind  string `json:"kind"`  // pending | stale | no_log | next_pass
	Since string `json:"since"` // pending: oldest_since; stale: baseline_at; next_pass: last_at
	Pass  int    `json:"pass,omitempty"`
	// BBox "w,s,e,n": the exact input bbox the pending/stale chunk versions were
	// built with (the peer's layout). A revisit MUST be built with this very bbox
	// — a different request layout moves chunk digests and umfeld answers
	// change_suspect:"layout_change", issues no want_chunks and confirms nothing
	// (2026-10-23: 84014/85205 with our wider head-derived bbox confirmed 0;
	// 73510 with the peer's bbox confirmed all 98). next_pass: the last
	// chunk-bearing report's bbox — ours must cover it (building with exactly
	// it does). Empty when umfeld sent none.
	BBox string `json:"bbox,omitempty"`
}

// revisitFetch: one ETag-cached GET of a revisit list kind. Returns the body
// (nil on 304 / failure), the HTTP status (0 on transport error) and keeps
// the ETag for the next call.
type revisitFetch struct {
	mu   sync.Mutex
	at   time.Time
	etag string
	err  string
}

func (f *revisitFetch) get(query string) ([]byte, int) {
	tok := contribToken()
	if tok == "" {
		f.err = "no token"
		return nil, 0
	}
	url := strings.TrimSuffix(umfeldAPI, "/api/v1") + "/contrib/api/v1/ne/revisit?observer=" + contribObserver + "&" + query
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	if f.etag != "" {
		req.Header.Set("If-None-Match", f.etag)
	}
	resp, err := upstreamClient.Do(req)
	if err != nil {
		f.err = err.Error()
		slog.Warn("contrib: revisit fetch failed", "query", query, "err", err)
		return nil, 0
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	switch resp.StatusCode {
	case 304:
		f.err = ""
		return nil, 304
	case 200:
		f.err = ""
		if et := resp.Header.Get("ETag"); et != "" {
			f.etag = et
		}
		return body, 200
	}
	f.err = "http " + strconv.Itoa(resp.StatusCode)
	slog.Warn("contrib: revisit fetch failed", "query", query, "status", resp.StatusCode)
	return nil, resp.StatusCode
}

var (
	contribRevisitCache struct { // pending + stale
		revisitFetch
		list []contribRevisitKG
	}
	contribNextPassCache struct { // next_pass, daily
		revisitFetch
		list   []contribRevisitKG
		limit  int
		target int // totals.pass_target: the pass the nation is on
		byPass map[string]int
	}
	// contribNoLogCache: umfeld's *whole* no_log list (?kind=no_log&all=1, 196
	// rows 2026-11-06) kg → bbox. contribPlanNow treats these KGs as unreported
	// this quarter: a digest-only or legacy-layout report never gave them a chunk
	// log. They re-enter the fill in due order (cheap when warm), built with
	// umfeld's bbox (run.sh passes every planned KG with a revisit_bbox as
	// --revisit). Umfeld hides KGs we reported < min_age_days (7) ago.
	contribNoLogCache struct {
		revisitFetch
		list map[string]string
	}
)

// contribNoLog returns kg → bbox for every KG umfeld lists without a chunk log
// (cached contribRevisitTTL; previous list on failure/304, nil when unavailable).
func contribNoLog(now time.Time) map[string]string {
	c := &contribNoLogCache
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.at.IsZero() && now.Sub(c.at) < contribRevisitTTL {
		return c.list
	}
	c.at = now
	body, _ := c.get("kind=no_log&all=1")
	if body == nil {
		return c.list
	}
	var d struct {
		NoLog []struct {
			KG   string `json:"kg"`
			BBox string `json:"bbox"`
		} `json:"no_log"`
	}
	if err := json.Unmarshal(body, &d); err != nil {
		return c.list
	}
	m := map[string]string{}
	for _, p := range d.NoLog {
		if kg := padKGCode(p.KG); len(kg) == 5 {
			m[kg] = parseRevisitBBox(p.BBox)
		}
	}
	c.list = m
	return m
}

// contribToken is the contributor (peer) token: ne-peer.key in the repo root, else
// $NE_PEER_TOKEN / $CONTRIBUTOR_TOKEN — the same sources ne_report.py uses.
func contribToken() string {
	if b, err := os.ReadFile("ne-peer.key"); err == nil {
		if t := strings.TrimSpace(string(b)); t != "" {
			return t
		}
	}
	if t := os.Getenv("NE_PEER_TOKEN"); t != "" {
		return t
	}
	return os.Getenv("CONTRIBUTOR_TOKEN")
}

// contribRevisit returns umfeld's revisit list for us, in the order the plan
// takes it: pending (oldest oldest_since first — a real change candidate waits
// for its second sighting), then the daily next_pass slice (umfeld's order:
// the fixed pass share the nation asked for, 2026-11-06), then stale (oldest
// baseline first — the same pass-2 work seen from the log, fallback when the
// slice is short), then no_log (re-queued into the fill anyway). nextN sizes
// the next_pass fetch (the revisit budget; 0 = reuse the cached slice).
func contribRevisit(now time.Time, nextN int) []contribRevisitKG {
	var list []contribRevisitKG
	seen := map[string]bool{}
	add := func(r contribRevisitKG) {
		if !seen[r.KG] {
			seen[r.KG] = true
			list = append(list, r)
		}
	}
	ps := contribPendingStale(now)
	for _, r := range ps {
		if r.Kind == "pending" {
			add(r)
		}
	}
	for _, r := range contribNextPass(now, nextN) {
		add(r)
	}
	for _, r := range ps {
		if r.Kind == "stale" {
			add(r)
		}
	}
	nolog := contribNoLog(now)
	kgs := make([]string, 0, len(nolog))
	for kg := range nolog {
		kgs = append(kgs, kg)
	}
	sort.Strings(kgs) // umfeld's order is lost in the map; stable by KG
	for _, kg := range kgs {
		add(contribRevisitKG{KG: kg, Kind: "no_log", BBox: nolog[kg]})
	}
	return list
}

// contribPendingStale: pending + stale lists, cached contribRevisitTTL.
func contribPendingStale(now time.Time) []contribRevisitKG {
	c := &contribRevisitCache
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.at.IsZero() && now.Sub(c.at) < contribRevisitTTL {
		return c.list
	}
	c.at = now
	body, _ := c.get("kind=pending,stale&limit=" + strconv.Itoa(contribRevisitLimit))
	if body == nil {
		return c.list // 304 or transient failure: keep the previous list
	}
	var d struct {
		Pending []struct {
			KG    string `json:"kg"`
			Since string `json:"oldest_since"`
			BBox  string `json:"bbox"`
		} `json:"pending"`
		Stale []struct {
			KG    string `json:"kg"`
			Since string `json:"baseline_at"`
			BBox  string `json:"bbox"`
		} `json:"stale"`
	}
	if err := json.Unmarshal(body, &d); err != nil {
		c.err = "bad json"
		return c.list
	}
	var list []contribRevisitKG
	seen := map[string]bool{}
	pend := d.Pending
	sort.SliceStable(pend, func(i, j int) bool { return pend[i].Since < pend[j].Since })
	for _, p := range pend {
		kg := padKGCode(p.KG)
		if len(kg) == 5 && !seen[kg] {
			seen[kg] = true
			list = append(list, contribRevisitKG{KG: kg, Kind: "pending", Since: p.Since, BBox: parseRevisitBBox(p.BBox)})
		}
	}
	stale := d.Stale
	sort.SliceStable(stale, func(i, j int) bool { return stale[i].Since < stale[j].Since })
	for _, p := range stale {
		kg := padKGCode(p.KG)
		if len(kg) == 5 && !seen[kg] {
			seen[kg] = true
			list = append(list, contribRevisitKG{KG: kg, Kind: "stale", Since: p.Since, BBox: parseRevisitBBox(p.BBox)})
		}
	}
	c.list = list
	return list
}

// contribNextPass: the daily next_pass slice (umfeld's order kept), fetched
// once per contribNextPassTTL with limit = n (304 ⇒ yesterday's list; the
// plan drops what we reported since). Rows we reported within
// contribRevisitMinAge are dropped here too (umfeld hides them on a fresh
// answer; a reused one may still carry them).
func contribNextPass(now time.Time, n int) []contribRevisitKG {
	c := &contribNextPassCache
	c.mu.Lock()
	defer c.mu.Unlock()
	if n <= 0 {
		n = c.limit
	}
	if n <= 0 {
		n = contribRevisitLimit
	}
	fresh := !c.at.IsZero() && now.Sub(c.at) < contribNextPassTTL && n <= c.limit
	if fresh {
		return c.list
	}
	if n != c.limit {
		c.etag = "" // a different limit is a different list
	}
	c.at, c.limit = now, n
	body, status := c.get("kind=next_pass&limit=" + strconv.Itoa(n))
	if body == nil {
		if status != 304 && status != 0 {
			c.at = now.Add(-contribNextPassTTL + 30*time.Minute) // retry in 30 min, not tomorrow
		}
		return c.list
	}
	var d struct {
		NextPass []struct {
			KG     string `json:"kg"`
			Pass   int    `json:"pass"`
			LastAt string `json:"last_at"`
			BBox   string `json:"bbox"`
		} `json:"next_pass"`
		Totals struct {
			PassTarget int            `json:"pass_target"`
			ByPass     map[string]int `json:"chunks_by_pass"`
		} `json:"totals"`
	}
	if err := json.Unmarshal(body, &d); err != nil {
		c.err = "bad json"
		return c.list
	}
	var list []contribRevisitKG
	for _, p := range d.NextPass {
		if kg := padKGCode(p.KG); len(kg) == 5 {
			list = append(list, contribRevisitKG{KG: kg, Kind: "next_pass", Since: p.LastAt, Pass: p.Pass, BBox: parseRevisitBBox(p.BBox)})
		}
	}
	c.list, c.target, c.byPass = list, d.Totals.PassTarget, d.Totals.ByPass
	slog.Info("contrib: next_pass slice", "n", len(list), "limit", n, "pass_target", c.target)
	return list
}

// contribPassTarget: umfeld's totals.pass_target from the last next_pass answer (0 = unknown).
func contribPassTarget() int {
	c := &contribNextPassCache
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.target
}

// parseRevisitBBox validates umfeld's "w,s,e,n" (4 finite numbers, w<e, s<n,
// ≤ 1° a side) and returns it verbatim, else "".
func parseRevisitBBox(s string) string {
	s = strings.TrimSpace(s)
	if _, ok := revisitBBoxFloats(s); !ok {
		return ""
	}
	return s
}

func revisitBBoxFloats(s string) ([]float64, bool) {
	parts := strings.Split(s, ",")
	if len(parts) != 4 {
		return nil, false
	}
	bb := make([]float64, 4)
	for i, p := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return nil, false
		}
		bb[i] = v
	}
	if !(bb[2] > bb[0] && bb[3] > bb[1] && bb[2]-bb[0] <= 1 && bb[3]-bb[1] <= 1) {
		return nil, false
	}
	return bb, true
}

// contribRevisitBBox: the exact bbox umfeld wants a revisit of kg built with
// (cached lists, no fetch unless a TTL is over); nil when kg is not a
// revisit hint or carries no bbox.
func contribRevisitBBox(kg string) []float64 {
	for _, r := range contribRevisit(time.Now(), 0) {
		if r.KG == kg && r.BBox != "" {
			bb, _ := revisitBBoxFloats(r.BBox)
			return bb
		}
	}
	return nil
}
