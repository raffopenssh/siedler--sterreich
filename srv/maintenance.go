package srv

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Maintenance mode: touch ./MAINTENANCE (or set SIEDLER_MAINTENANCE=1) and
// every request gets a 503 "Wir sind bald zurück" page (JSON for /api and
// agents). Remove the file to reopen — no restart needed (checked every 2 s).

var maintMu sync.Mutex
var maintChecked time.Time
var maintOn bool

func maintenanceActive() bool {
	maintMu.Lock()
	defer maintMu.Unlock()
	if time.Since(maintChecked) < 2*time.Second {
		return maintOn
	}
	maintChecked = time.Now()
	if os.Getenv("SIEDLER_MAINTENANCE") == "1" {
		maintOn = true
		return true
	}
	_, err := os.Stat("MAINTENANCE")
	maintOn = err == nil
	return maintOn
}

func (s *Server) maintenanceMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !maintenanceActive() {
			next.ServeHTTP(w, r)
			return
		}
		p := r.URL.Path
		// Owner/QA bypass while the site is closed: cookie `siedler_dev=1`
		// (set by ?dev=1 on the index) or the same query flag.
		if c, err := r.Cookie("siedler_dev"); (err == nil && c.Value == "1") || r.URL.Query().Get("dev") == "1" {
			if r.URL.Query().Get("dev") == "1" {
				http.SetCookie(w, &http.Cookie{Name: "siedler_dev", Value: "1", Path: "/", MaxAge: 86400, HttpOnly: true, SameSite: http.SameSiteLaxMode})
			}
			next.ServeHTTP(w, r)
			return
		}
		// assets the maintenance page itself needs
		if strings.HasPrefix(p, "/static/fonts/") || p == "/static/game-bg.jpg" || p == "/robots.txt" {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Retry-After", "3600")
		w.Header().Set("Cache-Control", "no-store")
		if strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/llm/") || isAgentUA(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]any{
				"error":         "maintenance",
				"status":        "maintenance",
				"message":       "Siedler Österreich is down for a major provider revision. We will be back soon.",
				"retry_after_s": 3600,
			})
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		http.ServeFile(&noStatusWriter{w}, r, filepath.Join(s.StaticDir, "maintenance.html"))
	})
}

// noStatusWriter swallows the WriteHeader from ServeFile so our 503 sticks.
type noStatusWriter struct{ http.ResponseWriter }

func (n *noStatusWriter) WriteHeader(int) {}
