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
// (GET /contrib/api/v1/ne/revisit?observer=…&limit=50, Bearer token): `pending[]`
// = KGs holding chunk versions sighted once (a second sighting ≥ same_pass_window
// later confirms the change), `stale[]` = KGs with a single baseline-only report.
// contribPlanNow puts them at the head of the night, capped at contribRevisitShare
// of night_min; the rest of the night is the normal order. The answer refreshes
// every 10 min on umfeld's side, so it is fetched at most once per 10 min here
// (one small JSON in RAM, no disk). An unreachable endpoint = no hints, never a
// failed plan.
const (
	contribObserver     = "siedler-oesterreich"
	contribRevisitShare = 0.2 // ≤ 20 % of night_min
	contribRevisitLimit = 50
	contribRevisitTTL   = 10 * time.Minute
)

type contribRevisitKG struct {
	KG    string `json:"kg"`
	Kind  string `json:"kind"`  // pending | stale
	Since string `json:"since"` // pending: oldest_since; stale: baseline_at
	// BBox "w,s,e,n": the exact input bbox the pending/stale chunk versions were
	// built with (the peer's layout). A revisit MUST be built with this very bbox
	// — a different request layout moves chunk digests and umfeld answers
	// change_suspect:"layout_change", issues no want_chunks and confirms nothing
	// (2026-10-23: 84014/85205 with our wider head-derived bbox confirmed 0;
	// 73510 with the peer's bbox confirmed all 98). Empty when umfeld sent none.
	BBox string `json:"bbox,omitempty"`
}

var contribRevisitCache struct {
	mu   sync.Mutex
	at   time.Time
	list []contribRevisitKG
	err  string
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

// contribRevisit returns umfeld's revisit list for us: pending first (oldest
// oldest_since first), then stale — cached contribRevisitTTL.
func contribRevisit(now time.Time) []contribRevisitKG {
	c := &contribRevisitCache
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.at.IsZero() && now.Sub(c.at) < contribRevisitTTL {
		return c.list
	}
	c.at = now
	tok := contribToken()
	if tok == "" {
		c.err, c.list = "no token", nil
		return nil
	}
	url := strings.TrimSuffix(umfeldAPI, "/api/v1") + "/contrib/api/v1/ne/revisit?observer=" + contribObserver + "&limit=" + strconv.Itoa(contribRevisitLimit)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := upstreamClient.Do(req)
	if err != nil {
		c.err = err.Error()
		slog.Warn("contrib: revisit fetch failed", "err", err)
		return c.list // keep the previous list on a transient failure
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != 200 {
		c.err = "http " + strconv.Itoa(resp.StatusCode)
		slog.Warn("contrib: revisit fetch failed", "status", resp.StatusCode)
		return c.list
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
			list = append(list, contribRevisitKG{kg, "pending", p.Since, parseRevisitBBox(p.BBox)})
		}
	}
	stale := d.Stale
	sort.SliceStable(stale, func(i, j int) bool { return stale[i].Since < stale[j].Since })
	for _, p := range stale {
		kg := padKGCode(p.KG)
		if len(kg) == 5 && !seen[kg] {
			seen[kg] = true
			list = append(list, contribRevisitKG{kg, "stale", p.Since, parseRevisitBBox(p.BBox)})
		}
	}
	c.err, c.list = "", list
	return list
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
// (cached list, no fetch unless the 10-min TTL is over); nil when kg is not a
// revisit hint or carries no bbox.
func contribRevisitBBox(kg string) []float64 {
	for _, r := range contribRevisit(time.Now()) {
		if r.KG == kg && r.BBox != "" {
			bb, _ := revisitBBoxFloats(r.BBox)
			return bb
		}
	}
	return nil
}
