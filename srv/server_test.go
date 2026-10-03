package srv

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	tempDB := filepath.Join(t.TempDir(), "test.sqlite3")
	server, err := New(tempDB, "test-hostname")
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	return server
}

func postJSON(t *testing.T, h http.HandlerFunc, body map[string]any, hdr map[string]string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(b))
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h(w, req)
	var out map[string]any
	json.Unmarshal(w.Body.Bytes(), &out)
	return w, out
}

func TestPlayerAuth(t *testing.T) {
	s := newTestServer(t)

	// Register a player
	w, res := postJSON(t, s.handleRegister, map[string]any{"name": "TestSpieler"}, nil)
	if w.Code != 200 {
		t.Fatalf("register failed: %d %s", w.Code, w.Body.String())
	}
	token, _ := res["rejoin_token"].(string)
	if token == "" {
		t.Fatal("expected rejoin_token in register response")
	}
	player, _ := res["player"].(map[string]any)
	pid, _ := player["id"].(string)
	if pid == "" {
		t.Fatal("expected player id")
	}
	if _, leaked := player["rejoin_token"]; leaked {
		t.Error("player struct must not serialize rejoin_token")
	}

	// Session create without token → 401
	w, _ = postJSON(t, s.handleCreateSession, map[string]any{"player_id": pid, "name": "g"}, nil)
	if w.Code != 401 {
		t.Errorf("expected 401 without token, got %d", w.Code)
	}

	// Wrong token → 401
	w, _ = postJSON(t, s.handleCreateSession, map[string]any{"player_id": pid, "name": "g"},
		map[string]string{"X-Player-Token": "bogus"})
	if w.Code != 401 {
		t.Errorf("expected 401 with wrong token, got %d", w.Code)
	}

	// Claim parcel spoofing another player_id with valid token → 401
	w, _ = postJSON(t, s.handleClaimParcel, map[string]any{
		"session_id": "x", "player_id": "someone-else", "parcel_id": "p1", "area_sqm": 100.0,
	}, map[string]string{"X-Player-Token": token})
	if w.Code != 401 {
		t.Errorf("expected 401 for spoofed player_id, got %d", w.Code)
	}

	// Correct token + matching player_id → session create succeeds
	w, res = postJSON(t, s.handleCreateSession, map[string]any{
		"player_id": pid, "name": "g", "municipality_name": "Testdorf",
		"center_lon": 16.37, "center_lat": 48.21,
	}, map[string]string{"X-Player-Token": token})
	if w.Code != 200 {
		t.Fatalf("expected 200 with valid token, got %d %s", w.Code, w.Body.String())
	}
	if res["invite_code"] == "" {
		t.Error("expected invite_code")
	}
}

func TestValidKG(t *testing.T) {
	for kg, want := range map[string]bool{
		"01004": true, "92113": true,
		"1004": false, "010041": false, "01a04": false,
		"../..": false, "": false, "01004?x=1": false,
	} {
		if got := validKG(kg); got != want {
			t.Errorf("validKG(%q) = %v, want %v", kg, got, want)
		}
	}
}
