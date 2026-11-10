package srv

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
)

// rewrite sends every request to the test server but keeps the logical host.
type rewrite struct{ to *url.URL }

func (r rewrite) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme, req.URL.Host = r.to.Scheme, r.to.Host
	return http.DefaultTransport.RoundTrip(req)
}

func TestFriendlyTransport(t *testing.T) {
	var n atomic.Int32
	var ua, tok atomic.Value
	t.Setenv("SIEDLER_FARM_PEER_TOKEN", "test-token")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua.Store(r.UserAgent())
		tok.Store(r.Header.Get("X-Peer-Token"))
		if n.Add(1) < 3 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(429)
			return
		}
		w.WriteHeader(200)
	}))
	defer ts.Close()
	u, _ := url.Parse(ts.URL)
	c := &http.Client{Transport: &friendlyTransport{base: rewrite{u}, component: "test"}}

	req, _ := http.NewRequest("GET", farmAPI+"/api/schlaege", nil)
	req.Header.Set("User-Agent", "something-else/1")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 || n.Load() != 3 {
		t.Fatalf("status %d after %d tries, want 200 after 3", resp.StatusCode, n.Load())
	}
	if got, want := ua.Load().(string), "siedler-oesterreich/test (+https://siedler-oesterreich.exe.xyz)"; got != want {
		t.Fatalf("UA %q, want %q", got, want)
	}
	if tok.Load().(string) != "test-token" {
		t.Fatalf("peer token not sent: %q", tok.Load())
	}
	if req.Header.Get("User-Agent") != "something-else/1" || req.Header.Get("X-Peer-Token") != "" {
		t.Fatal("caller's request was modified")
	}

	// other hosts are left alone
	n.Store(10)
	req, _ = http.NewRequest("GET", "https://example.invalid/x", nil)
	req.Header.Set("User-Agent", "keep/1")
	if resp, err = c.Do(req); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if tok.Load().(string) != "" {
		t.Fatal("peer token leaked to a foreign host")
	}
	if ua.Load().(string) != "keep/1" {
		t.Fatalf("foreign host UA rewritten: %q", ua.Load())
	}
}

// Live check against the farm sibling through the real clients:
// SIEDLER_LIVE=1 go test ./srv/ -run FriendlyLive -v
func TestFriendlyLive(t *testing.T) {
	if os.Getenv("SIEDLER_LIVE") == "" {
		t.Skip("set SIEDLER_LIVE=1")
	}
	for name, c := range map[string]*http.Client{"upstream": upstreamClient, "ahead": aheadProbeClient} {
		req, _ := http.NewRequest("GET", farmAPI+"/api/hofstellen?west=16.36&south=48.20&east=16.38&north=48.22", nil)
		req.Header.Set("User-Agent", "siedler-ahead-check/1")
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(name, err)
		}
		resp.Body.Close()
		if tier := resp.Header.Get("X-RateLimit-Tier"); tier != friendlyTier(farmPeerToken()) {
			t.Errorf("%s: tier %q (HTTP %d), want %s", name, tier, resp.StatusCode, friendlyTier(farmPeerToken()))
		}
	}
}
