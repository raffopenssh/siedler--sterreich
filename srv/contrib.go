package srv

// Contrib rotation — which v2.4 KGs the nightly NE epoch report
// (tools/ne-report, → umfeld /contrib/api/v1/ne/{kg}/report) should build today.
//
// Goal: cover the whole v2.4 universe (~1 400 KGs) once per quarter with a
// handful of KGs a day, so umfeld sees a fresh bevdirect digest for every KG
// about four times a year. Every KG is assigned a day of the quarter by
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
	"crypto/sha256"
	"encoding/binary"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

const (
	contribCatchUpDays = 3 // today + the two nights before (ne_report's 7-day skip dedups)
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
	KGs         []string `json:"kgs"`  // today's rotation incl. catch-up days
	Today       []string `json:"today"`
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
		KGs: []string{}, Today: []string{}, Source: "registry"}
	if len(uni) == 0 {
		p.Source = "none"
		return p
	}
	p.PerDayAvg = float64(len(uni)) / float64(days)
	for _, kg := range uni {
		d := contribDayOf(label, kg, days)
		if d == day {
			p.Today = append(p.Today, kg)
		}
		if d <= day && d > day-contribCatchUpDays {
			p.KGs = append(p.KGs, kg)
		}
	}
	// Today's first, then the catch-up days (oldest last — they are mostly skipped anyway).
	sort.SliceStable(p.KGs, func(i, j int) bool {
		return contribDayOf(label, p.KGs[i], days) > contribDayOf(label, p.KGs[j], days)
	})
	p.ReportedQ = contribReportedSince(start)
	return p
}

// contribReportedSince counts KG report files (KG.YYYY-MM-DD.json) dated on/after t.
func contribReportedSince(t time.Time) int {
	ents, err := os.ReadDir(contribReportDir)
	if err != nil {
		return 0
	}
	since := t.Format("2006-01-02")
	n := 0
	for _, e := range ents {
		name := e.Name()
		if !strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".meta.json") {
			continue
		}
		parts := strings.Split(strings.TrimSuffix(name, ".json"), ".")
		if len(parts) == 2 && len(parts[0]) == 5 && parts[1] >= since {
			n++
		}
	}
	return n
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
	return map[string]any{"quarter": p.Quarter, "day": p.Day, "days": p.Days, "today": len(p.Today),
		"with_catch_up": len(p.KGs), "universe": p.Universe, "per_day_avg": p.PerDayAvg, "reported_quarter": p.ReportedQ}
}
