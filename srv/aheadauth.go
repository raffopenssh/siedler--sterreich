package srv

// Token gate for /llm/ahead*. The roadmap names the sibling data services we
// build on; that is internal plumbing and must not be discoverable by agents
// browsing the public /llm/game edition. Unauthenticated requests get a plain
// 404 so the path does not even confirm its own existence.
//
// Token: env SIEDLER_AHEAD_TOKEN, else ./ahead.key (generated on first use,
// 0600, next to safety.key). Send it as header X-Ahead-Token (preferred) or
// ?token= (for a browser bookmark).

import (
	"bytes"
	"crypto/subtle"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

var (
	aheadTokOnce sync.Once
	aheadTok     []byte
)

func (s *Server) aheadToken() []byte {
	aheadTokOnce.Do(func() {
		if k := os.Getenv("SIEDLER_AHEAD_TOKEN"); k != "" {
			aheadTok = []byte(k)
			return
		}
		path := filepath.Join(filepath.Dir(s.StaticDir), "..", "ahead.key")
		if b, err := os.ReadFile(path); err == nil && len(bytes.TrimSpace(b)) >= 16 {
			aheadTok = bytes.TrimSpace(b)
			return
		}
		aheadTok = []byte(randomID(32))
		if err := os.WriteFile(path, aheadTok, 0o600); err != nil {
			slog.Warn("ahead token not persisted; regenerated on restart", "err", err)
		}
		slog.Info("generated /llm/ahead token", "path", path)
	})
	return aheadTok
}

// aheadAuthed reports whether r carries the roadmap token.
func (s *Server) aheadAuthed(r *http.Request) bool {
	got := r.Header.Get("X-Ahead-Token")
	if got == "" {
		got = r.URL.Query().Get("token")
	}
	want := s.aheadToken()
	return got != "" && subtle.ConstantTimeCompare([]byte(got), want) == 1
}

// requireAhead wraps a roadmap handler: 404 without token, never cached.
func (s *Server) requireAhead(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.aheadAuthed(r) {
			w.Header().Set("Cache-Control", "no-store")
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		h(w, r)
	}
}
