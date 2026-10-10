package srv

// Contrib rotation — which KGs the nightly NE epoch report
// (tools/ne-report, → umfeld /contrib/api/v1/ne/{kg}/report) should build today.
//
// Goal: cover the whole of Austria — every KG of the admin table (7 850; umfeld
// has built an lu digest for all of them, enhanced or not) — at least once per
// quarter, so umfeld sees a fresh bevdirect digest for every KG about four
// times a year. That cadence is what makes the change
// protocol (vtcseamless-py 0.2.0, tools/ne-report) effective: the first report
// of a KG baselines all its chunks, every later one uploads only the changed
// chunks — a KG that is never reported has no baseline and its changes are
// invisible, and the quarter is the change resolution for every KG. Hence
// night_min scales with the universe (contribNightMinFor: 1.5 × universe/days,
// 7 850 → 128/night ≈ 35 min, ~300 MB tiles), overdue KGs (promoted mid-quarter
// after their hash day) are pulled in, and warmContribRun prewarms tonight's
// list at 23:00 Vienna time regardless of the player activity tier.
// Every night runs at least night_min KGs, pulling not-yet-reported KGs forward. Every KG is assigned a day of the quarter by
// hash(quarter, kg) — stable for the quarter, random across Austria, and new
// KGs appearing mid-quarter simply fall onto some day without shifting the
// others. Today's list is the KGs of the last `contribCatchUpDays` days (a
// missed night is caught up; ne_report.py skips anything reported < 7 d ago).
//
// Completely separate from prewarming: it reads bevdirect directly, writes no
// kg_warm rows and no api_cache cells, so /api/lucky (which only trusts
// kg_warm + the enhanced registry) is unaffected. Load ≈ 16 KGs × ~10 cells
// a night ≈ 30 MB of BEV tiles, at 01:00 UTC with Nice=15, CPUQuota 60 %.
//
// GET /api/contrib/plan → {quarter, day, days, kgs[], per_day_avg, universe,
// reported_quarter, catch_up_days}; also `contrib{}` in /api/warm/status.

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

const (
	contribCatchUpDays = 3                  // today + the two nights before (ne_report's 7-day skip dedups)
	contribNightMin    = 200                // floor: fill every night to this many KGs — what the 00:00–05:00 window holds (prewarm ~90 min, report ~90 min); ≈ 0.45 GB tiles
	contribNightMargin = 1.5                // night_min = max(floor, margin × universe/days)
	contribNightMax    = 200                // cap on KGs needing fresh BEV tiles; warm (cheap) KGs are never capped
	contribWarmCap     = 200                // KGs of tonight's plan the prewarmer builds at midnight (warmContribRun) — the whole night
	contribRollMinAge  = 7 * 24 * time.Hour // rolling sweep: once the quarter's unreported KGs are done, re-report the least recently reported (ne_report skips < 7 d anyway)
	// Night schedule, Europe/Vienna wall clock (DST-safe): the prewarm (bevdirect assembly, paced
	// 1.5 s/cell, ~3 s per cell → ~85 min for a 200-KG night) starts at 22:00, the python report run at 01:00
	// (CPUQuota 100 %, ~34 s CPU + ~10 s overhead per KG → ~2.8 h) — everything settled before 05:00 (goal
	// 05:30), no CPU spike during the day. Measured 2026-10-10: 267 reports, 13 915 s build + 2 359 s fetch
	// at 60 % quota = 5.3 h, because the fill ignored the prewarmed KGs (see contribPlanNow).
	contribWarmHour   = 22             // Vienna hour the contrib prewarm fires (cells → RAM stash, 24 h); ~200 KGs × 25 s ≈ 85 min
	contribNightHour  = 1              // Vienna hour ne-report.timer fires (keep in sync with tools/ne-report/ne-report.timer); ~225 KGs × 45 s ≈ 2.8 h at CPUQuota 100 % → settled before 04:30
	contribWarmWindow = 24 * time.Hour // bevdirect's in-RAM tile LRU (-tile-ttl 24h) still holds the tiles of KGs built within this window
	contribReportDir  = "data/ne-reports"
)

// quarterOf returns the quarter label ("2026-Q4"), its first day and length in days.
func quarterOf(now time.Time) (label string, start time.Time, days int) {
	q := (int(now.Month()) - 1) / 3
	start = time.Date(now.Year(), time.Month(q*3+1), 1, 0, 0, 0, 0, now.Location())
	end := start.AddDate(0, 3, 0)
	days = int(end.Sub(start).Hours()/24 + 0.5)
	label = start.Format("2006") + "-Q" + string(rune('1'+q))
	return
}

// contribNightMinFor scales the nightly minimum with the universe so the
// whole v2.4 set is swept once a quarter with a 50 % margin for skipped nights.
func contribNightMinFor(universe, days int) int {
	n := int(float64(universe)/float64(max(days, 1))*contribNightMargin + 0.999)
	return max(contribNightMin, n)
}

// contribDayOf: the day of the quarter (0-based) a KG is reported on.
func contribDayOf(quarter, kg string, days int) int {
	h := sha256.Sum256([]byte("contrib:" + quarter + ":" + kg))
	return int(binary.BigEndian.Uint32(h[:4]) % uint32(days))
}

type contribPlan struct {
	Quarter     string   `json:"quarter"`
	Day         int      `json:"day"`  // 0-based day of the quarter
	Days        int      `json:"days"` // length of the quarter
	KGs         []string `json:"kgs"`  // tonight's list: today + catch-up + fill (ahead of schedule)
	Today       []string `json:"today"`
	Fill        []string `json:"fill"`       // not-yet-reported KGs pulled forward to reach night_min
	Cheap       []string `json:"cheap"`      // of those: warmed < 24 h ago (bevdirect tiles cached → no new BEV load)
	AheadDays   int      `json:"ahead_days"` // how far ahead of the hash schedule the fill reaches
	Overdue     int      `json:"overdue"`    // fill KGs whose hash day already passed (promoted mid-quarter)
	Rolling     int      `json:"rolling"`    // fill KGs already reported this quarter, re-reported oldest first (continuous coverage of the whole universe)
	Small       int      `json:"small"`      // unreported KGs left out: their declared umfeld viewport holds no whole aligned cell (ne_report --min-full-cells 1 skips them as viewport_too_small) or /head is 404 — known from the cached head only
	NightMin    int      `json:"night_min"`
	NightMax    int      `json:"night_max"`
	LeftQ       int      `json:"left_quarter"`     // v2.4 KGs without a report this quarter
	Universe    int      `json:"universe"`         // v2.4 KGs known
	PerDayAvg   float64  `json:"per_day_avg"`      // universe / days
	ReportedQ   int      `json:"reported_quarter"` // report files written this quarter
	CatchUpDays int      `json:"catch_up_days"`
	Source      string   `json:"source"` // "registry" | "none"
	NextForKG   string   `json:"next_for_kg,omitempty"`
}

// contribPlanNow builds today's contrib rotation from the v2.4 registry.
func (s *Server) contribPlanNow(now time.Time) contribPlan {
	label, start, days := quarterOf(now)
	day := int(now.Sub(start).Hours() / 24)
	if day < 0 {
		day = 0
	}
	if day >= days {
		day = days - 1
	}
	var uni []string
	for kg := range admin().KGs {
		if len(kg) == 5 {
			uni = append(uni, kg)
		}
	}
	sort.Strings(uni)
	p := contribPlan{Quarter: label, Day: day, Days: days, Universe: len(uni), CatchUpDays: contribCatchUpDays,
		NightMin: contribNightMinFor(len(uni), days), NightMax: contribNightMax, KGs: []string{}, Today: []string{}, Fill: []string{}, Cheap: []string{}, Source: "registry"}
	if len(uni) == 0 {
		p.Source = "none"
		return p
	}
	p.PerDayAvg = float64(len(uni)) / float64(days)
	lastReport := contribLastReport()
	reported := map[string]bool{}
	since := start.Format("2006-01-02")
	for kg, d := range lastReport {
		if d >= since {
			reported[kg] = true
		}
	}
	p.ReportedQ = len(reported)
	for _, kg := range uni {
		d := contribDayOf(label, kg, days)
		if d == day {
			p.Today = append(p.Today, kg)
			p.KGs = append(p.KGs, kg)
		} else if d < day && d > day-contribCatchUpDays && !reported[kg] {
			p.KGs = append(p.KGs, kg) // missed night, still unreported
		}
	}
	// Today's first, then the catch-up days (oldest last — they are mostly skipped anyway).
	sort.SliceStable(p.KGs, func(i, j int) bool {
		return contribDayOf(label, p.KGs[i], days) > contribDayOf(label, p.KGs[j], days)
	})
	// Ahead of schedule: when the hash schedule gives fewer than night_min
	// KGs, pull forward the unreported KGs in due order — overdue ones first
	// (a KG promoted to v2.4 mid-quarter whose hash day already passed would
	// otherwise never be reported this quarter), then the ones due later. A
	// small universe is then swept in weeks instead of a quarter; once
	// everything is reported the nights are quiet until the next quarter.
	// The cap protects the night when the universe jumps.
	// Tier 1 — KGs the prewarmer built in the last 24 h: their BEV tiles are
	// still on bevdirect's disk, so the report costs CPU only (~30 s/KG) —
	// all of them, no cap. Tier 2 — the rest in due order, up to night_min.
	// KGs the report would skip anyway (viewport_too_small / not built) are
	// left out so they neither occupy fill slots forever (128 of the 177 fill
	// slots on 2026-10-10 were such KGs) nor cost the prewarm tiles.
	warmed := s.recentlyWarmedKGs(contribWarmWindow)
	small := s.contribSmallKGs()
	in := map[string]bool{}
	for _, kg := range p.KGs {
		in[kg] = true
	}
	var cheap, later []string
	for _, kg := range uni {
		if reported[kg] {
			continue
		}
		if small[kg] {
			p.Small++
			continue
		}
		p.LeftQ++
		if in[kg] {
			continue
		}
		if _, ok := warmed[kg]; ok {
			cheap = append(cheap, kg)
		} else {
			later = append(later, kg) // overdue (< day-catchup) or due later
		}
	}
	sort.SliceStable(cheap, func(i, j int) bool { return warmed[cheap[i]].After(warmed[cheap[j]]) }) // freshest tiles first
	sort.SliceStable(later, func(i, j int) bool {
		return contribDayOf(label, later[i], days) < contribDayOf(label, later[j], days)
	})
	if len(p.KGs) > contribNightMax {
		p.KGs = p.KGs[:contribNightMax] // only the scheduled/catch-up part needs fresh tiles
	}
	// Cheap ones first and uncapped: everything warm and unreported is reported
	// tonight while the tiles are still there — and they count toward
	// night_min. Before 2026-10-10 the fill was drawn from the *unwarmed* KGs
	// first, so the 200 KGs the 23:00 prewarm had just built turned into
	// "cheap" extras at 02:00 and the fill pulled 177 new KGs whose cells the
	// night had to assemble live (1 577 stash misses, 416 KGs, 5.3 h).
	// Now the prewarm's KGs *are* the fill of the night that follows.
	for _, kg := range cheap {
		p.KGs = append(p.KGs, kg)
		p.Fill = append(p.Fill, kg)
		p.Cheap = append(p.Cheap, kg)
	}
	for _, kg := range later {
		if len(p.KGs) >= p.NightMin {
			break
		}
		p.KGs = append(p.KGs, kg)
		p.Fill = append(p.Fill, kg)
		if d := contribDayOf(label, kg, days) - day; d > 0 {
			p.AheadDays = d
		} else {
			p.Overdue++
		}
	}
	// Rolling sweep: the quarter's unreported KGs are exhausted and the night
	// still has room — re-report the least recently reported KGs (≥ 7 d old)
	// so the whole universe is observed continuously, not once a quarter.
	if len(p.KGs) < p.NightMin {
		for _, kg := range p.KGs {
			in[kg] = true
		}
		cut := now.Add(-contribRollMinAge).Format("2006-01-02")
		var roll []string
		for _, kg := range uni {
			if !in[kg] && lastReport[kg] != "" && lastReport[kg] <= cut {
				roll = append(roll, kg)
			}
		}
		sort.SliceStable(roll, func(i, j int) bool { return lastReport[roll[i]] < lastReport[roll[j]] })
		for _, kg := range roll {
			if len(p.KGs) >= p.NightMin {
				break
			}
			p.KGs = append(p.KGs, kg)
			p.Fill = append(p.Fill, kg)
			p.Rolling++
		}
	}
	return p
}

// recentlyWarmedKGs: kg_code → warmed_at for KGs the prewarmer built within the window.
func (s *Server) recentlyWarmedKGs(window time.Duration) map[string]time.Time {
	out := map[string]time.Time{}
	rows, err := s.DB.QueryContext(context.Background(), "SELECT kg_code, warmed_at FROM kg_warm WHERE warmed_at > ?",
		time.Now().Add(-window).UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var kg, ts string
		if rows.Scan(&kg, &ts) == nil {
			t, _ := parseSQLiteTime(ts)
			out[padKGCode(kg)] = t
		}
	}
	return out
}

// contribLastReport: kg → date (YYYY-MM-DD) of its newest report file (KG.YYYY-MM-DD.json).
func contribLastReport() map[string]string {
	out := map[string]string{}
	ents, err := os.ReadDir(contribReportDir)
	if err != nil {
		return out
	}
	for _, e := range ents {
		name := e.Name()
		if !strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".meta.json") {
			continue
		}
		parts := strings.Split(strings.TrimSuffix(name, ".json"), ".")
		if len(parts) == 2 && len(parts[0]) == 5 && parts[1] > out[parts[0]] {
			out[parts[0]] = parts[1]
		}
	}
	return out
}

// contribReportedSince: KGs with a report file dated on/after t.
func contribReportedSince(t time.Time) map[string]bool {
	out := map[string]bool{}
	since := t.Format("2006-01-02")
	for kg, d := range contribLastReport() {
		if d >= since {
			out[kg] = true
		}
	}
	return out
}

// GET /api/contrib/plan[?kg=NNNNN] — today's rotation; with ?kg the day that KG is due.
func (s *Server) handleContribPlan(w http.ResponseWriter, r *http.Request) {
	at := time.Now()
	if r.URL.Query().Get("night") == "1" { // the plan the next ne-report run (and the RAM stash) uses
		at = nextContribNight(at)
	}
	p := s.contribPlanNow(at)
	if kg := r.URL.Query().Get("kg"); len(kg) > 0 {
		kg = padKGCode(kg)
		_, start, _ := quarterOf(time.Now())
		p.NextForKG = start.AddDate(0, 0, contribDayOf(p.Quarter, kg, p.Days)).Format("2006-01-02")
	}
	w.Header().Set("Cache-Control", "no-store")
	jsonResp(w, p)
}

// contribStatusMap is the compact `contrib{}` block of /api/warm/status.
func (s *Server) contribStatusMap() map[string]any {
	p := s.contribPlanNow(time.Now())
	return map[string]any{"quarter": p.Quarter, "day": p.Day, "days": p.Days, "today": len(p.Today), "fill": len(p.Fill), "cheap": len(p.Cheap),
		"ahead_days": p.AheadDays, "overdue": p.Overdue, "rolling": p.Rolling, "small": p.Small, "tonight": len(p.KGs), "night_min": p.NightMin, "night_max": p.NightMax, "universe": p.Universe,
		"per_day_avg": p.PerDayAvg, "reported_quarter": p.ReportedQ, "left_quarter": p.LeftQ,
		"warm_cap": contribWarmCap, "warm_hour": contribWarmHour, "night_hour": contribNightHour, "tz": contribLoc.String(), "next_night": nextContribNight(time.Now()).UTC().Format(time.RFC3339), "warm_last": contribWarmLast.Load(), "warm_last_queued": contribWarmQueued.Load(), "stash": contribStashStatus()}
}

var (
	contribWarmLast   atomic.Value // RFC3339 of the last warmContribRun
	contribWarmQueued atomic.Int64
)

// nextContribNight: the next contribNightHour local (ne-report.timer) after now.
func nextContribNight(now time.Time) time.Time {
	return nextAt(now, contribNightHour)
}

var contribLoc = func() *time.Location {
	if l, err := time.LoadLocation("Europe/Vienna"); err == nil {
		return l
	}
	return time.FixedZone("CET", 3600)
}()

// nextAt: the next Vienna wall-clock `hour`:00 strictly after now.
func nextAt(now time.Time, hour int) time.Time {
	v := now.In(contribLoc)
	t := time.Date(v.Year(), v.Month(), v.Day(), hour, 0, 0, 0, contribLoc)
	if !t.After(now) {
		t = t.AddDate(0, 0, 1)
	}
	return t
}

// contribPrewarmPending: true between the prewarm slot and the night run that
// follows it — a restart in that window emptied the RAM stash, so the
// prewarm must run again (it skips fully stashed KGs).
func contribPrewarmPending(now time.Time) bool {
	lastWarm := nextAt(now, contribWarmHour).AddDate(0, 0, -1)
	return nextAt(lastWarm, contribNightHour).After(now)
}

// warmContribRun queues the KGs the nightly NE report will need fresh BEV
// tiles for (tonight's plan minus the already-warm `cheap[]`), capped at
// contribWarmCap, so the night run finds their raw documents in the RAM stash
// (contrib_stash.go; else bevdirect's tile LRU) and costs python CPU only. Independent of the activity tier: the tiles are fetched
// once either way (by us now or by ne-report at night) — this only moves the
// download to the late evening and makes the KGs playable (lucky) for a day.
// Returns the number queued.
func (s *Server) warmContribRun() int {
	if !kgUniverseOK() {
		return 0
	}
	p := s.contribPlanNow(nextContribNight(time.Now()))
	cheap := map[string]bool{}
	for _, kg := range p.Cheap {
		cheap[kg] = true
	}
	n, fresh, stashed, small := 0, 0, 0, 0
	for _, kg := range p.KGs {
		if admin().KGs[kg] == nil {
			continue
		}
		cells := s.contribKGCells(kg)
		if contribStashComplete(cells) {
			stashed++
			continue
		}
		if s.contribKGSmall(kg) { // fresh head says: no whole aligned cell → ne_report skips it, no tiles needed
			small++
			continue
		}
		if cheap[kg] { // cells cached, stash missing (restart / built before the plan): CPU-only rebuilds, uncapped
			if s.enqueueWarmOpt(kg, "contrib", 2, true) {
				n++
			}
			continue
		}
		if fresh >= contribWarmCap {
			continue
		}
		if s.enqueueWarmOpt(kg, "contrib", 2, true) {
			n++
			fresh++
		}
	}
	contribWarmLast.Store(time.Now().UTC().Format(time.RFC3339))
	contribWarmQueued.Store(int64(n))
	slog.Info("warm: contrib plan queued", "kgs", n, "fresh", fresh, "stashed", stashed, "small", small, "tonight", len(p.KGs), "cheap", len(p.Cheap), "night_min", p.NightMin, "overdue", p.Overdue)
	return n
}

// warmContribLoop fires warmContribRun every day at contribWarmHour (Vienna time).
func (s *Server) warmContribLoop() {
	// After a restart between the 23:00 prewarm and the night run the RAM
	// stash is empty: re-run once (skips KGs whose cells are all stashed).
	if contribPrewarmPending(time.Now()) {
		time.Sleep(90 * time.Second)
		s.warmContribRun()
	}
	for {
		time.Sleep(time.Until(nextAt(time.Now(), contribWarmHour)))
		s.warmContribRun()
	}
}
