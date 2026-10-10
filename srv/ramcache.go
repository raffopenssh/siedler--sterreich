package srv

// RAM-only store for cadastre-derived bodies (AGENTS.md rule 2).
//
// Everything that is a bevdirect document — assembled cells (`vp:v1:*`),
// parcel look-ups (`parcel:v1:*`) and folio look-ups (`ez:v1:*`) — never
// touches SQLite: Store routes those keys here. Bodies are kept gzipped
// (cachePack), expire after ≤ ramMaxTTL regardless of the TTL asked for, and
// the oldest fetch is evicted first once ramMaxBytes is exceeded. A restart
// empties the store; kg_warm is reconciled at boot (reconcileRAMCacheAtBoot)
// so the warmer rebuilds what was warm instead of trusting stale rows.

import (
	"context"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	ramMaxTTL   = 24 * time.Hour
	ramMaxBytes = 1400 << 20 // gzipped bodies; ~2 900 cells fit
)

// similar/bldg-info/timber:est answers quote parcel ids and land use from the
// cells, so they are cadastre-derived too.
var ramPrefixes = []string{vpKeyPrefix, "parcel:v1:", "ez:v1:", "similar:", "bldg-info:", "timber:est:"}

func isRAMKey(k string) bool {
	for _, p := range ramPrefixes {
		if strings.HasPrefix(k, p) {
			return true
		}
	}
	return false
}

type ramEntry struct {
	data    string // packed (gzip above compressThreshold)
	fetched time.Time
	expires time.Time
}

type ramCache struct {
	mu    sync.Mutex
	m     map[string]*ramEntry
	bytes int64
}

var cadastreRAM = &ramCache{m: map[string]*ramEntry{}}

func (r *ramCache) get(k string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e := r.m[k]
	if e == nil {
		return "", false
	}
	if time.Now().After(e.expires) {
		r.dropLocked(k)
		return "", false
	}
	return e.data, true
}

func (r *ramCache) set(k, packed string, exp time.Time) {
	now := time.Now()
	if exp.After(now.Add(ramMaxTTL)) {
		exp = now.Add(ramMaxTTL)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dropLocked(k)
	r.m[k] = &ramEntry{data: packed, fetched: now, expires: exp}
	r.bytes += int64(len(packed))
	if r.bytes > ramMaxBytes {
		r.evictLocked()
	}
}

func (r *ramCache) dropLocked(k string) {
	if e := r.m[k]; e != nil {
		r.bytes -= int64(len(e.data))
		delete(r.m, k)
	}
}

// evictLocked removes expired entries, then the oldest fetches until under cap.
func (r *ramCache) evictLocked() {
	now := time.Now()
	for k, e := range r.m {
		if now.After(e.expires) {
			r.dropLocked(k)
		}
	}
	if r.bytes <= ramMaxBytes {
		return
	}
	keys := make([]string, 0, len(r.m))
	for k := range r.m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return r.m[keys[i]].fetched.Before(r.m[keys[j]].fetched) })
	n := 0
	for _, k := range keys {
		if r.bytes <= ramMaxBytes*9/10 {
			break
		}
		r.dropLocked(k)
		n++
	}
	slog.Info("ram cache: evicted oldest cadastre bodies", "rows", n, "mb", r.bytes>>20)
}

// touch extends the expiry (clamped to ramMaxTTL from the original fetch).
func (r *ramCache) touch(k string, exp time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e := r.m[k]; e != nil {
		if lim := e.fetched.Add(ramMaxTTL); exp.After(lim) {
			exp = lim
		}
		e.expires = exp
	}
}

// deleteLike removes keys matching a SQL LIKE pattern (% and _ wildcards).
func (r *ramCache) deleteLike(pattern string) int64 {
	rx := regexp.MustCompile("^" + strings.NewReplacer("%", ".*", "_", ".").Replace(regexp.QuoteMeta(pattern)) + "$")
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int64
	for k := range r.m {
		if rx.MatchString(k) {
			r.dropLocked(k)
			n++
		}
	}
	return n
}

// sweep drops expired entries; returns how many.
func (r *ramCache) sweep() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	var n int64
	for k, e := range r.m {
		if now.After(e.expires) {
			r.dropLocked(k)
			n++
		}
	}
	return n
}

// keys lists live keys with the prefix.
func (r *ramCache) keys(prefix string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	var out []string
	for k, e := range r.m {
		if strings.HasPrefix(k, prefix) && now.Before(e.expires) {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// stats: rows, bytes and oldest fetch for keys with the prefix ("" = all).
func (r *ramCache) stats(prefix string) (rows int64, bytes int64, oldest time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, e := range r.m {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		rows++
		bytes += int64(len(e.data))
		if oldest.IsZero() || e.fetched.Before(oldest) {
			oldest = e.fetched
		}
	}
	return
}

// reconcileRAMCacheAtBoot: the RAM store is empty after a restart, so (a) any
// cadastre rows a previous build left in api_cache are deleted and the freed
// pages returned, and (b) kg_warm rows — which promise warm cells — are
// re-queued for warming and dropped, so /api/lucky never lands on a cold KG.
func (s *Server) reconcileRAMCacheAtBoot() {
	ctx := context.Background()
	var purged int64
	for _, p := range ramPrefixes {
		n, _ := s.Q.Queries.DeleteCacheLike(ctx, p+"%")
		purged += n
	}
	if purged > 0 {
		s.incrementalVacuum()
		slog.Warn("ram cache: purged cadastre rows left on disk by a previous build", "rows", purged)
	}
	rows, err := s.DB.QueryContext(ctx, "SELECT kg_code FROM kg_warm WHERE expires_at > CURRENT_TIMESTAMP ORDER BY warmed_at DESC")
	if err != nil {
		return
	}
	var kgs []string
	for rows.Next() {
		var kg string
		if rows.Scan(&kg) == nil {
			kgs = append(kgs, kg)
		}
	}
	rows.Close()
	s.DB.ExecContext(ctx, "DELETE FROM kg_warm")
	if len(kgs) == 0 {
		return
	}
	n := 0
	for _, kg := range kgs {
		if s.enqueueWarm(kg, "reboot", 1) {
			n++
		}
	}
	slog.Info("ram cache: cells lost on restart, previously warm KGs re-queued", "kgs", n)
}
