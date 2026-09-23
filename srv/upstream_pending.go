package srv

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Both upstreams (cadastre-process-api and srtm-lidar-at) serve per-KG
// products that are lazily fetched from the Zenodo mirror. Since Sept 2026
// they never hang on that: an endpoint whose whole result depends on a cold
// file answers HTTP 202 with a progress block and a Retry-After header, and
// keeps downloading server-side. Repeating the identical request converges.
//
//   cadastre 202: {status:"pending", warming:{kgs:[{kg,state,pct,eta_s,...}],
//                  zenodo:{status:healthy|slow|degraded|down}, retry_after_s}}
//                 (also on 200 + ready:false for viewport endpoints)
//   lidar    202: {status:"fetching", kg_code, fetch:{status,pct,eta_s,...},
//                  retry_after_s, zenodo:{slow}}
//
// This file normalises both into one shape we relay to the browser:
//
//   {"status":"pending","retry_after_s":N,"progress":{"state","pct","eta_s"},
//    "kgs":[{"kg","state","pct","eta_s"}],"zenodo":"healthy|slow|..."}
//
// plus a Retry-After header, so a single client-side rule ("202 → wait
// retry_after_s, re-GET") works for every proxied endpoint.

type pendingKG struct {
	KG    string  `json:"kg"`
	State string  `json:"state"`
	Pct   float64 `json:"pct,omitempty"`
	EtaS  float64 `json:"eta_s,omitempty"`
}

type pendingInfo struct {
	Status     string  `json:"status"`
	RetryAfter float64 `json:"retry_after_s"`
	Progress   struct {
		State string  `json:"state,omitempty"`
		Pct   float64 `json:"pct"`
		EtaS  float64 `json:"eta_s,omitempty"`
	} `json:"progress"`
	KGs    []pendingKG `json:"kgs,omitempty"`
	Zenodo string      `json:"zenodo,omitempty"`
}

func fnum(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case string:
		f, _ := strconv.ParseFloat(n, 64)
		return f
	}
	return 0
}
func fstr(v any) string {
	s, _ := v.(string)
	return s
}

// parsePending extracts the normalised pending state from an upstream
// response (headers + body). Works for both upstreams' 202 bodies and for the
// cadastre viewport "ready:false" 200 bodies (which carry the same warming
// block). Never fails: a body we can't parse yields a default 3s retry.
func parsePending(hdr http.Header, body []byte) pendingInfo {
	var p pendingInfo
	p.Status = "pending"
	if ra, err := strconv.ParseFloat(strings.TrimSpace(hdr.Get("Retry-After")), 64); err == nil && ra > 0 {
		p.RetryAfter = ra
	}
	if zs := hdr.Get("X-Zenodo-Status"); zs != "" {
		p.Zenodo = zs
	}
	var m map[string]any
	if json.Unmarshal(body, &m) == nil {
		if ra := fnum(m["retry_after_s"]); ra > 0 {
			p.RetryAfter = ra
		}
		// lidar: fetch{status,pct,eta_s}
		if f, ok := m["fetch"].(map[string]any); ok {
			p.Progress.State = fstr(f["status"])
			p.Progress.Pct = fnum(f["pct"])
			p.Progress.EtaS = fnum(f["eta_s"])
			if kg := fstr(m["kg_code"]); kg != "" {
				p.KGs = append(p.KGs, pendingKG{KG: kg, State: p.Progress.State, Pct: p.Progress.Pct, EtaS: p.Progress.EtaS})
			}
		}
		if z, ok := m["zenodo"].(map[string]any); ok {
			if slow, ok := z["slow"].(bool); ok && slow && p.Zenodo == "" {
				p.Zenodo = "slow"
			}
		}
		// cadastre: warming{kgs[],zenodo{status},retry_after_s}
		if w, ok := m["warming"].(map[string]any); ok {
			if ra := fnum(w["retry_after_s"]); ra > 0 && p.RetryAfter == 0 {
				p.RetryAfter = ra
			}
			if z, ok := w["zenodo"].(map[string]any); ok && fstr(z["status"]) != "" {
				p.Zenodo = fstr(z["status"])
			}
			if kgs, ok := w["kgs"].([]any); ok {
				var sumPct, maxEta float64
				for _, it := range kgs {
					k, _ := it.(map[string]any)
					if k == nil {
						continue
					}
					pk := pendingKG{KG: fstr(k["kg"]), State: fstr(k["state"]), Pct: fnum(k["pct"]), EtaS: fnum(k["eta_s"])}
					p.KGs = append(p.KGs, pk)
					sumPct += pk.Pct
					if pk.EtaS > maxEta {
						maxEta = pk.EtaS
					}
					if p.Progress.State == "" || pk.State == "downloading" {
						p.Progress.State = pk.State
					}
				}
				if n := len(p.KGs); n > 0 {
					p.Progress.Pct = sumPct / float64(n)
					p.Progress.EtaS = maxEta
				}
			}
		}
	}
	if p.RetryAfter <= 0 {
		p.RetryAfter = 3
	}
	if p.Zenodo == "" {
		p.Zenodo = "healthy"
	}
	return p
}

func (p pendingInfo) body() []byte {
	b, _ := json.Marshal(p)
	return b
}

// retryAfterOf reads retry_after_s from one of our own normalised pending
// bodies (used by cachedFetch to emit the Retry-After header on 202).
func retryAfterOf(body []byte) int {
	var p struct {
		RetryAfter float64 `json:"retry_after_s"`
	}
	if json.Unmarshal(body, &p) != nil || p.RetryAfter <= 0 {
		return 3
	}
	n := int(p.RetryAfter + 0.999)
	if n < 1 {
		n = 1
	}
	return n
}

// isReadyFalse reports whether a 200 body from a cadastre viewport/batch
// endpoint carries ready:false (partial result, some KG still warming).
func isReadyFalse(body []byte) bool {
	var m struct {
		Ready *bool `json:"ready"`
	}
	return json.Unmarshal(body, &m) == nil && m.Ready != nil && !*m.Ready
}

// upstreamGetWait GETs url and, while upstream answers 202 (product still
// coming from Zenodo), sleeps for min(Retry-After, remaining budget) and
// re-GETs. Upstream warming is triggered by the first call and downloads
// keep running between polls, so retries converge; doing the wait here
// means all singleflight waiters share one poll loop instead of every
// browser tab polling. Returns the final response's status, headers and
// body (body already read, ≤ maxBody). status 202 after the budget means
// "still pending" — callers relay parsePending(...).body().
//
// Pass budget=0 for a single non-waiting GET.
func upstreamGetWait(url string, budget time.Duration, maxBody int64) (int, http.Header, []byte, error) {
	deadline := time.Now().Add(budget)
	for {
		resp, err := upstreamGet(url)
		if err != nil {
			return 0, nil, nil, err
		}
		body, rerr := io.ReadAll(io.LimitReader(resp.Body, maxBody))
		resp.Body.Close()
		if rerr != nil {
			return 0, nil, nil, rerr
		}
		if resp.StatusCode != http.StatusAccepted {
			return resp.StatusCode, resp.Header, body, nil
		}
		remaining := time.Until(deadline)
		if remaining < 500*time.Millisecond {
			return resp.StatusCode, resp.Header, body, nil
		}
		p := parsePending(resp.Header, body)
		wait := time.Duration(p.RetryAfter * float64(time.Second))
		if wait > 8*time.Second {
			wait = 8 * time.Second
		}
		if wait < time.Second {
			wait = time.Second
		}
		if wait > remaining {
			wait = remaining
		}
		time.Sleep(wait)
	}
}

// withWait appends upstream's ?wait=<s> so it blocks on a cold Zenodo fetch
// itself (max 120) instead of us polling. Only added when the caller hasn't
// set it.
func withWait(url string, secs int) string {
	if strings.Contains(url, "wait=") {
		return url
	}
	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	return url + sep + "wait=" + strconv.Itoa(secs)
}
