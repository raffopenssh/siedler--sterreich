package srv

import (
	"log/slog"
	"math"
	"net/http"
	"sync"
	"time"

	"srv.exe.dev/db/dbgen"
)

// Schildersturm — the hidden "smash a map label" mini-game. The client
// shatters a decorative canvas label (place name, tree tag, Natura chip …)
// and reports how long it had been on screen; the fresher the sign, the more
// coins. Purely cosmetic economy, so the guard rails are light: a minimum
// spacing between smashes and a daily coin cap per player, both in memory.
const (
	smashMinGap  = 300 * time.Millisecond
	smashDayCap  = 400 // coins per player per UTC day
	smashMaxCoin = 12
)

type smashState struct {
	last  time.Time
	day   string
	coins int64
	n     int64
}

var (
	smashMu sync.Mutex
	smashBy = map[string]*smashState{}
)

// smashCoins mirrors smashCoinsFor() in game.js — keep in sync.
// 0 s → 12, 2 s → 9, 4 s → 6, 6 s → 4, 10 s → 2, ≥ 15 s → 1.
func smashCoins(visibleMs int64) int64 {
	if visibleMs < 0 {
		visibleMs = 0
	}
	c := math.Round(float64(smashMaxCoin) * math.Exp(-float64(visibleMs)/6000))
	if c < 1 {
		c = 1
	}
	return int64(c)
}

func (s *Server) handleSmashLabel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID  string `json:"player_id"`
		SessionID string `json:"session_id"`
		VisibleMs int64  `json:"visible_ms"`
		Label     string `json:"label"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonErr(w, "invalid request", 400)
		return
	}
	if _, ok := s.authPlayer(r, req.PlayerID); !ok {
		jsonErr(w, "unauthorized", 401)
		return
	}
	now := time.Now()
	day := now.UTC().Format("2006-01-02")

	smashMu.Lock()
	st := smashBy[req.PlayerID]
	if st == nil {
		st = &smashState{}
		smashBy[req.PlayerID] = st
	}
	if st.day != day {
		st.day, st.coins, st.n = day, 0, 0
	}
	if now.Sub(st.last) < smashMinGap {
		smashMu.Unlock()
		jsonErr(w, "zu schnell", 429)
		return
	}
	st.last = now
	coins := smashCoins(req.VisibleMs)
	capped := false
	if st.coins+coins > smashDayCap {
		coins = max(0, smashDayCap-st.coins)
		capped = true
	}
	st.coins += coins
	st.n++
	total, n := st.coins, st.n
	if len(smashBy) > 5000 { // forgetful: drop stale players
		for id, o := range smashBy {
			if o.day != day {
				delete(smashBy, id)
			}
		}
	}
	smashMu.Unlock()

	if coins > 0 {
		if err := s.Q.UpdatePlayerCoins(r.Context(), dbgen.UpdatePlayerCoinsParams{Coins: coins, ID: req.PlayerID}); err != nil {
			jsonErr(w, "db error", 500)
			return
		}
	}
	player, _ := s.Q.GetPlayerByID(r.Context(), req.PlayerID)
	if n == 1 || n%50 == 0 {
		slog.Info("smash: label", "player", req.PlayerID, "n", n, "today", total, "label", req.Label)
	}
	jsonResp(w, map[string]any{"coins": coins, "today": total, "cap": smashDayCap, "capped": capped, "player": player})
}
