package srv

import (
	"net/http"
	"testing"
)

func TestParsePendingCadastre(t *testing.T) {
	body := []byte(`{"status":"pending","warming":{"kgs":[{"kg":"30001","state":"downloading","pct":40,"eta_s":6},{"kg":"30002","state":"queued"}],"retry_after_s":8,"zenodo":{"status":"slow"}}}`)
	h := http.Header{}
	h.Set("Retry-After", "8")
	p := parsePending(h, body)
	if p.RetryAfter != 8 || p.Zenodo != "slow" || len(p.KGs) != 2 || p.Progress.State != "downloading" || p.Progress.Pct != 20 || p.Progress.EtaS != 6 {
		t.Fatalf("bad parse: %+v", p)
	}
	if retryAfterOf(p.body()) != 8 {
		t.Fatalf("retryAfterOf: %d", retryAfterOf(p.body()))
	}
}

func TestParsePendingLidar(t *testing.T) {
	body := []byte(`{"status":"fetching","kg_code":"34086","fetch":{"status":"downloading","pct":4.4,"eta_s":610.5},"retry_after_s":5,"zenodo":{"slow":true}}`)
	p := parsePending(http.Header{}, body)
	if p.RetryAfter != 5 || p.Zenodo != "slow" || len(p.KGs) != 1 || p.KGs[0].KG != "34086" || p.Progress.Pct != 4.4 {
		t.Fatalf("bad parse: %+v", p)
	}
}

func TestParsePendingGarbage(t *testing.T) {
	p := parsePending(http.Header{}, []byte("<html>"))
	if p.RetryAfter != 3 || p.Zenodo != "healthy" {
		t.Fatalf("defaults: %+v", p)
	}
	if !isReadyFalse([]byte(`{"ready":false}`)) || isReadyFalse([]byte(`{"ready":true}`)) || isReadyFalse([]byte(`{}`)) {
		t.Fatal("isReadyFalse")
	}
	if withWait("http://x/a?b=1", 30) != "http://x/a?b=1&wait=30" || withWait("http://x/a?wait=0", 30) != "http://x/a?wait=0" || withWait("http://x/a", 5) != "http://x/a?wait=5" {
		t.Fatal("withWait")
	}
}
