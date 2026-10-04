package srv

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Per-host circuit breaker for the sibling data services.
//
// When a sibling (srtm-lidar-at, cadastre, holz, farm, gw) goes down, the
// exe.dev proxy in front of it stalls ~10 s and then answers an HTML 503.
// Without a breaker every viewport tile × layer × client pays those 10 s,
// goroutines and singleflight keys pile up, MaxConnsPerHost saturates and
// even healthy paths start to queue. The breaker sits in upstreamClient's
// Transport (so *every* caller — upstreamGet, upstreamClient.Do, ad-hoc
// clients sharing the Transport — is covered) and:
//
//   - counts consecutive failures per host (transport errors, 502/503/504);
//     the caller's own context deadline/cancel is NOT a failure (short
//     budgets on cold KGs are normal);
//   - after breakerTrip failures opens and answers a synthetic 503 JSON
//     `{error, status:"down", service, retry_after_s}` in ~0 ms with
//     Retry-After + X-Upstream: down;
//   - while open exactly one probe request is let through at a time, the
//     first after 20 s, then doubling per failed probe up to 3 min; the first
//     probe that is not a transport error / 502-504 closes it again.
//
// The synthetic body never names the host; `service` is a generic label.

const (
	breakerTrip     = 3
	breakerBaseOpen = 20 * time.Second
	breakerMaxOpen  = 3 * time.Minute
)

type breakerHost struct {
	fails     int
	opens     int       // consecutive failed probes → backoff exponent
	open      bool      // tripped: reject until one probe succeeds
	nextProbe time.Time // earliest moment the next probe may go out
	probing   bool      // a probe is in flight (only one at a time)
	lastErr   string
	downSince time.Time
}

type breakerTransport struct {
	base  http.RoundTripper
	mu    sync.Mutex
	hosts map[string]*breakerHost
}

var upstreamBreaker = &breakerTransport{hosts: map[string]*breakerHost{}}

// hostOf returns the host part of an upstream base URL ("" on parse error).
func hostOf(u string) string {
	if p, err := url.Parse(u); err == nil {
		return p.Host
	}
	return u
}

// serviceLabel maps an upstream host to the generic label exposed to clients.
func serviceLabel(host string) string {
	switch {
	case strings.Contains(host, "srtm") || strings.Contains(host, "lidar"):
		return "terrain"
	case strings.Contains(host, "cadastre"), host == hostOf(bevAPI): // bevdirect-serve (local)
		return "cadastre"
	case strings.Contains(host, "holz"):
		return "forest"
	case strings.Contains(host, "farm"):
		return "farm"
	case strings.Contains(host, "gw") || strings.Contains(host, "water"):
		return "water"
	}
	return "data"
}

func (b *breakerTransport) state(host string) *breakerHost {
	h := b.hosts[host]
	if h == nil {
		h = &breakerHost{}
		b.hosts[host] = h
	}
	return h
}

func (b *breakerTransport) backoff(h *breakerHost) time.Duration {
	d := breakerBaseOpen << uint(h.opens)
	if d > breakerMaxOpen {
		d = breakerMaxOpen
	}
	return d
}

func (b *breakerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	host := req.URL.Host
	now := time.Now()
	b.mu.Lock()
	h := b.state(host)
	isProbe := false
	if h.open {
		if !h.probing && !now.Before(h.nextProbe) {
			h.probing, isProbe = true, true // half-open: exactly one probe in flight
		} else {
			ra := int(time.Until(h.nextProbe).Seconds()) + 1
			if ra < 5 {
				ra = 5
			}
			b.mu.Unlock()
			return breakerResponse(req, host, ra), nil
		}
	}
	b.mu.Unlock()

	if isProbe {
		if hu := breakerHealthURL(host); hu != "" {
			// Siblings with a cheap liveness endpoint are probed there (~2 ms)
			// instead of replaying the caller's (possibly expensive) request.
			ok := b.healthOK(hu)
			b.mu.Lock()
			h.probing = false
			if ok {
				if !h.downSince.IsZero() {
					log.Printf("breaker %s: recovered after %s", serviceLabel(host), time.Since(h.downSince).Round(time.Second))
				}
				*h = breakerHost{}
				b.mu.Unlock()
				isProbe = false // closed: the real request goes through normally
			} else {
				if h.opens < 10 {
					h.opens++
				}
				h.nextProbe = time.Now().Add(b.backoff(h))
				ra := int(b.backoff(h).Seconds()) + 1
				b.mu.Unlock()
				return breakerResponse(req, host, ra), nil
			}
		}
	}

	resp, err := b.base.RoundTrip(req)

	failed := false
	reason := ""
	if err != nil {
		if req.Context().Err() == nil { // not the caller giving up
			failed, reason = true, err.Error()
		}
	} else if resp.StatusCode == 502 || resp.StatusCode == 503 || resp.StatusCode == 504 {
		// A 503 *with* Retry-After is a sibling saying "busy, come back in n s"
		// (srtm tile renderer, farm host slots) — load shedding, not an outage.
		if !(resp.StatusCode == 503 && resp.Header.Get("Retry-After") != "") {
			failed, reason = true, "HTTP "+strconv.Itoa(resp.StatusCode)
		}
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if isProbe {
		h.probing = false
	}
	if !failed {
		if err == nil && (h.fails > 0 || h.open) {
			if !h.downSince.IsZero() {
				log.Printf("breaker %s: recovered after %s", serviceLabel(host), time.Since(h.downSince).Round(time.Second))
			}
			*h = breakerHost{}
		}
		return resp, err
	}
	h.fails++
	h.lastErr = reason
	if isProbe {
		// Failed probe: stay open, back off longer before the next one.
		if h.opens < 10 {
			h.opens++
		}
		h.nextProbe = time.Now().Add(b.backoff(h))
	} else if !h.open && h.fails >= breakerTrip {
		h.open = true
		h.downSince = time.Now()
		h.nextProbe = time.Now().Add(b.backoff(h))
		log.Printf("breaker %s: OPEN, next probe in %s (%s)", serviceLabel(host), b.backoff(h), reason)
	}
	return resp, err
}

// breakerHealthURL: the liveness probe to use while a host's breaker is open
// ("" = replay the caller's request as the probe).
func breakerHealthURL(host string) string {
	if host == hostOf(lidarAPI) {
		return "https://" + host + "/api/v1/health" // fast (~2 ms); never / or a tile
	}
	return ""
}

func (b *breakerTransport) healthOK(u string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	resp, err := b.base.RoundTrip(req)
	if err != nil {
		return false
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	resp.Body.Close()
	return resp.StatusCode < 500
}

func breakerResponse(req *http.Request, host string, retryAfter int) *http.Response {
	body, _ := json.Marshal(map[string]any{
		"error":         "upstream unavailable",
		"status":        "down",
		"service":       serviceLabel(host),
		"retry_after_s": retryAfter,
	})
	hdr := http.Header{}
	hdr.Set("Content-Type", "application/json")
	hdr.Set("Retry-After", strconv.Itoa(retryAfter))
	hdr.Set("X-Upstream", "down")
	return &http.Response{
		Status: "503 Service Unavailable", StatusCode: 503, Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
		Header: hdr, Body: io.NopCloser(bytes.NewReader(body)), ContentLength: int64(len(body)), Request: req,
	}
}

// upstreamDown reports whether the breaker for host is currently open.
func upstreamDown(host string) bool {
	upstreamBreaker.mu.Lock()
	defer upstreamBreaker.mu.Unlock()
	h := upstreamBreaker.hosts[host]
	return h != nil && h.open
}

// handleUpstreamStatus — GET /api/upstreams: breaker state per service label
// (no hostnames). Lets the client show "Geländedaten derzeit offline" and
// helps ops see which sibling is misbehaving.
func (s *Server) handleUpstreamStatus(w http.ResponseWriter, r *http.Request) {
	type st struct {
		Status    string `json:"status"`
		DownForS  int    `json:"down_for_s,omitempty"`
		RetryInS  int    `json:"retry_in_s,omitempty"`
		Fails     int    `json:"consecutive_failures,omitempty"`
		LastError string `json:"last_error,omitempty"`
	}
	out := map[string]st{}
	now := time.Now()
	upstreamBreaker.mu.Lock()
	for host, h := range upstreamBreaker.hosts {
		v := st{Status: "ok", Fails: h.fails}
		if h.open {
			v.Status = "down"
			v.DownForS = int(now.Sub(h.downSince).Seconds())
			v.RetryInS = int(h.nextProbe.Sub(now).Seconds()) + 1
			if v.RetryInS < 0 {
				v.RetryInS = 0
			}
			v.LastError = h.lastErr
		}
		out[serviceLabel(host)] = v
	}
	upstreamBreaker.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(out)
}
