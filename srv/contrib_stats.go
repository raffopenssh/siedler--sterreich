package srv

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// GET /api/contrib/stats — public counter for the Impressum: how much of Austria we have
// reported to umfeld-at's NE monitoring so far (latest report per KG, data/ne-reports/KG.<date>.json).
// {kgs, cells, area_km2, first, last, universe, share_pct}. No KG list, no digests. 10 min cache.

type contribStats struct {
	KGs      int     `json:"kgs"`
	Cells    int     `json:"cells"`     // Σ cells_n (H3 res-12, ~307 m² each) of the latest report per KG
	AreaKm2  float64 `json:"area_km2"`  // Σ KG area (BEV VGD admin table)
	First    string  `json:"first"`     // oldest report date
	Last     string  `json:"last"`      // newest report date
	Universe int     `json:"universe"`  // v2.4 KGs eligible for the rotation
	SharePct float64 `json:"share_pct"` // kgs / universe
	At       string  `json:"generated_at"`
}

var (
	contribStatsMu sync.Mutex
	contribStatsAt time.Time
	contribStatsV  contribStats
)

func (s *Server) contribStatsNow() contribStats {
	contribStatsMu.Lock()
	defer contribStatsMu.Unlock()
	if time.Since(contribStatsAt) < 10*time.Minute {
		return contribStatsV
	}
	st := contribStats{At: time.Now().UTC().Format(time.RFC3339)}
	latest := map[string]string{} // kg → newest date
	ents, _ := os.ReadDir(contribReportDir)
	for _, e := range ents {
		name := e.Name()
		if !strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".meta.json") {
			continue
		}
		parts := strings.Split(strings.TrimSuffix(name, ".json"), ".")
		if len(parts) != 2 || len(parts[0]) != 5 {
			continue
		}
		if parts[1] > latest[parts[0]] {
			latest[parts[0]] = parts[1]
		}
	}
	kgs := make([]string, 0, len(latest))
	for kg := range latest {
		kgs = append(kgs, kg)
	}
	sort.Strings(kgs)
	ad := admin()
	for _, kg := range kgs {
		d := latest[kg]
		if st.First == "" || d < st.First {
			st.First = d
		}
		if d > st.Last {
			st.Last = d
		}
		var rep struct {
			CellsN int `json:"cells_n"`
		}
		if b, err := os.ReadFile(filepath.Join(contribReportDir, kg+"."+d+".json")); err == nil && json.Unmarshal(b, &rep) == nil {
			st.Cells += rep.CellsN
		}
		if ad != nil {
			if k := ad.KGs[kg]; k != nil {
				st.AreaKm2 += k.AreaKm2
			}
		}
	}
	st.KGs = len(kgs)
	st.AreaKm2 = float64(int(st.AreaKm2*10+0.5)) / 10
	st.Universe = len(s.neReadyKGSet())
	if st.Universe > 0 {
		st.SharePct = float64(int(float64(st.KGs)*1000/float64(st.Universe)+0.5)) / 10
	}
	contribStatsV, contribStatsAt = st, time.Now()
	return st
}

func (s *Server) handleContribStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age=600")
	jsonResp(w, s.contribStatsNow())
}
