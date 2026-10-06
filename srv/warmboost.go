package srv

// Boost days — a deliberately larger prewarm budget for a bounded period
// (a presentation, a workshop), so GET /api/lucky has plenty of warm, enhanced
// destinations and nobody waits on a cold cell.
//
// Switch: a file ./WARM_BOOST (or SIEDLER_WARM_BOOST). Its first line is the
// end of the boost as `2006-01-02` (local midnight at the *start* of that
// day, i.e. "boost through the 8th" = `2026-10-09`) or RFC 3339; an empty
// file means 48 h from its mtime. Delete the file to stop early.
//
// Budget while boosted (warmBoostPatches × warmPatchSize KGs/day):
//   ~0.3 MB of BEV tiles per cell, ~7 cells per KG → ~2 MB/KG, so 250 KGs
//   ≈ 0.5 GB of cadastre tiles a day plus NE documents — about half the
//   1 GB/day ceiling we allow ourselves on such days, 2.5× the normal plan.
//   The worker still paces one cell at a time (warmCellPause, yields to
//   foreground builds), so this never bursts at BEV; it just runs longer.
// Boost also pins the activity tier to "boost" (never idle: every patch
// runs, recently played v2.4 KGs are kept warm) and the plan key carries a
// `boost` tag so turning it on mid-day builds a bigger plan immediately
// (patches already past due are queued at once — the queue serialises them).

import (
	"os"
	"strings"
	"sync"
	"time"
)

const (
	warmBoostFile    = "WARM_BOOST"
	warmBoostPatches = 50 // 250 KGs/day ≈ 0.5 GB BEV tiles; keep ≤ 100 (1 GB)
	warmBoostDefault = 48 * time.Hour
)

var boostCache struct {
	sync.Mutex
	at    time.Time
	until time.Time
}

// warmBoostUntil returns the end of the current boost period (zero when not
// boosted). Stat-ed at most every 20 s.
func warmBoostUntil() time.Time {
	boostCache.Lock()
	defer boostCache.Unlock()
	if time.Since(boostCache.at) < 20*time.Second {
		return boostCache.until
	}
	boostCache.at = time.Now()
	boostCache.until = readBoostFile()
	return boostCache.until
}

func readBoostFile() time.Time {
	path := warmBoostFile
	if p := os.Getenv("SIEDLER_WARM_BOOST"); p != "" {
		path = p
	}
	st, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	raw, _ := os.ReadFile(path)
	line := strings.TrimSpace(strings.SplitN(string(raw), "\n", 2)[0])
	if line == "" {
		return st.ModTime().Add(warmBoostDefault)
	}
	if t, err := time.ParseInLocation("2006-01-02", line, time.Local); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, line); err == nil {
		return t
	}
	return st.ModTime().Add(warmBoostDefault) // unparsable → default window
}

func warmBoosted() bool {
	u := warmBoostUntil()
	return !u.IsZero() && time.Now().Before(u)
}

// warmPatchesNow is the number of daily patches the planner should build.
func warmPatchesNow() int {
	if warmBoosted() {
		return warmBoostPatches
	}
	return warmPatches
}

func warmBoostStatus() map[string]any {
	u := warmBoostUntil()
	m := map[string]any{"active": warmBoosted(), "patches": warmBoostPatches, "daily_kgs": warmBoostPatches * warmPatchSize,
		"est_mb_per_day": warmBoostPatches * warmPatchSize * 2, "file": warmBoostFile}
	if !u.IsZero() {
		m["until"] = u.UTC().Format(time.RFC3339)
	}
	return m
}
