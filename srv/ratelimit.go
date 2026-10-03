package srv

// Request rate limiting for the game API (humans and /llm/game agents alike).
//
// Privacy contract (Datenschutz §4): the application does not log or store IP
// addresses. Buckets live in memory only. Authenticated calls are keyed by the
// pseudonymous X-Player-Token; the few unauthenticated ones (register) are
// keyed by an HMAC-style hash of the IP with a salt that rotates hourly, so
// the key is unlinkable after the hour and is never written anywhere.

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type rlBucket struct {
	tokens float64
	last   time.Time
}

// tokenBucket: `burst` requests instantly, refilling at `perMin` per minute.
type tokenBucket struct {
	mu      sync.Mutex
	m       map[string]*rlBucket
	perMin  float64
	burst   float64
	sweptAt time.Time
}

func newBucket(perMin, burst float64) *tokenBucket {
	return &tokenBucket{m: map[string]*rlBucket{}, perMin: perMin, burst: burst, sweptAt: time.Now()}
}

// take returns ok, seconds until the next token, and remaining tokens.
func (t *tokenBucket) take(key string) (bool, int, int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	if now.Sub(t.sweptAt) > 10*time.Minute || len(t.m) > 20000 {
		for k, b := range t.m {
			if now.Sub(b.last) > 15*time.Minute {
				delete(t.m, k)
			}
		}
		t.sweptAt = now
	}
	b := t.m[key]
	if b == nil {
		b = &rlBucket{tokens: t.burst, last: now}
		t.m[key] = b
	}
	b.tokens = math.Min(t.burst, b.tokens+now.Sub(b.last).Minutes()*t.perMin)
	b.last = now
	if b.tokens < 1 {
		wait := int(math.Ceil((1 - b.tokens) / t.perMin * 60))
		if wait < 1 {
			wait = 1
		}
		return false, wait, 0
	}
	b.tokens--
	return true, 0, int(b.tokens)
}

var (
	rlActions  = newBucket(60, 20) // any mutating /api/* call, per player token
	rlLook     = newBucket(30, 10) // GET /api/agent/*, per player token
	rlRegister = newBucket(0.5, 5) // POST /api/register: 5 burst, then 1 per 2 min, per salted IP hash
	rlSessions = newBucket(10, 3)  // POST /api/session/create, per player token

	ipSaltMu   sync.Mutex
	ipSalt     []byte
	ipSaltHour int
)

// ipKey hashes the client IP with an hourly-rotating random salt. The result
// is only ever used as an in-memory map key.
func ipKey(r *http.Request) string {
	ip := r.Header.Get("X-Forwarded-For")
	if i := strings.IndexByte(ip, ','); i > 0 {
		ip = ip[:i]
	}
	ip = strings.TrimSpace(ip)
	if ip == "" {
		ip, _, _ = net.SplitHostPort(r.RemoteAddr)
	}
	ipSaltMu.Lock()
	h := time.Now().Hour()
	if ipSalt == nil || h != ipSaltHour {
		ipSalt = make([]byte, 16)
		rand.Read(ipSalt)
		ipSaltHour = h
	}
	salt := ipSalt
	ipSaltMu.Unlock()
	sum := sha256.Sum256(append(append([]byte{}, salt...), ip...))
	return "ip:" + hex.EncodeToString(sum[:8])
}

func rlKey(r *http.Request) string {
	if tok := r.Header.Get("X-Player-Token"); tok != "" {
		return "tok:" + tok
	}
	return ipKey(r)
}

// rateLimitMiddleware applies the buckets above. Read-only game/data GETs are
// not limited here (they are cache-backed and the browser needs them freely).
func rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var (
			b   *tokenBucket
			key string
		)
		p := r.URL.Path
		switch {
		case r.Method == http.MethodPost && p == "/api/register":
			b, key = rlRegister, ipKey(r)
		case r.Method == http.MethodPost && p == "/api/session/create":
			b, key = rlSessions, rlKey(r)
		case strings.HasPrefix(p, "/api/agent/"):
			if r.Method == http.MethodGet {
				b, key = rlLook, rlKey(r)
			} else {
				b, key = rlActions, rlKey(r)
			}
		case r.Method != http.MethodGet && strings.HasPrefix(p, "/api/"):
			b, key = rlActions, rlKey(r)
		}
		if b == nil {
			next.ServeHTTP(w, r)
			return
		}
		ok, wait, remaining := b.take(key)
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(int(b.perMin))+"/min")
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
		if !ok {
			w.Header().Set("Retry-After", strconv.Itoa(wait))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]any{
				"error":         "rate limited",
				"retry_after_s": wait,
				"hint":          "see " + siteURL + "/llm/game#rate-limits",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}
