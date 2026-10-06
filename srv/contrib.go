package srv

// Contrib rotation — which v2.4 KGs the nightly NE epoch report
// (tools/ne-report, → umfeld /contrib/api/v1/ne/{kg}/report) should build today.
//
// Goal: cover the whole v2.4 universe (1 400 today, ~4 000 soon) at least once
// per quarter, so umfeld sees a fresh bevdirect digest for every KG about
// four times a year — and faster when the universe is small: every night
// runs at least contribNightMin KGs, pulling not-yet-reported KGs forward. Every KG is assigned a day of the quarter by
// hash(quarter, kg) — stable for the quarter, random across Austria, and new
// KGs appearing mid-quarter simply fall onto some day without shifting the
// others. Today's list is the KGs of the last `contribCatchUpDays` days (a
// missed night is caught up; ne_report.py skips anything reported < 7 d ago).
//
// Completely separate from prewarming: it reads bevdirect directly, writes no
// kg_warm rows and no api_cache cells, so /api/lucky (which only trusts
// kg_warm + the enhanced registry) is unaffected. Load ≈ 16 KGs × ~10 cells
// a night ≈ 30 MB of BEV tiles, at 03:30 UTC with Nice=15.
//
// GET /api/contrib/plan → {quarter, day, days, kgs[], per_day_avg, universe,
// reported_quarter, catch_up_days}; also `contrib{}` in /api/warm/status.

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

const (
	contribCatchUpDays = 3              // today + the two nights before (ne_report's 7-day skip dedups)
	contribNightMin    = 40             // run ahead of schedule: fill up to this many KGs a night (~90 MB tiles, ~30 min)
	contribNightMax    = 120            // cap on KGs needing fresh BEV tiles (≈ 0.25 GB); warm (cheap) KGs are never capped
	contribWarmWindow  = 24 * time.Hour // bevcache-prune deletes bevdirect tiles older than this
	contribReportDir   = "data/ne-reports"
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
	for kg := range s.neReadyKGSet() {
		if len(kg) == 5 {
			uni = append(uni, kg)
		}
	}
	sort.Strings(uni)
	p := contribPlan{Quarter: label, Day: day, Days: days, Universe: len(uni), CatchUpDays: contribCatchUpDays,
		NightMin: contribNightMin, NightMax: contribNightMax, KGs: []string{}, Today: []string{}, Fill: []string{}, Cheap: []string{}, Source: "registry"}
	if len(uni) == 0 {
		p.Source = "none"
		return p
	}
	p.PerDayAvg = float64(len(uni)) / float64(days)
	reported := contribReportedSince(start)
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
	// KGs, pull forward the KGs due later that have no report this quarter
	// yet (in due order). A small universe is then swept in weeks instead of
	// a quarter; once everything is reported the nights are quiet until the
	// next quarter. The cap protects the night when the universe jumps.
	// Tier 1 — KGs the prewarmer built in the last 24 h: their BEV tiles are
	// still on bevdirect's disk, so the report costs CPU only (~30 s/KG) —
	// all of them, no cap. Tier 2 — the rest in due order, up to night_min.
	warmed := s.recentlyWarmedKGs(contribWarmWindow)
	in := map[string]bool{}
	for _, kg := range p.KGs {
		in[kg] = true
	}
	var cheap, later []string
	for _, kg := range uni {
		if reported[kg] {
			continue
		}
		p.LeftQ++
		if in[kg] {
			continue
		}
		if _, ok := warmed[kg]; ok {
			cheap = append(cheap, kg)
		} else if contribDayOf(label, kg, days) > day {
			later = append(later, kg)
		}
	}
	sort.SliceStable(cheap, func(i, j int) bool { return warmed[cheap[i]].After(warmed[cheap[j]]) }) // freshest tiles first
	sort.SliceStable(later, func(i, j int) bool {
		return contribDayOf(label, later[i], days) < contribDayOf(label, later[j], days)
	})
	if len(p.KGs) > contribNightMax {
		p.KGs = p.KGs[:contribNightMax] // only the scheduled/catch-up part needs fresh tiles
	}
	for _, kg := range later {
		if len(p.KGs) >= contribNightMin {
			break
		}
		p.KGs = append(p.KGs, kg)
		p.Fill = append(p.Fill, kg)
		p.AheadDays = contribDayOf(label, kg, days) - day
	}
	// Cheap ones last in the list but uncapped: everything warm and unreported
	// is reported tonight while the tiles are still there.
	for _, kg := range cheap {
		p.KGs = append(p.KGs, kg)
		p.Fill = append(p.Fill, kg)
		p.Cheap = append(p.Cheap, kg)
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

// contribReportedSince: KGs with a report file (KG.YYYY-MM-DD.json) dated on/after t.
func contribReportedSince(t time.Time) map[string]bool {
	out := map[string]bool{}
	ents, err := os.ReadDir(contribReportDir)
	if err != nil {
		return out
	}
	since := t.Format("2006-01-02")
	for _, e := range ents {
		name := e.Name()
		if !strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".meta.json") {
			continue
		}
		parts := strings.Split(strings.TrimSuffix(name, ".json"), ".")
		if len(parts) == 2 && len(parts[0]) == 5 && parts[1] >= since {
			out[parts[0]] = true
		}
	}
	return out
}

// GET /api/contrib/plan[?kg=NNNNN] — today's rotation; with ?kg the day that KG is due.
func (s *Server) handleContribPlan(w http.ResponseWriter, r *http.Request) {
	p := s.contribPlanNow(time.Now())
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
		"ahead_days": p.AheadDays, "tonight": len(p.KGs), "night_min": p.NightMin, "universe": p.Universe,
		"per_day_avg": p.PerDayAvg, "reported_quarter": p.ReportedQ, "left_quarter": p.LeftQ}
}
