package srv

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The OpenAPI document is hand-written JSON — make sure an edit never ships
// a syntax error, and that the agent responses document the BEV notice.
func TestOpenAPIValidJSON(t *testing.T) {
	s := newTestServer(t)
	w := httptest.NewRecorder()
	s.handleOpenAPI(w, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	if w.Code != 200 {
		t.Fatalf("status %d", w.Code)
	}
	var doc map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("openapi.json is not valid JSON: %v", err)
	}
	comps := doc["components"].(map[string]any)
	resps := comps["responses"].(map[string]any)
	for _, k := range []string{"Pending", "Down", "Err", "TooMany"} {
		if resps[k] == nil {
			t.Errorf("components.responses.%s missing", k)
		}
	}
	if agentAttribution["notice"] != bevNotice {
		t.Errorf("agentAttribution must carry the BEV notice")
	}
}

func TestRelayCellStatus(t *testing.T) {
	w := httptest.NewRecorder()
	relayCellStatus(w, cellStatus{Status: 202, RetryAfter: 4.2}, map[string]any{"parcel_id": "x"})
	if w.Code != 202 || w.Header().Get("Retry-After") != "5" {
		t.Fatalf("202 relay: code %d Retry-After %q", w.Code, w.Header().Get("Retry-After"))
	}
	var b map[string]any
	json.Unmarshal(w.Body.Bytes(), &b)
	if b["status"] != "pending" || b["retry_after_s"].(float64) != 4.2 || b["parcel_id"] != "x" {
		t.Fatalf("202 body %v", b)
	}
	w = httptest.NewRecorder()
	relayCellStatus(w, cellStatus{Status: 503, RetryAfter: 20, Body: []byte(`{"error":"x","status":"down","service":"cadastre","retry_after_s":20}`)}, nil)
	if w.Code != 503 || w.Header().Get("X-Upstream") != "down" || w.Header().Get("Retry-After") != "20" {
		t.Fatalf("503 relay: %d %v", w.Code, w.Header())
	}
	w = httptest.NewRecorder()
	relayCellStatus(w, cellStatus{Status: 502}, nil)
	if w.Code != 502 {
		t.Fatalf("502 relay: %d", w.Code)
	}
}
