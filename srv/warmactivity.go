package srv

// Activity-aware warming budget.
//
// Prewarming only pays off when somebody is going to look at the cells within
// their 24 h lifetime. With nobody around, the full plan (100 KGs/day) plus
// keeping *every* v2.4 KG warm (1 300+ KGs and growing) re-downloads gigabytes
// of BEV vector tiles and NE documents a day for nothing — visible as a
// bandwidth spike every 72 min on an otherwise idle VM.
//
// `playerSeen()` is called from the handlers a human (or agent) hits while
// playing; `warmTier()` turns the time since then into a budget:
//
//   - active  (< warmIdleAfter since the last player): full daily plan, newly
//     adopted v2.4 KGs are warmed, recently played Gemeinden are kept warm.
//   - idle    (longer): only every warmIdleEvery-th daily patch runs (4
//     destinations/day keep /api/lucky meaningful), nothing else is kept warm;
//     cells are built lazily when a player actually arrives (bevdirect
//     assembles a cell in 1–3 s, the loading screen tolerates that).
//
// At startup the clock is seeded from the newest session / claim in the DB so a
// restart does not count as activity.

import (
	"context"
	"sync/atomic"
	"time"
)

const (
	warmIdleAfter    = 24 * time.Hour // no player for this long → idle tier
	warmIdleEvery    = 5              // idle: run every 5th patch (20 → 4 a day)
	warmRecentWindow = 7 * 24 * time.Hour
	warmKeepCap      = 40 // ≤ this many recently played v2.4 KGs are kept warm (active tier)
)

var lastPlayer atomic.Int64 // unix seconds of the last player-driven request

func playerSeen() { lastPlayer.Store(time.Now().Unix()) }

func lastPlayerAt() time.Time {
	v := lastPlayer.Load()
	if v == 0 {
		return time.Time{}
	}
	return time.Unix(v, 0)
}

func warmIdle() bool {
	if warmBoosted() { // boost days (warmboost.go) are never idle
		return false
	}
	t := lastPlayerAt()
	return t.IsZero() || time.Since(t) > warmIdleAfter
}

func warmTier() string {
	if warmBoosted() {
		return "boost"
	}
	if warmIdle() {
		return "idle"
	}
	return "active"
}

// seedPlayerActivity sets the activity clock from the DB at startup.
func (s *Server) seedPlayerActivity() {
	var ts string
	row := s.DB.QueryRowContext(context.Background(),
		`SELECT MAX(t) FROM (SELECT MAX(created_at) t FROM game_sessions UNION ALL SELECT MAX(claimed_at) FROM parcel_claims UNION ALL SELECT MAX(created_at) FROM chat_messages)`)
	if err := row.Scan(&ts); err != nil || ts == "" {
		return
	}
	if t, err := parseSQLiteTime(ts); err == nil {
		lastPlayer.Store(t.Unix())
	}
}

// recentV24KGs returns the v2.4 KGs of Gemeinden with a session in the last
// week (rejoin links land there), capped at warmKeepCap. Only the active tier
// keeps these warm.
func (s *Server) recentV24KGs(v24 map[string]bool) []string {
	rows, err := s.DB.QueryContext(context.Background(),
		`SELECT DISTINCT municipality_code FROM game_sessions WHERE created_at > ? ORDER BY created_at DESC`,
		time.Now().Add(-warmRecentWindow).UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return nil
	}
	defer rows.Close()
	adm := admin()
	var out []string
	for rows.Next() && len(out) < warmKeepCap {
		var code string
		if rows.Scan(&code) != nil {
			continue
		}
		g := adm.Gemeinde[code]
		if g == nil {
			continue
		}
		for _, kg := range g.KGs {
			if v24[kg] && len(out) < warmKeepCap {
				out = append(out, kg)
			}
		}
	}
	return out
}
