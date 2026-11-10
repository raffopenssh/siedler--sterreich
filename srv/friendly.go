package srv

import (
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// farm-subsidies-austria rate-limits per client IP (300 req/min, burst 100,
// 4 concurrent) and exempts identified siblings (32 concurrent, response
// header "X-RateLimit-Tier: friendly"). friendlyTransport is the one place
// that identifies us: every outbound request to that host — proxies, agent
// look-ups, dossier, health probes — passes through it and gets
//
//	User-Agent: siedler-oesterreich/<component> (+https://siedler-oesterreich.exe.xyz)
//
// A 429 is retried (Retry-After, else exponential back-off) while the
// caller's deadline allows, so a job never fails on a rate limit.
type friendlyTransport struct {
	base      http.RoundTripper
	component string
}

const (
	friendlyRetries = 4
	friendlyMaxWait = 20 * time.Second
)

var friendlyWarned atomic.Int64 // unix s of the last "not friendly" warning (≤ 1 per 10 min)

var friendlyHosts = map[string]bool{hostOf(farmAPI): true}

func friendlyUA(component string) string {
	return "siedler-oesterreich/" + component + " (+https://siedler-oesterreich.exe.xyz)"
}

func (t *friendlyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !friendlyHosts[req.URL.Host] {
		return t.base.RoundTrip(req)
	}
	req = req.Clone(req.Context()) // a RoundTripper must not modify the caller's request
	req.Header.Set("User-Agent", friendlyUA(t.component))
	replayable := req.Body == nil || req.Body == http.NoBody || req.GetBody != nil
	for try := 0; ; try++ {
		resp, err := t.base.RoundTrip(req)
		if err == nil && resp.Header.Get("X-RateLimit-Tier") != "friendly" && friendlyWarned.Swap(time.Now().Unix()) < time.Now().Unix()-600 {
			slog.Warn("sibling did not recognise us", "host", req.URL.Host, "tier", resp.Header.Get("X-RateLimit-Tier"), "status", resp.StatusCode)
		}
		if err != nil || resp.StatusCode != http.StatusTooManyRequests || try >= friendlyRetries || !replayable {
			return resp, err
		}
		wait := time.Duration(500<<uint(try)) * time.Millisecond
		if v, e := strconv.Atoi(strings.TrimSpace(resp.Header.Get("Retry-After"))); e == nil && v > 0 {
			wait = time.Duration(v) * time.Second
		}
		wait += time.Duration(rand.Int64N(int64(wait)/4 + 1)) // jitter: slots must not retry in lockstep
		if dl, ok := req.Context().Deadline(); wait > friendlyMaxWait || (ok && time.Until(dl) < wait+time.Second) {
			return resp, nil // no room to wait: the caller relays it as "busy"
		}
		io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		resp.Body.Close()
		select {
		case <-time.After(wait):
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
		if req.GetBody != nil {
			if req.Body, err = req.GetBody(); err != nil {
				return nil, err
			}
		}
	}
}
